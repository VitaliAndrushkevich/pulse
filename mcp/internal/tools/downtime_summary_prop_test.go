package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

// trackingPulseClient records whether any PulseClient method was called.
// Used to verify that invalid-window rejection happens before any Pulse call.
type trackingPulseClient struct {
	called bool
}

func (t *trackingPulseClient) ListMonitors(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	t.called = true
	return pulseapi.MonitorPage{}, nil
}

func (t *trackingPulseClient) GetMonitor(_ context.Context, _ string) (pulseapi.Monitor, error) {
	t.called = true
	return pulseapi.Monitor{}, nil
}

func (t *trackingPulseClient) GetMonitorStats(_ context.Context, _ string) (pulseapi.MonitorStats, error) {
	t.called = true
	return pulseapi.MonitorStats{}, nil
}

func (t *trackingPulseClient) GetMonitorHistory(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
	t.called = true
	return pulseapi.History{}, nil
}

func (t *trackingPulseClient) ListIncidents(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	t.called = true
	return pulseapi.IncidentPage{}, nil
}

func (t *trackingPulseClient) CreateMonitor(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	t.called = true
	return pulseapi.Monitor{}, nil
}

// genInvalidWindowSeconds generates window_seconds values that are invalid:
// negative, zero, 1–59, or > 604800.
func genInvalidWindowSeconds() *rapid.Generator[int] {
	return rapid.OneOf(
		// Negative values.
		rapid.IntRange(-1_000_000, -1),
		// Zero.
		rapid.Just(0),
		// Too small: 1–59.
		rapid.IntRange(1, 59),
		// Too large: 604801+.
		rapid.IntRange(604801, 10_000_000),
	)
}

// TestProperty16_InvalidWindowRejectedWithoutCallingPulse validates that for any
// window_seconds value that is < 60 or > 604800 (or non-positive), the handler
// returns INVALID_WINDOW without calling any PulseClient method.
//
// **Validates: Requirements 7.7**
func TestProperty16_InvalidWindowRejectedWithoutCallingPulse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		invalidWindow := genInvalidWindowSeconds().Draw(t, "window_seconds")

		tracker := &trackingPulseClient{}
		deps := Deps{Client: tracker, AccessMode: config.ReadOnly}

		// Use a valid UUID as monitor so we isolate the window validation path.
		input := DowntimeSummaryInput{
			Monitor:       "550e8400-e29b-41d4-a716-446655440000",
			WindowSeconds: &invalidWindow,
		}

		_, err := HandleDowntimeSummary(context.Background(), deps, input)

		// 1. Error must be returned with code INVALID_WINDOW.
		if err == nil {
			t.Fatalf("expected INVALID_WINDOW error for window_seconds=%d, got nil", invalidWindow)
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError for window_seconds=%d, got %T: %v", invalidWindow, err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidWindow {
			t.Fatalf("expected code %q for window_seconds=%d, got %q", mcperr.CodeInvalidWindow, invalidWindow, mcpErr.Code)
		}

		// 2. No PulseClient method should have been called.
		if tracker.called {
			t.Fatalf("PulseClient was called for invalid window_seconds=%d; expected no Pulse calls", invalidWindow)
		}
	})
}
