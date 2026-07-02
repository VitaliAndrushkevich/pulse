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

// testUUID is a valid UUID used in tests so resolve.Monitor passes it through directly.
const testUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

// monitorHistoryFakeClient is a minimal fake for testing the monitor-history handler.
type monitorHistoryFakeClient struct {
	fakePulseClient
	getMonitorHistoryFunc func(ctx context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error)
}

func (f *monitorHistoryFakeClient) GetMonitorHistory(ctx context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
	if f.getMonitorHistoryFunc != nil {
		return f.getMonitorHistoryFunc(ctx, id, r)
	}
	return pulseapi.History{}, nil
}

func TestHandleMonitorHistory_DefaultWindow(t *testing.T) {
	before := time.Now().UTC()

	var capturedRange pulseapi.TimeRange
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			capturedRange = r
			if id != testUUID {
				t.Errorf("expected id %q, got %q", testUUID, id)
			}
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
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := time.Now().UTC()

	// Default window should be approximately 24 hours ending at now.
	windowDuration := capturedRange.To.Sub(capturedRange.From)
	if windowDuration < 23*time.Hour+59*time.Minute || windowDuration > 24*time.Hour+1*time.Minute {
		t.Errorf("expected ~24h window, got %v", windowDuration)
	}

	// The 'to' time should be between before and after.
	if capturedRange.To.Before(before.Add(-time.Second)) || capturedRange.To.After(after.Add(time.Second)) {
		t.Errorf("'to' should be approximately now, got %v", capturedRange.To)
	}

	if out.Truncated {
		t.Error("expected truncated=false")
	}
	if len(out.Points) != 0 {
		t.Errorf("expected empty points, got %d", len(out.Points))
	}
}

func TestHandleMonitorHistory_ExplicitRange(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)

	latency := int32(42)
	statusCode := int32(200)

	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			if !r.From.Equal(from) {
				t.Errorf("expected from %v, got %v", from, r.From)
			}
			if !r.To.Equal(to) {
				t.Errorf("expected to %v, got %v", to, r.To)
			}
			return pulseapi.History{
				MonitorID: id,
				From:      r.From,
				To:        r.To,
				Truncated: false,
				Points: []pulseapi.HistoryPoint{
					{State: "up", LatencyMs: &latency, StatusCode: &statusCode, CheckedAt: checkedAt},
				},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(out.Points))
	}
	pt := out.Points[0]
	if pt.State != "up" {
		t.Errorf("expected state 'up', got %q", pt.State)
	}
	if pt.LatencyMs == nil || *pt.LatencyMs != 42 {
		t.Errorf("expected latency_ms=42, got %v", pt.LatencyMs)
	}
	if pt.StatusCode == nil || *pt.StatusCode != 200 {
		t.Errorf("expected status_code=200, got %v", pt.StatusCode)
	}
	if pt.CheckedAt != "2025-01-01T06:00:00Z" {
		t.Errorf("expected checked_at='2025-01-01T06:00:00Z', got %q", pt.CheckedAt)
	}
}

func TestHandleMonitorHistory_FromAfterTo(t *testing.T) {
	called := false
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
			called = true
			return pulseapi.History{}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-02T00:00:00Z",
		To:      "2025-01-01T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error when from > to")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidRange {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidRange, mcpErr.Code)
	}
	if called {
		t.Error("Pulse API should NOT be called when from > to")
	}
}

func TestHandleMonitorHistory_EmptyMonitor(t *testing.T) {
	client := &monitorHistoryFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: "",
	})
	if err == nil {
		t.Fatal("expected error for empty monitor")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidIdentifier {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidIdentifier, mcpErr.Code)
	}
}

func TestHandleMonitorHistory_WhitespaceOnlyMonitor(t *testing.T) {
	client := &monitorHistoryFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: "   ",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only monitor")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidIdentifier {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidIdentifier, mcpErr.Code)
	}
}

func TestHandleMonitorHistory_MalformedFromTimestamp(t *testing.T) {
	client := &monitorHistoryFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for malformed from")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleMonitorHistory_MalformedToTimestamp(t *testing.T) {
	client := &monitorHistoryFakeClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for malformed to")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleMonitorHistory_Truncated(t *testing.T) {
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{
				MonitorID: id,
				From:      r.From,
				To:        r.To,
				Truncated: true,
				Points:    []pulseapi.HistoryPoint{},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2020-01-01T00:00:00Z",
		To:      "2025-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Truncated {
		t.Error("expected truncated=true to be passed through from Pulse")
	}
}

func TestHandleMonitorHistory_EmptyPointsInRange(t *testing.T) {
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
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
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Points) != 0 {
		t.Errorf("expected empty points slice, got %d points", len(out.Points))
	}
}

func TestHandleMonitorHistory_PulseError(t *testing.T) {
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{}, &pulseapi.PulseError{
				Code:       "NOT_FOUND",
				Message:    "monitor not found",
				RequestID:  "req-456",
				HTTPStatus: 404,
			}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error from Pulse")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != "NOT_FOUND" {
		t.Errorf("expected preserved code 'NOT_FOUND', got %q", mcpErr.Code)
	}
	if mcpErr.RequestID != "req-456" {
		t.Errorf("expected request ID 'req-456', got %q", mcpErr.RequestID)
	}
}

func TestHandleMonitorHistory_ConnectivityError(t *testing.T) {
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err == nil {
		t.Fatal("expected connectivity error")
	}

	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodePulseUnreachable {
		t.Errorf("expected code %q, got %q", mcperr.CodePulseUnreachable, mcpErr.Code)
	}
}

func TestHandleMonitorHistory_NameResolution(t *testing.T) {
	// Uses the fake client which supports ListMonitors for resolve.Monitor to find by name.
	var capturedID string
	client := &monitorHistoryFakeClient{
		fakePulseClient: fakePulseClient{
			listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				return pulseapi.MonitorPage{
					Monitors: []pulseapi.Monitor{
						{ID: "resolved-uuid", Name: "my-web-service"},
					},
					Page:       1,
					Limit:      100,
					Total:      1,
					TotalPages: 1,
				}, nil
			},
		},
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			capturedID = id
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
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: "my-web-service",
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedID != "resolved-uuid" {
		t.Errorf("expected resolved ID 'resolved-uuid', got %q", capturedID)
	}
	if out.MonitorID != "resolved-uuid" {
		t.Errorf("expected output monitor_id 'resolved-uuid', got %q", out.MonitorID)
	}
}

func TestHandleMonitorHistory_PointWithError(t *testing.T) {
	errMsg := "connection refused"
	client := &monitorHistoryFakeClient{
		getMonitorHistoryFunc: func(_ context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
			return pulseapi.History{
				MonitorID: id,
				From:      r.From,
				To:        r.To,
				Truncated: false,
				Points: []pulseapi.HistoryPoint{
					{State: "down", Error: &errMsg, CheckedAt: time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)},
				},
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleMonitorHistory(context.Background(), deps, MonitorHistoryInput{
		Monitor: testUUID,
		From:    "2025-01-01T00:00:00Z",
		To:      "2025-01-01T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(out.Points))
	}
	pt := out.Points[0]
	if pt.State != "down" {
		t.Errorf("expected state 'down', got %q", pt.State)
	}
	if pt.Error == nil || *pt.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %v", pt.Error)
	}
	if pt.LatencyMs != nil {
		t.Errorf("expected nil latency_ms for error point, got %v", pt.LatencyMs)
	}
}
