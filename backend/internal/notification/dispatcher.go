package notification

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dispatcher is the main notification orchestrator. It manages a buffered
// channel of delivery jobs and a pool of worker goroutines that process them.
type Dispatcher struct {
	cfg        DispatcherConfig
	queries    *db.Queries
	pool       *pgxpool.Pool
	httpClient *http.Client
	jobs       chan DeliveryJob
	metrics    *Metrics
	state      *StateTracker
	done       chan struct{}
	wg         sync.WaitGroup

	// stopping is set to 1 when shutdown is initiated.
	// Used by Enqueue to reject new jobs during shutdown.
	stopping atomic.Int32

	// dispatchFn is an optional override for the dispatch method.
	// Used for testing; when nil, the default dispatch logic is used.
	dispatchFn func(DeliveryJob) error
}

// NewDispatcher creates a new Dispatcher with the given configuration.
// The jobs channel is created with capacity from cfg.BufferSize (default 256).
func NewDispatcher(cfg DispatcherConfig, queries *db.Queries, pool *pgxpool.Pool, metrics *Metrics, state *StateTracker) *Dispatcher {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 256
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 10
	}
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = 30 * time.Second
	}

	return &Dispatcher{
		cfg:        cfg,
		queries:    queries,
		pool:       pool,
		httpClient: &http.Client{},
		jobs:       make(chan DeliveryJob, cfg.BufferSize),
		metrics:    metrics,
		state:      state,
		done:       make(chan struct{}),
	}
}

// Start launches the worker goroutines. Each worker reads from the jobs channel,
// dispatches the delivery, and logs the result.
func (d *Dispatcher) Start() {
	for i := range d.cfg.Workers {
		d.wg.Add(1)
		go d.worker(i)
	}
	log.Printf("notification: dispatcher started with %d workers, buffer size %d", d.cfg.Workers, d.cfg.BufferSize)
}

// Stop signals all workers to stop and waits for them to finish.
// Deprecated: Use Shutdown for graceful drain with timeout.
func (d *Dispatcher) Stop() {
	close(d.done)
	d.wg.Wait()
	log.Printf("notification: dispatcher stopped")
}

// Shutdown performs a graceful shutdown of the dispatcher:
//  1. Stops accepting new jobs (Enqueue will drop them).
//  2. Closes the done channel to signal workers to finish their current job and exit.
//  3. Drains remaining jobs from the buffered channel by processing them.
//  4. Waits for all in-flight deliveries to complete within DrainTimeout.
//  5. If the timeout expires, logs the count of abandoned notifications and returns.
func (d *Dispatcher) Shutdown(ctx context.Context) {
	// Step 1: Mark as stopping — Enqueue will reject new jobs from this point.
	d.stopping.Store(1)

	// Step 2: Signal workers to stop picking up new jobs from the channel.
	close(d.done)

	// Step 3: Drain remaining buffered jobs. Workers have already exited or are
	// finishing their current job, so we drain what's left in the channel ourselves.
	drainTimeout := d.cfg.DrainTimeout
	drainCtx, drainCancel := context.WithTimeout(ctx, drainTimeout)
	defer drainCancel()

	drained := 0
	for {
		select {
		case job, ok := <-d.jobs:
			if !ok {
				// Channel was closed (shouldn't happen in normal flow, but handle gracefully).
				goto waitWorkers
			}
			d.wg.Add(1)
			go func(j DeliveryJob) {
				defer d.wg.Done()
				d.processJob(drainCtx, -1, j)
			}(job)
			drained++
		default:
			// No more buffered jobs.
			goto waitWorkers
		}
	}

waitWorkers:
	// Step 4: Wait for all in-flight deliveries (workers + drain goroutines) to complete.
	waitDone := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		log.Printf("notification: shutdown complete, drained %d jobs", drained)
	case <-drainCtx.Done():
		// Step 5: Timeout expired — count remaining jobs as abandoned.
		abandoned := len(d.jobs)
		log.Printf("notification: shutdown timeout, %d jobs abandoned", abandoned)
	}
}

