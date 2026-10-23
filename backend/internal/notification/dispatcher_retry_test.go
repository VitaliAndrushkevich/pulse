package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestDispatcherRetriesProductionFailures(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 4}, nil, nil, NewMetrics(prometheus.NewRegistry()), NewStateTracker(), nil)
	d.retries.backoff = func(int) time.Duration { return time.Millisecond }
	attempts := make(chan DeliveryJob, 4)
	d.dispatchFn = func(job DeliveryJob) error {
		attempts <- job
		if job.Attempt < 3 {
			return NewRetryableError(errors.New("unavailable"))
		}
		return nil
	}
	id := uuid.New()
	d.Start()
	defer d.Shutdown(context.Background())
	d.Enqueue(DeliveryJob{ID: id, ChannelID: uuid.New(), ChannelType: "matrix", Attempt: 1, MaxAttempts: 4})
	for want := 1; want <= 3; want++ {
		select {
		case job := <-attempts:
			if job.ID != id || job.Attempt != want {
				t.Fatalf("unexpected retry: %+v", job)
			}
		case <-time.After(time.Second):
			t.Fatal("retry not delivered")
		}
	}
}

func TestDispatcherPermanentFailureDoesNotRetry(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(DispatcherConfig{}, nil, nil, NewMetrics(prometheus.NewRegistry()), NewStateTracker(), nil)
	defer d.Stop()
	d.dispatchFn = func(DeliveryJob) error { return NewNonRetryableError(errors.New("invalid credentials")) }
	d.processJob(t.Context(), 0, DeliveryJob{ChannelType: "telegram", Attempt: 1, MaxAttempts: 4})
	if d.retries.Pending() != 0 {
		t.Fatal("permanent failure retried")
	}
	metric := &dto.Metric{}
	d.metrics.DeliveriesTotal.WithLabelValues("telegram", "failure").Write(metric)
	if got := metric.GetCounter().GetValue(); got != 1 {
		t.Fatalf("provider metric=%v", got)
	}
}

func TestRetryQueueCooldownDefersWithoutSpendingAttempt(t *testing.T) {
	t.Parallel()
	jobs := make(chan DeliveryJob, 4)
	rq := NewRetryQueue(jobs, newRetryQueueGauge(t))
	defer rq.Stop()
	rq.backoff = func(int) time.Duration { return time.Millisecond }
	id := uuid.New()
	job := DeliveryJob{ID: uuid.New(), ChannelID: id, Attempt: 1, MaxAttempts: 4}
	if !rq.Schedule(job, NewRateLimitError(errors.New("rate limited"), 30*time.Millisecond)) {
		t.Fatal("not scheduled")
	}
	delay := rq.Cooldown(id)
	if delay <= 0 {
		t.Fatal("missing cooldown")
	}
	other := job
	other.ID = uuid.New()
	if !rq.Defer(other, delay) {
		t.Fatal("not deferred")
	}
	if rq.Cooldown(uuid.New()) != 0 {
		t.Fatal("cooldown crossed channels")
	}
	seen := map[uuid.UUID]int{}
	for range 2 {
		select {
		case got := <-jobs:
			seen[got.ID] = got.Attempt
		case <-time.After(time.Second):
			t.Fatal("not requeued")
		}
	}
	if seen[job.ID] != 2 || seen[other.ID] != 1 {
		t.Fatal("incorrect attempts")
	}
	rq.Stop()
	if rq.Pending() != 0 || rq.Cooldown(id) != 0 {
		t.Fatal("shutdown retained retries")
	}
}

func TestRetryQueueCooldownPreservesLongestEvenAfterFinalAttempt(t *testing.T) {
	t.Parallel()
	rq := NewRetryQueue(make(chan DeliveryJob, 4), newRetryQueueGauge(t))
	defer rq.Stop()
	job := DeliveryJob{ChannelID: uuid.New(), Attempt: 4, MaxAttempts: 4}
	if rq.Schedule(job, NewRateLimitError(errors.New("rate limited"), time.Hour)) {
		t.Fatal("final attempt retried")
	}
	if rq.Cooldown(job.ChannelID) < 59*time.Minute {
		t.Fatal("final attempt lost cooldown")
	}
	rq.Schedule(job, NewRateLimitError(errors.New("rate limited"), time.Minute))
	if rq.Cooldown(job.ChannelID) < 59*time.Minute {
		t.Fatal("shortened provider cooldown")
	}
}
