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

// incidentFakePulseClient extends fakePulseClient with incident-specific behavior.
type incidentFakePulseClient struct {
	fakePulseClient
	listIncidentsFunc func(ctx context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error)
}

func (f *incidentFakePulseClient) ListIncidents(ctx context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	if f.listIncidentsFunc != nil {
		return f.listIncidentsFunc(ctx, q)
	}
	return pulseapi.IncidentPage{}, nil
}

func boolPtr(b bool) *bool { return &b }

func TestHandleListIncidents_Defaults(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			if q.Page != 1 {
				t.Errorf("expected page 1, got %d", q.Page)
			}
			if q.Limit != 20 {
				t.Errorf("expected limit 20, got %d", q.Limit)
			}
			if q.MonitorID != "" {
				t.Errorf("expected empty monitor ID, got %q", q.MonitorID)
			}
			if q.OpenOnly {
				t.Error("expected OpenOnly=false")
			}
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{
						ID:        "inc-1",
						MonitorID: "mon-1",
						Status:    "open",
						StartedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
					},
				},
				Page:       1,
				Limit:      20,
				Total:      1,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Page != 1 {
		t.Errorf("expected page 1, got %d", out.Page)
	}
	if out.Limit != 20 {
		t.Errorf("expected limit 20, got %d", out.Limit)
	}
	if out.Total != 1 {
		t.Errorf("expected total 1, got %d", out.Total)
	}
	if len(out.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(out.Incidents))
	}
	if out.Incidents[0].ID != "inc-1" {
		t.Errorf("expected id 'inc-1', got %q", out.Incidents[0].ID)
	}
	if out.Incidents[0].ResolvedAt != nil {
		t.Error("expected resolved_at to be nil for open incident")
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for single page")
	}
}

func TestHandleListIncidents_InvalidPage(t *testing.T) {
	client := &incidentFakePulseClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	_, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{Page: -1})
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

func TestHandleListIncidents_InvalidLimit(t *testing.T) {
	client := &incidentFakePulseClient{}
	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	cases := []int{-5, 101, 200}
	for _, limit := range cases {
		_, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{Page: 1, Limit: limit})
		if err == nil {
			t.Errorf("expected error for limit=%d", limit)
			continue
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Errorf("limit=%d: expected MCPError, got %T", limit, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidRange {
			t.Errorf("limit=%d: expected code %q, got %q", limit, mcperr.CodeInvalidRange, mcpErr.Code)
		}
	}
}

func TestHandleListIncidents_OpenOnlyGlobal(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			if !q.OpenOnly {
				t.Error("expected OpenOnly=true")
			}
			if q.MonitorID != "" {
				t.Errorf("expected empty monitor ID, got %q", q.MonitorID)
			}
			return pulseapi.IncidentPage{
				Incidents:  []pulseapi.Incident{},
				Page:       1,
				Limit:      20,
				Total:      0,
				TotalPages: 0,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{OpenOnly: boolPtr(true)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Incidents) != 0 {
		t.Errorf("expected empty incidents, got %d", len(out.Incidents))
	}
	if out.Total != 0 {
		t.Errorf("expected total 0, got %d", out.Total)
	}
}

func TestHandleListIncidents_PerMonitorByUUID(t *testing.T) {
	monitorUUID := "550e8400-e29b-41d4-a716-446655440000"
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			if q.MonitorID != monitorUUID {
				t.Errorf("expected monitor ID %q, got %q", monitorUUID, q.MonitorID)
			}
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "inc-1", MonitorID: monitorUUID, Status: "resolved",
						StartedAt:  time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC),
						ResolvedAt: timePtr(time.Date(2024, 1, 10, 9, 30, 0, 0, time.UTC)),
					},
				},
				Page:       1,
				Limit:      20,
				Total:      1,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{Monitor: monitorUUID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(out.Incidents))
	}
	inc := out.Incidents[0]
	if inc.Status != "resolved" {
		t.Errorf("expected status 'resolved', got %q", inc.Status)
	}
	if inc.ResolvedAt == nil {
		t.Fatal("expected resolved_at to be present for resolved incident")
	}
	if *inc.ResolvedAt != "2024-01-10T09:30:00Z" {
		t.Errorf("expected resolved_at '2024-01-10T09:30:00Z', got %q", *inc.ResolvedAt)
	}
}

