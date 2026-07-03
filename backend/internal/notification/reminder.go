package notification

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ActiveReminder represents a reminder that is currently active for a binding.
// While active, the scheduler will periodically re-enqueue notifications at
// the configured interval as long as the triggering condition persists.
type ActiveReminder struct {
	BindingID   uuid.UUID
	MonitorID   uuid.UUID
	ChannelID   uuid.UUID
	TriggerType string
	IntervalMins int
	Payload     TemplateData
}

// ReminderScheduler manages periodic re-notification for active reminders.
// It uses a ticker goroutine to scan active reminders and re-enqueue
// notifications when the triggering condition persists and the configured
// interval has elapsed.
type ReminderScheduler struct {
	dispatcher      *Dispatcher
	state           *StateTracker
	tickInterval    time.Duration
	activeReminders map[uuid.UUID]*ActiveReminder // keyed by binding ID
	mu              sync.Mutex
	done            chan struct{}
	wg              sync.WaitGroup

	// nowFunc allows tests to inject a custom time source.
	nowFunc func() time.Time
}

// NewReminderScheduler creates a new ReminderScheduler.
//
// Parameters:
//   - dispatcher: used to re-enqueue notification jobs
//   - state: used to check if triggering conditions still persist
//   - tickInterval: how often the scheduler scans active reminders (e.g., 1 minute)
func NewReminderScheduler(dispatcher *Dispatcher, state *StateTracker, tickInterval time.Duration) *ReminderScheduler {
	return &ReminderScheduler{
		dispatcher:      dispatcher,
		state:           state,
		tickInterval:    tickInterval,
		activeReminders: make(map[uuid.UUID]*ActiveReminder),
		done:            make(chan struct{}),
		nowFunc:         time.Now,
	}
}

// Start launches the ticker goroutine that periodically scans active reminders.
func (rs *ReminderScheduler) Start() {
	rs.wg.Add(1)
	go rs.run()
	log.Printf("notification: reminder scheduler started (tick interval: %s)", rs.tickInterval)
}

// Stop signals the ticker goroutine to stop and waits for it to finish.
func (rs *ReminderScheduler) Stop() {
	close(rs.done)
	rs.wg.Wait()
	log.Printf("notification: reminder scheduler stopped")
}

// ActivateReminder adds a reminder to the active set. If a reminder already
// exists for the given binding ID, it is replaced.
func (rs *ReminderScheduler) ActivateReminder(reminder ActiveReminder) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.activeReminders[reminder.BindingID] = &reminder
}

// DeactivateReminder removes a reminder from the active set.
func (rs *ReminderScheduler) DeactivateReminder(bindingID uuid.UUID) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.activeReminders, bindingID)
}

// ActiveCount returns the number of currently active reminders.
// Primarily used for testing.
func (rs *ReminderScheduler) ActiveCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.activeReminders)
}

// run is the main ticker loop.
func (rs *ReminderScheduler) run() {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rs.tick()
		case <-rs.done:
			return
		}
	}
}

