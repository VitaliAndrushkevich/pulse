package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

func TestDashboardSummaryPreservesHealthMetadata(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sampled := rapid.IntRange(0, 500).Draw(t, "sampled")
		partial := rapid.Bool().Draw(t, "partial")
		percent := rapid.Float64Range(0, 100).Draw(t, "uptime")
		recovered := rapid.Bool().Draw(t, "recovered")
		total := rapid.IntRange(1, 500).Draw(t, "incident_count")
		checked := rapid.IntRange(0, 500).Draw(t, "ssl_checked")
		latency := rapid.IntRange(0, 60000).Draw(t, "latency")
		var resolved *string
		start := "2026-10-23T10:00:00Z"
		end := "2026-10-23T11:00:00Z"
		if recovered {
			resolved = &end
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("period") != "24h" {
				t.Error("MCP must explicitly request its documented 24h default")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ssl_checked_monitor_count": checked,
				"top_latency_monitors":      []map[string]any{{"monitor_id": "monitor-1", "monitor_name": "API", "avg_latency_ms": latency}},
				"incidents":                 []map[string]any{{"id": "incident-1", "monitor_id": "monitor-1", "monitor_name": "API", "started_at": start, "resolved_at": resolved, "cause": "Timeout"}},
				"incident_count":            total, "period_from": "2026-10-23T00:00:00Z", "period_to": "2026-10-24T00:00:00Z",
				"health_score": map[string]any{"uptime_percent": percent, "active_monitor_count": 500, "sampled_monitor_count": sampled, "period": "24h", "partial_data": partial},
				"partial_data": partial, "widget_errors": map[string]string{"ssl-expiry": "DASHBOARD_QUERY_FAILED"},
			})
		}))
		defer srv.Close()
		out, err := HandleDashboardSummary(context.Background(), Deps{Client: pulseapi.NewHTTPClient(srv.URL, "test-token", time.Second)}, DashboardSummaryInput{})
		if err != nil {
			t.Fatal(err)
		}
		if out.SSLCheckedMonitorCount != checked || len(out.TopLatencyMonitors) != 1 || out.TopLatencyMonitors[0].AvgLatencyMs != latency {
			t.Fatalf("latency/certificate coverage lost: %+v", out)
		}
		if len(out.Incidents) != 1 || out.IncidentCount != total || out.PeriodFrom != "2026-10-23T00:00:00Z" || out.PeriodTo != "2026-10-24T00:00:00Z" {
			t.Fatalf("incident envelope lost: %+v", out)
		}
		incident := out.Incidents[0]
		if incident.ID != "incident-1" || incident.MonitorID != "monitor-1" || incident.StartedAt != start || (incident.ResolvedAt != nil) != recovered || (recovered && *incident.ResolvedAt != end) {
			t.Fatalf("incident boundaries lost: %+v", incident)
		}
		if out.HealthScore.UptimePercent != percent || out.HealthScore.SampledMonitorCount != sampled || out.HealthScore.PartialData != partial || out.HealthScore.Period != "24h" || out.PartialData != partial || out.WidgetErrors["ssl-expiry"] == "" {
			t.Fatalf("metadata lost: %+v", out)
		}
	})
}
