package resolve_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// fakeClient implements pulseapi.PulseClient for testing the resolve package.
type fakeClient struct {
	monitors []pulseapi.Monitor
	err      error
}

func (f *fakeClient) ListMonitors(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	if f.err != nil {
		return pulseapi.MonitorPage{}, f.err
	}

	start := (q.Page - 1) * q.Limit
	if start >= len(f.monitors) {
		return pulseapi.MonitorPage{
			Monitors:   nil,
			Page:       q.Page,
			Limit:      q.Limit,
			Total:      len(f.monitors),
			TotalPages: totalPages(len(f.monitors), q.Limit),
		}, nil
	}

	end := start + q.Limit
	if end > len(f.monitors) {
		end = len(f.monitors)
	}

	return pulseapi.MonitorPage{
		Monitors:   f.monitors[start:end],
		Page:       q.Page,
		Limit:      q.Limit,
		Total:      len(f.monitors),
		TotalPages: totalPages(len(f.monitors), q.Limit),
	}, nil
}

func totalPages(total, limit int) int {
	if total == 0 {
		return 0
	}
	pages := total / limit
	if total%limit != 0 {
		pages++
	}
	return pages
}

func (f *fakeClient) GetMonitor(context.Context, string) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}

func (f *fakeClient) GetMonitorStats(context.Context, string) (pulseapi.MonitorStats, error) {
	return pulseapi.MonitorStats{}, nil
}

func (f *fakeClient) GetMonitorHistory(context.Context, string, pulseapi.TimeRange) (pulseapi.History, error) {
	return pulseapi.History{}, nil
}

func (f *fakeClient) ListIncidents(context.Context, pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	return pulseapi.IncidentPage{}, nil
}

func (f *fakeClient) CreateMonitor(context.Context, pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", true},
		{"550e8400-e29b-41d4-a716-44665544000", false},  // too short
		{"550e8400-e29b-41d4-a716-4466554400000", false}, // too long
		{"not-a-uuid", false},
		{"", false},
		{"my-monitor-name", false},
		{"550e8400e29b41d4a716446655440000", false}, // no hyphens
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolve.IsUUID(tt.input)
			if got != tt.want {
				t.Errorf("IsUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMonitor_UUIDPassthrough(t *testing.T) {
	client := &fakeClient{}
	id := "550e8400-e29b-41d4-a716-446655440000"

	got, err := resolve.Monitor(context.Background(), client, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != id {
		t.Errorf("got %q, want %q", got, id)
	}
}

func TestMonitor_ExactNameMatch(t *testing.T) {
	client := &fakeClient{
		monitors: []pulseapi.Monitor{
			{ID: "aaa-111", Name: "api-server"},
			{ID: "bbb-222", Name: "web-frontend"},
			{ID: "ccc-333", Name: "database"},
		},
	}

	got, err := resolve.Monitor(context.Background(), client, "web-frontend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "bbb-222" {
		t.Errorf("got %q, want %q", got, "bbb-222")
	}
}

func TestMonitor_CaseSensitive(t *testing.T) {
	client := &fakeClient{
		monitors: []pulseapi.Monitor{
			{ID: "aaa-111", Name: "API-Server"},
		},
	}

	// Lowercase should not match uppercase name.
	_, err := resolve.Monitor(context.Background(), client, "api-server")
	if err == nil {
		t.Fatal("expected error for case-mismatch name, got nil")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected *mcperr.MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeNotFound {
		t.Errorf("got code %q, want %q", mcpErr.Code, mcperr.CodeNotFound)
	}
}

func TestMonitor_NotFound(t *testing.T) {
	client := &fakeClient{
		monitors: []pulseapi.Monitor{
			{ID: "aaa-111", Name: "api-server"},
		},
	}

	_, err := resolve.Monitor(context.Background(), client, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected *mcperr.MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeNotFound {
		t.Errorf("got code %q, want %q", mcpErr.Code, mcperr.CodeNotFound)
	}
}

func TestMonitor_AmbiguousName(t *testing.T) {
	client := &fakeClient{
		monitors: []pulseapi.Monitor{
			{ID: "aaa-111", Name: "shared-name"},
			{ID: "bbb-222", Name: "other"},
			{ID: "ccc-333", Name: "shared-name"},
		},
	}

	_, err := resolve.Monitor(context.Background(), client, "shared-name")
	if err == nil {
		t.Fatal("expected error for ambiguous name, got nil")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected *mcperr.MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeAmbiguousName {
		t.Errorf("got code %q, want %q", mcpErr.Code, mcperr.CodeAmbiguousName)
	}
}

func TestMonitor_PaginatedResolution(t *testing.T) {
	// Create more monitors than fit on one page (pageSize=100 in impl).
	// We simulate a smaller page to test pagination logic via the fake.
	monitors := make([]pulseapi.Monitor, 150)
	for i := range monitors {
		monitors[i] = pulseapi.Monitor{
			ID:   "id-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26)),
			Name: "monitor-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
		}
	}
	// Place the target monitor on the second page.
	monitors[120] = pulseapi.Monitor{ID: "target-id", Name: "target-monitor"}

	client := &fakeClient{monitors: monitors}

	got, err := resolve.Monitor(context.Background(), client, "target-monitor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "target-id" {
		t.Errorf("got %q, want %q", got, "target-id")
	}
}

func TestMonitor_ClientError(t *testing.T) {
	client := &fakeClient{
		err: &pulseapi.ConnectivityError{Reason: "timeout"},
	}

	_, err := resolve.Monitor(context.Background(), client, "some-name")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var connErr *pulseapi.ConnectivityError
	if !errors.As(err, &connErr) {
		t.Fatalf("expected *pulseapi.ConnectivityError, got %T: %v", err, err)
	}
}

func TestMonitor_EmptyMonitorList(t *testing.T) {
	client := &fakeClient{monitors: nil}

	_, err := resolve.Monitor(context.Background(), client, "anything")
	if err == nil {
		t.Fatal("expected NOT_FOUND error, got nil")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected *mcperr.MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeNotFound {
		t.Errorf("got code %q, want %q", mcpErr.Code, mcperr.CodeNotFound)
	}
}
