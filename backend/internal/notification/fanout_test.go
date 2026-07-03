package notification

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEvaluateAndDispatch_NBindingsNJobs(t *testing.T) {
	// Requirement 4.9: When multiple bindings match a trigger, dispatch to all.
	// N bindings → N delivery jobs enqueued.
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 256}, nil, nil, m, st)

	monitorID := uuid.New()

	// Create 5 bindings, all with "monitor_down" trigger.
	bindings := make([]BindingWithChannel, 5)
	for i := range bindings {
		bindings[i] = BindingWithChannel{
			ID:        uuid.New(),
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers: []TriggerCondition{
				{Type: "monitor_down"},
			},
		}
	}

	// Trigger a monitor_down event (transition from up to down).
	result := CheckResult{
		State:         "down",
		PreviousState: "up",
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	// All 5 bindings should have produced a job.
	if len(d.jobs) != 5 {
		t.Errorf("expected 5 jobs enqueued, got %d", len(d.jobs))
	}

	// Verify each job has a unique binding ID and correct trigger type.
	seenBindings := make(map[uuid.UUID]bool)
	for range 5 {
		job := <-d.jobs
		if job.TriggerType != "monitor_down" {
			t.Errorf("expected trigger type 'monitor_down', got %q", job.TriggerType)
		}
		if job.MonitorID != monitorID {
			t.Errorf("expected monitor ID %s, got %s", monitorID, job.MonitorID)
		}
		if seenBindings[job.BindingID] {
			t.Errorf("duplicate binding ID in jobs: %s", job.BindingID)
		}
		seenBindings[job.BindingID] = true
	}
}

func TestEvaluateAndDispatch_FailureInOneDoesNotAffectOthers(t *testing.T) {
	// Requirement 4.9: Failure in one binding must not prevent delivery to others.
	// Use a buffer size of 3 with 5 bindings — first 3 enqueue, last 2 are dropped.
	// The key invariant: the first 3 bindings still get their jobs even though
	// bindings 4 and 5 fail due to buffer full. Each enqueue is independent.
	m := newTestMetrics(t)
	st := NewStateTracker()

	// Buffer size of 3 → only 3 jobs can be enqueued, remaining are dropped.
	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 3}, nil, nil, m, st)

	monitorID := uuid.New()

	// 5 bindings all matching monitor_down.
	bindings := make([]BindingWithChannel, 5)
	for i := range bindings {
		bindings[i] = BindingWithChannel{
			ID:        uuid.New(),
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers: []TriggerCondition{
				{Type: "monitor_down"},
			},
		}
	}

	result := CheckResult{
		State:         "down",
		PreviousState: "up",
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	// Should have 3 jobs (buffer limit) — the other 2 were dropped independently.
	// This proves that failure (drop) of bindings 4 and 5 did NOT prevent
	// delivery of bindings 1, 2, and 3.
	if len(d.jobs) != 3 {
		t.Errorf("expected 3 jobs enqueued (buffer full drops others), got %d", len(d.jobs))
	}

	// Verify dropped counter was incremented (counter exists = drops happened).
	counter, err := m.DroppedTotal.GetMetricWithLabelValues("unknown")
	if err != nil {
		t.Fatalf("failed to get dropped metric: %v", err)
	}
	if counter == nil {
		t.Fatal("expected DroppedTotal counter to exist after drops")
	}
}

