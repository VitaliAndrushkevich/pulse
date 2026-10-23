package heartbeat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// fakeBindingQueries returns a fixed set of heartbeat bindings for every
// heartbeat id, satisfying the narrow dispatchQueries surface.
type fakeBindingQueries struct {
	bindings []db.ListBindingsByHeartbeatRow
	err      error
}

func (f fakeBindingQueries) ListBindingsByHeartbeat(_ context.Context, _ uuid.UUID) ([]db.ListBindingsByHeartbeatRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bindings, nil
}

// capturedJob records an enqueued delivery for assertions.
type capturedJob struct {
	resourceID  uuid.UUID
	triggerType string
	monitorName string
}

// recordingNotifier is a notificationDispatcher that reuses a real
// StateTracker for dedup — exactly as the production dispatcher does — and
// records the jobs that would be enqueued. It mirrors the fan-out matching of
// Dispatcher.EvaluateAndDispatchWithPayload (evaluate once per resource key,
// then one job per matching binding) so the test exercises the real
// once-per-transition / once-per-recovery guarantee keyed by the heartbeat UUID.
type recordingNotifier struct {
	state *notification.StateTracker
	jobs  []capturedJob
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{state: notification.NewStateTracker()}
}

func (r *recordingNotifier) EvaluateAndDispatchWithPayload(
	_ context.Context,
	resourceID uuid.UUID,
	result notification.CheckResult,
	bindings []notification.BindingWithChannel,
	payloadBuilder func(uuid.UUID, notification.CheckResult) notification.TemplateData,
) {
	var conditions []notification.TriggerCondition
	for _, b := range bindings {
		conditions = append(conditions, b.Triggers...)
	}

	events := r.state.Evaluate(resourceID, result, conditions)
	for _, event := range events {
		for _, b := range bindings {
			for _, tc := range b.Triggers {
				if tc.Type == event.Type {
					payload := payloadBuilder(resourceID, result)
					r.jobs = append(r.jobs, capturedJob{
						resourceID:  resourceID,
						triggerType: event.Type,
						monitorName: payload.Monitor.Name,
					})
					break
				}
			}
		}
	}
}

func (r *recordingNotifier) countFor(id uuid.UUID, trigger string) int {
	n := 0
	for _, j := range r.jobs {
		if j.resourceID == id && j.triggerType == trigger {
			n++
		}
	}
	return n
}

// heartbeatBinding builds a binding row with the given heartbeat trigger types.
func heartbeatBinding(hbID uuid.UUID, triggerTypes ...string) db.ListBindingsByHeartbeatRow {
	conds := make([]notification.TriggerCondition, 0, len(triggerTypes))
	for _, tt := range triggerTypes {
		conds = append(conds, notification.TriggerCondition{Type: tt})
	}
	raw, _ := json.Marshal(conds)
	return db.ListBindingsByHeartbeatRow{
		ID:          uuid.New(),
		ChannelID:   uuid.New(),
		HeartbeatID: hbID,
		Triggers:    raw,
	}
}

func TestDispatch_MissedRunFiresOncePerEpisode(t *testing.T) {
	hbID := uuid.New()
	hb := db.Heartbeat{ID: hbID, Name: "nightly-backup"}
	queries := fakeBindingQueries{bindings: []db.ListBindingsByHeartbeatRow{
		heartbeatBinding(hbID, "heartbeat_missed", "heartbeat_recovered"),
	}}
	notifier := newRecordingNotifier()
	d := NewDispatch(queries, notifier, "https://pulse.example.com")

	ctx := context.Background()

	// Sweep marks it down (up → down). Fires once.
	d.DispatchMissedRun(ctx, hb, "up")
	// A second sweep before recovery must NOT re-fire (StateTracker dedup).
	d.DispatchMissedRun(ctx, hb, "down")

	if got := notifier.countFor(hbID, "monitor_down"); got != 1 {
		t.Fatalf("expected monitor_down to fire exactly once, got %d", got)
	}
	if notifier.countFor(hbID, "monitor_up") != 0 {
		t.Fatalf("recovery must not fire on a missed run")
	}

	// Every enqueued job must carry the pre-filled heartbeat name so the
	// delivery path skips enrichPayload's GetMonitor lookup (HIGH-1).
	for _, j := range notifier.jobs {
		if j.monitorName != "nightly-backup" {
			t.Errorf("expected job to carry heartbeat name, got %q", j.monitorName)
		}
	}
}

