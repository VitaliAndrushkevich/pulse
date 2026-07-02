package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// fakePulseClient is a minimal fake for testing tool handlers.
type fakePulseClient struct {
	listMonitorsFunc  func(ctx context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error)
	createMonitorFunc func(ctx context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error)
}

func (f *fakePulseClient) ListMonitors(ctx context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	if f.listMonitorsFunc != nil {
		return f.listMonitorsFunc(ctx, q)
	}
	return pulseapi.MonitorPage{}, nil
}

func (f *fakePulseClient) GetMonitor(ctx context.Context, id string) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}

func (f *fakePulseClient) GetMonitorStats(ctx context.Context, id string) (pulseapi.MonitorStats, error) {
	return pulseapi.MonitorStats{}, nil
}

func (f *fakePulseClient) GetMonitorHistory(ctx context.Context, id string, r pulseapi.TimeRange) (pulseapi.History, error) {
	return pulseapi.History{}, nil
}

func (f *fakePulseClient) ListIncidents(ctx context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	return pulseapi.IncidentPage{}, nil
}

func (f *fakePulseClient) CreateMonitor(ctx context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	if f.createMonitorFunc != nil {
		return f.createMonitorFunc(ctx, in)
	}
	return pulseapi.Monitor{}, nil
}

func TestHandleListMonitors_Defaults(t *testing.T) {
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			if q.Page != 1 {
				t.Errorf("expected page 1, got %d", q.Page)
			}
			if q.Limit != 50 {
				t.Errorf("expected limit 50, got %d", q.Limit)
			}
			return pulseapi.MonitorPage{
				Monitors: []pulseapi.Monitor{
					{ID: "id-1", Name: "Web", Type: "http", Target: "https://example.com", Status: "up", State: "active"},
				},
				Page:       1,
				Limit:      50,
				Total:      1,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Page != 1 {
		t.Errorf("expected page 1, got %d", out.Page)
	}
	if out.Limit != 50 {
		t.Errorf("expected limit 50, got %d", out.Limit)
	}
	if out.Total != 1 {
		t.Errorf("expected total 1, got %d", out.Total)
	}
	if len(out.Monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(out.Monitors))
	}
	if out.Monitors[0].ID != "id-1" {
		t.Errorf("expected id 'id-1', got %q", out.Monitors[0].ID)
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for single page")
	}
}

func TestHandleListMonitors_InvalidType(t *testing.T) {
	called := false
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			called = true
			return pulseapi.MonitorPage{}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Type: "QUIC"})
	if err == nil {
		t.Fatal("expected error for unrecognized type")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidType {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidType, mcpErr.Code)
	}
	if called {
		t.Error("Pulse API should NOT be called when type is invalid")
	}
}

func TestHandleListMonitors_TypeNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http", "http"},
		{"HTTP", "http"},
		{"Http", "http"},
		{"HTTPS", "https"},
		{"HTTP/3", "http3"},
		{"http/3", "http3"},
		{"WebSocket", "websocket"},
		{"WEBSOCKET", "websocket"},
		{"gRPC", "grpc"},
		{"GRPC", "grpc"},
		{"tcp", "tcp"},
		{"UDP", "udp"},
		{"DNS", "dns"},
		{"ICMP", "icmp"},
		{"SMTP", "smtp"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var gotType string
			client := &fakePulseClient{
				listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
					gotType = q.Type
					return pulseapi.MonitorPage{Page: 1, Limit: 50, Total: 0, TotalPages: 0}, nil
				},
			}

			deps := Deps{Client: client, AccessMode: config.ReadOnly}
			_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Type: tt.input})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.expected {
				t.Errorf("expected normalized type %q, got %q", tt.expected, gotType)
			}
		})
	}
}

func TestHandleListMonitors_InvalidPage(t *testing.T) {
	client := &fakePulseClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Page: -1})
	if err == nil {
		t.Fatal("expected error for page < 1")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeInvalidRange {
		t.Errorf("expected code %q, got %q", mcperr.CodeInvalidRange, mcpErr.Code)
	}
}

func TestHandleListMonitors_InvalidLimit(t *testing.T) {
	client := &fakePulseClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	cases := []int{0, -5, 101, 200}
	for _, limit := range cases {
		// When limit is 0 it defaults to 50, so only test explicit out-of-range via Page=1.
		if limit == 0 {
			continue // 0 triggers the default, not an error
		}
		_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Page: 1, Limit: limit})
		if err == nil {
			t.Errorf("expected error for limit=%d", limit)
			continue
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Errorf("limit=%d: expected MCPError, got %T", limit, err)
		}
	}
}

func TestHandleListMonitors_EmptyResult(t *testing.T) {
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			return pulseapi.MonitorPage{
				Monitors:   []pulseapi.Monitor{},
				Page:       1,
				Limit:      50,
				Total:      0,
				TotalPages: 0,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 0 {
		t.Errorf("expected total 0, got %d", out.Total)
	}
	if len(out.Monitors) != 0 {
		t.Errorf("expected empty monitors slice, got %d", len(out.Monitors))
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for empty result")
	}
}

func TestHandleListMonitors_Pagination(t *testing.T) {
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			return pulseapi.MonitorPage{
				Monitors: []pulseapi.Monitor{
					{ID: "id-1", Name: "Mon1", Type: "http", Target: "https://a.com", Status: "up", State: "active"},
				},
				Page:       q.Page,
				Limit:      q.Limit,
				Total:      75,
				TotalPages: 2,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{Page: 1, Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TotalPages != 2 {
		t.Errorf("expected total_pages 2, got %d", out.TotalPages)
	}
	if !out.HasNextPage {
		t.Error("expected has_next_page=true for page 1 of 2")
	}

	// Page 2 — no next page.
	out, err = HandleListMonitors(context.Background(), deps, ListMonitorsInput{Page: 2, Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for last page")
	}
}

func TestHandleListMonitors_PulseError(t *testing.T) {
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			return pulseapi.MonitorPage{}, &pulseapi.PulseError{
				Code:       "INTERNAL_ERROR",
				Message:    "something went wrong",
				RequestID:  "req-123",
				HTTPStatus: 500,
			}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{})
	if err == nil {
		t.Fatal("expected error from Pulse")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != "INTERNAL_ERROR" {
		t.Errorf("expected preserved code, got %q", mcpErr.Code)
	}
	if mcpErr.RequestID != "req-123" {
		t.Errorf("expected request ID 'req-123', got %q", mcpErr.RequestID)
	}
}

func TestHandleListMonitors_ConnectivityError(t *testing.T) {
	client := &fakePulseClient{
		listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
			return pulseapi.MonitorPage{}, &pulseapi.ConnectivityError{Reason: "timeout"}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleListMonitors(context.Background(), deps, ListMonitorsInput{})
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
