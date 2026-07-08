package notification

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// newTestDispatcher creates a Dispatcher suitable for testing with a real
// buffered channel and metrics. Returns the dispatcher and a function to
// drain jobs from the channel.
func newTestDispatcher(t *testing.T) (*Dispatcher, func() []DeliveryJob) {
	t.Helper()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	state := NewStateTracker()

	d := NewDispatcher(DispatcherConfig{
		Workers:    1,
		BufferSize: 256,
	}, nil, nil, metrics, state, nil)

	drain := func() []DeliveryJob {
		var jobs []DeliveryJob
		for {
			select {
			case job := <-d.jobs:
				jobs = append(jobs, job)
			default:
				return jobs
			}
		}
	}

	return d, drain
}

func TestNewReminderScheduler(t *testing.T) {
	d, _ := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	if rs.dispatcher != d {
		t.Fatal("expected dispatcher to be set")
	}
	if rs.state != state {
		t.Fatal("expected state tracker to be set")
	}
	if rs.tickInterval != time.Minute {
		t.Fatalf("expected tick interval 1m, got %v", rs.tickInterval)
	}
	if rs.ActiveCount() != 0 {
		t.Fatal("expected 0 active reminders on creation")
	}
}

func TestActivateAndDeactivateReminder(t *testing.T) {
	d, _ := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	bindingID := uuid.New()
	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    uuid.New(),
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	}

	rs.ActivateReminder(reminder)
	if rs.ActiveCount() != 1 {
		t.Fatalf("expected 1 active reminder, got %d", rs.ActiveCount())
	}

	// Activate same binding again (replace)
	rs.ActivateReminder(reminder)
	if rs.ActiveCount() != 1 {
		t.Fatalf("expected still 1 active reminder after re-activate, got %d", rs.ActiveCount())
	}

	rs.DeactivateReminder(bindingID)
	if rs.ActiveCount() != 0 {
		t.Fatalf("expected 0 active reminders after deactivate, got %d", rs.ActiveCount())
	}

	// Deactivating non-existent binding is a no-op
	rs.DeactivateReminder(uuid.New())
	if rs.ActiveCount() != 0 {
		t.Fatalf("expected 0 active reminders, got %d", rs.ActiveCount())
	}
}

func TestStartAndStop(t *testing.T) {
	d, _ := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, 10*time.Millisecond)

	rs.Start()
	// Give the goroutine time to start
	time.Sleep(20 * time.Millisecond)
	rs.Stop()
	// Should not hang or panic
}

func TestTick_ReEnqueuesWhenConditionPersistsAndIntervalElapsed(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()
	channelID := uuid.New()

	// Set up state: monitor is degraded
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		IsDegraded:       true,
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    channelID,
		TriggerType:  "degraded",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	// Set nowFunc to simulate time has elapsed past the interval
	rs.nowFunc = func() time.Time {
		return time.Now().Add(10 * time.Minute)
	}

	// Execute one tick
	rs.tick()

	// Should have enqueued a job
	jobs := drain()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job enqueued, got %d", len(jobs))
	}
	if jobs[0].MonitorID != monitorID {
		t.Fatalf("expected monitor ID %s, got %s", monitorID, jobs[0].MonitorID)
	}
	if jobs[0].BindingID != bindingID {
		t.Fatalf("expected binding ID %s, got %s", bindingID, jobs[0].BindingID)
	}
	if jobs[0].ChannelID != channelID {
		t.Fatalf("expected channel ID %s, got %s", channelID, jobs[0].ChannelID)
	}
	if jobs[0].TriggerType != "degraded" {
		t.Fatalf("expected trigger type degraded, got %s", jobs[0].TriggerType)
	}

	// Reminder should still be active
	if rs.ActiveCount() != 1 {
		t.Fatalf("expected reminder to remain active, got %d", rs.ActiveCount())
	}
}

func TestTick_DoesNotReEnqueueBeforeIntervalElapsed(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	now := time.Now()

	// Set up state: monitor is degraded, last reminder was sent recently
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		IsDegraded: true,
		LastReminderSent: map[uuid.UUID]time.Time{
			bindingID: now,
		},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	// nowFunc returns only 2 minutes later — not enough
	rs.nowFunc = func() time.Time {
		return now.Add(2 * time.Minute)
	}

	rs.tick()

	jobs := drain()
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs enqueued before interval elapsed, got %d", len(jobs))
	}
}

func TestTick_DeactivatesWhenConditionResolves(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	// Set up state: monitor WAS degraded but now resolved
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		IsDegraded:       false, // condition resolved
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	rs.nowFunc = func() time.Time {
		return time.Now().Add(10 * time.Minute)
	}

	rs.tick()

	// No job should be enqueued
	jobs := drain()
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs after condition resolved, got %d", len(jobs))
	}

	// Reminder should be deactivated
	if rs.ActiveCount() != 0 {
		t.Fatalf("expected reminder deactivated, got %d active", rs.ActiveCount())
	}
}

