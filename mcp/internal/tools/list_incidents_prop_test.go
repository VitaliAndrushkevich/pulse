package tools

import (
	"context"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// genIncident generates a random Incident with consistent status/resolved_at semantics.
func genIncident(t *rapid.T, monitorID string) pulseapi.Incident {
	id := rapid.StringMatching(`inc-[a-z0-9]{4,8}`).Draw(t, "incidentID")
	status := rapid.SampledFrom([]string{"open", "resolved"}).Draw(t, "status")
	startedAt := time.Unix(rapid.Int64Range(1_000_000_000, 2_000_000_000).Draw(t, "startedAt"), 0).UTC()

	var resolvedAt *time.Time
	if status == "resolved" {
		// resolved_at is always after started_at
		offset := rapid.Int64Range(60, 86400).Draw(t, "resolveOffset")
		ra := startedAt.Add(time.Duration(offset) * time.Second)
		resolvedAt = &ra
	}

	return pulseapi.Incident{
		ID:         id,
		MonitorID:  monitorID,
		Status:     status,
		StartedAt:  startedAt,
		ResolvedAt: resolvedAt,
	}
}

// Feature: mcp-server, Property 17: Incidents are ordered by start time descending
//
// For any set of incidents from Pulse (possibly unordered), the output is always
// ordered by started_at descending (newest first).
//
// **Validates: Requirements 8.1**
func TestProperty17_IncidentsOrderedByStartTimeDescending(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 30).Draw(t, "numIncidents")
		monitorID := "mon-fixed"

		incidents := make([]pulseapi.Incident, n)
		for i := range incidents {
			incidents[i] = genIncident(t, monitorID)
		}

		client := &incidentFakePulseClient{
			listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
				return pulseapi.IncidentPage{
					Incidents:  incidents,
					Page:       q.Page,
					Limit:      q.Limit,
					Total:      n,
					TotalPages: 1,
				}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{
			Page:  1,
			Limit: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify descending order by started_at.
		for i := 1; i < len(out.Incidents); i++ {
			prev, err1 := time.Parse(time.RFC3339, out.Incidents[i-1].StartedAt)
			curr, err2 := time.Parse(time.RFC3339, out.Incidents[i].StartedAt)
			if err1 != nil || err2 != nil {
				t.Fatalf("failed to parse started_at timestamps: %v, %v", err1, err2)
			}
			if prev.Before(curr) {
				t.Fatalf("incidents not in descending order: index %d (%s) < index %d (%s)",
					i-1, out.Incidents[i-1].StartedAt, i, out.Incidents[i].StartedAt)
			}
		}
	})
}

// Feature: mcp-server, Property 18: Incident filters are applied conjunctively
//
// When both monitor and open_only filters are provided, only incidents that are
// BOTH from the specified monitor AND have status "open" appear in the output.
//
// **Validates: Requirements 8.2, 8.3**
func TestProperty18_IncidentFiltersAppliedConjunctively(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		targetMonitor := "550e8400-e29b-41d4-a716-446655440000"

		// Generate a mix of incidents for the target monitor: some open, some resolved.
		// The handler fetches per-monitor incidents, so the API returns only incidents
		// for that monitor. Client-side filtering then keeps only open ones.
		n := rapid.IntRange(1, 20).Draw(t, "numIncidents")
		incidents := make([]pulseapi.Incident, n)
		for i := range incidents {
			incidents[i] = genIncident(t, targetMonitor)
		}

		client := &incidentFakePulseClient{
			listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
				// Simulate Pulse per-monitor endpoint: return all incidents for this monitor.
				return pulseapi.IncidentPage{
					Incidents:  incidents,
					Page:       1,
					Limit:      100,
					Total:      len(incidents),
					TotalPages: 1,
				}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{
			Monitor:  targetMonitor,
			OpenOnly: boolPtr(true),
			Page:     1,
			Limit:    100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Every returned incident must be open AND belong to the target monitor.
		for i, inc := range out.Incidents {
			if inc.Status != "open" {
				t.Fatalf("incident[%d] %q has status %q, want 'open'", i, inc.ID, inc.Status)
			}
			if inc.MonitorID != targetMonitor {
				t.Fatalf("incident[%d] %q has monitor %q, want %q", i, inc.ID, inc.MonitorID, targetMonitor)
			}
		}

		// Count expected: only open incidents should survive the conjunctive filter.
		expectedCount := 0
		for _, inc := range incidents {
			if inc.Status == "open" {
				expectedCount++
			}
		}
		if out.Total != expectedCount {
			t.Fatalf("total=%d, want %d (count of open incidents for target monitor)", out.Total, expectedCount)
		}
	})
}

// Feature: mcp-server, Property 19: resolved_at present exactly when resolved
//
// For any incident: if status is "open", resolved_at is absent (nil);
// if status is "resolved", resolved_at is present.
//
// **Validates: Requirements 8.6**
func TestProperty19_ResolvedAtPresentExactlyWhenResolved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 30).Draw(t, "numIncidents")
		monitorID := "mon-fixed"

		incidents := make([]pulseapi.Incident, n)
		for i := range incidents {
			incidents[i] = genIncident(t, monitorID)
		}

		client := &incidentFakePulseClient{
			listIncidentsFunc: func(_ context.Context, q pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
				return pulseapi.IncidentPage{
					Incidents:  incidents,
					Page:       q.Page,
					Limit:      q.Limit,
					Total:      n,
					TotalPages: 1,
				}, nil
			},
		}

		deps := Deps{Client: client, AccessMode: config.ReadOnly}
		out, err := HandleListIncidents(context.Background(), deps, ListIncidentsInput{
			Page:  1,
			Limit: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i, inc := range out.Incidents {
			switch inc.Status {
			case "open":
				if inc.ResolvedAt != nil {
					t.Fatalf("incident[%d] %q is open but has resolved_at=%q",
						i, inc.ID, *inc.ResolvedAt)
				}
			case "resolved":
				if inc.ResolvedAt == nil {
					t.Fatalf("incident[%d] %q is resolved but has no resolved_at", i, inc.ID)
				}
			default:
				t.Fatalf("incident[%d] %q has unexpected status %q", i, inc.ID, inc.Status)
			}
		}
	})
}
