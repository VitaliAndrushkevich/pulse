package heartbeat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VitaliAndrushkevich/pulse/internal/heartbeat"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/migrations"
)

// testQueries connects to the Postgres test service and ensures all migrations
// (including 017_heartbeats) are applied. Gated on PULSE_RUN_DB_TESTS, mirroring
// backend/internal/store/postgres/heartbeats_integration_test.go.
func testQueries(t *testing.T) (*db.Queries, context.Context) {
	t.Helper()

	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 to run Postgres integration tests")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() { _, _ = m.Close() })

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)

	// The evaluator sweeps the WHOLE heartbeats table and transitions due rows to
	// down. If it ran concurrently with other heartbeat DB tests (e.g. the Phase 1
	// store tests that assert on rows they created), it would mutate their data.
	// So these evaluator tests take the heartbeat schema advisory lock in
	// EXCLUSIVE mode — the same lock store data tests take in shared mode and the
	// migration round-trip test takes exclusively — serializing the sweeper
	// against every other heartbeat-schema DB test under CI's `go test -race ./...`.
	acquireExclusiveHeartbeatSchemaLock(t, pool)

	return db.New(pool), context.Background()
}

// heartbeatSchemaLockKey mirrors the key in the store/postgres integration test;
// both must match for the shared/exclusive advisory lock to coordinate.
const heartbeatSchemaLockKey int64 = 0x48425F3031375F31 // "HB_017_1"

// acquireExclusiveHeartbeatSchemaLock takes the heartbeat schema advisory lock in
// exclusive mode on a dedicated connection held open for the whole test, so no
// other heartbeat-schema DB test runs while the sweeping evaluator is active.
func acquireExclusiveHeartbeatSchemaLock(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire schema-lock conn: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", heartbeatSchemaLockKey); err != nil {
		conn.Release()
		t.Fatalf("acquire exclusive heartbeat schema lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", heartbeatSchemaLockKey)
		conn.Release()
	})
}

func tsAt(ti time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: ti.UTC(), Valid: true}
}

func createHeartbeat(t *testing.T, q *db.Queries, ctx context.Context, name, status string) db.Heartbeat {
	t.Helper()
	hb, err := q.CreateHeartbeat(ctx, db.CreateHeartbeatParams{
		Name:                    name,
		ExpectedIntervalSeconds: 60,
		GraceSeconds:            10,
		Status:                  status,
		TokenPrefix:             uuid.NewString()[:8],
		TokenHash:               uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateHeartbeat(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = q.DeleteHeartbeat(context.Background(), hb.ID) })
	return hb
}

func getState(t *testing.T, q *db.Queries, ctx context.Context, id uuid.UUID) string {
	t.Helper()
	hb, err := q.GetHeartbeat(ctx, id)
	if err != nil {
		t.Fatalf("GetHeartbeat: %v", err)
	}
	return hb.State
}

// Verify the generated *db.Queries satisfies the evaluator's narrowed interface.
var _ heartbeat.Queries = (*db.Queries)(nil)

// TestEvaluator_MarksDueDown_Once drives the evaluator with a short tick against
// the real DB. A past-deadline active+up heartbeat must be marked down, and the
// transition must fire exactly once — subsequent ticks must not re-select or
// re-transition the now-down row (the MarkHeartbeatDown WHERE state='up' guard).
func TestEvaluator_MarksDueDown_Once(t *testing.T) {
	q, ctx := testQueries(t)

	// Due: active + up + deadline already passed.
	due := createHeartbeat(t, q, ctx, "evaluator-due", "active")
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID:                   due.ID,
		LastSuccessAt:        tsAt(past.Add(-time.Minute)),
		NextExpectedDeadline: tsAt(past),
	}); err != nil {
		t.Fatalf("prime due up: %v", err)
	}

	// A down-episode counter proves "exactly once": DispatchMissedRun is only
	// reached on a real up → down update.
	rec := &recordingDispatcher{}
	metrics := heartbeat.NewMetrics(newTestRegistry())

	ev := heartbeat.NewEvaluator(heartbeat.Config{
		Workers:      2,
		TickInterval: 20 * time.Millisecond,
		BatchSize:    100,
	}, q, metrics, nil, rec)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { ev.Run(runCtx); close(done) }()

	// Let several ticks elapse so a buggy re-fire would be observed.
	waitFor(t, 2*time.Second, func() bool {
		return getState(t, q, ctx, due.ID) == "down"
	})
	time.Sleep(200 * time.Millisecond) // extra ticks: a re-fire would bump the counter

	cancel()
	<-done

	if got := getState(t, q, ctx, due.ID); got != "down" {
		t.Errorf("due heartbeat state = %q; want down", got)
	}
	if n := rec.countFor(due.ID); n != 1 {
		t.Errorf("DispatchMissedRun fired %d times for the due heartbeat; want exactly 1", n)
	}
	for _, prev := range rec.prevStatesFor(due.ID) {
		if prev != "up" {
			t.Errorf("missed-run previousState = %q; want up", prev)
		}
	}
}

