// Package pulseapi is the sole bridge between the MCP server and Pulse.
// It defines the client interface, data transfer objects, and error types
// used to communicate with the Pulse REST API.
package pulseapi

import "time"

// Monitor represents a single monitor resource from Pulse.
type Monitor struct {
	ID              string
	Name            string
	Type            string
	Target          string
	IntervalSeconds int
	TimeoutSeconds  int
	Status          string // up | down | pending
	State           string // active | paused
	LastCheckedAt   *time.Time
	NextCheckAt     *time.Time
	Tags            []Tag
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MonitorStats contains computed statistics for a monitor.
type MonitorStats struct {
	MonitorID       string
	UptimePercent7d float64
	LastError       *CheckError
	SSL             *SSLInfo
}

// MonitorPage is a paginated list of monitors.
type MonitorPage struct {
	Monitors   []Monitor
	Page       int
	Limit      int
	Total      int
	TotalPages int
}

// HistoryPoint is a single check result at a point in time.
type HistoryPoint struct {
	State      string
	LatencyMs  *int32
	StatusCode *int32
	Error      *string
	CheckedAt  time.Time
}

// History is the check history for a monitor within a time range.
type History struct {
	MonitorID string
	From      time.Time
	To        time.Time
	Truncated bool
	Points    []HistoryPoint
}

// Incident represents a downtime incident for a monitor.
type Incident struct {
	ID         string
	MonitorID  string
	Status     string
	StartedAt  time.Time
	ResolvedAt *time.Time
}

// IncidentPage is a paginated list of incidents.
type IncidentPage struct {
	Incidents  []Incident
	Page       int
	Limit      int
	Total      int
	TotalPages int
}

// CreateMonitorInput holds the parameters for creating a new monitor.
type CreateMonitorInput struct {
	Type            string
	Name            string
	Target          string
	IntervalSeconds *int
	TimeoutSeconds  *int
	Settings        map[string]any
}

// MonitorQuery holds filter and pagination parameters for listing monitors.
type MonitorQuery struct {
	Type  string   // optional; already normalized to a canonical Pulse type
	Tags  []string // optional; "key:value" form, AND semantics
	Page  int
	Limit int
}

// IncidentQuery holds filter and pagination parameters for listing incidents.
type IncidentQuery struct {
	MonitorID string // optional; empty => global /incidents
	OpenOnly  bool   // maps to ?status=open
	Page      int
	Limit     int
}

// TimeRange defines a time window for history queries.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// Tag is a key-value label attached to a monitor.
type Tag struct {
	Key   string
	Value string
}

// CheckError records the most recent check failure for a monitor.
type CheckError struct {
	Message   string
	CheckedAt time.Time
}

// SSLInfo contains TLS certificate expiration details.
type SSLInfo struct {
	ExpiresAt     time.Time
	DaysRemaining int
}
