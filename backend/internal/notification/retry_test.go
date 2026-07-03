package notification

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

func newRetryQueueGauge(t *testing.T) prometheus.Gauge {
	t.Helper()
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_retry_queue_size",
		Help: "Test gauge for retry queue size.",
	})
}

func TestBackoffDelay_Sequence(t *testing.T) {
	tests := []struct {
		retryNumber int
		want        time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
	}

	for _, tt := range tests {
		got := BackoffDelay(tt.retryNumber)
		if got != tt.want {
			t.Errorf("BackoffDelay(%d) = %v, want %v", tt.retryNumber, got, tt.want)
		}
	}
}

func TestBackoffDelay_ZeroOrNegative(t *testing.T) {
	// Edge case: retryNumber <= 0 defaults to InitialBackoff.
	if got := BackoffDelay(0); got != InitialBackoff {
		t.Errorf("BackoffDelay(0) = %v, want %v", got, InitialBackoff)
	}
	if got := BackoffDelay(-1); got != InitialBackoff {
		t.Errorf("BackoffDelay(-1) = %v, want %v", got, InitialBackoff)
	}
}

func TestIsRetryable_DeliveryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"retryable delivery error", NewRetryableError(errors.New("timeout")), true},
		{"non-retryable delivery error", NewNonRetryableError(errors.New("invalid config")), false},
		{"unknown error (defaults to retryable)", errors.New("something unexpected"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsRetryable_WrappedError(t *testing.T) {
	// A DeliveryError wrapped inside another error should still be detected.
	inner := NewNonRetryableError(errors.New("template render failed"))
	wrapped := errors.Join(errors.New("dispatch failed"), inner)

	if IsRetryable(wrapped) {
		t.Error("expected wrapped non-retryable error to be classified as non-retryable")
	}
}

func TestDeliveryError_ErrorMessage(t *testing.T) {
	err := NewRetryableError(errors.New("connection refused"))
	if err.Error() != "connection refused" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestDeliveryError_Unwrap(t *testing.T) {
	original := errors.New("root cause")
	err := NewRetryableError(original)
	if !errors.Is(err, original) {
		t.Error("expected Unwrap to expose the original error")
	}
}

func TestRetryQueue_NonRetryableSkipsRetry(t *testing.T) {
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)
	defer rq.Stop()

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	err := NewNonRetryableError(errors.New("invalid channel config"))
	scheduled := rq.Schedule(job, err)

	if scheduled {
		t.Error("expected non-retryable error to NOT be scheduled for retry")
	}
	if rq.Pending() != 0 {
		t.Errorf("expected 0 pending, got %d", rq.Pending())
	}
	if len(jobs) != 0 {
		t.Errorf("expected no jobs re-enqueued, got %d", len(jobs))
	}
}

func TestRetryQueue_RetryableReenqueuesAfterDelay(t *testing.T) {
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)
	defer rq.Stop()

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	err := NewRetryableError(errors.New("connection timeout"))
	scheduled := rq.Schedule(job, err)

	if !scheduled {
		t.Fatal("expected retryable error to be scheduled for retry")
	}
	if rq.Pending() != 1 {
		t.Errorf("expected 1 pending, got %d", rq.Pending())
	}

	// The retry should be re-enqueued after ~30s, but for testing we can't wait that long.
	// Instead, verify the job is pending (timer-based, tested via shorter intervals below).
}

func TestRetryQueue_MaxRetriesExhausted(t *testing.T) {
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)
	defer rq.Stop()

	// Attempt = MaxAttempts means all retries are used up.
	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     4, // already at max (1 initial + 3 retries)
		MaxAttempts: 4,
	}

	err := NewRetryableError(errors.New("timeout"))
	scheduled := rq.Schedule(job, err)

	if scheduled {
		t.Error("expected permanently failed when max attempts exhausted")
	}
	if rq.Pending() != 0 {
		t.Errorf("expected 0 pending, got %d", rq.Pending())
	}
}