func TestHandleListIncidents_PerMonitorOpenOnlyClientSideFilter(t *testing.T) {
	monitorUUID := "550e8400-e29b-41d4-a716-446655440000"
	callCount := 0

	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			callCount++
			if q.MonitorID != monitorUUID {
				t.Errorf("expected monitor ID %q, got %q", monitorUUID, q.MonitorID)
			}
			// Should NOT pass OpenOnly to the per-monitor endpoint.
			if q.OpenOnly {
				t.Error("expected OpenOnly=false for per-monitor fetch (client-side filtering)")
			}
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "inc-open-1", MonitorID: monitorUUID, Status: "open",
						StartedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)},
					{ID: "inc-resolved-1", MonitorID: monitorUUID, Status: "resolved",
						StartedAt:  time.Date(2024, 1, 14, 8, 0, 0, 0, time.UTC),
						ResolvedAt: timePtr(time.Date(2024, 1, 14, 9, 0, 0, 0, time.UTC)),
					},
					{ID: "inc-open-2", MonitorID: monitorUUID, Status: "open",
						StartedAt: time.Date(2024, 1, 13, 12, 0, 0, 0, time.UTC)},
				},
				Page:       1,
				Limit:      100,
				Total:      3,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{
		Monitor:  monitorUUID,
		OpenOnly: boolPtr(true),
		Page:     1,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only open incidents should remain after client-side filtering.
	if out.Total != 2 {
		t.Errorf("expected total 2 (open only), got %d", out.Total)
	}
	if len(out.Incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(out.Incidents))
	}

	// Should be ordered by started_at descending.
	if out.Incidents[0].ID != "inc-open-1" {
		t.Errorf("expected first incident 'inc-open-1', got %q", out.Incidents[0].ID)
	}
	if out.Incidents[1].ID != "inc-open-2" {
		t.Errorf("expected second incident 'inc-open-2', got %q", out.Incidents[1].ID)
	}

	// No resolved_at for open incidents.
	for _, inc := range out.Incidents {
		if inc.ResolvedAt != nil {
			t.Errorf("expected no resolved_at for open incident %q", inc.ID)
		}
	}
}

func TestHandleListIncidents_OrderByStartedAtDesc(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			// Return in ascending order — handler should sort descending.
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "old", MonitorID: "m1", Status: "resolved",
						StartedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						ResolvedAt: timePtr(time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)),
					},
					{ID: "newest", MonitorID: "m2", Status: "open",
						StartedAt: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)},
					{ID: "middle", MonitorID: "m1", Status: "open",
						StartedAt: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)},
				},
				Page:       1,
				Limit:      20,
				Total:      3,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Incidents) != 3 {
		t.Fatalf("expected 3 incidents, got %d", len(out.Incidents))
	}
	expectedOrder := []string{"newest", "middle", "old"}
	for i, expected := range expectedOrder {
		if out.Incidents[i].ID != expected {
			t.Errorf("position %d: expected %q, got %q", i, expected, out.Incidents[i].ID)
		}
	}
}

func TestHandleListIncidents_EmptyResult(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			return pulseapi.IncidentPage{
				Incidents:  []pulseapi.Incident{},
				Page:       1,
				Limit:      20,
				Total:      0,
				TotalPages: 0,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 0 {
		t.Errorf("expected total 0, got %d", out.Total)
	}
	if len(out.Incidents) != 0 {
		t.Errorf("expected empty incidents, got %d", len(out.Incidents))
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for empty result")
	}
}

func TestHandleListIncidents_Pagination(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "inc-1", MonitorID: "m1", Status: "open",
						StartedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)},
				},
				Page:       q.Page,
				Limit:      q.Limit,
				Total:      45,
				TotalPages: 3,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	// Page 1 of 3 — has next.
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.HasNextPage {
		t.Error("expected has_next_page=true for page 1 of 3")
	}

	// Page 3 of 3 — no next.
	out, err = HandleListIncidents(context.Background(), deps, ListIncidentsInput{Page: 3, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for last page")
	}
}

func TestHandleListIncidents_ResolvedAtOnlyWhenResolved(t *testing.T) {
	resolvedTime := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "open-inc", MonitorID: "m1", Status: "open",
						StartedAt:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
						ResolvedAt: nil},
					{ID: "resolved-inc", MonitorID: "m1", Status: "resolved",
						StartedAt:  time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC),
						ResolvedAt: &resolvedTime},
				},
				Page:       1,
				Limit:      20,
				Total:      2,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sorted desc: open-inc (Jan 15) first, resolved-inc (Jan 10) second.
	if out.Incidents[0].ResolvedAt != nil {
		t.Error("open incident should not have resolved_at")
	}
	if out.Incidents[1].ResolvedAt == nil {
		t.Fatal("resolved incident should have resolved_at")
	}
	if *out.Incidents[1].ResolvedAt != "2024-01-10T12:00:00Z" {
		t.Errorf("expected resolved_at '2024-01-10T12:00:00Z', got %q", *out.Incidents[1].ResolvedAt)
	}
}

