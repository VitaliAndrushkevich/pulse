package handlers

import (
	"context"
	"os"
	"testing"
	"time"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// All fixtures live in connection-local temporary tables inside a rolled-back
// transaction; this test never changes the instance's monitors or incident data.
func TestDashboardIncidentWindowDB(t *testing.T) {
	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 to run PostgreSQL interval tests")
	}
	ctx := context.Background()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE monitors(id uuid PRIMARY KEY,name text,status text) ON COMMIT DROP;
 CREATE TEMP TABLE incidents(id uuid PRIMARY KEY,monitor_id uuid,started_at timestamptz,resolved_at timestamptz,cause text) ON COMMIT DROP;`)
	if err != nil {
		t.Fatal(err)
	}
	monitorID := uuid.New().String()
	if _, err = tx.Exec(ctx, `INSERT INTO monitors VALUES ($1,'Paused API','paused')`, monitorID); err != nil {
		t.Fatal(err)
	}
	to := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	from := to.Add(-24 * time.Hour)
	expected := map[string]bool{}
	for _, tc := range []struct {
		name  string
		start time.Time
		end   *time.Time
		match bool
	}{
		{"resolved inside", from.Add(time.Hour), ptrDashboardTime(from.Add(2 * time.Hour)), true},
		{"started before window", from.Add(-time.Hour), ptrDashboardTime(from.Add(time.Hour)), true},
		{"ongoing before window", from.Add(-48 * time.Hour), nil, true},
		{"ends at start boundary", from.Add(-time.Hour), &from, false},
		{"starts at end boundary", to, nil, false},
		{"starts at start boundary", from, ptrDashboardTime(to.Add(time.Hour)), true},
		{"resolved before", from.Add(-3 * time.Hour), ptrDashboardTime(from.Add(-time.Hour)), false},
	} {
		id := uuid.New().String()
		if _, err = tx.Exec(ctx, `INSERT INTO incidents VALUES ($1,$2,$3,$4,$5)`, id, monitorID, tc.start, tc.end, tc.name); err != nil {
			t.Fatal(err)
		}
		if tc.match {
			expected[id] = true
		}
	}
	handler := NewDashboardHandlerWithDB(db.New(tx), tx)
	rows, total, err := handler.queryPeriodIncidents(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if total != len(expected) || len(rows) != len(expected) {
		t.Fatalf("got rows=%d total=%d want=%d", len(rows), total, len(expected))
	}
	if rows[0].ResolvedAt != nil {
		t.Fatal("ongoing incidents must be shown before recovered incidents")
	}
	for _, row := range rows {
		if !expected[row.ID] {
			t.Fatalf("unexpected incident %+v", row)
		}
		delete(expected, row.ID)
	}
	if len(expected) != 0 {
		t.Fatalf("missing incidents %v", expected)
	}
	for i := 0; i < 12; i++ {
		if _, err = tx.Exec(ctx, `INSERT INTO incidents VALUES ($1,$2,$3,NULL,'ongoing')`, uuid.New().String(), monitorID, from.Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err = handler.queryPeriodIncidents(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 10 || total != 16 {
		t.Fatalf("limit/count mismatch: len=%d total=%d", len(rows), total)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].StartedAt > rows[i-1].StartedAt {
			t.Fatal("incidents must be newest first")
		}
	}
}
func ptrDashboardTime(value time.Time) *time.Time { return &value }

func TestDashboardLatencyAndCertificatesDB(t *testing.T) {
	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 to run PostgreSQL dashboard tests")
	}
	ctx := context.Background()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE monitors(id uuid PRIMARY KEY,name text,status text) ON COMMIT DROP;
 CREATE TEMP TABLE check_results(monitor_id uuid,checked_at timestamptz,latency_ms integer,ssl_days_remaining integer) ON COMMIT DROP;`)
	if err != nil {
		t.Fatal(err)
	}
	slow, fast, paused := uuid.New().String(), uuid.New().String(), uuid.New().String()
	for _, row := range []struct{ id, name, status string }{{slow, "Slow", "active"}, {fast, "Fast", "active"}, {paused, "Paused", "paused"}} {
		if _, err = tx.Exec(ctx, `INSERT INTO monitors VALUES ($1,$2,$3)`, row.id, row.name, row.status); err != nil {
			t.Fatal(err)
		}
	}
	to := time.Now().UTC().Truncate(time.Second)
	from := to.Add(-24 * time.Hour)
	for _, row := range []struct {
		id            string
		at            time.Time
		latency, days int
	}{
		{slow, from.Add(-time.Hour), 9000, 2}, // Old expiring certificate was renewed.
		{slow, from.Add(time.Hour), 100, 365},
		{slow, to, 8000, 365}, // End boundary excluded from latency average.
		{fast, from.Add(2 * time.Hour), 50, 5},
		{paused, from.Add(3 * time.Hour), 10000, 1},
	} {
		if _, err = tx.Exec(ctx, `INSERT INTO check_results VALUES ($1,$2,$3,$4)`, row.id, row.at, row.latency, row.days); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewDashboardHandlerWithDB(db.New(tx), tx)
	monitors, err := handler.queryTopLatency(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 2 || monitors[0].MonitorID != slow || monitors[0].AvgLatencyMs != 100 || monitors[1].AvgLatencyMs != 50 {
		t.Fatalf("wrong latency period/ranking: %+v", monitors)
	}
	monitors, err = handler.queryTopLatency(ctx, to.Add(time.Hour), to.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 0 {
		t.Fatalf("empty period returned latency: %+v", monitors)
	}
	entries, checked, err := handler.querySSLExpiry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 2 || len(entries) != 1 || entries[0].MonitorID != fast {
		t.Fatalf("renewed or paused certificate leaked into warnings: checked=%d entries=%+v", checked, entries)
	}
	if _, err = tx.Exec(ctx, `UPDATE pg_temp.check_results SET ssl_days_remaining=365`); err != nil {
		t.Fatal(err)
	}
	entries, checked, err = handler.querySSLExpiry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 2 || len(entries) != 0 {
		t.Fatalf("expected observed certificates without warnings, got checked=%d entries=%+v", checked, entries)
	}
	if _, err = tx.Exec(ctx, `UPDATE pg_temp.check_results SET ssl_days_remaining=NULL`); err != nil {
		t.Fatal(err)
	}
	entries, checked, err = handler.querySSLExpiry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checked != 0 || len(entries) != 0 {
		t.Fatalf("expected no certificate observations, got checked=%d entries=%+v", checked, entries)
	}
}
