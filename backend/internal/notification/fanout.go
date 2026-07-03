package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BindingWithChannel combines a channel binding's identifiers and trigger
// conditions with its associated channel information needed for dispatch.
type BindingWithChannel struct {
	ID         uuid.UUID          // binding ID
	ChannelID  uuid.UUID          // notification channel ID
	MonitorID  uuid.UUID          // monitor ID
	Triggers   []TriggerCondition // trigger conditions for this binding
}

// EvaluateAndDispatch evaluates trigger conditions for a monitor check result
// and enqueues independent delivery jobs for each matching binding.
//
// This is the "fan-out" step: one check result → N independent delivery jobs.
// Each binding gets its own separate DeliveryJob. Failure in one binding's
// enqueue (e.g., buffer full → drop) does NOT prevent delivery to other bindings.
//
// Parameters:
//   - ctx: context (unused currently, reserved for future use)
//   - monitorID: the monitor that produced the check result
//   - result: the check result to evaluate triggers against
//   - bindings: all bindings for this monitor with their trigger conditions
func (d *Dispatcher) EvaluateAndDispatch(ctx context.Context, monitorID uuid.UUID, result CheckResult, bindings []BindingWithChannel) {
	// Collect all trigger conditions from all bindings for state evaluation.
	allConditions := extractTriggerConditions(bindings)

	// Evaluate which triggers fire based on state transitions and deduplication.
	events := d.state.Evaluate(monitorID, result, allConditions)

	if len(events) == 0 {
		return
	}

	// Fan-out: for each fired trigger event, find all bindings that match
	// and enqueue an independent delivery job for each.
	for _, event := range events {
		for i := range bindings {
			if bindingMatchesTrigger(bindings[i], event) {
				job := DeliveryJob{
					ID:          uuid.New(),
					ChannelID:   bindings[i].ChannelID,
					MonitorID:   monitorID,
					BindingID:   bindings[i].ID,
					TriggerType: event.Type,
					Attempt:     1,
					MaxAttempts: DefaultMaxAttempts,
					Payload:     buildPayload(monitorID, result),
					ScheduledAt: time.Now(),
				}
				// Each Enqueue is independent — if one fails (buffer full),
				// the others still proceed.
				d.Enqueue(job)
			}
		}
	}
}

// extractTriggerConditions flattens all trigger conditions from all bindings
// into a single deduplicated slice for state evaluation.
func extractTriggerConditions(bindings []BindingWithChannel) []TriggerCondition {
	// Use a map to deduplicate by trigger type (keeping the most restrictive threshold
	// isn't necessary here — StateTracker evaluates all conditions independently).
	var conditions []TriggerCondition
	seen := make(map[string]bool)

	for _, b := range bindings {
		for _, tc := range b.Triggers {
			// For simple triggers (monitor_down, monitor_up), deduplicate by type.
			// For threshold triggers, include all unique combinations.
			key := tc.Type
			if tc.ThresholdMs != nil {
				key += fmt.Sprintf("_threshold_%d", *tc.ThresholdMs)
			}
			if tc.DaysBefore != nil {
				key += fmt.Sprintf("_days_%d", *tc.DaysBefore)
			}
			if tc.Count != nil {
				key += fmt.Sprintf("_count_%d", *tc.Count)
			}

			if !seen[key] {
				seen[key] = true
				conditions = append(conditions, tc)
			}
		}
	}

	return conditions
}

// bindingMatchesTrigger checks if a binding has a trigger condition that matches
// the given trigger event type.
func bindingMatchesTrigger(binding BindingWithChannel, event TriggerEvent) bool {
	for _, tc := range binding.Triggers {
		if tc.Type == event.Type {
			return true
		}
	}
	return false
}

// buildPayload constructs the TemplateData for a delivery job from the monitor ID
// and check result.
func buildPayload(monitorID uuid.UUID, result CheckResult) TemplateData {
	return TemplateData{
		Monitor: MonitorData{
			ID: monitorID,
			// Monitor name/URL/target will be resolved at delivery time from the DB.
			// For now we store what we have from the check result.
		},
		Status:         result.State,
		PreviousStatus: result.PreviousState,
		ResponseTime:   result.ResponseTimeMs,
		Timestamp:      time.Now(),
	}
}
