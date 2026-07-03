package notification

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Default retry configuration constants.
const (
	// InitialBackoff is the delay before the first retry attempt.
	InitialBackoff = 30 * time.Second

	// BackoffMultiplier doubles the delay on each subsequent retry.
	BackoffMultiplier = 2

	// MaxRetries is the maximum number of retry attempts (3 retries = 4 total attempts).
	MaxRetries = 3

	// DefaultMaxAttempts is the total number of delivery attempts (initial + retries).
	DefaultMaxAttempts = MaxRetries + 1 // 4
)

// DeliveryError wraps an underlying error with retry classification.
type DeliveryError struct {
	Err       error
	Retryable bool
}

func (e *DeliveryError) Error() string {
	return e.Err.Error()
}

func (e *DeliveryError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a DeliveryError marked as retryable.
func NewRetryableError(err error) *DeliveryError {
	return &DeliveryError{Err: err, Retryable: true}
}

// NewNonRetryableError creates a DeliveryError marked as non-retryable.
func NewNonRetryableError(err error) *DeliveryError {
	return &DeliveryError{Err: err, Retryable: false}
}

// IsRetryable determines whether an error should be retried.
// Returns true for DeliveryError with Retryable=true, and for unknown errors
// (conservative approach — retry unless explicitly non-retryable).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Retryable
	}
	// Unknown errors are treated as retryable (network issues, etc.).
	return true
}

// BackoffDelay calculates the backoff delay for a given attempt number.
// Attempt 1 (first retry) → 30s, attempt 2 → 60s, attempt 3 → 120s.
// The retryNumber is 1-indexed (1 = first retry, 2 = second retry, etc.).
func BackoffDelay(retryNumber int) time.Duration {
	if retryNumber <= 0 {
		return InitialBackoff
	}
	delay := InitialBackoff
	for i := 1; i < retryNumber; i++ {
		delay *= BackoffMultiplier
	}
	return delay
}

// RetryQueue manages retrying failed delivery jobs with exponential backoff.
// It holds a reference to the dispatcher's jobs channel to re-enqueue jobs
// after the appropriate backoff delay.
type RetryQueue struct {
	mu             sync.Mutex
	jobs           chan<- DeliveryJob
	retryQueueSize prometheus.Gauge
	pending        int
	timers         []*time.Timer
	closed         bool
}

// NewRetryQueue creates a RetryQueue that re-enqueues failed jobs onto the
// provided jobs channel after the appropriate backoff delay.
func NewRetryQueue(jobs chan<- DeliveryJob, retryQueueSize prometheus.Gauge) *RetryQueue {
	return &RetryQueue{
		jobs:           jobs,
		retryQueueSize: retryQueueSize,
	}
}

// Schedule evaluates a failed job and either schedules it for retry (if retryable
// and attempts remain) or marks it as permanently failed.
// Returns true if the job was scheduled for retry, false if permanently failed.
func (rq *RetryQueue) Schedule(job DeliveryJob, err error) bool {
	// Non-retryable errors → permanent failure immediately.
	if !IsRetryable(err) {
		log.Printf("notification: retry skipped (non-retryable) monitor=%s trigger=%s attempt=%d err=%v",
			job.MonitorID, job.TriggerType, job.Attempt, err)
		return false
	}

	// All retries exhausted → permanent failure.
	if job.Attempt >= job.MaxAttempts {
		log.Printf("notification: permanently failed (max retries exhausted) monitor=%s trigger=%s attempts=%d/%d err=%v",
			job.MonitorID, job.TriggerType, job.Attempt, job.MaxAttempts, err)
		return false
	}

	// Calculate retry number (how many retries have been done so far).
	// Attempt starts at 1 (initial), so retryNumber = current attempt (which becomes the Nth retry).
	retryNumber := job.Attempt // attempt 1 → retry 1 (30s), attempt 2 → retry 2 (60s), etc.
	delay := BackoffDelay(retryNumber)

	rq.mu.Lock()
	if rq.closed {
		rq.mu.Unlock()
		log.Printf("notification: retry skipped (queue closed) monitor=%s trigger=%s", job.MonitorID, job.TriggerType)
		return false
	}
	rq.pending++
	rq.retryQueueSize.Set(float64(rq.pending))

	timer := time.AfterFunc(delay, func() {
		// Increment the attempt counter for the next delivery.
		job.Attempt++
		job.ScheduledAt = time.Now()

		// Re-enqueue onto the dispatcher's jobs channel.
		select {
		case rq.jobs <- job:
			log.Printf("notification: retry re-enqueued monitor=%s trigger=%s attempt=%d/%d",
				job.MonitorID, job.TriggerType, job.Attempt, job.MaxAttempts)
		default:
			// Buffer full — drop the retry.
			log.Printf("notification: retry dropped (buffer full) monitor=%s trigger=%s attempt=%d/%d",
				job.MonitorID, job.TriggerType, job.Attempt, job.MaxAttempts)
		}

		rq.mu.Lock()
		rq.pending--
		rq.retryQueueSize.Set(float64(rq.pending))
		rq.mu.Unlock()
	})

	rq.timers = append(rq.timers, timer)
	rq.mu.Unlock()

	log.Printf("notification: retry scheduled monitor=%s trigger=%s attempt=%d/%d delay=%s",
		job.MonitorID, job.TriggerType, job.Attempt, job.MaxAttempts, delay)
	return true
}

// Stop cancels all pending retry timers and prevents new retries from being scheduled.
func (rq *RetryQueue) Stop() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.closed = true
	for _, t := range rq.timers {
		t.Stop()
	}
	rq.timers = nil
	rq.pending = 0
	rq.retryQueueSize.Set(0)
}

// Pending returns the number of retries currently waiting in the queue.
func (rq *RetryQueue) Pending() int {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	return rq.pending
}
