package notification

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func int32Ptr(v int32) *int32 { return &v }

func TestEvaluate_MonitorDown_TransitionFiresOnce(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	bindings := []TriggerCondition{{Type: "monitor_down"}}

	// First transition: non-down → down → should fire
	result := CheckResult{State: "down", PreviousState: "up"}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "monitor_down" {
		t.Fatalf("expected 1 monitor_down event, got %+v", events)
	}

	// Second check: still down → should NOT fire again
	result = CheckResult{State: "down", PreviousState: "down"}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for ongoing down state, got %+v", events)
	}
}

func TestEvaluate_MonitorUp_RecoveryFiresOnce(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	bindings := []TriggerCondition{{Type: "monitor_up"}}

	// Recovery: down → up → should fire
	result := CheckResult{State: "up", PreviousState: "down"}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "monitor_up" {
		t.Fatalf("expected 1 monitor_up event, got %+v", events)
	}

	// Continued up → should NOT fire
	result = CheckResult{State: "up", PreviousState: "up"}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for continued up, got %+v", events)
	}
}

func TestEvaluate_Degraded_FiresOnceUntilRecovery(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	threshold := 5000
	bindings := []TriggerCondition{{Type: "degraded", ThresholdMs: &threshold}}

	// First degraded check
	result := CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 6000}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "degraded" {
		t.Fatalf("expected 1 degraded event, got %+v", events)
	}

	// Still degraded → no repeat
	result = CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 7000}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for ongoing degraded, got %+v", events)
	}

	// Response time recovers
	result = CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 3000}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for recovery below threshold, got %+v", events)
	}

	// Re-degrades → should fire again
	result = CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 6000}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "degraded" {
		t.Fatalf("expected 1 degraded event after re-trigger, got %+v", events)
	}
}

func TestEvaluate_SSLExpiring_FiresOnceUntilRenewal(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	daysBefore := 14
	bindings := []TriggerCondition{{Type: "ssl_expiring", DaysBefore: &daysBefore}}

	// SSL below threshold
	result := CheckResult{State: "up", PreviousState: "up", SSLDaysRemaining: int32Ptr(10)}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "ssl_expiring" {
		t.Fatalf("expected 1 ssl_expiring event, got %+v", events)
	}

	// Still below threshold → no repeat
	result = CheckResult{State: "up", PreviousState: "up", SSLDaysRemaining: int32Ptr(9)}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for ongoing SSL below threshold, got %+v", events)
	}

	// SSL renewed (above threshold)
	result = CheckResult{State: "up", PreviousState: "up", SSLDaysRemaining: int32Ptr(90)}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for SSL renewal, got %+v", events)
	}

	// Drops below threshold again → should fire
	result = CheckResult{State: "up", PreviousState: "up", SSLDaysRemaining: int32Ptr(5)}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "ssl_expiring" {
		t.Fatalf("expected 1 ssl_expiring event after re-trigger, got %+v", events)
	}
}

func TestEvaluate_NFailuresInRow_FiresOnceUntilRecovery(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	count := 3
	bindings := []TriggerCondition{{Type: "n_failures_in_row", Count: &count}}

	// Below threshold
	result := CheckResult{State: "down", PreviousState: "down", ConsecutiveFailures: 2}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events below threshold, got %+v", events)
	}

	// Reaches threshold → fires
	result = CheckResult{State: "down", PreviousState: "down", ConsecutiveFailures: 3}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "n_failures_in_row" {
		t.Fatalf("expected 1 n_failures_in_row event, got %+v", events)
	}

	// Exceeds threshold → no repeat
	result = CheckResult{State: "down", PreviousState: "down", ConsecutiveFailures: 5}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events for ongoing failures beyond threshold, got %+v", events)
	}

	// Recovery
	result = CheckResult{State: "up", PreviousState: "down", ConsecutiveFailures: 0}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		// No monitor_up binding, so no events
		t.Fatalf("expected no events (no monitor_up binding), got %+v", events)
	}

	// Failures again reach threshold → should fire again
	result = CheckResult{State: "down", PreviousState: "up", ConsecutiveFailures: 3}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "n_failures_in_row" {
		t.Fatalf("expected 1 n_failures_in_row event after recovery, got %+v", events)
	}
}

func TestEvaluate_RecoveryClearsAllFlags(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()

	threshold := 5000
	daysBefore := 14
	count := 3
	bindings := []TriggerCondition{
		{Type: "degraded", ThresholdMs: &threshold},
		{Type: "ssl_expiring", DaysBefore: &daysBefore},
		{Type: "n_failures_in_row", Count: &count},
		{Type: "monitor_up"},
	}

	// Step 1: Set degraded and SSL warned while monitor is up
	result := CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 6000, SSLDaysRemaining: int32Ptr(5)}
	st.Evaluate(monitorID, result, bindings)

	// Verify degraded and SSL flags are set
	state := st.GetState(monitorID)
	if !state.IsDegraded || !state.SSLWarned {
		t.Fatalf("expected IsDegraded and SSLWarned set, got %+v", state)
	}

	// Step 2: Monitor goes down with enough consecutive failures
	result = CheckResult{State: "down", PreviousState: "up", ConsecutiveFailures: 3}
	st.Evaluate(monitorID, result, bindings)

	// Verify ConsecFailuresFired is now set.
	// Note: IsDegraded gets cleared by the degraded case since State="down" means
	// isDegradedNow is false, which clears the flag. That's correct behavior —
	// a downed monitor is not "degraded". SSLWarned remains because there's no
	// recovery logic for SSL in the down transition.
	state = st.GetState(monitorID)
	if !state.ConsecFailuresFired {
		t.Fatalf("expected ConsecFailuresFired set, got %+v", state)
	}
	if !state.SSLWarned {
		t.Fatalf("expected SSLWarned to persist through down state, got %+v", state)
	}

	// Step 3: Recovery (down → up) should clear all flags
	result = CheckResult{State: "up", PreviousState: "down", ConsecutiveFailures: 0, ResponseTimeMs: 100, SSLDaysRemaining: int32Ptr(90)}
	events := st.Evaluate(monitorID, result, bindings)

	// Should fire monitor_up
	found := false
	for _, e := range events {
		if e.Type == "monitor_up" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected monitor_up event on recovery, got %+v", events)
	}

	// Check all state flags are cleared
	state = st.GetState(monitorID)
	if state.IsDegraded || state.SSLWarned || state.ConsecFailuresFired {
		t.Fatalf("expected all flags cleared after recovery, got %+v", state)
	}
}

