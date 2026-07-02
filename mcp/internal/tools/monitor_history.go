package tools

import (
	"context"
	"strings"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// MonitorHistoryInput is the input schema for the monitor-history tool.
type MonitorHistoryInput struct {
	Monitor string `json:"monitor" jsonschema:"Monitor identifier (UUID or name),required"`
	From    string `json:"from,omitempty" jsonschema:"Start of time range (RFC3339). Defaults to now minus 24 hours."`
	To      string `json:"to,omitempty" jsonschema:"End of time range (RFC3339). Defaults to now."`
}

// HistoryPointOutput is a single check result point in the history output.
type HistoryPointOutput struct {
	State      string `json:"state"`
	LatencyMs  *int32 `json:"latency_ms,omitempty"`
	StatusCode *int32 `json:"status_code,omitempty"`
	Error      *string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

// MonitorHistoryOutput is the result returned by the monitor-history tool.
type MonitorHistoryOutput struct {
	MonitorID string               `json:"monitor_id"`
	From      string               `json:"from"`
	To        string               `json:"to"`
	Truncated bool                 `json:"truncated"`
	Points    []HistoryPointOutput `json:"points"`
}

// HandleMonitorHistory implements the monitor-history tool.
// It validates inputs, resolves the monitor identifier, fetches history from Pulse,
// and returns the check history points within the requested time range.
func HandleMonitorHistory(ctx context.Context, deps Deps, input MonitorHistoryInput) (*MonitorHistoryOutput, error) {
	// Validate monitor identifier: empty/whitespace-only is invalid.
	monitor := strings.TrimSpace(input.Monitor)
	if monitor == "" {
		return nil, mcperr.InvalidIdentifier("monitor identifier must not be empty")
	}

	// Determine the effective time range.
	now := time.Now().UTC()
	var from, to time.Time

	if input.From != "" {
		parsed, err := time.Parse(time.RFC3339, input.From)
		if err != nil {
			return nil, mcperr.Validation("from: invalid RFC3339 timestamp")
		}
		from = parsed
	} else {
		from = now.Add(-24 * time.Hour)
	}

	if input.To != "" {
		parsed, err := time.Parse(time.RFC3339, input.To)
		if err != nil {
			return nil, mcperr.Validation("to: invalid RFC3339 timestamp")
		}
		to = parsed
	} else {
		to = now
	}

	// Validate: from must not be after to.
	if from.After(to) {
		return nil, mcperr.InvalidRange("from must not be later than to")
	}

	// Resolve monitor identifier (UUID or name).
	monitorID, err := resolve.Monitor(ctx, deps.Client, monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// Fetch history from Pulse.
	history, err := deps.Client.GetMonitorHistory(ctx, monitorID, pulseapi.TimeRange{
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Build output points.
	points := make([]HistoryPointOutput, 0, len(history.Points))
	for _, p := range history.Points {
		points = append(points, HistoryPointOutput{
			State:      p.State,
			LatencyMs:  p.LatencyMs,
			StatusCode: p.StatusCode,
			Error:      p.Error,
			CheckedAt:  p.CheckedAt.UTC().Format(time.RFC3339),
		})
	}

	return &MonitorHistoryOutput{
		MonitorID: history.MonitorID,
		From:      history.From.UTC().Format(time.RFC3339),
		To:        history.To.UTC().Format(time.RFC3339),
		Truncated: history.Truncated,
		Points:    points,
	}, nil
}
