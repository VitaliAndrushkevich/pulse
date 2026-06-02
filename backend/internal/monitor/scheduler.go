package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

const (
	// DefaultWorkers is the default worker pool size for production.
	DefaultWorkers = 200
	// DefaultDevWorkers is a reduced pool size suitable for local development.
	DefaultDevWorkers = 50
	// DefaultTickInterval is how often the scheduler polls for due monitors.
	DefaultTickInterval = 1 * time.Second
	// DefaultBatchSize is the maximum number of monitors fetched per tick.
	DefaultBatchSize int32 = 500
)

// SchedulerConfig holds tunable scheduler parameters.
type SchedulerConfig struct {
	Workers      int
	TickInterval time.Duration
	BatchSize    int32
}

// Scheduler orchestrates periodic monitor checks using a bounded worker pool.
type Scheduler struct {
	cfg       SchedulerConfig
	registry  *Registry
	queries   *db.Queries
	tsStore   *timescale.Store
	metrics   *handlers.Metrics
	hub       *hub.Hub // WebSocket broadcast hub (may be nil)
	wakeupCh  chan struct{} // signals immediate re-poll (from LISTEN/NOTIFY)
	stopOnce  sync.Once
}

// NewScheduler creates a scheduler with the given configuration and dependencies.
func NewScheduler(cfg SchedulerConfig, registry *Registry, queries *db.Queries, tsStore *timescale.Store, metrics *handlers.Metrics, wsHub *hub.Hub) *Scheduler {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = DefaultTickInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	return &Scheduler{
		cfg:      cfg,
		registry: registry,
		queries:  queries,
		tsStore:  tsStore,
		metrics:  metrics,
		hub:      wsHub,
		wakeupCh: make(chan struct{}, 1),
	}
}

// Wakeup sends a non-blocking signal to the scheduler to re-poll immediately.
// Used by the LISTEN/NOTIFY listener when a monitor is created or updated.
func (s *Scheduler) Wakeup() {
	select {
	case s.wakeupCh <- struct{}{}:
	default:
		// Already signaled, skip.
	}
}

// Run starts the scheduling loop. It blocks until ctx is cancelled.
// Workers are bounded — no unbounded goroutine growth regardless of monitor count.
func (s *Scheduler) Run(ctx context.Context) {
	log.Printf("scheduler: starting with %d workers, tick=%s, batch=%d",
		s.cfg.Workers, s.cfg.TickInterval, s.cfg.BatchSize)

	// Buffered job channel provides backpressure.
	jobs := make(chan db.Monitor, s.cfg.BatchSize)

	// Start bounded worker pool.
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx, jobs)
		}()
	}

	ticker := time.NewTicker(s.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("scheduler: shutting down")
			close(jobs)
			wg.Wait()
			log.Printf("scheduler: all workers stopped")
			return
		case <-ticker.C:
			s.poll(ctx, jobs)
		case <-s.wakeupCh:
			s.poll(ctx, jobs)
		}
	}
}

// poll fetches due monitors and dispatches them to the worker pool.
func (s *Scheduler) poll(ctx context.Context, jobs chan<- db.Monitor) {
	monitors, err := s.queries.ListActiveMonitorsDue(ctx, s.cfg.BatchSize)
	if err != nil {
		log.Printf("scheduler: poll error: %v", err)
		return
	}
	if len(monitors) == 0 {
		return
	}

	for _, m := range monitors {
		select {
		case <-ctx.Done():
			return
		case jobs <- m:
		}
	}
}

// worker processes monitors from the jobs channel.
func (s *Scheduler) worker(ctx context.Context, jobs <-chan db.Monitor) {
	for m := range jobs {
		if ctx.Err() != nil {
			return
		}
		s.executeCheck(ctx, m)
	}
}

