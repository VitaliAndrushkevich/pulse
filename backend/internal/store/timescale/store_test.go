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
