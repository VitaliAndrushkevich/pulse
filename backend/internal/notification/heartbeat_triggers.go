package notification

import "fmt"

// ValidHeartbeatTriggerTypes is the set of trigger types accepted on a
// heartbeat binding. Heartbeats only distinguish "missed/failed" from
// "recovered"; the monitor-specific trigger types (degraded, ssl_expiring,
// n_failures_in_row, and the monitor_* vocabulary) are rejected.
var ValidHeartbeatTriggerTypes = map[string]bool{
	"heartbeat_missed":    true,
	"heartbeat_recovered": true,
}

// ValidateHeartbeatTriggers validates a slice of trigger conditions submitted
// for a heartbeat binding, returning field-level errors for any invalid entry.
// It mirrors ValidateTriggers but restricts the allowed set to the heartbeat
// vocabulary {heartbeat_missed, heartbeat_recovered} and takes no thresholds.
func ValidateHeartbeatTriggers(triggers []TriggerCondition) []FieldError {
	var errs []FieldError

	if len(triggers) == 0 {
		errs = append(errs, FieldError{
			Field:   "triggers",
			Message: "at least one trigger is required",
		})
		return errs
	}

	for i, t := range triggers {
		if !ValidHeartbeatTriggerTypes[t.Type] {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("triggers[%d].type", i),
				Message: "must be one of: heartbeat_missed, heartbeat_recovered",
			})
		}
	}

	return errs
}

// TranslateHeartbeatTriggers maps heartbeat trigger vocabulary onto the monitor
// vocabulary understood by StateTracker.Evaluate and bindingMatchesTrigger:
//
//	heartbeat_missed    -> monitor_down
//	heartbeat_recovered -> monitor_up
//
// The translation is applied to the whole BindingWithChannel.Triggers slice at
// the dispatch call site (design §5.3) so both the event evaluation and the
// fan-out matcher operate on monitor_down/monitor_up and a DeliveryJob is
// enqueued. Unknown types are passed through unchanged (validation rejects them
// before persistence, so this is defensive).
func TranslateHeartbeatTriggers(triggers []TriggerCondition) []TriggerCondition {
	out := make([]TriggerCondition, 0, len(triggers))
	for _, t := range triggers {
		switch t.Type {
		case "heartbeat_missed":
			t.Type = "monitor_down"
		case "heartbeat_recovered":
			t.Type = "monitor_up"
		}
		out = append(out, t)
	}
	return out
}
