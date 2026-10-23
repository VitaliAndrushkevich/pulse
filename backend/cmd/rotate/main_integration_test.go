//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/VitaliAndrushkevich/pulse/internal/notification/native"
	"github.com/VitaliAndrushkevich/pulse/internal/notification/webhook"
	"github.com/VitaliAndrushkevich/pulse/migrations"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Every subtest creates and drops a unique database. DATABASE_URL is used only
// for server connection settings; its named database is never migrated or seeded.
func TestRotationCommandDatabaseIntegration(t *testing.T) {
	if os.Getenv("PULSE_RUN_DB_TESTS") != "1" {
		t.Skip("set PULSE_RUN_DB_TESTS=1 with disposable PostgreSQL access")
	}
	binary := filepath.Join(t.TempDir(), "pulse-rotate")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build rotation command: %v\n%s", err, output)
	}
	for _, tc := range []struct {
		name string
		fail bool
	}{{name: "all secret kinds and delivery"}, {name: "failure rolls back every prior write", fail: true}} {
		t.Run(tc.name, func(t *testing.T) {
			conn, databaseURL := rotationTestDatabase(t)
			oldKey, newKey := bytes.Repeat([]byte{11}, 32), bytes.Repeat([]byte{22}, 32)
			want := rotationSeedFixtures(t, conn, oldKey)
			if tc.fail {
				// SMTP is rotated last, after the other secret kinds were updated.
				if _, err := conn.Exec(t.Context(), `UPDATE smtp_settings SET password_enc = $1`, []byte("malformed-ciphertext")); err != nil {
					t.Fatal(err)
				}
			}
			before := rotationReadCiphertexts(t, conn)
			command := exec.CommandContext(t.Context(), binary)
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "DATABASE_URL=") && !strings.HasPrefix(value, "PULSE_SECRET_KEY=") && !strings.HasPrefix(value, "PULSE_SECRET_KEY_NEW=") {
					command.Env = append(command.Env, value)
				}
			}
			command.Env = append(command.Env, "DATABASE_URL="+databaseURL, "PULSE_SECRET_KEY="+base64.StdEncoding.EncodeToString(oldKey), "PULSE_SECRET_KEY_NEW="+base64.StdEncoding.EncodeToString(newKey))
			output, err := command.CombinedOutput()
			for _, plaintext := range want {
				if plaintext != "" && bytes.Contains(output, []byte(plaintext)) {
					t.Fatal("rotation output exposed a fixture secret")
				}
			}
			after := rotationReadCiphertexts(t, conn)
			if tc.fail {
				if err == nil {
					t.Fatal("rotation accepted malformed ciphertext")
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("failed rotation partially committed changes")
				}
				rotationAssertDecrypts(t, after, oldKey, want, "smtp")
				return
			}
			if err != nil {
				t.Fatalf("rotation failed: %v\n%s", err, output)
			}
			rotationAssertDecrypts(t, after, newKey, want, "")
			for kind, value := range after {
				if _, err := crypto.Decrypt(oldKey, value); err == nil {
					t.Fatalf("old key still decrypts %s", kind)
				}
			}
			// Exercise a real HTTP delivery using only persisted new-key ciphertext.
			received := make(chan string, 1)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer receiver.Close()
			cfg := webhook.WebhookConfig{URL: receiver.URL, Method: "POST", BodyTemplate: "test", Headers: []webhook.WebhookHeader{{Name: "Authorization", Value: base64.StdEncoding.EncodeToString(after["webhook"])}}}
			if err := webhook.NewClient(newKey).Deliver(t.Context(), cfg, notification.TemplateData{}); err != nil {
				t.Fatal(err)
			}
			if got := <-received; got != want["webhook"] {
				t.Fatal("new-key delivery sent the wrong credential")
			}
		})
	}
}

