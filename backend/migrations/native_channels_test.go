//go:build integration

package migrations

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Run with PULSE_RUN_DB_TESTS=1 and DATABASE_URL pointing to disposable Postgres.
// The temporary table shadows the real table; no existing resources are changed.
func TestNativeChannelsMigrationRoundTrip(t *testing.T) {
	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 with disposable PostgreSQL")
	}
	conn, err := pgx.Connect(t.Context(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	_, err = conn.Exec(t.Context(), `CREATE TEMP TABLE notification_channels (channel_type VARCHAR(20) NOT NULL CHECK (channel_type IN ('email','webhook')))`) // no application tables touched
	if err != nil {
		t.Fatal(err)
	}
	up, err := FS.ReadFile("021_native_notification_channels.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := FS.ReadFile("021_native_notification_channels.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), string(up)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"email", "webhook", "slack", "matrix", "telegram"} {
		if _, err := conn.Exec(t.Context(), `INSERT INTO notification_channels VALUES ($1)`, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(t.Context(), string(down)); err == nil {
		t.Fatal("downgrade accepted native channels")
	}
	if _, err := conn.Exec(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO notification_channels VALUES ('slack')`); err != nil {
		t.Fatal("failed downgrade removed original constraint")
	}
	if _, err := conn.Exec(t.Context(), `DELETE FROM notification_channels WHERE channel_type NOT IN ('email','webhook')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO notification_channels VALUES ('slack')`); err == nil {
		t.Fatal("downgrade still accepts native channels")
	}
}
