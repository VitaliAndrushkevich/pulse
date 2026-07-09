package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

// authFakeDB implements db.DBTX for ChangePassword tests.
type authFakeDB struct {
	user *db.User
}

func (f *authFakeDB) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (f *authFakeDB) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (f *authFakeDB) QueryRow(_ context.Context, _ string, args ...interface{}) pgx.Row {
	return &authFakeRow{db: f, args: args}
}

type authFakeRow struct {
	db   *authFakeDB
	args []interface{}
}

func (r *authFakeRow) Scan(dest ...interface{}) error {
	if r.db.user == nil {
		return pgx.ErrNoRows
	}
	switch len(r.args) {
	case 1:
		// GetUser: args = (uuid.UUID), scans id, email, password_hash, created_at, updated_at
		*dest[0].(*uuid.UUID) = r.db.user.ID
		*dest[1].(*string) = r.db.user.Email
		*dest[2].(*string) = r.db.user.PasswordHash
		*dest[3].(*time.Time) = r.db.user.CreatedAt
		*dest[4].(*time.Time) = r.db.user.UpdatedAt
	case 2:
		// UpdateUserPassword: args = (uuid.UUID, string), scans id, email, password_hash, created_at, updated_at
		newHash := r.args[1].(string)
		r.db.user.PasswordHash = newHash
		r.db.user.UpdatedAt = time.Now().UTC()
		*dest[0].(*uuid.UUID) = r.db.user.ID
		*dest[1].(*string) = r.db.user.Email
		*dest[2].(*string) = r.db.user.PasswordHash
		*dest[3].(*time.Time) = r.db.user.CreatedAt
		*dest[4].(*time.Time) = r.db.user.UpdatedAt
	}
	return nil
}

const testCurrentPassword = "oldpassword123"

func setupAuthRouter(fdb *authFakeDB, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	queries := db.New(fdb)
	h := handlers.NewAuthHandler(queries, []byte("test-jwt-secret"), 15*time.Minute)

	// Simulate auth middleware setting user_id
	auth := r.Group("/api/v1", func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Next()
	})
	auth.PUT("/auth/password", h.ChangePassword)

	return r
}

func newTestUser(t *testing.T) (uuid.UUID, *db.User) {
	t.Helper()
	uid := uuid.New()
	hash, err := bcrypt.GenerateFromPassword([]byte(testCurrentPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	now := time.Now().UTC()
	return uid, &db.User{
		ID:           uid,
		Email:        "test@example.com",
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestChangePassword_Success(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	body := `{"current_password":"oldpassword123","new_password":"newsecure99"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Req 4.7: response only contains "message" field
	if _, ok := resp["message"]; !ok {
		t.Error("response missing message field")
	}
	if len(resp) != 1 {
		t.Errorf("response should only have 'message', got keys: %v", resp)
	}
	// Must not echo password or hash
	if _, ok := resp["password"]; ok {
		t.Error("response must not contain password")
	}
	if _, ok := resp["password_hash"]; ok {
		t.Error("response must not contain password_hash")
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	body := `{"current_password":"wrongpassword","new_password":"newsecure99"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error envelope in response")
	}
	if errObj["code"] != "UNAUTHORIZED" {
		t.Errorf("expected code UNAUTHORIZED, got %v", errObj["code"])
	}
}

func TestChangePassword_NewPasswordTooShort(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	body := `{"current_password":"oldpassword123","new_password":"short"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error envelope in response")
	}
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected code VALIDATION_ERROR, got %v", errObj["code"])
	}
}

func TestChangePassword_NewPasswordTooLong(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	// 73 bytes exceeds the 72-byte bcrypt limit
	longPassword := strings.Repeat("a", 73)
	body := `{"current_password":"oldpassword123","new_password":"` + longPassword + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error envelope in response")
	}
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected code VALIDATION_ERROR, got %v", errObj["code"])
	}
}

func TestChangePassword_MissingBody(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	tests := []struct {
		name string
		body string
	}{
		{"empty body", "{}"},
		{"missing new_password", `{"current_password":"oldpassword123"}`},
		{"missing current_password", `{"new_password":"newsecure99"}`},
		{"invalid json", `{not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			errObj, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error envelope in response")
			}
			if errObj["code"] != "VALIDATION_ERROR" {
				t.Errorf("expected code VALIDATION_ERROR, got %v", errObj["code"])
			}
		})
	}
}
