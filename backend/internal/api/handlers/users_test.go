package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

type managedUserDB struct {
	user      *db.User
	duplicate bool
	writes    int
}

func (f *managedUserDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *managedUserDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return &managedUserRows{user: f.user}, nil
}
func (f *managedUserDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	if strings.Contains(sql, "name: CountUsers") {
		return managedCountRow{count: 1}
	}
	if strings.Contains(sql, "name: CreateManagedUser") {
		if f.duplicate {
			return managedUserRow{err: &pgconn.PgError{Code: "23505"}}
		}
		f.writes++
		f.user = &db.User{ID: uuid.New(), Email: args[0].(string), PasswordHash: args[1].(string), Role: args[2].(string), Enabled: true, MustChangePassword: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}
	if strings.Contains(sql, "name: ResetUserPassword") {
		if f.user == nil || f.user.ID != args[0].(uuid.UUID) {
			return managedUserRow{err: pgx.ErrNoRows}
		}
		f.writes++
		f.user.PasswordHash = args[1].(string)
		f.user.MustChangePassword = true
		f.user.SessionVersion++
	}
	return managedUserRow{user: f.user}
}

type managedCountRow struct{ count int64 }

func (r managedCountRow) Scan(dest ...interface{}) error { *dest[0].(*int64) = r.count; return nil }

type managedUserRow struct {
	user *db.User
	err  error
}

func (r managedUserRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
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

type managedUserRows struct {
	pgx.Rows
	user *db.User
	done bool
}

func (r *managedUserRows) Close()     {}
func (r *managedUserRows) Err() error { return nil }
func (r *managedUserRows) Next() bool {
	if r.done || r.user == nil {
		return false
	}
	r.done = true
	return true
}
func (r *managedUserRows) Scan(dest ...interface{}) error {
	return (managedUserRow{user: r.user}).Scan(dest...)
}

func managedUserRouter(f *managedUserDB, actor uuid.UUID) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", actor.String()); c.Next() })
	handlers.NewUserHandler(db.New(f), nil).Register(r.Group("/api/v1"))
	return r
}
func managedRequest(t *testing.T, r *gin.Engine, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("%s %s: got %d (%s), want %d", method, path, w.Code, w.Body.String(), want)
	}
	return w
}

func TestManagedUserLifecycle(t *testing.T) {
	fake := &managedUserDB{}
	router := managedUserRouter(fake, uuid.New())
	const temporary = "temporary-password"
	created := managedRequest(t, router, "POST", "/api/v1/users", `{"email":"Viewer@Example.com","password":"`+temporary+`","role":"read_only"}`, 201)
	if fake.user.Email != "viewer@example.com" || !fake.user.MustChangePassword || fake.user.Role != "read_only" {
		t.Fatalf("incorrect provisioned account: %+v", fake.user)
	}
	if bcrypt.CompareHashAndPassword([]byte(fake.user.PasswordHash), []byte(temporary)) != nil {
		t.Fatal("temporary password was not hashed")
	}
	assertSafe := func(w *httptest.ResponseRecorder) {
		t.Helper()
		if strings.Contains(w.Body.String(), temporary) || strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "session_version") {
			t.Fatalf("secret/internal fields leaked: %s", w.Body.String())
		}
	}
	assertSafe(created)
	listed := managedRequest(t, router, "GET", "/api/v1/users?page=1&limit=10", "", 200)
	assertSafe(listed)
	var page struct {
		Data []struct {
			Email string `json:"email"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Data) != 1 || page.Total != 1 || page.Data[0].Email != fake.user.Email {
		t.Fatalf("invalid user listing: %s", listed.Body.String())
	}
	fake.user.MustChangePassword = false
	reset := managedRequest(t, router, "POST", "/api/v1/users/"+fake.user.ID.String()+"/reset-password", `{"password":"another-temporary"}`, 200)
	assertSafe(reset)
	if strings.Contains(reset.Body.String(), "another-temporary") || !fake.user.MustChangePassword || fake.user.SessionVersion != 1 {
		t.Fatalf("reset did not invalidate sessions and require password change: %s", reset.Body.String())
	}
	if bcrypt.CompareHashAndPassword([]byte(fake.user.PasswordHash), []byte("another-temporary")) != nil {
		t.Fatal("reset password was not hashed")
	}
}

func TestManagedUserValidationAndSelfProtection(t *testing.T) {
	actor := uuid.New()
	fake := &managedUserDB{}
	router := managedUserRouter(fake, actor)
	for _, body := range []string{
		`{"email":"viewer@example.com","password":"temporary-pass","role":"owner"}`,
		`{"email":"viewer@example.com","password":"short","role":"read_only"}`,
		`{"email":"invalid","password":"temporary-pass","role":"read_only"}`,
	} {
		managedRequest(t, router, "POST", "/api/v1/users", body, 400)
	}
	for _, body := range []string{`{"role":"read_only"}`, `{"enabled":false}`} {
		managedRequest(t, router, "PATCH", "/api/v1/users/"+actor.String(), body, 409)
	}
	managedRequest(t, router, "GET", "/api/v1/users?page=999999999&limit=100", "", 400)
	if fake.writes != 0 {
		t.Fatal("invalid request reached database writes")
	}
	fake.duplicate = true
	w := managedRequest(t, router, "POST", "/api/v1/users", `{"email":"viewer@example.com","password":"temporary-pass","role":"read_only"}`, 409)
	if !strings.Contains(w.Body.String(), "EMAIL_EXISTS") {
		t.Fatalf("unexpected duplicate email error: %s", w.Body.String())
	}
}
