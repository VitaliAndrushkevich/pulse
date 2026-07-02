package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// GetMonitorInput defines the input schema for the get-monitor tool.
type GetMonitorInput struct {
	// Monitor is a UUID or an exact monitor name (1–255 characters).
	Monitor string `json:"monitor" jsonschema:"Monitor UUID or exact name (1-255 chars)"`
}

// GetMonitorOutput contains the full monitor configuration and current status.
type GetMonitorOutput struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Target          string   `json:"target"`
	IntervalSeconds int      `json:"interval_seconds"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Status          string   `json:"status"`
	State           string   `json:"state"`
	LastCheckedAt   *string  `json:"last_checked_at,omitempty"`
	NextCheckAt     *string  `json:"next_check_at,omitempty"`
	Tags            []string `json:"tags"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// HandleGetMonitor resolves a monitor reference and returns its full configuration
// and current status, including the latest check state and most recent check timestamp.
func HandleGetMonitor(ctx context.Context, deps Deps, input GetMonitorInput) (*GetMonitorOutput, error) {
	// Validate input: monitor reference must not be empty.
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

	// Fetch monitor from Pulse API.
	monitor, err := deps.Client.GetMonitor(ctx, monitorID)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Project non-secret fields into the output.
	output := projectMonitor(monitor)
	return output, nil
}

// projectMonitor maps an internal Monitor DTO to the tool output, formatting
// timestamps as RFC3339 and rendering tags as "key:value" strings.
func projectMonitor(m pulseapi.Monitor) *GetMonitorOutput {
	out := &GetMonitorOutput{
		ID:              m.ID,
		Name:            m.Name,
		Type:            m.Type,
		Target:          m.Target,
		IntervalSeconds: m.IntervalSeconds,
		TimeoutSeconds:  m.TimeoutSeconds,
		Status:          m.Status,
		State:           m.State,
		Tags:            formatTags(m.Tags),
		CreatedAt:       m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       m.UpdatedAt.Format(time.RFC3339),
	}

	if m.LastCheckedAt != nil {
		t := m.LastCheckedAt.Format(time.RFC3339)
		out.LastCheckedAt = &t
	}
	if m.NextCheckAt != nil {
		t := m.NextCheckAt.Format(time.RFC3339)
		out.NextCheckAt = &t
	}

	return out
}

// formatTags converts a slice of Tag structs to "key:value" string representation.
func formatTags(tags []pulseapi.Tag) []string {
	if len(tags) == 0 {
		return []string{}
	}
	result := make([]string, len(tags))
	for i, tag := range tags {
		result[i] = fmt.Sprintf("%s:%s", tag.Key, tag.Value)
	}
	return result
}