// TestEvaluator_SkipsNewAndPaused confirms the sweep never selects never-pinged
// (state 'new', NULL deadline) or paused heartbeats, even with a past deadline.
func TestEvaluator_SkipsNewAndPaused(t *testing.T) {
	q, ctx := testQueries(t)

	// new: created, never pinged → NULL deadline, excluded.
	newHb := createHeartbeat(t, q, ctx, "evaluator-new", "active")

	// paused: up with a past deadline but status 'paused' → excluded.
	pausedHb := createHeartbeat(t, q, ctx, "evaluator-paused", "paused")
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID:                   pausedHb.ID,
		LastSuccessAt:        tsAt(past.Add(-time.Minute)),
		NextExpectedDeadline: tsAt(past),
	}); err != nil {
		t.Fatalf("prime paused up: %v", err)
	}

	rec := &recordingDispatcher{}
	ev := heartbeat.NewEvaluator(heartbeat.Config{
		Workers:      2,
		TickInterval: 20 * time.Millisecond,
		BatchSize:    100,
	}, q, heartbeat.NewMetrics(newTestRegistry()), nil, rec)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { ev.Run(runCtx); close(done) }()

	time.Sleep(500 * time.Millisecond) // allow many ticks
	cancel()
	<-done

	if got := getState(t, q, ctx, newHb.ID); got != "new" {
		t.Errorf("new heartbeat state = %q; want new (never selected)", got)
	}
	if got := getState(t, q, ctx, pausedHb.ID); got != "up" {
		t.Errorf("paused heartbeat state = %q; want up (never selected)", got)
	}
	// Scope assertions to this test's own rows: the evaluator sweeps the whole
	// table and other packages/tests may share the DB in CI (go test ./...).
	if n := rec.countFor(newHb.ID); n != 0 {
		t.Errorf("DispatchMissedRun fired %d times for the new heartbeat; want 0", n)
	}
	if n := rec.countFor(pausedHb.ID); n != 0 {
		t.Errorf("DispatchMissedRun fired %d times for the paused heartbeat; want 0", n)
	}
}

// TestEvaluator_ShutdownDrainsNoLeak verifies that cancelling the context stops
// Run cleanly and leaves no evaluator goroutines behind.
func TestEvaluator_ShutdownDrainsNoLeak(t *testing.T) {
	q, ctx := testQueries(t)

	// Snapshot evaluator goroutines before start. We count by name rather than
	// using runtime.NumGoroutine() because go test ./... runs packages in
	// parallel against one DB, so the absolute count is noisy; the evaluator's
	// own stack frames are a precise, isolation-safe signal.
	before := countEvaluatorGoroutines()

	ev := heartbeat.NewEvaluator(heartbeat.Config{
		Workers:      8,
		TickInterval: 10 * time.Millisecond,
		BatchSize:    100,
	}, q, heartbeat.NewMetrics(newTestRegistry()), nil, &recordingDispatcher{})

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { ev.Run(runCtx); close(done) }()

	time.Sleep(100 * time.Millisecond) // let workers spin up and poll
	cancel()

	// Run returns only after close(jobs) + wg.Wait(), i.e. every worker drained.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("evaluator did not shut down within 3s after context cancel")
	}

	// After a clean shutdown no evaluator goroutine should remain.
	waitFor(t, time.Second, func() bool {
		return countEvaluatorGoroutines() <= before
	})
	if after := countEvaluatorGoroutines(); after > before {
		t.Errorf("goroutine leak: evaluator goroutines before=%d after=%d (worker pool not drained)", before, after)
	}
}

// waitFor polls cond until it is true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}
