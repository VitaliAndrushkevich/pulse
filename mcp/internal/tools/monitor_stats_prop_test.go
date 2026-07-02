package tools

import (
	"context"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

// statsFakePulseClient is a fake that returns a pre-configured MonitorStats
// and supports resolve.Monitor by returning a single monitor with matching ID.
type statsFakePulseClient struct {
	stats pulseapi.MonitorStats
}

func (f *statsFakePulseClient) ListMonitors(_ context.Context, q pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	// Return a single monitor to support name resolution.
	return pulseapi.MonitorPage{
		Monitors: []pulseapi.Monitor{
			{ID: f.stats.MonitorID, Name: "test-monitor"},
		},
		Page:       1,
		Limit:      q.Limit,
		Total:      1,
		TotalPages: 1,
	}, nil
}

func (f *statsFakePulseClient) GetMonitor(_ context.Context, _ string) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{ID: f.stats.MonitorID, Name: "test-monitor"}, nil
}

func (f *statsFakePulseClient) GetMonitorStats(_ context.Context, _ string) (pulseapi.MonitorStats, error) {
	return f.stats, nil
}

func (f *statsFakePulseClient) GetMonitorHistory(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
	// Return a single "up" point so computeUptime7d produces 100%.
	return pulseapi.History{
		Points: []pulseapi.HistoryPoint{
			{State: "up", CheckedAt: time.Now().UTC()},
		},
	}, nil
}

func (f *statsFakePulseClient) ListIncidents(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	return pulseapi.IncidentPage{}, nil
}

func (f *statsFakePulseClient) CreateMonitor(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}

// --- Generators ---

// genSSLInfo generates a random SSLInfo with a valid expiration date and days remaining.
func genSSLInfo() *rapid.Generator[*pulseapi.SSLInfo] {
	return rapid.Custom(func(t *rapid.T) *pulseapi.SSLInfo {
		daysRemaining := rapid.IntRange(0, 3650).Draw(t, "daysRemaining")
		// Generate a future expiration time based on days remaining.
		expiresAt := time.Now().UTC().AddDate(0, 0, daysRemaining).Truncate(time.Second)
		return &pulseapi.SSLInfo{
			ExpiresAt:     expiresAt,
			DaysRemaining: daysRemaining,
		}
	})
}

// genMonitorStats generates a MonitorStats with optional SSL info.
func genMonitorStats(hasSSL bool) *rapid.Generator[pulseapi.MonitorStats] {
	return rapid.Custom(func(t *rapid.T) pulseapi.MonitorStats {
		stats := pulseapi.MonitorStats{
			MonitorID:       rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).Draw(t, "monitorID"),
			UptimePercent7d: rapid.Float64Range(0, 100).Draw(t, "uptimePercent7d"),
		}

		if hasSSL {
			stats.SSL = genSSLInfo().Draw(t, "ssl")
		}
		// When hasSSL is false, stats.SSL remains nil.

		return stats
	})
}

// --- Property 10: SSL info is included exactly when the monitor is TLS-based ---
// Validates: Requirements 5.3
//
// For any MonitorStats response: when SSL is non-nil, the output includes ssl
// with correct expires_at and days_remaining. When SSL is nil, the output omits
// the ssl field entirely.
func TestProperty10_SSLInfoIncludedExactlyWhenTLSBased(t *testing.T) {
	t.Run("ssl_present_when_tls", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate stats WITH SSL info (TLS-based monitor).
			stats := genMonitorStats(true).Draw(t, "stats")

			client := &statsFakePulseClient{stats: stats}
			deps := Deps{Client: client, AccessMode: config.ReadOnly}

			// Use the monitor UUID directly to skip name resolution.
			output, err := HandleMonitorStats(context.Background(), deps, MonitorStatsInput{
				Monitor: stats.MonitorID,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// SSL MUST be present in the output.
			if output.SSL == nil {
				t.Fatal("expected SSL to be present in output for TLS-based monitor, got nil")
			}

			// Verify expires_at is correctly formatted from the source time.
			expectedExpiresAt := stats.SSL.ExpiresAt.Format(time.RFC3339)
			if output.SSL.ExpiresAt != expectedExpiresAt {
				t.Fatalf("expected ssl.expires_at=%q, got %q", expectedExpiresAt, output.SSL.ExpiresAt)
			}

			// Verify days_remaining matches the source value.
			if output.SSL.DaysRemaining != stats.SSL.DaysRemaining {
				t.Fatalf("expected ssl.days_remaining=%d, got %d", stats.SSL.DaysRemaining, output.SSL.DaysRemaining)
			}
		})
	})

	t.Run("ssl_absent_when_not_tls", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate stats WITHOUT SSL info (non-TLS monitor).
			stats := genMonitorStats(false).Draw(t, "stats")

			client := &statsFakePulseClient{stats: stats}
			deps := Deps{Client: client, AccessMode: config.ReadOnly}

			// Use the monitor UUID directly to skip name resolution.
			output, err := HandleMonitorStats(context.Background(), deps, MonitorStatsInput{
				Monitor: stats.MonitorID,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// SSL MUST be absent in the output.
			if output.SSL != nil {
				t.Fatalf("expected SSL to be nil for non-TLS monitor, got %+v", output.SSL)
			}
		})
	})
}