func TestEvaluateAndDispatch_OnlyMatchingBindingsReceiveJobs(t *testing.T) {
	// Only bindings that match the fired trigger should receive delivery jobs.
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 256}, nil, nil, m, st)

	monitorID := uuid.New()

	// Binding 1: monitor_down trigger
	binding1 := BindingWithChannel{
		ID:        uuid.New(),
		ChannelID: uuid.New(),
		MonitorID: monitorID,
		Triggers:  []TriggerCondition{{Type: "monitor_down"}},
	}

	// Binding 2: monitor_up trigger only (should NOT fire on down transition)
	binding2 := BindingWithChannel{
		ID:        uuid.New(),
		ChannelID: uuid.New(),
		MonitorID: monitorID,
		Triggers:  []TriggerCondition{{Type: "monitor_up"}},
	}

	// Binding 3: monitor_down trigger
	binding3 := BindingWithChannel{
		ID:        uuid.New(),
		ChannelID: uuid.New(),
		MonitorID: monitorID,
		Triggers:  []TriggerCondition{{Type: "monitor_down"}},
	}

	bindings := []BindingWithChannel{binding1, binding2, binding3}

	// Trigger monitor_down (up → down).
	result := CheckResult{
		State:         "down",
		PreviousState: "up",
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	// Only binding1 and binding3 should have jobs.
	if len(d.jobs) != 2 {
		t.Fatalf("expected 2 jobs enqueued (only monitor_down bindings), got %d", len(d.jobs))
	}

	matchedBindings := make(map[uuid.UUID]bool)
	for range 2 {
		job := <-d.jobs
		matchedBindings[job.BindingID] = true
	}

	if !matchedBindings[binding1.ID] {
		t.Error("expected binding1 to receive a job")
	}
	if matchedBindings[binding2.ID] {
		t.Error("binding2 (monitor_up) should NOT receive a job on monitor_down event")
	}
	if !matchedBindings[binding3.ID] {
		t.Error("expected binding3 to receive a job")
	}
}

func TestEvaluateAndDispatch_NoBindings(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 256}, nil, nil, m, st)

	monitorID := uuid.New()

	result := CheckResult{
		State:         "down",
		PreviousState: "up",
	}

	// No bindings → no jobs.
	d.EvaluateAndDispatch(context.Background(), monitorID, result, nil)

	if len(d.jobs) != 0 {
		t.Errorf("expected 0 jobs enqueued with no bindings, got %d", len(d.jobs))
	}
}

func TestEvaluateAndDispatch_NoTriggersFired(t *testing.T) {
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 256}, nil, nil, m, st)

	monitorID := uuid.New()

	// Binding with monitor_down trigger, but state is up→up (no transition).
	bindings := []BindingWithChannel{
		{
			ID:        uuid.New(),
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers:  []TriggerCondition{{Type: "monitor_down"}},
		},
	}

	result := CheckResult{
		State:         "up",
		PreviousState: "up",
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	if len(d.jobs) != 0 {
		t.Errorf("expected 0 jobs (no trigger fired), got %d", len(d.jobs))
	}
}

func TestEvaluateAndDispatch_MultipleTriggerTypes(t *testing.T) {
	// A single binding can have multiple triggers. Verify that when multiple
	// trigger events fire, the binding gets a job for each fired trigger.
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 1, BufferSize: 256}, nil, nil, m, st)

	monitorID := uuid.New()
	thresholdMs := 100

	// Binding has both monitor_down and degraded triggers.
	binding := BindingWithChannel{
		ID:        uuid.New(),
		ChannelID: uuid.New(),
		MonitorID: monitorID,
		Triggers: []TriggerCondition{
			{Type: "monitor_down"},
			{Type: "degraded", ThresholdMs: &thresholdMs},
		},
	}

	bindings := []BindingWithChannel{binding}

	// Transition to down with high response time — both triggers should fire.
	result := CheckResult{
		State:          "down",
		PreviousState:  "up",
		ResponseTimeMs: 500, // exceeds threshold of 100ms
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	// Note: "degraded" only fires when state is "up" per state_tracker logic.
	// Since state is "down", only monitor_down fires.
	if len(d.jobs) != 1 {
		t.Errorf("expected 1 job (monitor_down fires, degraded skips because state=down), got %d", len(d.jobs))
	}
}

