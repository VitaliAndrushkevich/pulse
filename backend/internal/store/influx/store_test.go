package influx_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/store/influx"
)

// testStore creates a Store for integration tests.
// Tests are skipped when INFLUXDB_URL is not set.
func testStore(t *testing.T) *influx.Store {
	t.Helper()

	// Prefer explicit URL if provided, otherwise use Docker Compose service defaults.
	// This lets tests run inside the compose network without extra env configuration.
	if os.Getenv("INFLUXDB_URL") == "" {
		os.Setenv("INFLUXDB_URL", "http://influxdb:8086")
	}
	if os.Getenv("INFLUXDB_ORG") == "" {
		os.Setenv("INFLUXDB_ORG", "pulse")
	}
	if os.Getenv("INFLUXDB_BUCKET") == "" {
		os.Setenv("INFLUXDB_BUCKET", "pulse")
	}

	s := influx.NewFromEnv()
	t.Cleanup(s.Close)
	return s
}

func TestPing(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestWriteAndQueryHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	monitorID := "test-monitor-" + time.Now().Format("20060102150405")
	checkTime := time.Now().UTC().Truncate(time.Millisecond)

	want := influx.CheckPoint{
		MonitorID:   monitorID,
		MonitorType: "http",
		State:       "up",
		LatencyMs:   42.5,
		StatusCode:  200,
		CheckedAt:   checkTime,
	}

	if err := s.WriteCheckResult(ctx, want); err != nil {
		t.Fatalf("WriteCheckResult() error: %v", err)
	}

	// Allow InfluxDB a moment to make the point queryable.
	time.Sleep(1 * time.Second)

	start := checkTime.Add(-time.Second)
	end := checkTime.Add(time.Minute)

	points, err := s.QueryHistory(ctx, monitorID, start, end)
	if err != nil {
		t.Fatalf("QueryHistory() error: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("QueryHistory() returned no points; expected at least 1")
	}

	got := points[0]
	if got.MonitorID != want.MonitorID {
		t.Errorf("MonitorID = %q; want %q", got.MonitorID, want.MonitorID)
	}
	if got.MonitorType != want.MonitorType {
		t.Errorf("MonitorType = %q; want %q", got.MonitorType, want.MonitorType)
	}
	if got.State != want.State {
		t.Errorf("State = %q; want %q", got.State, want.State)
	}
	if got.LatencyMs != want.LatencyMs {
		t.Errorf("LatencyMs = %v; want %v", got.LatencyMs, want.LatencyMs)
	}
	if got.StatusCode != want.StatusCode {
		t.Errorf("StatusCode = %v; want %v", got.StatusCode, want.StatusCode)
	}
}

func TestQueryHistory_EmptyRange(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	monitorID := "nonexistent-monitor-" + time.Now().Format("20060102150405")
	start := time.Now().Add(-time.Hour)
	end := time.Now()

	points, err := s.QueryHistory(ctx, monitorID, start, end)
	if err != nil {
		t.Fatalf("QueryHistory() error: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("QueryHistory() returned %d points; want 0", len(points))
	}
}
