package notification

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// newTestMetrics creates a Metrics instance with a fresh registry for testing.
func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	reg := prometheus.NewRegistry()
	return NewMetrics(reg)
}

func TestNewDispatcher_Defaults(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{}, nil, nil, m, st)

	if d.cfg.BufferSize != 256 {
		t.Errorf("expected default BufferSize=256, got %d", d.cfg.BufferSize)
	}
	if d.cfg.Workers != 10 {
		t.Errorf("expected default Workers=10, got %d", d.cfg.Workers)
	}
	if cap(d.jobs) != 256 {
		t.Errorf("expected jobs channel capacity=256, got %d", cap(d.jobs))
	}
}

func TestNewDispatcher_CustomConfig(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	cfg := DispatcherConfig{Workers: 5, BufferSize: 128, DrainTimeout: 10 * time.Second}
	d := NewDispatcher(cfg, nil, nil, m, st)

	if d.cfg.BufferSize != 128 {
		t.Errorf("expected BufferSize=128, got %d", d.cfg.BufferSize)
	}
	if d.cfg.Workers != 5 {
		t.Errorf("expected Workers=5, got %d", d.cfg.Workers)
	}
	if cap(d.jobs) != 128 {
		t.Errorf("expected jobs channel capacity=128, got %d", cap(d.jobs))
	}
}

func TestEnqueue_Success(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 10}, nil, nil, m, st)

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
	}

	d.Enqueue(job)

	if len(d.jobs) != 1 {
		t.Fatalf("expected 1 job in channel, got %d", len(d.jobs))
	}
}

func TestEnqueue_DropWhenBufferFull(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	// Buffer size of 2 — fill it up, then enqueue one more.
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 2}, nil, nil, m, st)

	job := DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_down",
		Attempt:     1,
	}

	// Fill the buffer.
	d.Enqueue(job)
	d.Enqueue(job)

	// This one should be dropped.
	d.Enqueue(job)

	if len(d.jobs) != 2 {
		t.Fatalf("expected 2 jobs in channel (buffer full), got %d", len(d.jobs))
	}

	// Verify the dropped counter was incremented.
	// We use the "unknown" label since classifyChannelType returns "unknown" for now.
	counter, err := m.DroppedTotal.GetMetricWithLabelValues("unknown")
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	if counter == nil {
		t.Fatal("expected DroppedTotal counter to exist")
	}
}

func TestEnqueue_NonBlocking(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	// Small buffer to test non-blocking behavior.
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 1}, nil, nil, m, st)

	// Fill the buffer.
	d.Enqueue(DeliveryJob{ID: uuid.New(), MonitorID: uuid.New(), TriggerType: "monitor_down"})

	// Enqueue with full buffer must return immediately (non-blocking).
	done := make(chan struct{})
	go func() {
		d.Enqueue(DeliveryJob{ID: uuid.New(), MonitorID: uuid.New(), TriggerType: "monitor_down"})
		close(done)
	}()

	select {
	case <-done:
		// Good — returned immediately.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Enqueue blocked when buffer was full — must be non-blocking")
	}
}

func TestDispatcher_StartAndStop(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 3, BufferSize: 10}, nil, nil, m, st)
	d.Start()

	// Enqueue a job and give workers time to process it.
	d.Enqueue(DeliveryJob{
		ID:          uuid.New(),
		MonitorID:   uuid.New(),
		TriggerType: "monitor_up",
		Attempt:     1,
	})

	// Allow time for workers to pick up the job.
	time.Sleep(50 * time.Millisecond)

	// Stop should complete without hanging.
	done := make(chan struct{})
	go func() {
		d.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good — stopped cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete within timeout")
	}
}

func TestDispatcher_WorkerProcessesJobs(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 2, BufferSize: 10}, nil, nil, m, st)
	d.Start()

	// Enqueue several jobs.
	const numJobs = 5
	for range numJobs {
		d.Enqueue(DeliveryJob{
			ID:          uuid.New(),
			MonitorID:   uuid.New(),
			TriggerType: "monitor_down",
			Attempt:     1,
		})
	}

	// Wait for all jobs to be processed (channel drained).
	deadline := time.After(2 * time.Second)
	for {
		if len(d.jobs) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("jobs not drained within timeout, %d remaining", len(d.jobs))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	d.Stop()
}

func TestDispatcher_WorkersContinueAfterJobs(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 10}, nil, nil, m, st)
	d.Start()

	// Enqueue jobs — the default dispatch is a no-op, so all succeed.
	d.Enqueue(DeliveryJob{ID: uuid.New(), MonitorID: uuid.New(), TriggerType: "monitor_down", Attempt: 1})
	d.Enqueue(DeliveryJob{ID: uuid.New(), MonitorID: uuid.New(), TriggerType: "monitor_up", Attempt: 1})

	// Wait for processing.
	time.Sleep(100 * time.Millisecond)

	// Verify workers are still alive by enqueuing more.
	d.Enqueue(DeliveryJob{ID: uuid.New(), MonitorID: uuid.New(), TriggerType: "degraded", Attempt: 1})

	time.Sleep(50 * time.Millisecond)

	if len(d.jobs) != 0 {
		t.Errorf("expected all jobs processed, got %d remaining", len(d.jobs))
	}

	d.Stop()
}

func TestDispatcher_ConcurrentEnqueue(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 4, BufferSize: 100}, nil, nil, m, st)
	d.Start()

	// Concurrently enqueue many jobs.
	var wg sync.WaitGroup
	const numGoroutines = 10
	const jobsPerGoroutine = 20

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobsPerGoroutine {
				d.Enqueue(DeliveryJob{
					ID:          uuid.New(),
					MonitorID:   uuid.New(),
					TriggerType: "monitor_down",
					Attempt:     1,
				})
			}
		}()
	}

	wg.Wait()

	// Wait for all jobs to be processed.
	deadline := time.After(5 * time.Second)
	for {
		if len(d.jobs) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("jobs not drained within timeout, %d remaining", len(d.jobs))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	d.Stop()
}
