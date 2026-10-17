package heartbeat

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// dispatchQueries is the subset of the generated db.Queries the heartbeat
// dispatch helper depends on. Narrowing the surface keeps the helper unit
// testable with a focused fake and documents exactly what it touches.
type dispatchQueries interface {
	ListBindingsByHeartbeat(ctx context.Context, heartbeatID uuid.UUID) ([]db.HeartbeatBinding, error)
}

// notificationDispatcher is the slice of *notification.Dispatcher the helper
// uses. It is the single additive entry point from the notification package
// (design §5.3); everything else — StateTracker dedup, the delivery worker
// pool, the retry queue, delivery_logs, and reminders — is reused unchanged.
type notificationDispatcher interface {
	EvaluateAndDispatchWithPayload(
		ctx context.Context,
		resourceID uuid.UUID,
		result notification.CheckResult,
		bindings []notification.BindingWithChannel,
		payloadBuilder func(uuid.UUID, notification.CheckResult) notification.TemplateData,
	)
}

// Dispatch is the thin heartbeat notification entry point. It loads a
// heartbeat's bindings, translates their heartbeat-vocabulary triggers to the
// monitor vocabulary understood by the shared dispatcher, and fans a single
// observed state transition out to every matching binding by reusing the
// existing notification dispatcher (design §5.3).
//
// It implements both the evaluator's Dispatcher seam (missed-run sweep) and the
// ping handler's PingDispatcher seam (success / explicit-fail pings), so a
// single code path keeps dedup, retry, delivery logging, and reminders
// identical to the monitor path.
type Dispatch struct {
	queries    dispatchQueries
	dispatcher notificationDispatcher
	baseURL    string
}

// NewDispatch creates a heartbeat dispatch helper. baseURL is the public base
// URL used for notification links (may be empty, matching the monitor path).
func NewDispatch(queries dispatchQueries, dispatcher notificationDispatcher, baseURL string) *Dispatch {
	return &Dispatch{queries: queries, dispatcher: dispatcher, baseURL: baseURL}
}

// DispatchMissedRun satisfies the evaluator's Dispatcher seam. It fires the
// heartbeat_missed path for a heartbeat the sweep just transitioned up → down.
func (d *Dispatch) DispatchMissedRun(ctx context.Context, hb db.Heartbeat, previousState string) {
	d.dispatch(ctx, hb, "down", previousState)
}

// DispatchHeartbeat satisfies the ping handler's PingDispatcher seam. newState
// is "up" (success ping) or "down" (explicit fail); previousState is the state
// read atomically with the ping write.
func (d *Dispatch) DispatchHeartbeat(c *gin.Context, hb db.Heartbeat, newState, previousState string) {
	d.dispatch(c.Request.Context(), hb, newState, previousState)
}

// dispatch loads the heartbeat's bindings, builds the translated
// BindingWithChannel slice and the pre-filled payload builder, and calls the
// shared dispatcher. It is non-fatal: a load error is logged and skipped so a
// notification failure never blocks the ping/evaluator write that preceded it.
func (d *Dispatch) dispatch(ctx context.Context, hb db.Heartbeat, newState, previousState string) {
	if d.dispatcher == nil {
		return
	}

	rows, err := d.queries.ListBindingsByHeartbeat(ctx, hb.ID)
	if err != nil {
		log.Printf("heartbeat dispatch: list bindings heartbeat=%s: %v", hb.ID, err)
		return
	}
	if len(rows) == 0 {
		return
	}

	bindings := make([]notification.BindingWithChannel, 0, len(rows))
	for _, row := range rows {
		triggers, err := decodeHeartbeatTriggers(row.Triggers)
		if err != nil {
			log.Printf("heartbeat dispatch: decode triggers heartbeat=%s binding=%s: %v", hb.ID, row.ID, err)
			continue
		}
		bindings = append(bindings, notification.BindingWithChannel{
			ID:        row.ID,
			ChannelID: row.ChannelID,
			// MonitorID carries the heartbeat UUID — it is the resource key used
			// for StateTracker dedup and for the DeliveryJob.MonitorID audit
			// column (delivery_logs.monitor_id NOT NULL, MEDIUM-1).
			MonitorID: hb.ID,
			// Translate heartbeat_missed → monitor_down, heartbeat_recovered →
			// monitor_up on the whole slice so both StateTracker.Evaluate and
			// bindingMatchesTrigger operate on the monitor vocabulary (HIGH-2).
			Triggers: notification.TranslateHeartbeatTriggers(triggers),
		})
	}
	if len(bindings) == 0 {
		return
	}

	result := notification.CheckResult{
		State:         newState,
		PreviousState: previousState,
	}

	d.dispatcher.EvaluateAndDispatchWithPayload(ctx, hb.ID, result, bindings, d.payloadBuilder(hb))
}

// payloadBuilder returns a closure that builds a fully-populated TemplateData
// for the given heartbeat. Pre-filling Monitor.Name (and BaseURL/timestamps)
// makes the delivery path skip enrichPayload's GetMonitor lookup, which would
// otherwise fail with pgx.ErrNoRows for a heartbeat id and render a nameless
// alert (design §5.3, HIGH-1).
func (d *Dispatch) payloadBuilder(hb db.Heartbeat) func(uuid.UUID, notification.CheckResult) notification.TemplateData {
	return func(id uuid.UUID, result notification.CheckResult) notification.TemplateData {
		return notification.TemplateData{
			Monitor: notification.MonitorData{
				ID:   id,
				Name: hb.Name,
			},
			Status:         result.State,
			PreviousStatus: result.PreviousState,
			BaseURL:        d.baseURL,
			Timestamp:      time.Now(),
		}
	}
}

// decodeHeartbeatTriggers unmarshals the stored triggers JSONB column into the
// notification trigger-condition shape. An empty/NULL column yields an empty
// slice (no triggers → no dispatch).
func decodeHeartbeatTriggers(raw json.RawMessage) ([]notification.TriggerCondition, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var triggers []notification.TriggerCondition
	if err := json.Unmarshal(raw, &triggers); err != nil {
		return nil, err
	}
	return triggers, nil
}
