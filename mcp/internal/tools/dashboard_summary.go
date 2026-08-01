package tools

import (
	"context"
)

// DashboardSummaryInput defines the input schema for the dashboard-summary tool.
// This tool takes no parameters — it always returns the current system overview.
type DashboardSummaryInput struct{}

// DashboardSummaryOutput contains the aggregated monitoring overview.
type DashboardSummaryOutput struct {
	HealthScore        HealthScoreOutput        `json:"health_score"`
	StatusDistribution StatusDistributionOutput `json:"status_distribution"`
	ActiveIncidents    []IncidentOutput         `json:"active_incidents"`
	TopLatencyMonitors []LatencyOutput          `json:"top_latency_monitors"`
	SSLExpiry          []SSLExpiryOutput        `json:"ssl_expiry"`
	GeneratedAt        string                   `json:"generated_at"`
}

// HealthScoreOutput shows the global uptime percentage.
type HealthScoreOutput struct {
	UptimePercent      float64 `json:"uptime_percent"`
	ActiveMonitorCount int     `json:"active_monitor_count"`
}

// StatusDistributionOutput shows the count of monitors by state.
type StatusDistributionOutput struct {
	Up      int `json:"up"`
	Down    int `json:"down"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// IncidentOutput represents an active incident on the dashboard.
type IncidentOutput struct {
	MonitorID   string  `json:"monitor_id"`
	MonitorName string  `json:"monitor_name"`
	StartedAt   string  `json:"started_at"`
	Cause       *string `json:"cause,omitempty"`
}

// LatencyOutput represents a high-latency monitor.
type LatencyOutput struct {
	MonitorID    string `json:"monitor_id"`
	MonitorName  string `json:"monitor_name"`
	AvgLatencyMs int    `json:"avg_latency_ms"`
}

// SSLExpiryOutput represents a monitor with an expiring SSL certificate.
type SSLExpiryOutput struct {
	MonitorID     string `json:"monitor_id"`
	MonitorName   string `json:"monitor_name"`
	DaysRemaining int    `json:"days_remaining"`
	ExpiresAt     string `json:"expires_at"`
}

// HandleDashboardSummary returns the aggregated dashboard summary.
func HandleDashboardSummary(ctx context.Context, deps Deps, _ DashboardSummaryInput) (*DashboardSummaryOutput, error) {
	summary, err := deps.Client.GetDashboardSummary(ctx)
	if err != nil {
		return nil, mapPulseError(err)
	}

	incidents := make([]IncidentOutput, 0, len(summary.ActiveIncidents))
	for _, inc := range summary.ActiveIncidents {
		incidents = append(incidents, IncidentOutput{
			MonitorID:   inc.MonitorID,
			MonitorName: inc.MonitorName,
			StartedAt:   inc.StartedAt,
			Cause:       inc.Cause,
		})
	}

	latency := make([]LatencyOutput, 0, len(summary.TopLatencyMonitors))
	for _, m := range summary.TopLatencyMonitors {
		latency = append(latency, LatencyOutput{
			MonitorID:    m.MonitorID,
			MonitorName:  m.MonitorName,
			AvgLatencyMs: m.AvgLatencyMs,
		})
	}

	ssl := make([]SSLExpiryOutput, 0, len(summary.SSLExpiry))
	for _, e := range summary.SSLExpiry {
		ssl = append(ssl, SSLExpiryOutput{
			MonitorID:     e.MonitorID,
			MonitorName:   e.MonitorName,
			DaysRemaining: e.DaysRemaining,
			ExpiresAt:     e.ExpiresAt,
		})
	}

	return &DashboardSummaryOutput{
		HealthScore: HealthScoreOutput{
			UptimePercent:      summary.HealthScore.UptimePercent,
			ActiveMonitorCount: summary.HealthScore.ActiveMonitorCount,
		},
		StatusDistribution: StatusDistributionOutput{
			Up:      summary.StatusDistribution.Up,
			Down:    summary.StatusDistribution.Down,
			Unknown: summary.StatusDistribution.Unknown,
			Total:   summary.StatusDistribution.Total,
		},
		ActiveIncidents:    incidents,
		TopLatencyMonitors: latency,
		SSLExpiry:          ssl,
		GeneratedAt:        summary.GeneratedAt,
	}, nil
}
