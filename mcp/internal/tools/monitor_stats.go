package tools

import (
	"context"
	"math"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// MonitorStatsInput defines the input schema for the monitor-stats tool.
type MonitorStatsInput struct {
	// Monitor is a UUID or an exact monitor name.
	Monitor string `json:"monitor" jsonschema:"Monitor UUID or exact name"`
}

// MonitorStatsLastError represents the most recent check error.
type MonitorStatsLastError struct {
	Message   string `json:"message"`
	CheckedAt string `json:"checked_at"`
}

// MonitorStatsSSL contains TLS certificate expiration details.
type MonitorStatsSSL struct {
	ExpiresAt     string `json:"expires_at"`
	DaysRemaining int    `json:"days_remaining"`
}

// MonitorStatsOutput is the result returned by the monitor-stats tool.
type MonitorStatsOutput struct {
	MonitorID       string                 `json:"monitor_id"`
	UptimePercent7d float64                `json:"uptime_percent_7d"`
	LastError       *MonitorStatsLastError `json:"last_error,omitempty"`
	SSL             *MonitorStatsSSL       `json:"ssl,omitempty"`
}

// HandleMonitorStats resolves a monitor reference, fetches its stats from the
// Pulse API, and returns uptime, last error, and SSL info (when present).
// If the stats endpoint does not provide a 7-day uptime figure, it is computed
// from the trailing 7-day check history.
func HandleMonitorStats(ctx context.Context, deps Deps, input MonitorStatsInput) (*MonitorStatsOutput, error) {
	// Validate input.
	if input.Monitor == "" {
		return nil, mcperr.Validation("monitor is required")
	}
	if len(input.Monitor) > 255 {
		return nil, mcperr.Validation("monitor must be 1–255 characters")
	}

	// Resolve monitor reference (UUID passthrough or name lookup).
	monitorID, err := resolve.Monitor(ctx, deps.Client, input.Monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// Fetch stats from Pulse API.
	stats, err := deps.Client.GetMonitorStats(ctx, monitorID)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Determine 7-day uptime. If the stats endpoint provides it (non-zero),
	// use it directly. Otherwise compute from the trailing 7-day history.
	uptime7d := stats.UptimePercent7d
	if uptime7d == 0 {
		computed, computeErr := computeUptime7d(ctx, deps.Client, monitorID)
		if computeErr != nil {
			return nil, mapPulseError(computeErr)
		}
		uptime7d = computed
	}

	// Build output.
	output := &MonitorStatsOutput{
		MonitorID:       stats.MonitorID,
		UptimePercent7d: uptime7d,
	}

	// Include last_error only when present.
	if stats.LastError != nil {
		output.LastError = &MonitorStatsLastError{
			Message:   stats.LastError.Message,
			CheckedAt: stats.LastError.CheckedAt.Format(time.RFC3339),
		}
	}

	// Include SSL info only when present (TLS monitors); omit otherwise (Req 5.3).
	if stats.SSL != nil {
		output.SSL = &MonitorStatsSSL{
			ExpiresAt:     stats.SSL.ExpiresAt.Format(time.RFC3339),
			DaysRemaining: stats.SSL.DaysRemaining,
		}
	}

	return output, nil
}

// computeUptime7d fetches the trailing 7-day history and calculates uptime as
// the ratio of up checks to total checks (excluding pending).
func computeUptime7d(ctx context.Context, client pulseapi.PulseClient, monitorID string) (float64, error) {
	now := time.Now().UTC()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	history, err := client.GetMonitorHistory(ctx, monitorID, pulseapi.TimeRange{
		From: sevenDaysAgo,
		To:   now,
	})
	if err != nil {
		return 0, err
	}

	// Count up and total checks (excluding pending from denominator).
	var upCount, totalCount int
	for _, p := range history.Points {
		switch p.State {
		case "up":
			upCount++
			totalCount++
		case "down":
			totalCount++
		// "pending" is excluded from the calculation.
		}
	}

	if totalCount == 0 {
		// No actionable data — report 100% uptime (no evidence of downtime).
		return 100.0, nil
	}

	// Round to two decimal places.
	pct := float64(upCount) / float64(totalCount) * 100.0
	return math.Round(pct*100) / 100, nil
}
