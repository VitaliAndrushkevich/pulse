package notification

import (
	"time"

	"github.com/google/uuid"
)

// CheckResult carries the data needed by the StateTracker to evaluate
// which triggers should fire for a given monitor check.
type CheckResult struct {
	State               string // "up" or "down"
	PreviousState       string // "up" or "down" (state before this check)
	ResponseTimeMs      int32
	SSLDaysRemaining    *int32
	ConsecutiveFailures int
}

// TriggerEvent represents a trigger that should fire as a result of
// state evaluation.
type TriggerEvent struct {
	Type string // one of the ValidTriggerTypes keys
}

// Evaluate examines the current check result against the monitor's bindings
// and returns which triggers should fire. It is thread-safe.
//
// Key behaviors:
//   - monitor_down: fires once on transition from non-down to down
//   - monitor_up: fires once on transition from down to up (recovery)
//   - degraded: fires once when response time first exceeds threshold
//   - ssl_expiring: fires once when SSL days remaining first falls below threshold
//   - n_failures_in_row: fires once when consecutive failures reach threshold
//   - Recovery (down→up): clears IsDegraded, SSLWarned, ConsecFailuresFired flags
func (st *StateTracker) Evaluate(monitorID uuid.UUID, result CheckResult, bindings []TriggerCondition) []TriggerEvent {
	st.mu.Lock()
	defer st.mu.Unlock()

	state := st.getOrCreateState(monitorID)
	var events []TriggerEvent

	// Recovery: when transitioning from down to up, clear dedup flags.
	isRecovery := result.PreviousState == "down" && result.State == "up"
	if isRecovery {
		state.IsDegraded = false
		state.SSLWarned = false
		state.ConsecFailuresFired = false
	}

	for _, binding := range bindings {
		switch binding.Type {
		case "monitor_down":
			if result.State == "down" && result.PreviousState != "down" {
				events = append(events, TriggerEvent{Type: "monitor_down"})
			}

		case "monitor_up":
			if isRecovery {
				events = append(events, TriggerEvent{Type: "monitor_up"})
			}

		case "degraded":
			if binding.ThresholdMs == nil {
				continue
			}
			threshold := int32(*binding.ThresholdMs)
			isDegradedNow := result.State == "up" && result.ResponseTimeMs > threshold
			if isDegradedNow && !state.IsDegraded {
				state.IsDegraded = true
				events = append(events, TriggerEvent{Type: "degraded"})
			} else if !isDegradedNow && state.IsDegraded {
				// Response time recovered below threshold
				state.IsDegraded = false
			}

		case "ssl_expiring":
			if binding.DaysBefore == nil || result.SSLDaysRemaining == nil {
				continue
			}
			threshold := int32(*binding.DaysBefore)
			isBelowThreshold := *result.SSLDaysRemaining < threshold
			if isBelowThreshold && !state.SSLWarned {
				state.SSLWarned = true
				events = append(events, TriggerEvent{Type: "ssl_expiring"})
			} else if !isBelowThreshold && state.SSLWarned {
				// SSL renewed, clear the warning
				state.SSLWarned = false
			}

		case "n_failures_in_row":
			if binding.Count == nil {
				continue
			}
			threshold := *binding.Count
			if result.ConsecutiveFailures >= threshold && !state.ConsecFailuresFired {
				state.ConsecFailuresFired = true
				events = append(events, TriggerEvent{Type: "n_failures_in_row"})
			}
			// ConsecFailuresFired is only cleared on recovery (handled above)
		}
	}

	return events
}

// GetState returns the current notification state for a monitor (read-only copy).
// Returns nil if no state exists for the given monitor.
func (st *StateTracker) GetState(monitorID uuid.UUID) *MonitorNotifState {
	st.mu.RLock()
	defer st.mu.RUnlock()

	s, ok := st.states[monitorID]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races
	cp := *s
	if s.LastReminderSent != nil {
		cp.LastReminderSent = make(map[uuid.UUID]time.Time, len(s.LastReminderSent))
		for k, v := range s.LastReminderSent {
			cp.LastReminderSent[k] = v
		}
	}
	return &cp
}

// ClearState removes the state for a monitor. Used when a monitor is deleted.
func (st *StateTracker) ClearState(monitorID uuid.UUID) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.states, monitorID)
}

// getOrCreateState returns the existing state or creates a new one.
// Must be called with st.mu held.
func (st *StateTracker) getOrCreateState(monitorID uuid.UUID) *MonitorNotifState {
	s, ok := st.states[monitorID]
	if !ok {
		s = &MonitorNotifState{
			LastReminderSent: make(map[uuid.UUID]time.Time),
		}
		st.states[monitorID] = s
	}
	return s
}
