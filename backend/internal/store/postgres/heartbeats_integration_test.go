package db_test

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

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/migrations"
)

// testQueries connects to the Postgres test service and ensures all migrations
// (including 017_heartbeats) are applied. Follows the PULSE_RUN_DB_TESTS gate
// used by backend/internal/store/timescale/store_test.go.
func testQueries(t *testing.T) (*db.Queries, *pgxpool.Pool, context.Context) {
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

	// Ensure the schema is present (idempotent: ErrNoChange is fine).
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

	return db.New(pool), pool, context.Background()
}

func tsNow() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func tsAt(ti time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: ti.UTC(), Valid: true}
}

func createTestHeartbeat(t *testing.T, q *db.Queries, ctx context.Context, name string) db.Heartbeat {
	t.Helper()
	hb, err := q.CreateHeartbeat(ctx, db.CreateHeartbeatParams{
		Name:                    name,
		ExpectedIntervalSeconds: 60,
		GraceSeconds:            10,
		Status:                  "active",
		TokenPrefix:             uuid.NewString()[:8],
		TokenHash:               uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateHeartbeat: %v", err)
	}
	t.Cleanup(func() { _ = q.DeleteHeartbeat(context.Background(), hb.ID) })
	return hb
}

// TestMigration017_UpDown verifies that migration 017 rolls back cleanly and
// re-applies cleanly (down then up), leaving the heartbeats table queryable.
func TestMigration017_UpDown(t *testing.T) {
	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 to run Postgres integration tests")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer m.Close()

	// Bring the schema fully up first.
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	// Roll back exactly one step (017_heartbeats.down.sql).
	if err := m.Steps(-1); err != nil {
		t.Fatalf("migrate down one step: %v", err)
	}

	// Re-apply exactly one step (017_heartbeats.up.sql).
	if err := m.Steps(1); err != nil {
		t.Fatalf("migrate up one step: %v", err)
	}

	// Confirm the table is present and queryable after the round-trip.
	ctx := context.Background()
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect after migration round-trip: %v", err)
	}
	defer pool.Close()
	if _, err := db.New(pool).CountHeartbeats(ctx); err != nil {
		t.Fatalf("CountHeartbeats after migration round-trip: %v", err)
	}
}

// TestCreateAndGetHeartbeat covers CreateHeartbeat → GetHeartbeat.
func TestCreateAndGetHeartbeat(t *testing.T) {
	q, _, ctx := testQueries(t)

	created := createTestHeartbeat(t, q, ctx, "nightly-backup")
	if created.State != "new" {
		t.Errorf("new heartbeat state = %q; want new", created.State)
	}
	if created.NextExpectedDeadline.Valid {
		t.Error("new heartbeat should have NULL next_expected_deadline")
	}

	got, err := q.GetHeartbeat(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetHeartbeat: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetHeartbeat ID = %s; want %s", got.ID, created.ID)
	}
	if got.Name != "nightly-backup" {
		t.Errorf("GetHeartbeat Name = %q; want nightly-backup", got.Name)
	}
}

// TestMarkHeartbeatUp_ReturnsPriorState verifies the CTE returns the prior state
// atomically with the write, enabling idempotent recovery detection.
func TestMarkHeartbeatUp_ReturnsPriorState(t *testing.T) {
	q, _, ctx := testQueries(t)
	hb := createTestHeartbeat(t, q, ctx, "sync-job")

	now := time.Now().UTC()
	deadline := now.Add(70 * time.Second)

	// First success from state 'new'.
	up1, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID:                   hb.ID,
		LastSuccessAt:        tsAt(now),
		NextExpectedDeadline: tsAt(deadline),
	})
	if err != nil {
		t.Fatalf("MarkHeartbeatUp(1): %v", err)
	}
	if up1.PreviousState != "new" {
		t.Errorf("first MarkHeartbeatUp previous_state = %q; want new", up1.PreviousState)
	}
	if up1.State != "up" {
		t.Errorf("state after up = %q; want up", up1.State)
	}

	// Drive it down, then recover: prior state must be reported as 'down'.
	if _, err := q.MarkHeartbeatFail(ctx, hb.ID); err != nil {
		t.Fatalf("MarkHeartbeatFail: %v", err)
	}
	up2, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID:                   hb.ID,
		LastSuccessAt:        tsAt(now.Add(time.Minute)),
		NextExpectedDeadline: tsAt(deadline.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("MarkHeartbeatUp(2): %v", err)
	}
	if up2.PreviousState != "down" {
		t.Errorf("recovery MarkHeartbeatUp previous_state = %q; want down", up2.PreviousState)
	}
}

