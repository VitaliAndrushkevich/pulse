package timescale_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testStore(t *testing.T) (*timescale.Store, *pgxpool.Pool, context.Context) {
	t.Helper()

	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 to run TimescaleDB integration tests")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)

	s := timescale.New(pool)
	if err := s.Ping(ctx); err != nil {
		t.Skipf("timescaledb unavailable: %v", err)
	}

	return s, pool, context.Background()
}

func TestWriteAndQueryHistory(t *testing.T) {
	s, pool, ctx := testStore(t)

	monitorID := uuid.New()
	_, err := createMonitorForCheckResults(ctx, pool, monitorID)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	latency := int32(42)
	statusCode := int32(200)
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	want := timescale.CheckPoint{
		MonitorID:  monitorID,
		State:      "up",
		LatencyMs:  &latency,
		StatusCode: &statusCode,
		CheckedAt:  checkedAt,
	}

	if err := s.WriteCheckResult(ctx, want); err != nil {
		t.Fatalf("WriteCheckResult() error: %v", err)
	}

	points, err := s.QueryHistory(ctx, monitorID, checkedAt.Add(-time.Second), checkedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryHistory() error: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("QueryHistory() returned no points; expected at least 1")
	}

	got := points[0]
	if got.MonitorID != want.MonitorID {
		t.Errorf("MonitorID = %s; want %s", got.MonitorID, want.MonitorID)
	}
	if got.State != want.State {
		t.Errorf("State = %q; want %q", got.State, want.State)
	}
	if got.LatencyMs == nil || *got.LatencyMs != *want.LatencyMs {
		t.Errorf("LatencyMs = %v; want %v", got.LatencyMs, want.LatencyMs)
	}
	if got.StatusCode == nil || *got.StatusCode != *want.StatusCode {
		t.Errorf("StatusCode = %v; want %v", got.StatusCode, want.StatusCode)
	}
}