// executeCheck runs the appropriate checker for a monitor and persists results.
func (s *Scheduler) executeCheck(ctx context.Context, m db.Monitor) {
	checker, err := s.registry.Get(m.Type)
	if err != nil {
		log.Printf("scheduler: monitor %s: %v", m.ID, err)
		s.recordFailure(ctx, m, "unknown monitor type: "+m.Type)
		return
	}

	// Create a per-check context with the monitor's timeout.
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := checker.Check(checkCtx, m.Target, m.Settings)
	result.MonitorID = m.ID

	// Persist check result to TimescaleDB.
	if err := s.tsStore.WriteCheckResult(ctx, timescale.CheckPoint{
		MonitorID:  m.ID,
		State:      result.State,
		LatencyMs:  &result.LatencyMs,
		StatusCode: result.StatusCode,
		Error:      strPtr(result.Error),
		CheckedAt:  result.CheckedAt,
	}); err != nil {
		log.Printf("scheduler: monitor %s: write result: %v", m.ID, err)
	}

	// Also persist to the regular check_results table for the API layer.
	if _, err := s.queries.CreateCheckResult(ctx, db.CreateCheckResultParams{
		MonitorID:  m.ID,
		CheckedAt:  result.CheckedAt,
		State:      result.State,
		LatencyMs:  &result.LatencyMs,
		StatusCode: result.StatusCode,
		Error:      strPtr(result.Error),
	}); err != nil {
		log.Printf("scheduler: monitor %s: create check result: %v", m.ID, err)
	}

	// Update monitor state and schedule next check.
	nextCheck := time.Now().Add(time.Duration(m.IntervalSeconds) * time.Second)
	if _, err := s.queries.UpdateMonitorState(ctx, db.UpdateMonitorStateParams{
		ID:    m.ID,
		State: result.State,
		LastCheckedAt: pgtype.Timestamptz{
			Time:  result.CheckedAt,
			Valid: true,
		},
		NextCheckAt: pgtype.Timestamptz{
			Time:  nextCheck,
			Valid: true,
		},
	}); err != nil {
		log.Printf("scheduler: monitor %s: update state: %v", m.ID, err)
	}

	// Update Prometheus metrics (TASK-026).
	if s.metrics != nil {
		labels := []string{m.ID.String(), m.Name, m.Type}
		upVal := float64(0)
		if result.State == "up" {
			upVal = 1
		}
		s.metrics.MonitorUp.WithLabelValues(labels...).Set(upVal)
		s.metrics.MonitorResponseTime.WithLabelValues(labels...).Set(float64(result.LatencyMs) / 1000.0)
	}

	// Broadcast status update via WebSocket hub (TASK-029).
	if s.hub != nil {
		s.hub.Broadcast(hub.NewMonitorStatusMessage(
			m.ID.String(),
			result.State,
			result.LatencyMs,
			result.StatusCode,
			result.SSLDaysRemaining,
			result.Error,
			result.CheckedAt,
		))
	}
}

// recordFailure handles cases where the checker itself couldn't be resolved.
func (s *Scheduler) recordFailure(ctx context.Context, m db.Monitor, errMsg string) {
	now := time.Now().UTC()
	nextCheck := now.Add(time.Duration(m.IntervalSeconds) * time.Second)

	var latency int32
	if _, err := s.queries.CreateCheckResult(ctx, db.CreateCheckResultParams{
		MonitorID:  m.ID,
		CheckedAt:  now,
		State:      "down",
		LatencyMs:  &latency,
		StatusCode: nil,
		Error:      &errMsg,
	}); err != nil {
		log.Printf("scheduler: monitor %s: record failure: %v", m.ID, err)
	}

	if _, err := s.queries.UpdateMonitorState(ctx, db.UpdateMonitorStateParams{
		ID:    m.ID,
		State: "down",
		LastCheckedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		NextCheckAt: pgtype.Timestamptz{
			Time:  nextCheck,
			Valid: true,
		},
	}); err != nil {
		log.Printf("scheduler: monitor %s: update state after failure: %v", m.ID, err)
	}
}

// strPtr returns nil for empty strings, otherwise a pointer to the string.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}


