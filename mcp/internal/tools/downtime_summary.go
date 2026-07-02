package tools

import (
	"context"
	"strings"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/downtime"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

const (
	// defaultWindowSeconds is the default window for downtime summary (24 hours).
	defaultWindowSeconds = 86400
	// minWindowSeconds is the minimum allowed window (60 seconds).
	minWindowSeconds = 60
	// maxWindowSeconds is the maximum allowed window (7 days = retention window).
	maxWindowSeconds = 604800
)

// DowntimeSummaryInput is the input schema for the downtime-summary tool.
type DowntimeSummaryInput struct {
	Monitor       string `json:"monitor" jsonschema:"Monitor identifier (UUID or name),required"`
	WindowSeconds *int   `json:"window_seconds,omitempty" jsonschema:"Lookback window in seconds (60-604800, default 86400 = 24h)"`
}

// DowntimePeriodOutput represents a single contiguous downtime interval.
type DowntimePeriodOutput struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	DurationSeconds int64  `json:"duration_seconds"`
}

// DowntimeSummaryOutput is the result returned by the downtime-summary tool.
type DowntimeSummaryOutput struct {
	MonitorID            string                 `json:"monitor_id"`
	HadDowntime          bool                   `json:"had_downtime"`
	DowntimePeriodCount  int                    `json:"downtime_period_count"`
	TotalDowntimeSeconds int64                  `json:"total_downtime_seconds"`
	WindowStart          string                 `json:"window_start"`
	WindowEnd            string                 `json:"window_end"`
	Truncated            bool                   `json:"truncated"`
	Periods              []DowntimePeriodOutput `json:"periods"`
}

// HandleDowntimeSummary implements the downtime-summary tool.
// It validates the window, resolves the monitor, fetches history from Pulse,
// and delegates to downtime.Summarize to compute the downtime summary.
func HandleDowntimeSummary(ctx context.Context, deps Deps, input DowntimeSummaryInput) (*DowntimeSummaryOutput, error) {
	// Validate monitor identifier.
	monitor := strings.TrimSpace(input.Monitor)
	if monitor == "" {
		return nil, mcperr.Validation("monitor is required")
	}

	// Determine and validate window_seconds.
	windowSeconds := defaultWindowSeconds
	if input.WindowSeconds != nil {
		windowSeconds = *input.WindowSeconds
	}

	if windowSeconds < minWindowSeconds {
		return nil, mcperr.InvalidWindow("window_seconds must be ≥ 60")
	}
	if windowSeconds > maxWindowSeconds {
		return nil, mcperr.InvalidWindow("window_seconds must be ≤ 604800 (7 days retention)")
	}

	// Resolve monitor reference (UUID or name).
	monitorID, err := resolve.Monitor(ctx, deps.Client, monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// Compute time range: to = now, from = now - window_seconds.
	now := time.Now().UTC()
	to := now
	from := now.Add(-time.Duration(windowSeconds) * time.Second)

	// Fetch history from Pulse.
	history, err := deps.Client.GetMonitorHistory(ctx, monitorID, pulseapi.TimeRange{
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Delegate to downtime.Summarize for the pure computation.
	summary := downtime.Summarize(history.Points, from, to, history.Truncated)

	// Map the Summary to the output struct.
	periods := make([]DowntimePeriodOutput, 0, len(summary.Periods))
	for _, p := range summary.Periods {
		periods = append(periods, DowntimePeriodOutput{
			Start:           p.Start.UTC().Format(time.RFC3339),
			End:             p.End.UTC().Format(time.RFC3339),
			DurationSeconds: p.DurationSeconds,
		})
	}

	return &DowntimeSummaryOutput{
		MonitorID:            monitorID,
		HadDowntime:          summary.HadDowntime,
		DowntimePeriodCount:  summary.DowntimePeriodCount,
		TotalDowntimeSeconds: summary.TotalDowntimeSeconds,
		WindowStart:          summary.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:            summary.WindowEnd.UTC().Format(time.RFC3339),
		Truncated:            summary.Truncated,
		Periods:              periods,
	}, nil
}