// Enqueue attempts to place a DeliveryJob onto the buffered channel.
// If the buffer is full or shutdown is in progress, the notification is dropped
// (non-blocking) and the DroppedTotal counter is incremented.
func (d *Dispatcher) Enqueue(job DeliveryJob) {
	// Reject new jobs during shutdown.
	if d.stopping.Load() != 0 {
		channelType := classifyChannelType(job)
		d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
		log.Printf("notification: job dropped (shutting down) monitor=%s trigger=%s", job.MonitorID, job.TriggerType)
		return
	}

	select {
	case d.jobs <- job:
		// Successfully enqueued.
	default:
		// Buffer full — drop with metrics and warning log.
		channelType := classifyChannelType(job)
		d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
		log.Printf("notification: job dropped (buffer full) monitor=%s trigger=%s", job.MonitorID, job.TriggerType)
	}
}

// worker is the main loop for a single worker goroutine. It dequeues jobs,
// dispatches them, and updates metrics.
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()

	for {
		select {
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			d.processJob(context.Background(), id, job)

		case <-d.done:
			return
		}
	}
}

// processJob handles a single delivery job with panic recovery and metric tracking.
func (d *Dispatcher) processJob(ctx context.Context, workerID int, job DeliveryJob) {
	channelType := classifyChannelType(job)

	// Increment in-flight gauge.
	d.metrics.InFlight.Inc()
	defer d.metrics.InFlight.Dec()

	// Panic recovery — worker must never crash. Records failure in delivery_logs.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("notification: worker %d panic recovered: %v (monitor=%s trigger=%s)",
				workerID, r, job.MonitorID, job.TriggerType)
			d.metrics.DeliveriesTotal.WithLabelValues(channelType, "failure").Inc()

			// Record panic as a delivery failure in the logs.
			panicDetail := fmt.Sprintf("panic: %v", r)
			d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
				job.TriggerType, job.Attempt, "failure", panicDetail)
		}
	}()

	// Dispatch the job. Actual SMTP/webhook delivery will be implemented in
	// tasks 6.1 and 6.3. For now this is a placeholder dispatch point.
	err := d.dispatch(job)

	if err != nil {
		d.metrics.DeliveriesTotal.WithLabelValues(channelType, "failure").Inc()
		log.Printf("notification: delivery failed worker=%d monitor=%s trigger=%s attempt=%d err=%v",
			workerID, job.MonitorID, job.TriggerType, job.Attempt, err)

		// Record failure in delivery_logs.
		d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
			job.TriggerType, job.Attempt, "failure", err.Error())
	} else {
		d.metrics.DeliveriesTotal.WithLabelValues(channelType, "success").Inc()
		log.Printf("notification: delivery success worker=%d monitor=%s trigger=%s attempt=%d",
			workerID, job.MonitorID, job.TriggerType, job.Attempt)

		// Record success in delivery_logs.
		d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
			job.TriggerType, job.Attempt, "success", "")
	}
}

// dispatch routes the job to the appropriate delivery method based on channel type.
// This is a placeholder that will be completed when SMTP (task 6.1) and
// webhook (task 6.3) clients are implemented.
func (d *Dispatcher) dispatch(job DeliveryJob) error {
	// Allow test injection of dispatch behavior.
	if d.dispatchFn != nil {
		return d.dispatchFn(job)
	}

	// TODO: Route to SMTP or webhook client based on channel configuration.
	// For now, jobs are processed but no actual delivery occurs.
	_ = job
	return nil
}

// classifyChannelType returns the channel type label for metrics.
// This is a simple heuristic based on the job; the actual channel type
// will be resolved from the channel config in later tasks.
func classifyChannelType(job DeliveryJob) string {
	// Default to "unknown" until channel lookup is wired in task 6.x.
	_ = job
	return "unknown"
}