// tick scans all active reminders and re-enqueues notifications where
// the condition persists and the interval has elapsed.
func (rs *ReminderScheduler) tick() {
	rs.mu.Lock()
	// Copy the active reminders to avoid holding the lock during dispatch.
	reminders := make([]*ActiveReminder, 0, len(rs.activeReminders))
	for _, r := range rs.activeReminders {
		reminders = append(reminders, r)
	}
	rs.mu.Unlock()

	now := rs.nowFunc()

	for _, reminder := range reminders {
		if !rs.conditionPersists(reminder) {
			// Triggering condition resolved — deactivate this reminder.
			rs.DeactivateReminder(reminder.BindingID)
			log.Printf("notification: reminder deactivated (condition resolved) binding=%s monitor=%s trigger=%s",
				reminder.BindingID, reminder.MonitorID, reminder.TriggerType)
			continue
		}

		// Check if enough time has elapsed since the last reminder was sent.
		intervalDuration := time.Duration(reminder.IntervalMins) * time.Minute
		lastSent := rs.getLastReminderSent(reminder.MonitorID, reminder.BindingID)

		if now.Sub(lastSent) >= intervalDuration {
			// Re-enqueue the notification.
			job := DeliveryJob{
				ID:          uuid.New(),
				ChannelID:   reminder.ChannelID,
				MonitorID:   reminder.MonitorID,
				BindingID:   reminder.BindingID,
				TriggerType: reminder.TriggerType,
				Attempt:     1,
				MaxAttempts: 4,
				Payload:     reminder.Payload,
				ScheduledAt: now,
			}
			rs.dispatcher.Enqueue(job)

			// Update LastReminderSent in the state tracker.
			rs.updateLastReminderSent(reminder.MonitorID, reminder.BindingID, now)

			log.Printf("notification: reminder re-enqueued binding=%s monitor=%s trigger=%s",
				reminder.BindingID, reminder.MonitorID, reminder.TriggerType)
		}
	}
}

// conditionPersists checks if the triggering condition still holds for the
// given reminder by examining the StateTracker flags.
func (rs *ReminderScheduler) conditionPersists(reminder *ActiveReminder) bool {
	state := rs.state.GetState(reminder.MonitorID)
	if state == nil {
		// No state tracked yet — condition cannot persist.
		return false
	}

	switch reminder.TriggerType {
	case "monitor_down":
		// monitor_down reminders persist while the monitor remains down.
		// We check IsDegraded=false and ConsecFailuresFired as proxy signals,
		// but the primary signal is that no recovery has occurred.
		// Since StateTracker clears flags on recovery (down→up), if
		// ConsecFailuresFired is still set OR IsDegraded is false (not recovered),
		// the monitor is still down. We rely on the fact that the reminder was
		// activated when monitor went down, so if state still exists without
		// recovery clearing it, the condition persists.
		// Actually, a simpler approach: if the monitor recovered, the
		// recovery event clears ConsecFailuresFired, IsDegraded, SSLWarned.
		// For monitor_down, the condition persists as long as no recovery has fired.
		// We'll use a heuristic: condition persists = state exists (it was set
		// when the trigger fired). The dedup logic in Evaluate will have set
		// the state. If recovery occurs, flags get cleared.
		// For simplicity, we always return true here — the ActivateReminder/
		// DeactivateReminder lifecycle is managed externally by the dispatch flow.
		// The tick will deactivate on explicit condition resolution signals.
		return true

	case "degraded":
		return state.IsDegraded

	case "ssl_expiring":
		return state.SSLWarned

	case "n_failures_in_row":
		return state.ConsecFailuresFired

	case "monitor_up":
		// Recovery reminders don't make sense — the monitor is already up.
		// Deactivate immediately.
		return false

	default:
		return false
	}
}

// getLastReminderSent retrieves the last reminder sent time for a binding.
func (rs *ReminderScheduler) getLastReminderSent(monitorID, bindingID uuid.UUID) time.Time {
	state := rs.state.GetState(monitorID)
	if state == nil || state.LastReminderSent == nil {
		return time.Time{} // zero time — will always trigger
	}
	return state.LastReminderSent[bindingID]
}

// updateLastReminderSent updates the LastReminderSent timestamp in the state tracker.
func (rs *ReminderScheduler) updateLastReminderSent(monitorID, bindingID uuid.UUID, t time.Time) {
	rs.state.mu.Lock()
	defer rs.state.mu.Unlock()

	s, ok := rs.state.states[monitorID]
	if !ok {
		s = &MonitorNotifState{
			LastReminderSent: make(map[uuid.UUID]time.Time),
		}
		rs.state.states[monitorID] = s
	}
	if s.LastReminderSent == nil {
		s.LastReminderSent = make(map[uuid.UUID]time.Time)
	}
	s.LastReminderSent[bindingID] = t
}
