package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// downtimeFakeClient extends fakePulseClient with a customizable GetMonitorHistory.
type downtimeFakeClient struct {
	fakePulseClient
	getMonitorHistoryFunc func(ctx context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error)
	listMonitorsFn        func(ctx context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error)
}

func (f *downtimeFakeClient) GetMonitorHistory(ctx context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
	if f.getMonitorHistoryFunc != nil {
		return f.getMonitorHistoryFunc(ctx, id, r)
	}
	return pulseapi.History{}, nil
}

func (f *downtimeFakeClient) ListMonitors(ctx context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	if f.listMonitorsFn != nil {
		return f.listMonitorsFn(ctx, q)
	}
	return pulseapi.MonitorPage{}, nil
}

func intPtr(v int) *int { return &v }

func TestHandleDowntimeSummary_DefaultWindow(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"
	var capturedRange pulseapi.TimeRange

	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			if id != monitorID {
				t.Errorf("expected id %q, got %q", monitorID, id)
			}
			capturedRange = r
			return pulseapi.History{
				MonitorID: id,
				From:      r.From,
				To:        r.To,
				Truncated: false,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor: monitorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default window should be 86400 seconds (24h).
	windowDuration := capturedRange.To.Sub(capturedRange.From)
	if windowDuration != 24*time.Hour {
		t.Errorf("expected 24h window, got %v", windowDuration)
	}

	if out.MonitorID != monitorID {
		t.Errorf("expected monitor_id %q, got %q", monitorID, out.MonitorID)
	}
	if out.HadDowntime {
		t.Error("expected had_downtime=false with no points")
	}
	if out.DowntimePeriodCount != 0 {
		t.Errorf("expected 0 periods, got %d", out.DowntimePeriodCount)
	}
	if out.TotalDowntimeSeconds != 0 {
		t.Errorf("expected 0 total downtime, got %d", out.TotalDowntimeSeconds)
	}
}

func TestHandleDowntimeSummary_CustomWindow(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"
	var capturedRange pulseapi.TimeRange

	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, r pulseapi.TimeRange) (pulseapi.History, error) {
			capturedRange = r
			return pulseapi.History{
				MonitorID: monitorID,
				From:      r.From,
				To:        r.To,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	window := 3600 // 1 hour
	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(window),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	windowDuration := capturedRange.To.Sub(capturedRange.From)
	if windowDuration != time.Hour {
		t.Errorf("expected 1h window, got %v", windowDuration)
	}
}

func TestHandleDowntimeSummary_WindowTooSmall(t *testing.T) {
	client := &downtimeFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       "550e8400-e29b-41d4-a716-446655440000",
		WindowSeconds: intPtr(30),
	})
	if err == nil {
		t.Fatal("expected error for window < 60")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidWindow {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidWindow, mcpErr.Code)
	}
}

func TestHandleDowntimeSummary_WindowTooLarge(t *testing.T) {
	client := &downtimeFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       "550e8400-e29b-41d4-a716-446655440000",
		WindowSeconds: intPtr(700000),
	})
	if err == nil {
		t.Fatal("expected error for window > 604800")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidWindow {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidWindow, mcpErr.Code)
	}
}

func TestHandleDowntimeSummary_EmptyMonitor(t *testing.T) {
	client := &downtimeFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor: "",
	})
	if err == nil {
		t.Fatal("expected error for empty monitor")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleDowntimeSummary_WithDowntime(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"
	now := time.Now().UTC().Truncate(time.Second)

	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, r pulseapi.TimeRange) (pulseapi.History, error) {
			// Simulate a down period from -600s to -300s (5 min downtime).
			return pulseapi.History{
				MonitorID: monitorID,
				From:      r.From,
				To:        r.To,
				Truncated: false,
				Points: []pulseapi.HistoryPoint{
					{State: "up", CheckedAt: now.Add(-900 * time.Second)},
					{State: "down", CheckedAt: now.Add(-600 * time.Second)},
					{State: "down", CheckedAt: now.Add(-450 * time.Second)},
					{State: "up", CheckedAt: now.Add(-300 * time.Second)},
					{State: "up", CheckedAt: now.Add(-60 * time.Second)},
				},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	window := 3600
	out, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(window),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.HadDowntime {
		t.Error("expected had_downtime=true")
	}
	if out.DowntimePeriodCount != 1 {
		t.Errorf("expected 1 period, got %d", out.DowntimePeriodCount)
	}
	if out.TotalDowntimeSeconds != 300 {
		t.Errorf("expected 300s downtime, got %d", out.TotalDowntimeSeconds)
	}
	if len(out.Periods) != 1 {
		t.Fatalf("expected 1 period in output, got %d", len(out.Periods))
	}
	if out.Periods[0].DurationSeconds != 300 {
		t.Errorf("expected period duration 300s, got %d", out.Periods[0].DurationSeconds)
	}
}

func TestHandleDowntimeSummary_NameResolution(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"

	client := &downtimeFakeClient{
		listMonitorsFn: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			return pulseapi.MonitorPage{
				Monitors: []pulseapi.Monitor{
					{ID: monitorID, Name: "my-web-app"},
				},
				Page:       1,
				Limit:      100,
				Total:      1,
				TotalPages: 1,
			}, nil
		},
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			if id != monitorID {
				t.Errorf("expected resolved id %q, got %q", monitorID, id)
			}
			return pulseapi.History{
				MonitorID: id,
				From:      r.From,
				To:        r.To,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor: "my-web-app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MonitorID != monitorID {
		t.Errorf("expected monitor_id %q, got %q", monitorID, out.MonitorID)
	}
}

func TestHandleDowntimeSummary_PulseConnectivityError(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"

	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{}, &pulseapi.ConnectivityError{Reason: "timeout"}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor: monitorID,
	})
	if err == nil {
		t.Fatal("expected error from connectivity issue")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodePulseTimeout {
		t.Errorf("expected code %q, got %q", mcperr.CodePulseTimeout, mcpErr.Code)
	}
}

func TestHandleDowntimeSummary_Truncated(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"

	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, r pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{
				MonitorID: monitorID,
				From:      r.From,
				To:        r.To,
				Truncated: true,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(604800),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("expected truncated=true to be passed through")
	}
}

func TestHandleDowntimeSummary_WindowBoundaries(t *testing.T) {
	monitorID := "550e8400-e29b-41d4-a716-446655440000"
	client := &downtimeFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, r pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{
				MonitorID: monitorID,
				From:      r.From,
				To:        r.To,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	// Min boundary: 60 seconds should succeed.
	_, err := HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(60),
	})
	if err != nil {
		t.Fatalf("expected success for window=60, got: %v", err)
	}

	// Max boundary: 604800 seconds should succeed.
	_, err = HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(604800),
	})
	if err != nil {
		t.Fatalf("expected success for window=604800, got: %v", err)
	}

	// Just below min: 59 should fail.
	_, err = HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(59),
	})
	if err == nil {
		t.Fatal("expected error for window=59")
	}

	// Just above max: 604801 should fail.
	_, err = HandleDowntimeSummary(context.Background(), deps, DowntimeSummaryInput{
		Monitor:       monitorID,
		WindowSeconds: intPtr(604801),
	})
	if err == nil {
		t.Fatal("expected error for window=604801")
	}
}
