package notification

import "fmt"

// ValidTriggerTypes is the set of accepted trigger type values.
var ValidTriggerTypes = map[string]bool{
	"monitor_down":     true,
	"monitor_up":       true,
	"degraded":         true,
	"ssl_expiring":     true,
	"n_failures_in_row": true,
}

// TriggerCondition represents a single trigger configuration from the
// channel_bindings triggers JSONB column.
type TriggerCondition struct {
	Type        string `json:"type"`
	ThresholdMs *int   `json:"threshold_ms,omitempty"` // required for "degraded"
	DaysBefore  *int   `json:"days_before,omitempty"`  // required for "ssl_expiring"
	Count       *int   `json:"count,omitempty"`        // required for "n_failures_in_row"
}

// FieldError represents a single field-level validation error.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateTriggers validates a slice of trigger conditions, returning
// field-level errors for any invalid entries. It checks:
//   - At least one trigger is provided
//   - Each trigger type is in the allowed set
//   - Threshold values are within required ranges
//   - Required thresholds are present for their trigger types
func ValidateTriggers(triggers []TriggerCondition) []FieldError {
	var errs []FieldError

	if len(triggers) == 0 {
		errs = append(errs, FieldError{
			Field:   "triggers",
			Message: "at least one trigger is required",
		})
		return errs
	}

	for i, t := range triggers {
		prefix := fmt.Sprintf("triggers[%d]", i)

		if !ValidTriggerTypes[t.Type] {
			errs = append(errs, FieldError{
				Field:   prefix + ".type",
				Message: fmt.Sprintf("must be one of: monitor_down, monitor_up, degraded, ssl_expiring, n_failures_in_row"),
			})
			continue // skip threshold checks if type is unknown
		}

		switch t.Type {
		case "degraded":
			if t.ThresholdMs == nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".threshold_ms",
					Message: "is required for trigger type \"degraded\"",
				})
			} else if *t.ThresholdMs < 1 || *t.ThresholdMs > 60000 {
				errs = append(errs, FieldError{
					Field:   prefix + ".threshold_ms",
					Message: "must be between 1 and 60000",
				})
			}

		case "ssl_expiring":
			if t.DaysBefore == nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".days_before",
					Message: "is required for trigger type \"ssl_expiring\"",
				})
			} else if *t.DaysBefore < 1 || *t.DaysBefore > 365 {
				errs = append(errs, FieldError{
					Field:   prefix + ".days_before",
					Message: "must be between 1 and 365",
				})
			}

		case "n_failures_in_row":
			if t.Count == nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".count",
					Message: "is required for trigger type \"n_failures_in_row\"",
				})
			} else if *t.Count < 1 || *t.Count > 100 {
				errs = append(errs, FieldError{
					Field:   prefix + ".count",
					Message: "must be between 1 and 100",
				})
			}

		case "monitor_down", "monitor_up":
			// No thresholds required.
		}
	}

	return errs
}
