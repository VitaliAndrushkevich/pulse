package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LogLevel controls verbosity of notification delivery logs.
type LogLevel int

const (
	// LogLevelWarn logs only errors and warnings (default).
	LogLevelWarn LogLevel = iota
	// LogLevelInfo logs delivery events with payload details.
	LogLevelInfo
)

// ParseLogLevel parses PULSE_LOG_LEVEL from environment.
// Returns LogLevelWarn by default.
func ParseLogLevel() LogLevel {
	switch strings.ToLower(os.Getenv("PULSE_LOG_LEVEL")) {
	case "info", "debug":
		return LogLevelInfo
	default:
		return LogLevelWarn
	}
}

// EmailDeliverer is the interface the Dispatcher uses to send email notifications.
type EmailDeliverer interface {
	SendNotification(ctx context.Context, recipients []string, data TemplateData) error
}

// WebhookDeliverer is the interface the Dispatcher uses to send webhook notifications.
type WebhookDeliverer interface {
	Deliver(ctx context.Context, config interface{}, data TemplateData) error
}

// webhookDeliverAdapter adapts webhook.Client to the WebhookDeliverer interface.
type webhookDeliverAdapter struct {
	deliverFn func(ctx context.Context, rawConfig json.RawMessage, secretKey []byte, data TemplateData) error
	secretKey []byte
}

func (a *webhookDeliverAdapter) Deliver(ctx context.Context, config interface{}, data TemplateData) error {
	raw, ok := config.(json.RawMessage)
	if !ok {
		return NewNonRetryableError(fmt.Errorf("webhook: invalid config type"))
	}
	return a.deliverFn(ctx, raw, a.secretKey, data)
}

// Dispatcher is the main notification orchestrator. It manages a buffered
// channel of delivery jobs and a pool of worker goroutines that process them.
type Dispatcher struct {
	cfg        DispatcherConfig
	queries    *db.Queries
	pool       *pgxpool.Pool
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

	// Delivery clients — set via SetSMTPClient / SetWebhookClient after construction.
	smtpClient   EmailDeliverer
	smtpMu       sync.RWMutex
	webhookDelFn func(ctx context.Context, rawConfig json.RawMessage, secretKey []byte, data TemplateData) error
	secretKey    []byte
	logLevel     LogLevel
}

// NewDispatcher creates a new Dispatcher with the given configuration.
// The jobs channel is created with capacity from cfg.BufferSize (default 256).
func NewDispatcher(cfg DispatcherConfig, queries *db.Queries, pool *pgxpool.Pool, metrics *Metrics, state *StateTracker, secretKey []byte) *Dispatcher {
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
		cfg:       cfg,
		queries:   queries,
		pool:      pool,
		jobs:      make(chan DeliveryJob, cfg.BufferSize),
		metrics:    metrics,
		state:      state,
		done:       make(chan struct{}),
		secretKey:  secretKey,
		logLevel:   ParseLogLevel(),
	}
}

// SetSMTPClient sets the SMTP delivery client. Thread-safe — can be updated
// at runtime when SMTP settings change.
func (d *Dispatcher) SetSMTPClient(client EmailDeliverer) {
	d.smtpMu.Lock()
	defer d.smtpMu.Unlock()
	d.smtpClient = client
}

// getSMTPClient returns the current SMTP client (thread-safe).
func (d *Dispatcher) getSMTPClient() EmailDeliverer {
	d.smtpMu.RLock()
	defer d.smtpMu.RUnlock()
	return d.smtpClient
}

// SetWebhookDeliverFn sets the webhook delivery function used by dispatch().
func (d *Dispatcher) SetWebhookDeliverFn(fn func(ctx context.Context, rawConfig json.RawMessage, secretKey []byte, data TemplateData) error) {
	d.webhookDelFn = fn
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

	// Dispatch the job to the appropriate delivery channel.
	err := d.dispatch(job)

	if err != nil {
		d.metrics.DeliveriesTotal.WithLabelValues(channelType, "failure").Inc()
		log.Printf("notification: delivery failed worker=%d monitor=%s channel=%s trigger=%s attempt=%d/%d err=%v",
			workerID, job.MonitorID, job.ChannelID, job.TriggerType, job.Attempt, job.MaxAttempts, err)

		// Record failure in delivery_logs.
		d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
			job.TriggerType, job.Attempt, "failure", err.Error())
	} else {
		d.metrics.DeliveriesTotal.WithLabelValues(channelType, "success").Inc()
		log.Printf("notification: delivery success worker=%d monitor=%s channel=%s trigger=%s attempt=%d",
			workerID, job.MonitorID, job.ChannelID, job.TriggerType, job.Attempt)

		// Record success in delivery_logs.
		d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
			job.TriggerType, job.Attempt, "success", "")
	}
}

