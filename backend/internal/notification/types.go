// Package notification implements the asynchronous notification dispatch
// subsystem for Pulse monitors.
package notification

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// DispatcherConfig holds tunable parameters for the notification dispatcher.
type DispatcherConfig struct {
	Workers      int           // PULSE_NOTIFICATION_WORKERS
	BufferSize   int           // 256
	DrainTimeout time.Duration // PULSE_NOTIFICATION_DRAIN_TIMEOUT
	BaseURL      string        // Public base URL for notification links (e.g. "https://pulse.example.com")
}

// DeliveryJob represents a single notification to deliver.
type DeliveryJob struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	MonitorID   uuid.UUID
	BindingID   uuid.UUID
	TriggerType string
	Attempt     int
	MaxAttempts int
	Payload     TemplateData
	ScheduledAt time.Time
}

// TemplateData holds all template variables available for rendering.
type TemplateData struct {
	Monitor        MonitorData
	Status         string
	PreviousStatus string
	ResponseTime   int32
	Incident       IncidentData
	Timestamp      time.Time
	BaseURL        string // Public base URL for generating links (e.g. "https://pulse.example.com")
}

// MonitorData contains monitor information for template rendering.
type MonitorData struct {
	ID     uuid.UUID
	Name   string
	URL    string
	Target string
}

// IncidentData contains incident information for template rendering.
type IncidentData struct {
	ID        uuid.UUID
	StartedAt time.Time
	Duration  time.Duration
}

// MonitorNotifState tracks per-monitor notification deduplication state.
type MonitorNotifState struct {
	IsDegraded          bool
	SSLWarned           bool
	ConsecFailuresFired bool
	LastReminderSent    map[uuid.UUID]time.Time // keyed by binding ID
}

// StateTracker prevents duplicate notifications for ongoing conditions.
type StateTracker struct {
	mu     sync.RWMutex
	states map[uuid.UUID]*MonitorNotifState // keyed by monitor ID
}

// NewStateTracker creates a new StateTracker.
func NewStateTracker() *StateTracker {
	return &StateTracker{
		states: make(map[uuid.UUID]*MonitorNotifState),
	}
}

// Metrics holds Prometheus metric collectors for the notification subsystem.
type Metrics struct {
	DeliveriesTotal *prometheus.CounterVec // labels: channel_type, outcome
	DroppedTotal    *prometheus.CounterVec // labels: channel_type
	InFlight        prometheus.Gauge
	RetryQueueSize  prometheus.Gauge
}

// NewMetrics creates and registers notification Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		DeliveriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pulse_notification_deliveries_total",
				Help: "Total number of notification delivery attempts by channel type and outcome.",
			},
			[]string{"channel_type", "outcome"},
		),
		DroppedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pulse_notification_dropped_total",
				Help: "Total number of notifications dropped due to buffer overflow.",
			},
			[]string{"channel_type"},
		),
		InFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pulse_notification_in_flight",
				Help: "Number of notification deliveries currently in progress.",
			},
		),
		RetryQueueSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pulse_notification_retry_queue_size",
				Help: "Number of notifications waiting in the retry queue.",
			},
		),
	}

	reg.MustRegister(m.DeliveriesTotal)
	reg.MustRegister(m.DroppedTotal)
	reg.MustRegister(m.InFlight)
	reg.MustRegister(m.RetryQueueSize)

	return m
}