func TestEvaluate_MultipleBindingsFireIndependently(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	bindings := []TriggerCondition{
		{Type: "monitor_down"},
		{Type: "monitor_down"}, // duplicate trigger — both should produce events
	}

	result := CheckResult{State: "down", PreviousState: "up"}
	events := st.Evaluate(monitorID, result, bindings)
	// Both monitor_down bindings match the transition
	if len(events) != 2 {
		t.Fatalf("expected 2 monitor_down events (one per binding), got %+v", events)
	}
}

func TestEvaluate_NoBindingsNoEvents(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()

	result := CheckResult{State: "down", PreviousState: "up"}
	events := st.Evaluate(monitorID, result, nil)
	if len(events) != 0 {
		t.Fatalf("expected no events with nil bindings, got %+v", events)
	}

	events = st.Evaluate(monitorID, result, []TriggerCondition{})
	if len(events) != 0 {
		t.Fatalf("expected no events with empty bindings, got %+v", events)
	}
}

func TestEvaluate_DegradedNotFiredWhenDown(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	threshold := 5000
	bindings := []TriggerCondition{{Type: "degraded", ThresholdMs: &threshold}}

	// High response time but monitor is down — degraded should NOT fire
	result := CheckResult{State: "down", PreviousState: "up", ResponseTimeMs: 10000}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no degraded event when monitor is down, got %+v", events)
	}
}

func TestEvaluate_SSLExpiringNilSSLDays(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	daysBefore := 14
	bindings := []TriggerCondition{{Type: "ssl_expiring", DaysBefore: &daysBefore}}

	// No SSL data available
	result := CheckResult{State: "up", PreviousState: "up", SSLDaysRemaining: nil}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events when SSL data is nil, got %+v", events)
	}
}

func TestEvaluate_ThreadSafety(t *testing.T) {
	st := NewStateTracker()
	bindings := []TriggerCondition{{Type: "monitor_down"}}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitorID := uuid.New()
			result := CheckResult{State: "down", PreviousState: "up"}
			st.Evaluate(monitorID, result, bindings)
		}()
	}
	wg.Wait()
}

func TestClearState_RemovesMonitor(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	bindings := []TriggerCondition{{Type: "monitor_down"}}

	result := CheckResult{State: "down", PreviousState: "up"}
	st.Evaluate(monitorID, result, bindings)

	if st.GetState(monitorID) == nil {
		t.Fatal("expected state to exist after Evaluate")
	}

	st.ClearState(monitorID)

	if st.GetState(monitorID) != nil {
		t.Fatal("expected state to be nil after ClearState")
	}
}

func TestGetState_ReturnsNilForUnknownMonitor(t *testing.T) {
	st := NewStateTracker()
	if st.GetState(uuid.New()) != nil {
		t.Fatal("expected nil for unknown monitor")
	}
}

func TestEvaluate_DegradedExactThresholdDoesNotFire(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	threshold := 5000
	bindings := []TriggerCondition{{Type: "degraded", ThresholdMs: &threshold}}

	// Response time exactly at threshold — should NOT fire (requires exceeding)
	result := CheckResult{State: "up", PreviousState: "up", ResponseTimeMs: 5000}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events when response time equals threshold, got %+v", events)
	}
}

func TestEvaluate_NFailuresInRow_BelowThresholdAfterRecovery(t *testing.T) {
	st := NewStateTracker()
	monitorID := uuid.New()
	count := 5
	bindings := []TriggerCondition{
		{Type: "n_failures_in_row", Count: &count},
		{Type: "monitor_up"},
	}

	// Go down with consecutive failures reaching threshold
	result := CheckResult{State: "down", PreviousState: "down", ConsecutiveFailures: 5}
	events := st.Evaluate(monitorID, result, bindings)
	if len(events) != 1 || events[0].Type != "n_failures_in_row" {
		t.Fatalf("expected n_failures_in_row, got %+v", events)
	}

	// Recovery
	result = CheckResult{State: "up", PreviousState: "down", ConsecutiveFailures: 0}
	events = st.Evaluate(monitorID, result, bindings)
	// Should get monitor_up
	if len(events) != 1 || events[0].Type != "monitor_up" {
		t.Fatalf("expected monitor_up on recovery, got %+v", events)
	}

	// Failures below threshold — should NOT fire
	result = CheckResult{State: "down", PreviousState: "up", ConsecutiveFailures: 2}
	events = st.Evaluate(monitorID, result, bindings)
	if len(events) != 0 {
		t.Fatalf("expected no events below threshold after recovery, got %+v", events)
	}
}