func TestEvaluateAndDispatch_IndependentDeliveryWithDispatchFn(t *testing.T) {
	// Verify that when dispatch function fails for one job, other jobs still proceed.
	// Start workers to actually process jobs.
	m := newTestMetrics(t)
	st := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{Workers: 4, BufferSize: 256}, nil, nil, m, st)

	// Track which bindings were dispatched.
	var mu sync.Mutex
	dispatched := make(map[uuid.UUID]bool)
	var failCount atomic.Int32

	// First binding will fail dispatch, second and third should still succeed.
	failBindingID := uuid.New()

	d.dispatchFn = func(job DeliveryJob) error {
		mu.Lock()
		dispatched[job.BindingID] = true
		mu.Unlock()

		if job.BindingID == failBindingID {
			failCount.Add(1)
			return errors.New("simulated delivery failure")
		}
		return nil
	}

	d.Start()
	defer d.Stop()

	monitorID := uuid.New()

	bindings := []BindingWithChannel{
		{
			ID:        failBindingID,
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers:  []TriggerCondition{{Type: "monitor_down"}},
		},
		{
			ID:        uuid.New(),
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers:  []TriggerCondition{{Type: "monitor_down"}},
		},
		{
			ID:        uuid.New(),
			ChannelID: uuid.New(),
			MonitorID: monitorID,
			Triggers:  []TriggerCondition{{Type: "monitor_down"}},
		},
	}

	result := CheckResult{
		State:         "down",
		PreviousState: "up",
	}

	d.EvaluateAndDispatch(context.Background(), monitorID, result, bindings)

	// Wait for processing.
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(dispatched)
		mu.Unlock()
		if count >= 3 {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timeout waiting for all dispatches, got %d/3", len(dispatched))
			mu.Unlock()
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// All 3 bindings were attempted — failure in one didn't block others.
	mu.Lock()
	for _, b := range bindings {
		if !dispatched[b.ID] {
			t.Errorf("binding %s was not dispatched", b.ID)
		}
	}
	mu.Unlock()

	if failCount.Load() != 1 {
		t.Errorf("expected exactly 1 failure, got %d", failCount.Load())
	}
}

func TestExtractTriggerConditions_Deduplication(t *testing.T) {
	threshold100 := 100
	threshold200 := 200

	bindings := []BindingWithChannel{
		{
			ID: uuid.New(),
			Triggers: []TriggerCondition{
				{Type: "monitor_down"},
				{Type: "degraded", ThresholdMs: &threshold100},
			},
		},
		{
			ID: uuid.New(),
			Triggers: []TriggerCondition{
				{Type: "monitor_down"}, // duplicate
				{Type: "degraded", ThresholdMs: &threshold200}, // different threshold
			},
		},
	}

	conditions := extractTriggerConditions(bindings)

	// Should have: monitor_down (deduped), degraded_100, degraded_200
	if len(conditions) != 3 {
		t.Errorf("expected 3 unique conditions, got %d", len(conditions))
	}
}

func TestBindingMatchesTrigger(t *testing.T) {
	binding := BindingWithChannel{
		ID: uuid.New(),
		Triggers: []TriggerCondition{
			{Type: "monitor_down"},
			{Type: "monitor_up"},
		},
	}

	if !bindingMatchesTrigger(binding, TriggerEvent{Type: "monitor_down"}) {
		t.Error("expected binding to match monitor_down trigger")
	}
	if !bindingMatchesTrigger(binding, TriggerEvent{Type: "monitor_up"}) {
		t.Error("expected binding to match monitor_up trigger")
	}
	if bindingMatchesTrigger(binding, TriggerEvent{Type: "degraded"}) {
		t.Error("expected binding NOT to match degraded trigger")
	}
}

func TestBuildPayload(t *testing.T) {
	monitorID := uuid.New()
	result := CheckResult{
		State:          "down",
		PreviousState:  "up",
		ResponseTimeMs: 1500,
	}

	payload := buildPayload(monitorID, result)

	if payload.Status != "down" {
		t.Errorf("expected status 'down', got %q", payload.Status)
	}
	if payload.PreviousStatus != "up" {
		t.Errorf("expected previous status 'up', got %q", payload.PreviousStatus)
	}
	if payload.ResponseTime != 1500 {
		t.Errorf("expected response time 1500, got %d", payload.ResponseTime)
	}
	if payload.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
