package handlers_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// fakeDB implements db.DBTX for testing without a real database.
type fakeDB struct {
	secrets map[uuid.UUID]db.Secret
	order   []uuid.UUID
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		secrets: make(map[uuid.UUID]db.Secret),
	}
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	// DELETE
	if len(args) > 0 {
		id := args[0].(uuid.UUID)
		delete(f.secrets, id)
		for i, oid := range f.order {
			if oid == id {
				f.order = append(f.order[:i], f.order[i+1:]...)
				break
			}
		}
	}
	return pgconn.NewCommandTag("DELETE 1"), nil
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	// Not used directly in our tests — ListSecrets uses Query but we test via HTTP
	return nil, nil
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	return &fakeRow{db: f, sql: sql, args: args}
}

type fakeRow struct {
	db   *fakeDB
	sql  string
	args []interface{}
}

func (r *fakeRow) Scan(dest ...interface{}) error {
	// Detect which query based on arg count and sql content
	switch {
	case len(r.args) == 2:
		// CreateSecret: args = (name, encrypted_value)
		id := uuid.New()
		now := time.Now().UTC()
		s := db.Secret{
			ID:             id,
			Name:           r.args[0].(string),
			EncryptedValue: r.args[1].(string),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		r.db.secrets[id] = s
		r.db.order = append(r.db.order, id)
		// RETURNING id, name, encrypted_value, created_at, updated_at
		*dest[0].(*uuid.UUID) = s.ID
		*dest[1].(*string) = s.Name
		*dest[2].(*string) = s.EncryptedValue
		*dest[3].(*time.Time) = s.CreatedAt
		*dest[4].(*time.Time) = s.UpdatedAt
	case len(r.args) == 3:
		// UpdateSecret: args = (id, name, encrypted_value)
		id := r.args[0].(uuid.UUID)
		s, ok := r.db.secrets[id]
		if !ok {
			return pgx.ErrNoRows
		}
		s.Name = r.args[1].(string)
		s.EncryptedValue = r.args[2].(string)
		s.UpdatedAt = time.Now().UTC()
		r.db.secrets[id] = s
		*dest[0].(*uuid.UUID) = s.ID
		*dest[1].(*string) = s.Name
		*dest[2].(*string) = s.EncryptedValue
		*dest[3].(*time.Time) = s.CreatedAt
		*dest[4].(*time.Time) = s.UpdatedAt
	case len(r.args) == 1:
		// GetSecret: args = (id)
		id := r.args[0].(uuid.UUID)
		s, ok := r.db.secrets[id]
		if !ok {
			return pgx.ErrNoRows
		}
		*dest[0].(*uuid.UUID) = s.ID
		*dest[1].(*string) = s.Name
		*dest[2].(*string) = s.EncryptedValue
		*dest[3].(*time.Time) = s.CreatedAt
		*dest[4].(*time.Time) = s.UpdatedAt
	case len(r.args) == 0:
		// CountSecrets
		*dest[0].(*int64) = int64(len(r.db.secrets))
	}
	return nil
}

func testKey() []byte {
	// Generate a deterministic 32-byte key for tests.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func setupRouter(fdb *fakeDB, key []byte) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	queries := db.New(fdb)
	h := handlers.NewSecretHandler(queries, key)
	h.Register(v1)
	return r
}

func TestCreateSecret_ReturnsRedactedResponse(t *testing.T) {
	fdb := newFakeDB()
	key := testKey()
	router := setupRouter(fdb, key)

	body := `{"name":"db-password","value":"super-secret-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Must have id, name, created_at, updated_at
	if _, ok := resp["id"]; !ok {
		t.Error("response missing id")
	}
	if resp["name"] != "db-password" {
		t.Errorf("expected name=db-password, got %v", resp["name"])
	}

	// Must NOT have value or encrypted_value
	if _, ok := resp["value"]; ok {
		t.Error("response must not contain value field")
	}
	if _, ok := resp["encrypted_value"]; ok {
		t.Error("response must not contain encrypted_value field")
	}
}

func TestCreateSecret_EncryptsValue(t *testing.T) {
	fdb := newFakeDB()
	key := testKey()
	router := setupRouter(fdb, key)

	body := `{"name":"api-key","value":"my-secret-value"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Verify the stored value is encrypted (not plaintext)
	if len(fdb.secrets) != 1 {
		t.Fatalf("expected 1 secret stored, got %d", len(fdb.secrets))
	}

	for _, s := range fdb.secrets {
		if s.EncryptedValue == "my-secret-value" {
			t.Error("secret stored as plaintext, expected encrypted")
		}

		// Verify we can decrypt it back
		ciphertext, err := base64.StdEncoding.DecodeString(s.EncryptedValue)
		if err != nil {
			t.Fatalf("stored value is not valid base64: %v", err)
		}
		plaintext, err := crypto.Decrypt(key, ciphertext)
		if err != nil {
			t.Fatalf("failed to decrypt stored value: %v", err)
		}
		if string(plaintext) != "my-secret-value" {
			t.Errorf("decrypted value = %q, want %q", plaintext, "my-secret-value")
		}
	}
}

func TestGetSecret_NeverReturnsValue(t *testing.T) {
	fdb := newFakeDB()
	key := testKey()
	router := setupRouter(fdb, key)

	// Create a secret first
	body := `{"name":"token","value":"secret-token-value"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// GET the secret
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+id, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if _, ok := resp["value"]; ok {
		t.Error("GET response must not contain value")
	}
	if _, ok := resp["encrypted_value"]; ok {
		t.Error("GET response must not contain encrypted_value")
	}
	if resp["name"] != "token" {
		t.Errorf("expected name=token, got %v", resp["name"])
	}
}

func TestGetSecret_InvalidUUID(t *testing.T) {
	fdb := newFakeDB()
	router := setupRouter(fdb, testKey())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	fdb := newFakeDB()
	router := setupRouter(fdb, testKey())

	id := uuid.NewString()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateSecret_ReturnsRedactedResponse(t *testing.T) {
	fdb := newFakeDB()
	key := testKey()
	router := setupRouter(fdb, key)

	// Create first
	body := `{"name":"old-name","value":"old-value"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Update
	body = `{"name":"new-name","value":"new-value"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/"+id, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if _, ok := resp["value"]; ok {
		t.Error("PUT response must not contain value")
	}
	if _, ok := resp["encrypted_value"]; ok {
		t.Error("PUT response must not contain encrypted_value")
	}
	if resp["name"] != "new-name" {
		t.Errorf("expected name=new-name, got %v", resp["name"])
	}
}

func TestDeleteSecret(t *testing.T) {
	fdb := newFakeDB()
	router := setupRouter(fdb, testKey())

	// Create first
	body := `{"name":"to-delete","value":"val"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+id, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Verify it's gone
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+id, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

func TestCreateSecret_ValidationError(t *testing.T) {
	fdb := newFakeDB()
	router := setupRouter(fdb, testKey())

	tests := []struct {
		name string
		body string
	}{
		{"missing value", `{"name":"test"}`},
		{"missing name", `{"value":"test"}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			if _, ok := resp["error"]; !ok {
				t.Error("expected error envelope in response")
			}
		})
	}
}
