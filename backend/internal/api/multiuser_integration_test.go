package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// authorizationDB provides only live user lookup. Any unexpected access to
// application data fails, so denied requests cannot silently reach a handler.
type authorizationDB struct {
	user     *db.User
	apiToken *db.ApiToken
}

func (f *authorizationDB) Exec(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "name: TouchAPIToken") {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.CommandTag{}, fmt.Errorf("unexpected write")
}
func (f *authorizationDB) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if strings.Contains(sql, "name: ListAPITokensByPrefix") && f.apiToken != nil && args[0] == f.apiToken.Prefix {
		return &authorizationTokenRows{token: f.apiToken}, nil
	}
	return nil, fmt.Errorf("unexpected query")
}

type authorizationTokenRows struct {
	pgx.Rows
	token *db.ApiToken
	done  bool
}

func (r *authorizationTokenRows) Close()     {}
func (r *authorizationTokenRows) Err() error { return nil }
func (r *authorizationTokenRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return true
}
func (r *authorizationTokenRows) Scan(dest ...interface{}) error {
	t := r.token
	*dest[0].(*uuid.UUID) = t.ID
	*dest[1].(*uuid.UUID) = t.UserID
	*dest[2].(*string) = t.Name
	*dest[3].(*string) = t.TokenHash
	*dest[4].(*pgtype.Timestamptz) = t.LastUsedAt
	*dest[5].(*pgtype.Timestamptz) = t.ExpiresAt
	*dest[6].(*pgtype.Timestamptz) = t.RevokedAt
	*dest[7].(*time.Time) = t.CreatedAt
	*dest[8].(*string) = t.Prefix
	return nil
}
func (f *authorizationDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	if strings.Contains(sql, "name: GetUser :one") && len(args) == 1 && f.user != nil && args[0] == f.user.ID {
		return authorizationRow{user: f.user}
	}
	return authorizationRow{}
}

type authorizationRow struct{ user *db.User }

func (r authorizationRow) Scan(dest ...interface{}) error {
	if r.user == nil {
		return pgx.ErrNoRows
	}
	u := r.user
	*dest[0].(*uuid.UUID) = u.ID
	*dest[1].(*string) = u.Email
	*dest[2].(*string) = u.PasswordHash
	*dest[3].(*time.Time) = u.CreatedAt
	*dest[4].(*time.Time) = u.UpdatedAt
	*dest[5].(*string) = u.Role
	*dest[6].(*bool) = u.Enabled
	*dest[7].(*bool) = u.MustChangePassword
	*dest[8].(*int64) = u.SessionVersion
	return nil
}

func TestMultiuserRouterAuthorization(t *testing.T) {
	secret := []byte("multiuser-test-secret")
	user := &db.User{ID: uuid.New(), Email: "viewer@example.com", Role: "read_only", Enabled: true}
	fake := &authorizationDB{user: user}
	router := NewRouter(Deps{Queries: db.New(fake), JWTSecret: secret, JWTExpiry: time.Hour, Hub: hub.New()})
	session, _, err := middleware.GenerateJWT(secret, user.ID, user.Email, time.Hour, 0)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, status int, code string) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: got %d: %s, want %d", method, path, w.Code, w.Body.String(), status)
		}
		if code != "" && !strings.Contains(w.Body.String(), `"code":"`+code+`"`) {
			t.Fatalf("%s %s: missing error %s: %s", method, path, code, w.Body.String())
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing request id")
		}
	}

	t.Run("viewer can inspect monitoring", func(t *testing.T) {
		// Invalid IDs reach read handlers and return validation errors.
		request(http.MethodGet, "/api/v1/monitors/invalid", 400, "INVALID_ID")
		request(http.MethodGet, "/api/v1/heartbeats/invalid", 400, "INVALID_ID")
	})
	t.Run("viewer cannot mutate or read administration", func(t *testing.T) {
		for _, route := range []struct{ method, path string }{
			{"POST", "/api/v1/monitors"}, {"PUT", "/api/v1/monitors/invalid"}, {"DELETE", "/api/v1/monitors/invalid"},
			{"POST", "/api/v1/heartbeats"}, {"POST", "/api/v1/heartbeats/invalid/rotate-token"},
			{"GET", "/api/v1/users"}, {"POST", "/api/v1/users"}, {"GET", "/api/v1/tokens"},
			{"GET", "/api/v1/notifications/channels"}, {"GET", "/api/v1/notifications/smtp-settings"},
			{"GET", "/api/v1/monitors/invalid/credentials"}, {"POST", "/api/v1/grpc/reflect"},
		} {
			request(route.method, route.path, 403, "FORBIDDEN")
		}
	})
	t.Run("role changes apply to an existing session", func(t *testing.T) {
		user.Role = "admin"
		request(http.MethodDelete, "/api/v1/monitors/invalid", 400, "INVALID_ID")
		user.Role = "read_only"
		request(http.MethodDelete, "/api/v1/monitors/invalid", 403, "FORBIDDEN")
	})
	t.Run("first login session is restricted", func(t *testing.T) {
		user.MustChangePassword = true
		request(http.MethodGet, "/api/v1/monitors/invalid", 403, "PASSWORD_CHANGE_REQUIRED")
		request(http.MethodGet, "/api/v1/auth/me", 200, "")
		request(http.MethodPut, "/api/v1/auth/password", 400, "VALIDATION_ERROR")
		// A forced-change session must not upgrade into a realtime connection.
		request(http.MethodGet, "/ws?token="+session, 401, "UNAUTHORIZED")
		user.MustChangePassword = false
	})
	t.Run("disabled user loses access immediately", func(t *testing.T) {
		user.Enabled = false
		request(http.MethodGet, "/api/v1/monitors/invalid", 401, "UNAUTHORIZED")
		request(http.MethodGet, "/ws?token="+session, 401, "UNAUTHORIZED")
		user.Enabled = true
	})
	t.Run("password reset invalidates existing sessions", func(t *testing.T) {
		user.SessionVersion++
		request(http.MethodGet, "/api/v1/auth/me", 401, "UNAUTHORIZED")
		request(http.MethodGet, "/ws?token="+session, 401, "UNAUTHORIZED")
	})
}

func TestMultiuserAPITokenInheritsLiveOwnerPermissions(t *testing.T) {
	raw, prefix, hash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	user := &db.User{ID: uuid.New(), Role: "admin", Enabled: true}
	fake := &authorizationDB{user: user, apiToken: &db.ApiToken{ID: uuid.New(), UserID: user.ID, Prefix: prefix, TokenHash: hash}}
	router := NewRouter(Deps{Queries: db.New(fake), JWTSecret: []byte("test"), JWTExpiry: time.Hour, Hub: hub.New()})
	check := func(method, path string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: got %d (%s), want %d", method, path, w.Code, w.Body.String(), want)
		}
	}
	check("DELETE", "/api/v1/monitors/invalid", 400)
	user.Role = "read_only"
	check("GET", "/api/v1/monitors/invalid", 400)
	check("DELETE", "/api/v1/monitors/invalid", 403)
	check("PUT", "/api/v1/auth/password", 403)
	check("GET", "/api/v1/auth/me", 403)
	user.MustChangePassword = true
	check("GET", "/api/v1/monitors/invalid", 401)
	check("GET", "/ws?token="+raw, 401)
	user.MustChangePassword = false
	user.Enabled = false
	check("GET", "/api/v1/monitors/invalid", 401)
	check("GET", "/ws?token="+raw, 401)
}