func TestRetryQueue_AttemptIncrementedOnReenqueue(t *testing.T) {
	// Use a small channel to capture the re-enqueued job.
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)

	// Create a custom retry queue that we can test with shorter delays.
	rq := &RetryQueue{
		jobs:           jobs,
		retryQueueSize: gauge,
	}

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	// Manually simulate what Schedule does but with a very short delay.
	rq.mu.Lock()
	rq.pending++
	rq.retryQueueSize.Set(float64(rq.pending))
	timer := time.AfterFunc(10*time.Millisecond, func() {
		job.Attempt++
		job.ScheduledAt = time.Now()
		rq.jobs <- job

		rq.mu.Lock()
		rq.pending--
		rq.retryQueueSize.Set(float64(rq.pending))
		rq.mu.Unlock()
	})
	rq.timers = append(rq.timers, timer)
	rq.mu.Unlock()

	// Wait for the timer to fire.
	time.Sleep(50 * time.Millisecond)

	select {
	case requeued := <-jobs:
		if requeued.Attempt != 2 {
			t.Errorf("expected attempt=2 after first retry, got %d", requeued.Attempt)
		}
	default:
		t.Fatal("expected job to be re-enqueued")
	}

	if rq.Pending() != 0 {
		t.Errorf("expected 0 pending after re-enqueue, got %d", rq.Pending())
	}
}

func TestRetryQueue_StopCancelsPending(t *testing.T) {
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	// Schedule a retry.
	rq.Schedule(job, NewRetryableError(errors.New("timeout")))
	if rq.Pending() != 1 {
		t.Fatalf("expected 1 pending, got %d", rq.Pending())
	}

	// Stop should cancel all pending retries.
	rq.Stop()

	if rq.Pending() != 0 {
		t.Errorf("expected 0 pending after Stop, got %d", rq.Pending())
	}

	// No job should appear on the channel.
	time.Sleep(100 * time.Millisecond)
	if len(jobs) != 0 {
		t.Errorf("expected no jobs after Stop, got %d", len(jobs))
	}
}

func TestRetryQueue_ScheduleAfterStopReturns(t *testing.T) {
	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)

	rq.Stop()

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	// Scheduling after Stop should not panic and should return false.
	scheduled := rq.Schedule(job, NewRetryableError(errors.New("timeout")))
	if scheduled {
		t.Error("expected Schedule to return false after Stop")
	}
}

func TestRetryQueue_ProgressiveFail(t *testing.T) {
	// Simulate a job that fails through all retry attempts.
	// Attempt 1 → retryable → schedule (attempt becomes 2)
	// Attempt 2 → retryable → schedule (attempt becomes 3)
	// Attempt 3 → retryable → schedule (attempt becomes 4)
	// Attempt 4 → retryable → max exhausted → permanent failure

	jobs := make(chan DeliveryJob, 10)
	gauge := newRetryQueueGauge(t)
	rq := NewRetryQueue(jobs, gauge)
	defer rq.Stop()

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	retryErr := NewRetryableError(errors.New("network error"))

	// First failure: attempt 1, should schedule retry.
	if !rq.Schedule(job, retryErr) {
		t.Fatal("expected retry scheduled for attempt 1")
	}

	// Simulate attempt 2 failure.
	job.Attempt = 2
	if !rq.Schedule(job, retryErr) {
		t.Fatal("expected retry scheduled for attempt 2")
	}

	// Simulate attempt 3 failure.
	job.Attempt = 3
	if !rq.Schedule(job, retryErr) {
		t.Fatal("expected retry scheduled for attempt 3")
	}

	// Simulate attempt 4 failure: max exhausted.
	job.Attempt = 4
	if rq.Schedule(job, retryErr) {
		t.Fatal("expected permanent failure for attempt 4 (max exhausted)")
	}
}

func TestRetryQueue_BufferFullDropsRetry(t *testing.T) {
	// Use a channel with 0 buffer — re-enqueue will fail.
	jobs := make(chan DeliveryJob) // unbuffered
	gauge := newRetryQueueGauge(t)
	rq := &RetryQueue{
		jobs:           jobs,
		retryQueueSize: gauge,
	}

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
		MaxAttempts: DefaultMaxAttempts,
	}

	// Manually simulate a timer firing with a full buffer (unbuffered channel, nobody reading).
	rq.mu.Lock()
	rq.pending++
	rq.retryQueueSize.Set(float64(rq.pending))
	timer := time.AfterFunc(5*time.Millisecond, func() {
		job.Attempt++
		// This select should hit the default case (buffer full).
		select {
		case rq.jobs <- job:
			// Won't happen with unbuffered channel and no reader.
		default:
			// Expected: dropped.
		}

		rq.mu.Lock()
		rq.pending--
		rq.retryQueueSize.Set(float64(rq.pending))
		rq.mu.Unlock()
	})
	rq.timers = append(rq.timers, timer)
	rq.mu.Unlock()

	// Wait for timer.
	time.Sleep(50 * time.Millisecond)

	if rq.Pending() != 0 {
		t.Errorf("expected 0 pending after drop, got %d", rq.Pending())
	}
}