func TestQueryHistory_EmptyRange(t *testing.T) {
	s, _, ctx := testStore(t)

	monitorID := uuid.New()
	points, err := s.QueryHistory(ctx, monitorID, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("QueryHistory() error: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("QueryHistory() returned %d points; want 0", len(points))
	}
}

func TestWriteAndQueryHeartbeatEvents(t *testing.T) {
	s, pool, ctx := testStore(t)

	hbID := createHeartbeatForEvents(t, ctx, pool, 30)

	base := time.Now().UTC().Truncate(time.Millisecond)
	up := "up"
	down := "down"

	// Three in-window events (ascending), plus one out-of-window (before start).
	writes := []timescale.HeartbeatEvent{
		{HeartbeatID: hbID, EventType: "success", PreviousState: &down, OccurredAt: base.Add(1 * time.Minute)},
		{HeartbeatID: hbID, EventType: "missed", PreviousState: &up, OccurredAt: base.Add(2 * time.Minute)},
		{HeartbeatID: hbID, EventType: "success", PreviousState: &up, OccurredAt: base.Add(3 * time.Minute)},
		{HeartbeatID: hbID, EventType: "fail", PreviousState: &up, OccurredAt: base.Add(-1 * time.Hour)},
	}
	for _, ev := range writes {
		if err := s.WriteHeartbeatEvent(ctx, ev); err != nil {
			t.Fatalf("WriteHeartbeatEvent: %v", err)
		}
	}

	// Query the [base, base+10m) window — the out-of-window fail must be excluded.
	start := base
	end := base.Add(10 * time.Minute)
	got, err := s.QueryHeartbeatEvents(ctx, hbID, start, end)
	if err != nil {
		t.Fatalf("QueryHeartbeatEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("QueryHeartbeatEvents returned %d events; want 3", len(got))
	}
	// Ascending by occurred_at.
	wantTypes := []string{"success", "missed", "success"}
	for i, ev := range got {
		if ev.EventType != wantTypes[i] {
			t.Errorf("event[%d].EventType = %q; want %q", i, ev.EventType, wantTypes[i])
		}
		if i > 0 && got[i].OccurredAt.Before(got[i-1].OccurredAt) {
			t.Errorf("events not ascending: [%d]=%s before [%d]=%s", i, got[i].OccurredAt, i-1, got[i-1].OccurredAt)
		}
	}
}

func TestCountHeartbeatEventsByType(t *testing.T) {
	s, pool, ctx := testStore(t)

	hbID := createHeartbeatForEvents(t, ctx, pool, 30)

	base := time.Now().UTC().Truncate(time.Millisecond)
	up := "up"
	events := []timescale.HeartbeatEvent{
		{HeartbeatID: hbID, EventType: "success", PreviousState: &up, OccurredAt: base.Add(1 * time.Minute)},
		{HeartbeatID: hbID, EventType: "success", PreviousState: &up, OccurredAt: base.Add(2 * time.Minute)},
		{HeartbeatID: hbID, EventType: "missed", PreviousState: &up, OccurredAt: base.Add(3 * time.Minute)},
		{HeartbeatID: hbID, EventType: "fail", PreviousState: &up, OccurredAt: base.Add(4 * time.Minute)},
	}
	for _, ev := range events {
		if err := s.WriteHeartbeatEvent(ctx, ev); err != nil {
			t.Fatalf("WriteHeartbeatEvent: %v", err)
		}
	}

	counts, err := s.CountHeartbeatEventsByType(ctx, hbID, base, base.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("CountHeartbeatEventsByType: %v", err)
	}
	if counts["success"] != 2 {
		t.Errorf("success count = %d; want 2", counts["success"])
	}
	if counts["missed"] != 1 {
		t.Errorf("missed count = %d; want 1", counts["missed"])
	}
	if counts["fail"] != 1 {
		t.Errorf("fail count = %d; want 1", counts["fail"])
	}
}

func TestHeartbeatEventRetentionPruning(t *testing.T) {
	s, pool, ctx := testStore(t)

	// Retention of 1 day: events older than 24h are expired.
	hbID := createHeartbeatForEvents(t, ctx, pool, 1)

	now := time.Now().UTC()
	up := "up"
	old := timescale.HeartbeatEvent{HeartbeatID: hbID, EventType: "success", PreviousState: &up, OccurredAt: now.Add(-48 * time.Hour)}
	recent := timescale.HeartbeatEvent{HeartbeatID: hbID, EventType: "success", PreviousState: &up, OccurredAt: now.Add(-1 * time.Hour)}
	if err := s.WriteHeartbeatEvent(ctx, old); err != nil {
		t.Fatalf("WriteHeartbeatEvent old: %v", err)
	}
	if err := s.WriteHeartbeatEvent(ctx, recent); err != nil {
		t.Fatalf("WriteHeartbeatEvent recent: %v", err)
	}

	// Prune using the same bounded DELETE the retention service issues.
	cutoff := now.Add(-24 * time.Hour)
	tag, err := pool.Exec(ctx,
		`DELETE FROM heartbeat_events
		 WHERE id IN (
		   SELECT id FROM heartbeat_events
		   WHERE heartbeat_id = $1 AND occurred_at < $2
		   LIMIT $3
		 )`,
		hbID, cutoff, 10000,
	)
	if err != nil {
		t.Fatalf("prune delete: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("prune deleted %d rows; want 1 (only the expired event)", tag.RowsAffected())
	}

	// The recent event must remain; the expired one must be gone.
	remaining, err := s.QueryHeartbeatEvents(ctx, hbID, now.Add(-72*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("QueryHeartbeatEvents: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining events = %d; want 1", len(remaining))
	}
	if remaining[0].OccurredAt.Before(cutoff) {
		t.Errorf("remaining event occurred_at = %s; expected a post-cutoff row", remaining[0].OccurredAt)
	}
}

// createHeartbeatForEvents inserts a minimal heartbeat row (required by the
// heartbeat_events FK) with the given retention and returns its id. The row is
// cleaned up (cascading its events) at test end.
func createHeartbeatForEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, retentionDays int32) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO heartbeats (id, name, expected_interval_seconds, grace_seconds, status, state, token_prefix, token_hash, history_retention_days)
		VALUES ($1, 'test-heartbeat', 60, 10, 'active', 'up', $2, $3, $4)
	`, id, id.String()[:8], id.String(), retentionDays)
	if err != nil {
		t.Fatalf("insert heartbeat: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM heartbeats WHERE id = $1`, id)
	})
	return id
}

// createMonitorForCheckResults inserts a minimal monitor row required by FK in check_results.
func createMonitorForCheckResults(ctx context.Context, pool *pgxpool.Pool, monitorID uuid.UUID) (int64, error) {
	cmdTag, err := pool.Exec(ctx, `
		INSERT INTO monitors (id, name, type, target, interval_seconds, timeout_seconds, status, state, settings)
		VALUES ($1, 'test-monitor', 'http', 'https://example.com', 60, 10, 'active', 'unknown', '{}'::jsonb)
	`, monitorID)
	if err != nil {
		// The monitor may already exist in re-runs; treat unique violation as success.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, nil
		}
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}
