//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/migrations"
)

// TestMultiuserPostgresUpgradeAndLifecycle runs only against an explicitly named,
// disposable verification database. It never falls back to the application DB.
func TestMultiuserPostgresUpgradeAndLifecycle(t *testing.T) {
	dsn := os.Getenv("PULSE_MULTIUSER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PULSE_MULTIUSER_TEST_DATABASE_URL to a new isolated verification database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/pulse_multiuser_verify_") {
		t.Fatal("verification database name must start with pulse_multiuser_verify_")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = pool.QueryRow(ctx, "SELECT to_regclass('public.users') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("verification database must be empty; refusing to modify an existing schema")
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err = migrator.Migrate(17); err != nil {
		t.Fatalf("migrate to 17: %v", err)
	}
	originalHash, err := bcrypt.GenerateFromPassword([]byte("OriginalAdmin123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var adminID uuid.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id", "legacy-admin@example.com", string(originalHash)).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err = migrator.Migrate(18); err != nil {
		t.Fatalf("upgrade to 18: %v", err)
	}
	queries := db.New(pool)
	legacy, err := queries.GetUser(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Role != "admin" || !legacy.Enabled || legacy.MustChangePassword || legacy.SessionVersion != 0 || legacy.PasswordHash != string(originalHash) {
		t.Fatalf("legacy administrator changed unexpectedly: role=%s enabled=%v forced=%v version=%d", legacy.Role, legacy.Enabled, legacy.MustChangePassword, legacy.SessionVersion)
	}
	t.Log("migration 17 → 18 preserves existing administrator credentials and access")
	if err = migrator.Up(); err != nil {
		t.Fatalf("normal 18 → 19 upgrade: %v", err)
	}
	normal, err := queries.GetUser(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	if normal != legacy {
		t.Fatal("idempotent repair migration changed an existing administrator")
	}
	// Reproduce a database whose ledger says 18 completed while users still has
	// the original five columns. Only this disposable database is altered.
	if _, err = pool.Exec(ctx, `DROP INDEX IF EXISTS users_email_normalized;
        ALTER TABLE users DROP COLUMN session_version, DROP COLUMN must_change_password,
        DROP COLUMN enabled, DROP COLUMN role;`); err != nil {
		t.Fatal(err)
	}
	if err = migrator.Force(18); err != nil {
		t.Fatal(err)
	}
	if err = migrator.Up(); err != nil {
		t.Fatalf("repair incomplete version 18 schema: %v", err)
	}
	repaired, err := queries.GetUser(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired != legacy {
		t.Fatal("repair migration changed existing administrator credentials or timestamps")
	}
	var normalizedIndex bool
	if err = pool.QueryRow(ctx, "SELECT to_regclass('public.users_email_normalized') IS NOT NULL").Scan(&normalizedIndex); err != nil {
		t.Fatal(err)
	}
	if !normalizedIndex {
		t.Fatal("repair migration did not recreate normalized email index")
	}
	t.Log("normal 18 → 19 is idempotent; recorded-18 missing-columns schema is repaired without changing credentials")

	server := httptest.NewServer(NewRouter(Deps{Queries: queries, Pool: pool, JWTSecret: []byte("isolated-multiuser-verification-secret"), JWTExpiry: time.Hour}))
	defer server.Close()
	type result struct {
		status int
		body   map[string]any
	}
	call := func(method, path, token string, body any) result {
		t.Helper()
		var payload io.Reader
		if body != nil {
			raw, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			payload = bytes.NewReader(raw)
		}
		req, e := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1"+path, payload)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var decoded map[string]any
		if e = json.NewDecoder(resp.Body).Decode(&decoded); e != nil {
			t.Fatal(e)
		}
		return result{resp.StatusCode, decoded}
	}
	expect := func(r result, status int) map[string]any {
		t.Helper()
		if r.status != status {
			t.Fatalf("status=%d want=%d body=%v", r.status, status, r.body)
		}
		return r.body
	}
	login := func(email, password string) string {
		t.Helper()
		r := expect(call("POST", "/auth/login", "", map[string]any{"email": email, "password": password}), 200)
		return r["token"].(string)
	}
	adminToken := login("LEGACY-ADMIN@example.com", "OriginalAdmin123!")
	expect(call("POST", "/auth/setup", "", map[string]any{"email": "extra@example.com", "password": "Secret123!"}), 409)
	newUser := expect(call("POST", "/users", adminToken, map[string]any{"email": "Viewer@Example.com", "password": "Temporary123!", "role": "read_only"}), 201)
	viewerID := newUser["id"].(string)
	if newUser["must_change_password"] != true || newUser["enabled"] != true || newUser["email"] != "viewer@example.com" {
		t.Fatalf("invalid provisioned user: %v", newUser)
	}
	expect(call("POST", "/users", adminToken, map[string]any{"email": "viewer@example.com", "password": "Temporary123!", "role": "read_only"}), 409)
	forced := login("viewer@example.com", "Temporary123!")
	expect(call("GET", "/auth/me", forced, nil), 200)
	expect(call("GET", "/dashboard/summary", forced, nil), 403)
	expect(call("PUT", "/auth/password", forced, map[string]any{"current_password": "Temporary123!", "new_password": "Temporary123!"}), 400)
	changed := expect(call("PUT", "/auth/password", forced, map[string]any{"current_password": "Temporary123!", "new_password": "Personal123!"}), 200)
	viewerToken := changed["token"].(string)
	expect(call("GET", "/auth/me", forced, nil), 401)
	expect(call("GET", "/monitors", viewerToken, nil), 200)
	expect(call("POST", "/monitors", viewerToken, map[string]any{}), 403)
	expect(call("GET", "/users", viewerToken, nil), 403)
	expect(call("GET", "/tokens", viewerToken, nil), 403)
	expect(call("GET", "/notifications/channels", viewerToken, nil), 403)
	listed := expect(call("GET", "/users?page=1&limit=1", adminToken, nil), 200)
	if listed["total"] != float64(2) || len(listed["data"].([]any)) != 1 {
		t.Fatalf("invalid user pagination: %v", listed)
	}
	t.Log("provisioning, duplicate email, temporary-password gate, password replacement, session revocation and viewer restrictions pass")
	expect(call("PATCH", "/users/"+viewerID, adminToken, map[string]any{"role": "admin"}), 200)
	expect(call("GET", "/auth/me", viewerToken, nil), 401)
	promoted := login("viewer@example.com", "Personal123!")
	expect(call("GET", "/users", promoted, nil), 200)
	expect(call("PATCH", "/users/"+viewerID, adminToken, map[string]any{"enabled": false}), 200)
	expect(call("GET", "/auth/me", promoted, nil), 401)
	expect(call("POST", "/auth/login", "", map[string]any{"email": "viewer@example.com", "password": "Personal123!"}), 401)
	expect(call("PATCH", "/users/"+viewerID, adminToken, map[string]any{"enabled": true, "role": "read_only"}), 200)
	expect(call("GET", "/auth/me", promoted, nil), 401)
	reset := expect(call("POST", "/users/"+viewerID+"/reset-password", adminToken, map[string]any{"password": "ResetTemporary123!"}), 200)
	if reset["must_change_password"] != true {
		t.Fatal("reset did not require a password change")
	}
	expect(call("POST", "/auth/login", "", map[string]any{"email": "viewer@example.com", "password": "Personal123!"}), 401)
	resetToken := login("viewer@example.com", "ResetTemporary123!")
	expect(call("GET", "/monitors", resetToken, nil), 403)
	expect(call("PUT", "/auth/password", resetToken, map[string]any{"current_password": "ResetTemporary123!", "new_password": "NewPersonal123!"}), 200)
	t.Log("role changes, disabling/re-enabling, and administrator password reset revoke old sessions")
	uid, _ := uuid.Parse(viewerID)
	snapshot, err := queries.GetUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = queries.ResetUserPassword(ctx, db.ResetUserPasswordParams{ID: uid, PasswordHash: string(originalHash)}); err != nil {
		t.Fatal(err)
	}
	if _, err = queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: uid, PasswordHash: "stale-hash", SessionVersion: snapshot.SessionVersion}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale password update bypassed reset: %v", err)
	}
	t.Log("database compare-and-swap rejects a password change that races an administrator reset")
	second := expect(call("POST", "/users", adminToken, map[string]any{"email": "second-admin@example.com", "password": "SecondTemp123!", "role": "admin"}), 201)
	secondID := second["id"].(string)
	secondForced := login("second-admin@example.com", "SecondTemp123!")
	secondChanged := expect(call("PUT", "/auth/password", secondForced, map[string]any{"current_password": "SecondTemp123!", "new_password": "SecondPersonal123!"}), 200)
	secondToken := secondChanged["token"].(string)
	expect(call("PATCH", "/users/"+adminID.String(), adminToken, map[string]any{"enabled": false}), 409)
	// Competing administrators cannot demote one another and leave no admin.
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	start := make(chan struct{})
	for _, attempt := range []struct{ id, token string }{{secondID, adminToken}, {adminID.String(), secondToken}} {
		wg.Add(1)
		go func(id, token string) {
			defer wg.Done()
			<-start
			statuses <- call("PATCH", "/users/"+id, token, map[string]any{"role": "read_only"}).status
		}(attempt.id, attempt.token)
	}
	close(start)
	wg.Wait()
	close(statuses)
	successes := 0
	for status := range statuses {
		if status == 200 {
			successes++
		} else if status != 401 && status != 403 && status != 409 {
			t.Fatalf("unexpected concurrent membership status: %d", status)
		}
	}
	count, err := queries.CountActiveAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if successes != 1 || count != 1 {
		t.Fatalf("membership race: successes=%d active admins=%d", successes, count)
	}
	t.Log("concurrent administrator demotions preserve exactly one active administrator")
}
