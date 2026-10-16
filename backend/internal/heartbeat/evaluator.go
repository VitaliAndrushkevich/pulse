package heartbeat

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

const (
	// DefaultWorkers is the default evaluator worker-pool size. The per-job work
	// is a single UPDATE plus non-blocking dispatch/broadcast — far cheaper than
	// a probe — so a smaller pool than the probe scheduler's 200 is sufficient.
	DefaultWorkers = 50
	// DefaultTickInterval is how often the evaluator sweeps for due heartbeats.
	// It bounds missed-run detection latency.
	DefaultTickInterval = 1 * time.Second
	// DefaultBatchSize is the maximum number of due heartbeats fetched per tick.
	DefaultBatchSize int32 = 500
)

// Config holds tunable evaluator parameters. Zero values fall back to the
// Default* constants so tests can supply only the fields they care about.
type Config struct {
	Workers      int
	TickInterval time.Duration
	BatchSize    int32
}

// Queries is the subset of the generated db.Queries the evaluator depends on.
// Narrowing the surface keeps the evaluator unit-/integration-testable with a
// focused fake and documents exactly what it touches.
type Queries interface {
	ListDueHeartbeats(ctx context.Context, limit int32) ([]db.Heartbeat, error)
	MarkHeartbeatDown(ctx context.Context, id uuid.UUID) (db.Heartbeat, error)
	CountHeartbeats(ctx context.Context) (int64, error)
}

// Dispatcher is the seam the evaluator uses to fire the missed-run notification
// path. The concrete heartbeat dispatch helper is wired in Phase 4; until then a
// nil Dispatcher is valid and simply skips notification dispatch.
type Dispatcher interface {
	// DispatchMissedRun enqueues the heartbeat_missed notification for a
	// heartbeat that just transitioned up → down via the sweep. previousState is
	// the state observed before the transition (always "up" for a sweep miss).
	DispatchMissedRun(ctx context.Context, hb db.Heartbeat, previousState string)
}

// Evaluator detects missed heartbeat runs with a periodic sweep over a bounded
// worker pool, modeled on monitor.Scheduler. A missed deadline is the absence of
// an event, so a sweep (not LISTEN/NOTIFY) is the correct mechanism.
type Evaluator struct {
	cfg        Config
	queries    Queries
	metrics    *Metrics
	hub        *hub.Hub   // WebSocket broadcast hub (may be nil)
	dispatcher Dispatcher // missed-run notification seam (may be nil until Phase 4)
	stopOnce   sync.Once
}

// NewEvaluator creates an evaluator with the given configuration and
// dependencies. hub and dispatcher may be nil (broadcast/dispatch are then
// skipped). Zero-valued Config fields fall back to the Default* constants.
func NewEvaluator(cfg Config, queries Queries, metrics *Metrics, wsHub *hub.Hub, dispatcher Dispatcher) *Evaluator {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = DefaultTickInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	return &Evaluator{
		cfg:        cfg,
		queries:    queries,
		metrics:    metrics,
		hub:        wsHub,
		dispatcher: dispatcher,
	}
}

// Run starts the sweep loop. It blocks until ctx is cancelled, then closes the
// jobs channel, waits for in-flight work to drain, and returns. Workers are
// bounded — no unbounded goroutine growth regardless of heartbeat count.
func (e *Evaluator) Run(ctx context.Context) {
	log.Printf("heartbeat evaluator: starting with %d workers, tick=%s, batch=%d",
		e.cfg.Workers, e.cfg.TickInterval, e.cfg.BatchSize)

	// Buffered job channel provides backpressure.
	jobs := make(chan db.Heartbeat, e.cfg.BatchSize)

	// Start the bounded worker pool.
	var wg sync.WaitGroup
	for i := 0; i < e.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.worker(ctx, jobs)
		}()
	}

	ticker := time.NewTicker(e.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("heartbeat evaluator: shutting down")
			e.stopOnce.Do(func() { close(jobs) })
			wg.Wait()
			log.Printf("heartbeat evaluator: all workers stopped")
			return
		case <-ticker.C:
			e.poll(ctx, jobs)
		}
	}
}

// poll refreshes the total gauge and dispatches due heartbeats to the worker pool.
func (e *Evaluator) poll(ctx context.Context, jobs chan<- db.Heartbeat) {
	// Refresh the total-heartbeats gauge each tick (mirrors how the scheduler
	// sets pulse_monitors_total).
	if e.metrics != nil {
		if count, err := e.queries.CountHeartbeats(ctx); err == nil {
			e.metrics.SetTotal(int(count))
		} else {
			log.Printf("heartbeat evaluator: count heartbeats: %v", err)
		}
	}

	rows, err := e.queries.ListDueHeartbeats(ctx, e.cfg.BatchSize)
	if err != nil {
		log.Printf("heartbeat evaluator: poll error: %v", err)
		return
	}

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return
		case jobs <- row:
		}
	}
}

// worker processes due heartbeats from the jobs channel.
func (e *Evaluator) worker(ctx context.Context, jobs <-chan db.Heartbeat) {
	for row := range jobs {
		if ctx.Err() != nil {
			return
		}
		e.markDown(ctx, row)
	}
}

// markDown transitions a due heartbeat to the down state. The side effects
// (metric, broadcast, notification) run only when MarkHeartbeatDown actually
// updates a row — its WHERE state='up' guard makes a missed-run episode fire
// exactly once even if the row is swept on multiple ticks before recovery.
func (e *Evaluator) markDown(ctx context.Context, row db.Heartbeat) {
	// The row we loaded was active+up+past-deadline; its prior state is "up".
	previousState := row.State

	updated, err := e.queries.MarkHeartbeatDown(ctx, row.ID)
	if err != nil {
		// No row updated (already down → raced with another transition) or a DB
		// error. Either way the row stays as-is and will be retried next tick if
		// still eligible. The guard keeps this idempotent, so we only log.
		log.Printf("heartbeat evaluator: heartbeat %s: mark down (no update): %v", row.ID, err)
		return
	}

	// A real up → down transition occurred. Fire the side effects exactly once.
	if e.metrics != nil {
		e.metrics.SetUp(updated.ID.String(), updated.Name, false)
	}

	if e.hub != nil {
		var lastSuccess, nextDeadline *time.Time
		if updated.LastSuccessAt.Valid {
			t := updated.LastSuccessAt.Time
			lastSuccess = &t
		}
		if updated.NextExpectedDeadline.Valid {
			t := updated.NextExpectedDeadline.Time
			nextDeadline = &t
		}
		e.hub.Broadcast(hub.NewHeartbeatStatusMessage(
			updated.ID.String(),
			updated.State,
			lastSuccess,
			nextDeadline,
		))
	}

	// Missed-run notification dispatch. The concrete helper is wired in Phase 4;
	// a nil dispatcher simply skips it.
	if e.dispatcher != nil {
		e.dispatcher.DispatchMissedRun(ctx, updated, previousState)
	}
}