// dispatch routes the job to the appropriate delivery method based on channel type.
func (d *Dispatcher) dispatch(job DeliveryJob) error {
	// Allow test injection of dispatch behavior.
	if d.dispatchFn != nil {
		return d.dispatchFn(job)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load the channel from DB to determine type and config.
	channel, err := d.queries.GetChannel(ctx, job.ChannelID)
	if err != nil {
		return NewNonRetryableError(fmt.Errorf("dispatch: failed to load channel %s: %w", job.ChannelID, err))
	}

	// Enrich payload with monitor details if not already populated.
	if job.Payload.Monitor.Name == "" {
		d.enrichPayload(ctx, &job)
	}

	switch channel.ChannelType {
	case "email":
		return d.deliverEmail(ctx, channel, job)
	case "webhook":
		return d.deliverWebhook(ctx, channel, job)
	default:
		return NewNonRetryableError(fmt.Errorf("dispatch: unsupported channel type %q", channel.ChannelType))
	}
}

// deliverEmail sends an email notification using the configured SMTP client.
func (d *Dispatcher) deliverEmail(ctx context.Context, channel db.NotificationChannel, job DeliveryJob) error {
	client := d.getSMTPClient()
	if client == nil {
		return NewNonRetryableError(fmt.Errorf("dispatch: SMTP not configured"))
	}

	// Parse email config to get recipients.
	type emailCfg struct {
		Recipients []string `json:"recipients"`
	}
	var cfg emailCfg
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return NewNonRetryableError(fmt.Errorf("dispatch: invalid email config: %w", err))
	}
	if len(cfg.Recipients) == 0 {
		return NewNonRetryableError(fmt.Errorf("dispatch: no email recipients configured"))
	}

	if d.logLevel >= LogLevelInfo {
		log.Printf("notification: delivering email monitor=%s trigger=%s recipients=%v payload=%+v",
			job.MonitorID, job.TriggerType, cfg.Recipients, job.Payload)
	}

	return client.SendNotification(ctx, cfg.Recipients, job.Payload)
}

// deliverWebhook sends a webhook notification.
func (d *Dispatcher) deliverWebhook(ctx context.Context, channel db.NotificationChannel, job DeliveryJob) error {
	if d.webhookDelFn == nil {
		return NewNonRetryableError(fmt.Errorf("dispatch: webhook client not configured"))
	}

	if d.logLevel >= LogLevelInfo {
		log.Printf("notification: delivering webhook monitor=%s trigger=%s channel=%s payload=%+v",
			job.MonitorID, job.TriggerType, job.ChannelID, job.Payload)
	}

	return d.webhookDelFn(ctx, channel.Config, d.secretKey, job.Payload)
}

// enrichPayload fills in monitor name/URL/target and incident data from the database.
func (d *Dispatcher) enrichPayload(ctx context.Context, job *DeliveryJob) {
	// Set BaseURL for "View Monitor" link generation.
	job.Payload.BaseURL = d.cfg.BaseURL

	row, err := d.queries.GetMonitor(ctx, job.MonitorID)
	if err != nil {
		log.Printf("notification: enrich payload failed monitor=%s: %v", job.MonitorID, err)
		return
	}
	job.Payload.Monitor.Name = row.Name
	job.Payload.Monitor.Target = row.Target
	// URL is typically same as target for HTTP monitors.
	if row.Type == "http" || row.Type == "http3" || row.Type == "https" {
		job.Payload.Monitor.URL = row.Target
	}

	// Fetch open incident for the monitor (if any).
	incident, err := d.queries.GetOpenIncidentByMonitor(ctx, job.MonitorID)
	if err == nil {
		job.Payload.Incident = IncidentData{
			ID:        incident.ID,
			StartedAt: incident.StartedAt,
			Duration:  time.Since(incident.StartedAt),
		}
	}
	// If no open incident found (pgx.ErrNoRows), leave Incident as zero-value —
	// the template will hide the section.
}

// classifyChannelType returns the channel type label for metrics.
// Resolves actual channel type from the database for accurate metric labeling.
func classifyChannelType(job DeliveryJob) string {
	// For now, we cannot look up from DB here without a reference to queries.
	// Return "unknown" — metrics are still tracked by outcome.
	// The actual channel type is logged in the delivery methods.
	_ = job
	return "unknown"
}