func TestDispatch_RecoveryFiresOncePerTransition(t *testing.T) {
	hbID := uuid.New()
	hb := db.Heartbeat{ID: hbID, Name: "cert-renewal"}
	queries := fakeBindingQueries{bindings: []db.ListBindingsByHeartbeatRow{
		heartbeatBinding(hbID, "heartbeat_missed", "heartbeat_recovered"),
	}}
	notifier := newRecordingNotifier()
	d := NewDispatch(queries, notifier, "")

	ctx := context.Background()

	// Down first (explicit fail), then a recovery success ping (down → up).
	// dispatch is the context-based core that DispatchHeartbeat (gin seam) and
	// DispatchMissedRun (evaluator seam) both delegate to.
	d.dispatch(ctx, hb, "down", "up")
	d.dispatch(ctx, hb, "up", "down")
	// A second success ping while already up must NOT re-fire recovery.
	d.dispatch(ctx, hb, "up", "up")

	if got := notifier.countFor(hbID, "monitor_up"); got != 1 {
		t.Fatalf("expected recovery to fire exactly once, got %d", got)
	}
	if got := notifier.countFor(hbID, "monitor_down"); got != 1 {
		t.Fatalf("expected explicit fail to fire monitor_down once, got %d", got)
	}
}

func TestDispatch_KeyedByHeartbeatUUID(t *testing.T) {
	// Two heartbeats sharing identical bindings must dedup independently —
	// proving the StateTracker is keyed by the heartbeat UUID, not globally.
	hbA := db.Heartbeat{ID: uuid.New(), Name: "job-a"}
	hbB := db.Heartbeat{ID: uuid.New(), Name: "job-b"}
	notifier := newRecordingNotifier()

	queriesA := fakeBindingQueries{bindings: []db.ListBindingsByHeartbeatRow{heartbeatBinding(hbA.ID, "heartbeat_missed")}}
	queriesB := fakeBindingQueries{bindings: []db.ListBindingsByHeartbeatRow{heartbeatBinding(hbB.ID, "heartbeat_missed")}}

	ctx := context.Background()
	NewDispatch(queriesA, notifier, "").DispatchMissedRun(ctx, hbA, "up")
	NewDispatch(queriesB, notifier, "").DispatchMissedRun(ctx, hbB, "up")

	if notifier.countFor(hbA.ID, "monitor_down") != 1 {
		t.Errorf("heartbeat A should fire once")
	}
	if notifier.countFor(hbB.ID, "monitor_down") != 1 {
		t.Errorf("heartbeat B should fire once independently of A")
	}
}

func TestDispatch_NoBindingsNoDispatch(t *testing.T) {
	hb := db.Heartbeat{ID: uuid.New(), Name: "no-bindings"}
	notifier := newRecordingNotifier()
	d := NewDispatch(fakeBindingQueries{bindings: nil}, notifier, "")

	d.DispatchMissedRun(context.Background(), hb, "up")

	if len(notifier.jobs) != 0 {
		t.Fatalf("expected no jobs when no bindings exist, got %d", len(notifier.jobs))
	}
}

func TestDispatch_NilDispatcherIsNoop(t *testing.T) {
	hb := db.Heartbeat{ID: uuid.New(), Name: "nil-dispatcher"}
	queries := fakeBindingQueries{bindings: []db.ListBindingsByHeartbeatRow{heartbeatBinding(hb.ID, "heartbeat_missed")}}
	d := NewDispatch(queries, nil, "")

	// Must not panic.
	d.DispatchMissedRun(context.Background(), hb, "up")
}