func rotationTestDatabase(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		base = "postgres://pulse:pulse@localhost:5432/postgres?sslmode=disable"
	}
	config, err := pgx.ParseConfig(base)
	if err != nil {
		t.Fatal("invalid PostgreSQL connection configuration")
	}
	config.Database = "postgres"
	admin, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	name := "pulse_notification_verify_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+quoted); err != nil {
		admin.Close(t.Context())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("clean disposable verification database: %v", err)
		}
		admin.Close(ctx)
	})
	parsedURL, err := url.Parse(base)
	if err != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") {
		t.Fatal("DATABASE_URL must be a PostgreSQL URL")
	}
	parsedURL.Path = "/" + name
	databaseURL := parsedURL.String()

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(); err != nil {
		migrator.Close()
		t.Fatal(err)
	}
	migrator.Close()
	conn, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn, databaseURL
}

func rotationSeedFixtures(t *testing.T, conn *pgx.Conn, key []byte) map[string]string {
	t.Helper()
	want := map[string]string{"legacy": "fixture-legacy-secret", "credential": "fixture-monitor-token", "smtp": "fixture-smtp-password", "webhook": "Bearer fixture-webhook", "slack": "https://hooks.slack.com/services/T/B/fixture-secret", "telegram": "123:fixture-token", "matrix": "fixture-matrix-token"}
	encrypt := func(kind string) []byte {
		value, err := crypto.Encrypt(key, []byte(want[kind]))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO secrets (name, encrypted_value) VALUES ('fixture-legacy', $1)`, base64.StdEncoding.EncodeToString(encrypt("legacy"))); err != nil {
		t.Fatal(err)
	}
	var monitorID uuid.UUID
	if err := conn.QueryRow(t.Context(), `INSERT INTO monitors (name,type,target,status) VALUES ('fixture','http','https://example.com','paused') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO monitor_credentials (monitor_id,auth_type,name,encrypted_value) VALUES ($1,'bearer','fixture',$2)`, monitorID, base64.StdEncoding.EncodeToString(encrypt("credential"))); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO smtp_settings (host,port,password_enc,from_address) VALUES ('localhost',2525,$1,'fixture@example.com')`, encrypt("smtp")); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"webhook", "slack", "telegram", "matrix"} {
		var raw json.RawMessage
		if kind == "webhook" {
			raw, _ = json.Marshal(map[string]any{"url": "https://example.com", "method": "POST", "body_template": "test", "headers": []map[string]string{{"name": "Authorization", "value": base64.StdEncoding.EncodeToString(encrypt(kind))}}})
		} else {
			var input map[string]any
			switch kind {
			case "slack":
				input = map[string]any{"webhook_url": want[kind]}
			case "telegram":
				input = map[string]any{"bot_token": want[kind], "chat_id": "-123"}
			case "matrix":
				input = map[string]any{"homeserver_url": "https://matrix.example", "room_id": "!ops:example", "access_token": want[kind]}
			}
			plain, _ := json.Marshal(input)
			var err error
			raw, err = native.Prepare(kind, plain, nil, key)
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(t.Context(), `INSERT INTO notification_channels (name,channel_type,config) VALUES ($1,$2,$3)`, "fixture-"+kind, kind, raw); err != nil {
			t.Fatal(err)
		}
	}
	return want
}

func rotationReadCiphertexts(t *testing.T, conn *pgx.Conn) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	rows, err := conn.Query(t.Context(), `SELECT 'legacy', encrypted_value FROM secrets UNION ALL SELECT 'credential', encrypted_value FROM monitor_credentials UNION ALL SELECT channel_type, CASE channel_type WHEN 'webhook' THEN config->'headers'->0->>'value' WHEN 'slack' THEN config->>'webhook_url' WHEN 'telegram' THEN config->>'bot_token' WHEN 'matrix' THEN config->>'access_token' END FROM notification_channels`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		got[kind] = decoded
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var smtp []byte
	if err := conn.QueryRow(t.Context(), `SELECT password_enc FROM smtp_settings`).Scan(&smtp); err != nil {
		t.Fatal(err)
	}
	got["smtp"] = smtp
	return got
}

func rotationAssertDecrypts(t *testing.T, values map[string][]byte, key []byte, want map[string]string, skip string) {
	t.Helper()
	if len(values) != len(want) {
		t.Fatal("missing encrypted credential type")
	}
	for kind, value := range values {
		if kind == skip {
			continue
		}
		plain, err := crypto.Decrypt(key, value)
		if err != nil || string(plain) != want[kind] {
			t.Fatalf("wrong decrypted value for %s", kind)
		}
	}
}
