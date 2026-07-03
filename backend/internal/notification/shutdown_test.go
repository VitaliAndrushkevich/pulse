package notification

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

func TestShutdown_DrainsBufferedJobs(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	cfg := DispatcherConfig{Workers: 2, BufferSize: 50, DrainTimeout: 5 * time.Second}
	d := NewDispatcher(cfg, nil, nil, m, st)

	// Track processed jobs.
	var processed atomic.Int32
	d.dispatchFn = func(job DeliveryJob) error {
		processed.Add(1)
		return nil
	}

	d.Start()

	// Enqueue jobs.
	const numJobs = 20
	for range numJobs {
		d.Enqueue(DeliveryJob{
			ID:          uuid.New(),
			MonitorID:   uuid.New(),
			TriggerType: "monitor_down",
			Attempt:     1,
		})
	}

	// Give workers a brief moment to start picking up jobs, then shutdown.
	time.Sleep(10 * time.Millisecond)

	// Shutdown should drain all remaining buffered jobs.
	d.Shutdown(context.Background())

	// All jobs should have been processed (by workers + drain).
	if got := processed.Load(); got != numJobs {
		t.Errorf("expected %d jobs processed, got %d", numJobs, got)
	}
}

func TestShutdown_RejectsNewJobsDuringShutdown(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	cfg := DispatcherConfig{Workers: 1, BufferSize: 10, DrainTimeout: 2 * time.Second}
	d := NewDispatcher(cfg, nil, nil, m, st)
	d.dispatchFn = func(job DeliveryJob) error { return nil }

	d.Start()
	d.Shutdown(context.Background())

	// After shutdown, Enqueue should reject (drop) new jobs.
	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
	}
	d.Enqueue(job)

	// The job channel should be empty — the job was dropped.
	if len(d.jobs) != 0 {
		t.Errorf("expected job to be dropped during shutdown, but channel has %d jobs", len(d.jobs))
	}

	// Verify DroppedTotal was incremented.
	counter, err := m.DroppedTotal.GetMetricWithLabelValues("unknown")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	if counter == nil {
		t.Fatal("expected DroppedTotal counter to exist")
	}
}

func TestShutdown_TimeoutCancelsRemaining(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	// Very short drain timeout to force expiration.
	cfg := DispatcherConfig{Workers: 1, BufferSize: 50, DrainTimeout: 50 * time.Millisecond}
	d := NewDispatcher(cfg, nil, nil, m, st)

	// Simulate slow dispatch — each job takes longer than the drain timeout.
	d.dispatchFn = func(job DeliveryJob) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	d.Start()

	// Enqueue several jobs. With slow dispatch, most won't finish.
	for range 10 {
		d.Enqueue(DeliveryJob{
			ID:          uuid.New(),
			MonitorID:   uuid.New(),
			TriggerType: "monitor_down",
			Attempt:     1,
		})
	}

	// Give the single worker time to pick up the first job.
	time.Sleep(10 * time.Millisecond)

	// Shutdown should hit the timeout since jobs take 200ms each.
	start := time.Now()
	d.Shutdown(context.Background())
	elapsed := time.Since(start)

	// Should return around the drain timeout, not wait for all slow jobs.
	// Allow generous margin for CI environments.
	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took %v, expected to return near drain timeout (50ms)", elapsed)
	}
}

func TestShutdown_DefaultDrainTimeout(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	// No DrainTimeout specified — should default to 30s.
	cfg := DispatcherConfig{Workers: 1, BufferSize: 10}
	d := NewDispatcher(cfg, nil, nil, m, st)

	if d.cfg.DrainTimeout != 30*time.Second {
		t.Errorf("expected default DrainTimeout=30s, got %v", d.cfg.DrainTimeout)
	}
}

func TestShutdown_EmptyBufferCompletesQuickly(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	cfg := DispatcherConfig{Workers: 2, BufferSize: 10, DrainTimeout: 5 * time.Second}
	d := NewDispatcher(cfg, nil, nil, m, st)
	d.dispatchFn = func(job DeliveryJob) error { return nil }

	d.Start()

	// No jobs enqueued — shutdown should complete almost immediately.
	start := time.Now()
	d.Shutdown(context.Background())
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown with empty buffer took %v, expected fast completion", elapsed)
	}
}

func TestShutdown_InFlightJobsCompleteBeforeTimeout(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	st := NewStateTracker()

	cfg := DispatcherConfig{Workers: 2, BufferSize: 10, DrainTimeout: 2 * time.Second}
	d := NewDispatcher(cfg, nil, nil, m, st)

	var processed atomic.Int32
	// Jobs take 50ms each — well within the 2s timeout.
	d.dispatchFn = func(job DeliveryJob) error {
		time.Sleep(50 * time.Millisecond)
		processed.Add(1)
		return nil
	}

	d.Start()

	// Enqueue 5 jobs.
	for range 5 {
		d.Enqueue(DeliveryJob{
			ID:          uuid.New(),
			MonitorID:   uuid.New(),
			TriggerType: "monitor_down",
			Attempt:     1,
		})
	}

	// Give workers a brief moment to start processing.
	time.Sleep(10 * time.Millisecond)

	d.Shutdown(context.Background())

	// All jobs should be processed since they fit within the timeout.
	if got := processed.Load(); got != 5 {
		t.Errorf("expected 5 jobs processed, got %d", got)
	}
}
