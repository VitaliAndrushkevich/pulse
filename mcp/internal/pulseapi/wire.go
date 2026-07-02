package pulseapi

import "time"

// Wire-format types for JSON deserialization of Pulse REST API responses.
// These map the JSON envelope structure to internal model types.

// errorEnvelope is the standard Pulse error response format.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Monitors ---

// listMonitorsEnvelope is the paginated response for GET /monitors.
type listMonitorsEnvelope struct {
	Data       []wireMonitor `json:"data"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalPages int           `json:"total_pages"`
}

// wireMonitor is the JSON representation of a monitor from the Pulse API.
type wireMonitor struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`
	Target          string     `json:"target"`
	IntervalSeconds int        `json:"interval_seconds"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	Status          string     `json:"status"`
	State           string     `json:"state"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	NextCheckAt     *time.Time `json:"next_check_at"`
	Tags            []wireTag  `json:"tags"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (w wireMonitor) toModel() Monitor {
	tags := make([]Tag, 0, len(w.Tags))
	for _, t := range w.Tags {
		tags = append(tags, Tag{Key: t.Key, Value: t.Value})
	}
	return Monitor{
		ID:              w.ID,
		Name:            w.Name,
		Type:            w.Type,
		Target:          w.Target,
		IntervalSeconds: w.IntervalSeconds,
		TimeoutSeconds:  w.TimeoutSeconds,
		Status:          w.Status,
		State:           w.State,
		LastCheckedAt:   w.LastCheckedAt,
		NextCheckAt:     w.NextCheckAt,
		Tags:            tags,
		CreatedAt:       w.CreatedAt,
		UpdatedAt:       w.UpdatedAt,
	}
}

// wireTag is a JSON tag entry.
type wireTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// --- Stats ---

// wireMonitorStats is the JSON response from GET /monitors/:id/stats.
// Note: Pulse returns uptime_24h and uptime_30d windows. The design computes 7-day
// from history; for v1 we use uptime_24h.uptime_percent as an approximation when
// a dedicated 7-day call is not available.
type wireMonitorStats struct {
	MonitorID string           `json:"monitor_id"`
	Uptime24h wireUptimeWindow `json:"uptime_24h"`
	Uptime30d wireUptimeWindow `json:"uptime_30d"`
	SSL       *wireSSL         `json:"ssl"`
	LastError *wireLastError   `json:"last_error"`
}

type wireUptimeWindow struct {
	TotalChecks   int64   `json:"total_checks"`
	UpChecks      int64   `json:"up_checks"`
	UptimePercent float64 `json:"uptime_percent"`
	AvgLatencyMs  int32   `json:"avg_latency_ms"`
}

type wireSSL struct {
	DaysRemaining int    `json:"days_remaining"`
	ExpiresAt     string `json:"expires_at"` // date string "2006-01-02"
}

type wireLastError struct {
	Error     string `json:"error"`
	CheckedAt string `json:"checked_at"` // RFC3339
}

func (w wireMonitorStats) toModel() MonitorStats {
	stats := MonitorStats{
		MonitorID:       w.MonitorID,
		UptimePercent7d: w.Uptime24h.UptimePercent, // approximation; see design note
	}
	if w.LastError != nil {
		checkedAt, _ := time.Parse(time.RFC3339, w.LastError.CheckedAt)
		stats.LastError = &CheckError{
			Message:   w.LastError.Error,
			CheckedAt: checkedAt,
		}
	}
	if w.SSL != nil {
		expiresAt, _ := time.Parse("2006-01-02", w.SSL.ExpiresAt)
		stats.SSL = &SSLInfo{
			ExpiresAt:     expiresAt,
			DaysRemaining: w.SSL.DaysRemaining,
		}
	}
	return stats
}

// --- History ---

// historyEnvelope is the direct response from GET /monitors/:id/history.
// Note: The history endpoint does NOT use the data-envelope pattern.
type historyEnvelope struct {
	MonitorID string           `json:"monitor_id"`
	From      string           `json:"from"`
	To        string           `json:"to"`
	Truncated bool             `json:"truncated"`
	Points    []wireHistoryPt  `json:"points"`
}

type wireHistoryPt struct {
	State      string  `json:"state"`
	LatencyMs  *int32  `json:"latency_ms"`
	StatusCode *int32  `json:"status_code"`
	Error      *string `json:"error"`
	CheckedAt  string  `json:"checked_at"` // RFC3339
}

func (h historyEnvelope) toModel(monitorID string) History {
	from, _ := time.Parse(time.RFC3339, h.From)
	to, _ := time.Parse(time.RFC3339, h.To)

	points := make([]HistoryPoint, 0, len(h.Points))
	for _, p := range h.Points {
		checkedAt, _ := time.Parse(time.RFC3339, p.CheckedAt)
		points = append(points, HistoryPoint{
			State:      p.State,
			LatencyMs:  p.LatencyMs,
			StatusCode: p.StatusCode,
			Error:      p.Error,
			CheckedAt:  checkedAt,
		})
	}
	return History{
		MonitorID: monitorID,
		From:      from,
		To:        to,
		Truncated: h.Truncated,
		Points:    points,
	}
}

// --- Incidents ---

// listIncidentsEnvelope is the paginated response for GET /incidents.
type listIncidentsEnvelope struct {
	Data       []wireIncident `json:"data"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

// wireIncident is the JSON representation of an incident from the Pulse API.
// Note: Pulse does not include a "status" field directly; we derive it from resolved_at.
type wireIncident struct {
	ID         string     `json:"id"`
	MonitorID  string     `json:"monitor_id"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Cause      *string    `json:"cause"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (w wireIncident) toModel() Incident {
	status := "open"
	if w.ResolvedAt != nil {
		status = "resolved"
	}
	return Incident{
		ID:         w.ID,
		MonitorID:  w.MonitorID,
		Status:     status,
		StartedAt:  w.StartedAt,
		ResolvedAt: w.ResolvedAt,
	}
}

// --- Create Monitor ---

// createMonitorRequest is the request body for POST /monitors.
type createMonitorRequest struct {
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	Target          string         `json:"target"`
	IntervalSeconds *int           `json:"interval_seconds,omitempty"`
	TimeoutSeconds  *int           `json:"timeout_seconds,omitempty"`
	Settings        map[string]any `json:"settings,omitempty"`
}
