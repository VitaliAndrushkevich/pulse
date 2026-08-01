package tools

import (
	"context"
	"fmt"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// DeleteMonitorInput defines the input schema for the delete-monitor tool.
type DeleteMonitorInput struct {
	// Monitor is a UUID or an exact monitor name (1–255 characters).
	Monitor string `json:"monitor" jsonschema:"Monitor UUID or exact name (1-255 chars)"`

	// Confirm must be true to perform the deletion. When false or omitted,
	// the tool returns a preview of the monitor that would be deleted without
	// actually removing it. This prevents accidental deletions by AI agents.
	Confirm bool `json:"confirm" jsonschema:"Must be true to actually delete. When false, returns a preview without deleting."`
}

// DeleteMonitorOutput contains the result of a delete operation.
type DeleteMonitorOutput struct {
	// Deleted is true when the monitor was actually removed.
	Deleted bool `json:"deleted"`

	// Message describes what happened.
	Message string `json:"message"`

	// Monitor contains identification details of the (to-be-)deleted monitor.
	Monitor *DeleteMonitorInfo `json:"monitor"`
}

// DeleteMonitorInfo holds identification details for the delete preview/confirmation.
type DeleteMonitorInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Status string `json:"status"`
	State  string `json:"state"`
}

// HandleDeleteMonitor handles the delete-monitor tool.
// Safety: requires confirm=true to actually delete. Without it, returns a preview.
func HandleDeleteMonitor(ctx context.Context, deps Deps, input DeleteMonitorInput) (*DeleteMonitorOutput, error) {
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

	// 4. Fetch monitor details for the preview/confirmation.
	monitor, err := deps.Client.GetMonitor(ctx, monitorID)
	if err != nil {
		return nil, mapPulseError(err)
	}

	info := &DeleteMonitorInfo{
		ID:     monitor.ID,
		Name:   monitor.Name,
		Type:   monitor.Type,
		Target: monitor.Target,
		Status: monitor.Status,
		State:  monitor.State,
	}

	// 5. If not confirmed, return preview only.
	if !input.Confirm {
		return &DeleteMonitorOutput{
			Deleted: false,
			Message: fmt.Sprintf("Monitor %q (%s, %s) will be permanently deleted. Set confirm=true to proceed.", monitor.Name, monitor.Type, monitor.Target),
			Monitor: info,
		}, nil
	}

	// 6. Perform deletion.
	if err := deps.Client.DeleteMonitor(ctx, monitorID); err != nil {
		return nil, mapPulseError(err)
	}

	return &DeleteMonitorOutput{
		Deleted: true,
		Message: fmt.Sprintf("Monitor %q deleted successfully.", monitor.Name),
		Monitor: info,
	}, nil
}