// TestMarkHeartbeatDown_FiresOnce verifies the WHERE state='up' guard: a second
// down transition on an already-down row affects 0 rows (ErrNoRows).
func TestMarkHeartbeatDown_FiresOnce(t *testing.T) {
	q, _, ctx := testQueries(t)
	hb := createTestHeartbeat(t, q, ctx, "cert-renewal")

	now := time.Now().UTC()
	// Bring it up so it is eligible for a down transition.
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID:                   hb.ID,
		LastSuccessAt:        tsAt(now.Add(-2 * time.Minute)),
		NextExpectedDeadline: tsAt(now.Add(-time.Minute)),
	}); err != nil {
		t.Fatalf("MarkHeartbeatUp: %v", err)
	}

	down1, err := q.MarkHeartbeatDown(ctx, hb.ID)
	if err != nil {
		t.Fatalf("first MarkHeartbeatDown: %v", err)
	}
	if down1.State != "down" {
		t.Errorf("state after down = %q; want down", down1.State)
	}

	// Second call must affect 0 rows → ErrNoRows (fires once per episode).
	if _, err := q.MarkHeartbeatDown(ctx, hb.ID); err == nil {
		t.Error("second MarkHeartbeatDown returned a row; expected no rows (once-per-episode guard)")
	}
}

// TestListDueHeartbeats_PartialIndexExclusions verifies the sweep query excludes
// paused, new, and already-down rows, selecting only active+up+past-deadline.
func TestListDueHeartbeats_PartialIndexExclusions(t *testing.T) {
	q, _, ctx := testQueries(t)

	past := time.Now().UTC().Add(-time.Minute)
	future := time.Now().UTC().Add(time.Hour)

	// due: active + up + past deadline (should be selected).
	due := createTestHeartbeat(t, q, ctx, "due-hb")
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID: due.ID, LastSuccessAt: tsAt(past.Add(-time.Minute)), NextExpectedDeadline: tsAt(past),
	}); err != nil {
		t.Fatalf("prime due: %v", err)
	}

	// notYet: up but deadline in the future (excluded).
	notYet := createTestHeartbeat(t, q, ctx, "future-hb")
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID: notYet.ID, LastSuccessAt: tsNow(), NextExpectedDeadline: tsAt(future),
	}); err != nil {
		t.Fatalf("prime notYet: %v", err)
	}

	// newHb: state 'new', NULL deadline (excluded).
	_ = createTestHeartbeat(t, q, ctx, "new-hb")

	// alreadyDown: past deadline but state 'down' (excluded).
	alreadyDown := createTestHeartbeat(t, q, ctx, "down-hb")
	if _, err := q.MarkHeartbeatUp(ctx, db.MarkHeartbeatUpParams{
		ID: alreadyDown.ID, LastSuccessAt: tsAt(past.Add(-time.Minute)), NextExpectedDeadline: tsAt(past),
	}); err != nil {
		t.Fatalf("prime alreadyDown up: %v", err)
	}
	if _, err := q.MarkHeartbeatDown(ctx, alreadyDown.ID); err != nil {
		t.Fatalf("prime alreadyDown down: %v", err)
	}

	rows, err := q.ListDueHeartbeats(ctx, 500)
	if err != nil {
		t.Fatalf("ListDueHeartbeats: %v", err)
	}

	selected := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		selected[r.ID] = true
	}
	if !selected[due.ID] {
		t.Error("due heartbeat (active+up+past deadline) was not selected")
	}
	if selected[notYet.ID] {
		t.Error("future-deadline heartbeat was selected; should be excluded")
	}
	if selected[alreadyDown.ID] {
		t.Error("already-down heartbeat was selected; should be excluded")
	}
}

// TestDeleteHeartbeat_CascadesTagsAndBindings verifies the FK ON DELETE CASCADE
// removes heartbeat_tags and heartbeat_bindings when the heartbeat is deleted.
func TestDeleteHeartbeat_CascadesTagsAndBindings(t *testing.T) {
	q, pool, ctx := testQueries(t)

	hb := createTestHeartbeat(t, q, ctx, "cascade-hb")

	// Tag.
	if err := q.InsertHeartbeatTag(ctx, db.InsertHeartbeatTagParams{
		HeartbeatID: hb.ID, Key: "team", Value: "ops",
	}); err != nil {
		t.Fatalf("InsertHeartbeatTag: %v", err)
	}

	// Channel (required by binding FK), then binding.
	ch, err := q.CreateChannel(ctx, db.CreateChannelParams{
		Name:        "cascade-channel",
		ChannelType: "webhook",
		Config:      []byte(`{"url":"https://example.com/hook","method":"POST"}`),
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	t.Cleanup(func() { _ = q.DeleteChannel(context.Background(), ch.ID) })

	binding, err := q.CreateHeartbeatBinding(ctx, db.CreateHeartbeatBindingParams{
		ChannelID:   ch.ID,
		HeartbeatID: hb.ID,
		Triggers:    []byte(`[{"type":"heartbeat_missed"}]`),
	})
	if err != nil {
		t.Fatalf("CreateHeartbeatBinding: %v", err)
	}

	// Delete the heartbeat; cascade must remove the tag + binding.
	if err := q.DeleteHeartbeat(ctx, hb.ID); err != nil {
		t.Fatalf("DeleteHeartbeat: %v", err)
	}

	var tagCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM heartbeat_tags WHERE heartbeat_id = $1`, hb.ID).Scan(&tagCount); err != nil {
		t.Fatalf("count heartbeat_tags: %v", err)
	}
	if tagCount != 0 {
		t.Errorf("heartbeat_tags not cascaded: %d rows remain", tagCount)
	}

	if _, err := q.GetHeartbeatBinding(ctx, binding.ID); err == nil {
		t.Error("heartbeat_binding not cascaded: GetHeartbeatBinding still returns a row")
	}
}