func TestHandleListIncidents_PulseError(t *testing.T) {
	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			return pulseapi.IncidentPage{}, &pulseapi.PulseError{
				Code:       "INTERNAL_ERROR",
				Message:    "something went wrong",
				RequestID:  "req-456",
				HTTPStatus: 500,
			}
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{})
	if err == nil {
		t.Fatal("expected error from Pulse")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code 'INTERNAL_ERROR', got %q", mcpErr.Code)
	}
	if mcpErr.RequestID != "req-456" {
		t.Errorf("expected request ID 'req-456', got %q", mcpErr.RequestID)
	}
}

func TestHandleListIncidents_MonitorNotFound(t *testing.T) {
	client := &incidentFakePulseClient{
		fakePulseClient: fakePulseClient{
			listMonitorsFunc: func(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
				return pulseapi.MonitorPage{
					Monitors:   []pulseapi.Monitor{},
					Page:       1,
					Limit:      100,
					Total:      0,
					TotalPages: 0,
				}, nil
			},
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{Monitor: "nonexistent-monitor"})
	if err == nil {
		t.Fatal("expected error for nonexistent monitor")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeNotFound {
		t.Errorf("expected code %q, got %q", mcperr.CodeNotFound, mcpErr.Code)
	}
}

func TestHandleListIncidents_ClientSidePagination(t *testing.T) {
	monitorUUID := "550e8400-e29b-41d4-a716-446655440000"

	client := &incidentFakePulseClient{
		listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
			// Return 5 incidents: 3 open, 2 resolved.
			return pulseapi.IncidentPage{
				Incidents: []pulseapi.Incident{
					{ID: "open-1", MonitorID: monitorUUID, Status: "open",
						StartedAt: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)},
					{ID: "open-2", MonitorID: monitorUUID, Status: "open",
						StartedAt: time.Date(2024, 1, 18, 0, 0, 0, 0, time.UTC)},
					{ID: "resolved-1", MonitorID: monitorUUID, Status: "resolved",
						StartedAt:  time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
						ResolvedAt: timePtr(time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC)),
					},
					{ID: "open-3", MonitorID: monitorUUID, Status: "open",
						StartedAt: time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)},
					{ID: "resolved-2", MonitorID: monitorUUID, Status: "resolved",
						StartedAt:  time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC),
						ResolvedAt: timePtr(time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC)),
					},
				},
				Page:       1,
				Limit:      100,
				Total:      5,
				TotalPages: 1,
			}, nil
		},
	}

	deps := Deps{Client: client, AccessMode: config.ReadOnly}

	// Page 1, limit 2 — should get first 2 open incidents.
	out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{
		Monitor:  monitorUUID,
		OpenOnly: boolPtr(true),
		Page:     1,
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 3 {
		t.Errorf("expected total 3 open incidents, got %d", out.Total)
	}
	if out.TotalPages != 2 {
		t.Errorf("expected total_pages 2, got %d", out.TotalPages)
	}
	if !out.HasNextPage {
		t.Error("expected has_next_page=true for page 1 of 2")
	}
	if len(out.Incidents) != 2 {
		t.Fatalf("expected 2 incidents on page 1, got %d", len(out.Incidents))
	}
	if out.Incidents[0].ID != "open-1" || out.Incidents[1].ID != "open-2" {
		t.Errorf("unexpected incident IDs: %q, %q", out.Incidents[0].ID, out.Incidents[1].ID)
	}

	// Page 2, limit 2 — should get the last open incident.
	out, err = HandleListIncidents(context.Background(), deps, ListIncidentsInput{
		Monitor:  monitorUUID,
		OpenOnly: boolPtr(true),
		Page:     2,
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Incidents) != 1 {
		t.Fatalf("expected 1 incident on page 2, got %d", len(out.Incidents))
	}
	if out.Incidents[0].ID != "open-3" {
		t.Errorf("expected 'open-3', got %q", out.Incidents[0].ID)
	}
	if out.HasNextPage {
		t.Error("expected has_next_page=false for last page")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
