package tools

import (
	"context"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// PauseMonitorInput defines the input schema for the pause-monitor tool.
type PauseMonitorInput struct {
	// Monitor is a UUID or an exact monitor name (1–255 characters).
	Monitor string `json:"monitor" jsonschema:"Monitor UUID or exact name (1-255 chars)"`
}

// PauseMonitorOutput contains the result of pausing a monitor.
type PauseMonitorOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	State  string `json:"state"`
}

// HandlePauseMonitor pauses a monitor (sets status to "paused").
func HandlePauseMonitor(ctx context.Context, deps Deps, input PauseMonitorInput) (*PauseMonitorOutput, error) {
	// 1. Check access mode.
	if deps.AccessMode != config.ReadWrite {
		return nil, mcperr.WriteDisabled()
	}

	// 2. Validate input.
	if input.Monitor == "" {
		return nil, mcperr.Validation("monitor is required")
	}
	if len(input.Monitor) > 255 {
		return nil, mcperr.Validation("monitor must be 1–255 characters")
	}

	// 3. Resolve monitor reference.
	monitorID, err := resolve.Monitor(ctx, deps.Client, input.Monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// 4. Update status to paused.
	monitor, err := deps.Client.UpdateMonitorStatus(ctx, monitorID, "paused")
	if err != nil {
		return nil, mapPulseError(err)
	}

	return &PauseMonitorOutput{
		ID:     monitor.ID,
		Name:   monitor.Name,
		Status: monitor.Status,
		State:  monitor.State,
	}, nil
}

// ResumeMonitorInput defines the input schema for the resume-monitor tool.
type ResumeMonitorInput struct {
	// Monitor is a UUID or an exact monitor name (1–255 characters).
	Monitor string `json:"monitor" jsonschema:"Monitor UUID or exact name (1-255 chars)"`
}

// ResumeMonitorOutput contains the result of resuming a monitor.
type ResumeMonitorOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	State  string `json:"state"`
}

// HandleResumeMonitor resumes a paused monitor (sets status to "active").
func HandleResumeMonitor(ctx context.Context, deps Deps, input ResumeMonitorInput) (*ResumeMonitorOutput, error) {
	// 1. Check access mode.
	if deps.AccessMode != config.ReadWrite {
		return nil, mcperr.WriteDisabled()
	}

	// 2. Validate input.
	if input.Monitor == "" {
		return nil, mcperr.Validation("monitor is required")
	}
	if len(input.Monitor) > 255 {
		return nil, mcperr.Validation("monitor must be 1–255 characters")
	}

	// 3. Resolve monitor reference.
	monitorID, err := resolve.Monitor(ctx, deps.Client, input.Monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// 4. Update status to active.
	monitor, err := deps.Client.UpdateMonitorStatus(ctx, monitorID, "active")
	if err != nil {
		return nil, mapPulseError(err)
	}

	return &ResumeMonitorOutput{
		ID:     monitor.ID,
		Name:   monitor.Name,
		Status: monitor.Status,
		State:  monitor.State,
	}, nil
}