func TestTick_SSLExpiringCondition(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	// SSL warning is active
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		SSLWarned:        true,
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "ssl_expiring",
		IntervalMins: 60,
	}
	rs.ActivateReminder(reminder)

	rs.nowFunc = func() time.Time {
		return time.Now().Add(61 * time.Minute)
	}

	rs.tick()

	jobs := drain()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job for SSL reminder, got %d", len(jobs))
	}
	if jobs[0].TriggerType != "ssl_expiring" {
		t.Fatalf("expected trigger type ssl_expiring, got %s", jobs[0].TriggerType)
	}
}

func TestTick_NFailuresInRowCondition(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		ConsecFailuresFired: true,
		LastReminderSent:    map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "n_failures_in_row",
		IntervalMins: 15,
	}
	rs.ActivateReminder(reminder)

	rs.nowFunc = func() time.Time {
		return time.Now().Add(20 * time.Minute)
	}

	rs.tick()

	jobs := drain()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job for n_failures_in_row reminder, got %d", len(jobs))
	}
}

func TestTick_MonitorUpDeactivatesImmediately(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	// Even with state present, monitor_up should deactivate
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "monitor_up",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	rs.tick()

	jobs := drain()
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs for monitor_up reminder, got %d", len(jobs))
	}
	if rs.ActiveCount() != 0 {
		t.Fatalf("expected monitor_up reminder deactivated, got %d", rs.ActiveCount())
	}
}

func TestTick_NoStateDeactivatesReminder(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	// No state for the monitor — condition can't persist
	reminder := ActiveReminder{
		BindingID:    uuid.New(),
		MonitorID:    uuid.New(),
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	rs.tick()

	jobs := drain()
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs when no state exists, got %d", len(jobs))
	}
	if rs.ActiveCount() != 0 {
		t.Fatalf("expected reminder deactivated when no state, got %d", rs.ActiveCount())
	}
}

func TestTick_MultipleRemindersProcessed(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitor1 := uuid.New()
	monitor2 := uuid.New()
	binding1 := uuid.New()
	binding2 := uuid.New()

	// Both monitors degraded
	state.mu.Lock()
	state.states[monitor1] = &MonitorNotifState{
		IsDegraded:       true,
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.states[monitor2] = &MonitorNotifState{
		IsDegraded:       true,
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	rs.ActivateReminder(ActiveReminder{
		BindingID:    binding1,
		MonitorID:    monitor1,
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	})
	rs.ActivateReminder(ActiveReminder{
		BindingID:    binding2,
		MonitorID:    monitor2,
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 10,
	})

	rs.nowFunc = func() time.Time {
		return time.Now().Add(15 * time.Minute)
	}

	rs.tick()

	jobs := drain()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs for both reminders, got %d", len(jobs))
	}
}

func TestTick_UpdatesLastReminderSent(t *testing.T) {
	d, _ := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		IsDegraded:       true,
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "degraded",
		IntervalMins: 5,
	}
	rs.ActivateReminder(reminder)

	fakeNow := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	rs.nowFunc = func() time.Time {
		return fakeNow
	}

	// First tick — should fire (no previous LastReminderSent)
	rs.tick()

	// Check that LastReminderSent was updated
	s := state.GetState(monitorID)
	if s == nil {
		t.Fatal("expected state to exist")
	}
	lastSent, ok := s.LastReminderSent[bindingID]
	if !ok {
		t.Fatal("expected LastReminderSent to be set for binding")
	}
	if !lastSent.Equal(fakeNow) {
		t.Fatalf("expected LastReminderSent=%v, got %v", fakeNow, lastSent)
	}
}

func TestReminderScheduler_ConcurrentAccess(t *testing.T) {
	d, _ := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, 10*time.Millisecond)

	rs.Start()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bindingID := uuid.New()
			rs.ActivateReminder(ActiveReminder{
				BindingID:    bindingID,
				MonitorID:    uuid.New(),
				ChannelID:    uuid.New(),
				TriggerType:  "degraded",
				IntervalMins: 5,
			})
			time.Sleep(5 * time.Millisecond)
			rs.DeactivateReminder(bindingID)
		}()
	}
	wg.Wait()
	rs.Stop()
}

func TestTick_MonitorDownConditionPersists(t *testing.T) {
	d, drain := newTestDispatcher(t)
	state := NewStateTracker()
	rs := NewReminderScheduler(d, state, time.Minute)

	monitorID := uuid.New()
	bindingID := uuid.New()

	// monitor_down: state exists (was set when trigger fired)
	state.mu.Lock()
	state.states[monitorID] = &MonitorNotifState{
		LastReminderSent: map[uuid.UUID]time.Time{},
	}
	state.mu.Unlock()

	reminder := ActiveReminder{
		BindingID:    bindingID,
		MonitorID:    monitorID,
		ChannelID:    uuid.New(),
		TriggerType:  "monitor_down",
		IntervalMins: 10,
	}
	rs.ActivateReminder(reminder)

	rs.nowFunc = func() time.Time {
		return time.Now().Add(15 * time.Minute)
	}

	rs.tick()

	jobs := drain()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job for monitor_down reminder, got %d", len(jobs))
	}
	if rs.ActiveCount() != 1 {
		t.Fatalf("expected monitor_down reminder to remain active, got %d", rs.ActiveCount())
	}
}
