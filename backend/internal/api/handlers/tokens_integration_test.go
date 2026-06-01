package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// --- Fake DB for token integration tests ---

// tokenFakeDB implements db.DBTX and simulates the token-related queries.
type tokenFakeDB struct {
	mu     sync.Mutex
	tokens map[uuid.UUID]db.ApiToken
	order  []uuid.UUID // insertion order for listing
}

func newTokenFakeDB() *tokenFakeDB {
	return &tokenFakeDB{
		tokens: make(map[uuid.UUID]db.ApiToken),
	}
}

// seedToken inserts a token directly into the fake DB (for auth testing).
func (f *tokenFakeDB) seedToken(t db.ApiToken) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[t.ID] = t
	f.order = append(f.order, t.ID)
}

func (f *tokenFakeDB) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// TouchAPIToken: UPDATE api_tokens SET last_used_at = now() WHERE id = $1
	if strings.Contains(sql, "last_used_at") && len(args) == 1 {
		id := args[0].(uuid.UUID)
		if tok, ok := f.tokens[id]; ok {
			tok.LastUsedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
			f.tokens[id] = tok
		}
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}

	return pgconn.NewCommandTag(""), nil
}

func (f *tokenFakeDB) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// ListAPITokensByPrefix: WHERE prefix = $1 AND revoked_at IS NULL AND ...
	if strings.Contains(sql, "prefix") && len(args) == 1 {
		prefix := args[0].(string)
		var results []db.ApiToken
		for _, tok := range f.tokens {
			if tok.Prefix == prefix && !tok.RevokedAt.Valid {
				if !tok.ExpiresAt.Valid || tok.ExpiresAt.Time.After(time.Now()) {
					results = append(results, tok)
				}
			}
		}
		return &fakeTokenRows{tokens: results, idx: -1}, nil
	}

	// ListAPITokensByUser: WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
	if strings.Contains(sql, "user_id") && len(args) == 3 {
		userID := args[0].(uuid.UUID)
		limit := args[1].(int32)
		offset := args[2].(int32)

		// Collect tokens for user in descending created_at order.
		var userTokens []db.ApiToken
		// Iterate in reverse insertion order (newest first).
		for i := len(f.order) - 1; i >= 0; i-- {
			tok := f.tokens[f.order[i]]
			if tok.UserID == userID {
				userTokens = append(userTokens, tok)
			}
		}

		// Apply offset and limit.
		start := int(offset)
		if start > len(userTokens) {
			start = len(userTokens)
		}
		end := start + int(limit)
		if end > len(userTokens) {
			end = len(userTokens)
		}
		results := userTokens[start:end]
		return &fakeTokenRows{tokens: results, idx: -1}, nil
	}

	return &fakeTokenRows{tokens: nil, idx: -1}, nil
}

func (f *tokenFakeDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	f.mu.Lock()
	defer f.mu.Unlock()

	// CreateAPIToken: INSERT INTO api_tokens (user_id, name, prefix, token_hash, expires_at)
	if strings.Contains(sql, "INSERT") && len(args) == 5 {
		userID := args[0].(uuid.UUID)
		name := args[1].(string)
		prefix := args[2].(string)
		tokenHash := args[3].(string)
		expiresAt := args[4].(pgtype.Timestamptz)

		tok := db.ApiToken{
			ID:        uuid.New(),
			UserID:    userID,
			Name:      name,
			Prefix:    prefix,
			TokenHash: tokenHash,
			ExpiresAt: expiresAt,
			CreatedAt: time.Now().UTC(),
		}
		f.tokens[tok.ID] = tok
		f.order = append(f.order, tok.ID)
		return &fakeTokenRow{token: &tok}
	}

	// RevokeAPIToken: UPDATE ... SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1 AND user_id = $2
	if strings.Contains(sql, "revoked_at") && strings.Contains(sql, "COALESCE") && len(args) == 2 {
		id := args[0].(uuid.UUID)
		userID := args[1].(uuid.UUID)
		tok, ok := f.tokens[id]
		if !ok || tok.UserID != userID {
			return &fakeTokenRow{err: pgx.ErrNoRows}
		}
		// Idempotent: only set revoked_at if not already set.
		if !tok.RevokedAt.Valid {
			tok.RevokedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
			f.tokens[id] = tok
		}
		return &fakeTokenRow{token: &tok}
	}

	// CountAPITokensByUser: SELECT COUNT(*) FROM api_tokens WHERE user_id = $1
	if strings.Contains(sql, "COUNT") && len(args) == 1 {
		userID := args[0].(uuid.UUID)
		var count int64
		for _, tok := range f.tokens {
			if tok.UserID == userID {
				count++
			}
		}
		return &fakeCountRow{count: count}
	}

	return &fakeTokenRow{err: pgx.ErrNoRows}
}

// --- Fake pgx.Row implementations ---

type fakeTokenRow struct {
	token *db.ApiToken
	err   error
}

func (r *fakeTokenRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
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

type fakeCountRow struct {
	count int64
}

func (r *fakeCountRow) Scan(dest ...interface{}) error {
	*dest[0].(*int64) = r.count
	return nil
}

// --- Fake pgx.Rows implementation ---

type fakeTokenRows struct {
	tokens []db.ApiToken
	idx    int
}

func (r *fakeTokenRows) Close()                        {}
func (r *fakeTokenRows) Err() error                    { return nil }
func (r *fakeTokenRows) CommandTag() pgconn.CommandTag  { return pgconn.NewCommandTag("") }
func (r *fakeTokenRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeTokenRows) RawValues() [][]byte           { return nil }
func (r *fakeTokenRows) Conn() *pgx.Conn               { return nil }

func (r *fakeTokenRows) Next() bool {
	r.idx++
	return r.idx < len(r.tokens)
}

func (r *fakeTokenRows) Scan(dest ...interface{}) error {
	t := r.tokens[r.idx]
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

func (r *fakeTokenRows) Values() ([]interface{}, error) { return nil, nil }

// --- Test setup helpers ---

// setupTokenRouter creates a gin router with BearerAuth middleware and TokenHandler.
func setupTokenRouter(fdb *tokenFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// X-Request-ID middleware (mirrors router.go).
	r.Use(func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	})

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")

	protected := v1.Group("")
	protected.Use(middleware.BearerAuth(queries))

	tokenHandler := handlers.NewTokenHandler(queries)
	tokenHandler.Register(protected)

	return r
}

// setupTokenRouterWithUser creates a router that bypasses auth and sets user_id directly.
// This is useful for testing token endpoints without needing a pre-seeded auth token.
func setupTokenRouterWithUser(fdb *tokenFakeDB, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// X-Request-ID middleware.
	r.Use(func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	})

	// Inject user_id directly (simulates auth middleware).
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Next()
	})

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")
	tokenHandler := handlers.NewTokenHandler(queries)
	tokenHandler.Register(v1)

	return r
}

// --- Integration Tests ---

// TestTokenLifecycle_CreateAuthRevokeAuthFails tests the full lifecycle:
// create → authenticate → revoke → authenticate-fails.
func TestTokenLifecycle_CreateAuthRevokeAuthFails(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed an initial auth token so we can authenticate to create new tokens.
	rawAuth, prefix, hash, err := token.Generate()
	if err != nil {
		t.Fatalf("failed to generate auth token: %v", err)
	}
	authToken := db.ApiToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      "auth-token",
		Prefix:    prefix,
		TokenHash: hash,
		CreatedAt: time.Now().UTC(),
	}
	fdb.seedToken(authToken)

	router := setupTokenRouter(fdb)

	// Step 1: Create a new token via POST /api/v1/tokens.
	body := `{"name":"my-new-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawAuth)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createResp struct {
		Token     string `json:"token"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("create: invalid json: %v", err)
	}
	if createResp.Token == "" {
		t.Fatal("create: raw token must be returned")
	}
	if createResp.ID == "" {
		t.Fatal("create: id must be returned")
	}
	if createResp.Name != "my-new-token" {
		t.Errorf("create: expected name=my-new-token, got %s", createResp.Name)
	}

	newRawToken := createResp.Token
	tokenID := createResp.ID

	// Step 2: Authenticate using the newly created token.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+newRawToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("auth: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Step 3: Revoke the new token.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/"+tokenID, nil)
	req.Header.Set("Authorization", "Bearer "+rawAuth)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var revokeResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &revokeResp); err != nil {
		t.Fatalf("revoke: invalid json: %v", err)
	}
	if revokeResp["revoked_at"] == nil {
		t.Fatal("revoke: revoked_at must be set")
	}

	// Step 4: Authenticate with the revoked token should fail.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+newRawToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("auth-after-revoke: expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTokenList_DescendingCreatedAt verifies Property 6: list ordering invariant.
func TestTokenList_DescendingCreatedAt(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed tokens with known created_at values (oldest first in insertion order).
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		tok := db.ApiToken{
			ID:        uuid.New(),
			UserID:    userID,
			Name:      fmt.Sprintf("token-%d", i),
			Prefix:    fmt.Sprintf("prefix%02d", i),
			TokenHash: "hash",
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		fdb.seedToken(tok)
	}

	router := setupTokenRouterWithUser(fdb, userID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if len(resp.Data) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(resp.Data))
	}

	// Verify descending order.
	for i := 0; i < len(resp.Data)-1; i++ {
		t1, _ := time.Parse(time.RFC3339Nano, resp.Data[i].CreatedAt)
		t2, _ := time.Parse(time.RFC3339Nano, resp.Data[i+1].CreatedAt)
		if t1.Before(t2) {
			t.Errorf("list not in descending order: item[%d]=%v < item[%d]=%v",
				i, resp.Data[i].CreatedAt, i+1, resp.Data[i+1].CreatedAt)
		}
	}
}

// TestTokenResponse_FieldCompleteness verifies Property 7: all required fields present.
func TestTokenResponse_FieldCompleteness(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed a token with all optional fields populated.
	now := time.Now().UTC()
	tok := db.ApiToken{
		ID:         uuid.New(),
		UserID:     userID,
		Name:       "complete-token",
		Prefix:     "complete",
		TokenHash:  "hash",
		LastUsedAt: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true},
		ExpiresAt:  pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true},
		RevokedAt:  pgtype.Timestamptz{Time: now.Add(-30 * time.Minute), Valid: true},
		CreatedAt:  now.Add(-2 * time.Hour),
	}
	fdb.seedToken(tok)

	router := setupTokenRouterWithUser(fdb, userID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if len(resp.Data) == 0 {
		t.Fatal("expected at least 1 token in response")
	}

	item := resp.Data[0]
	requiredFields := []string{"id", "name", "last_used_at", "expires_at", "revoked_at", "created_at"}
	for _, field := range requiredFields {
		if _, ok := item[field]; !ok {
			t.Errorf("response missing required field: %s", field)
		}
	}

	// Verify token_hash is NOT present.
	if _, ok := item["token_hash"]; ok {
		t.Error("response must not contain token_hash field")
	}
}

// TestTokenRevocation_Idempotence verifies Property 9: same revoked_at on re-revoke.
func TestTokenRevocation_Idempotence(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed a token to revoke.
	tok := db.ApiToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      "to-revoke",
		Prefix:    "torevoke",
		TokenHash: "hash",
		CreatedAt: time.Now().UTC(),
	}
	fdb.seedToken(tok)

	router := setupTokenRouterWithUser(fdb, userID)

	// First revocation.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/"+tok.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("first revoke: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var firstResp struct {
		RevokedAt string `json:"revoked_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("first revoke: invalid json: %v", err)
	}
	if firstResp.RevokedAt == "" {
		t.Fatal("first revoke: revoked_at must be set")
	}

	// Second revocation (idempotent).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/"+tok.ID.String(), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("second revoke: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var secondResp struct {
		RevokedAt string `json:"revoked_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("second revoke: invalid json: %v", err)
	}

	// revoked_at must be the same on both revocations.
	if firstResp.RevokedAt != secondResp.RevokedAt {
		t.Errorf("idempotence violated: first=%s, second=%s",
			firstResp.RevokedAt, secondResp.RevokedAt)
	}
}

// TestTokenEndpoints_XRequestIDEchoed verifies Requirement 7.5: X-Request-ID echoing.
func TestTokenEndpoints_XRequestIDEchoed(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed a token for DELETE test.
	tok := db.ApiToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      "req-id-test",
		Prefix:    "reqidtes",
		TokenHash: "hash",
		CreatedAt: time.Now().UTC(),
	}
	fdb.seedToken(tok)

	router := setupTokenRouterWithUser(fdb, userID)
	customRequestID := "test-request-id-12345"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST /tokens", http.MethodPost, "/api/v1/tokens", `{"name":"echo-test"}`},
		{"GET /tokens", http.MethodGet, "/api/v1/tokens", ""},
		{"DELETE /tokens/:id", http.MethodDelete, "/api/v1/tokens/" + tok.ID.String(), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			req.Header.Set("X-Request-ID", customRequestID)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			got := w.Header().Get("X-Request-ID")
			if got != customRequestID {
				t.Errorf("X-Request-ID: expected %q, got %q", customRequestID, got)
			}
		})
	}
}

// TestTokenEndpoints_ContentTypeJSON verifies Requirement 7.8: Content-Type is application/json.
func TestTokenEndpoints_ContentTypeJSON(t *testing.T) {
	fdb := newTokenFakeDB()
	userID := uuid.New()

	// Seed a token for DELETE test.
	tok := db.ApiToken{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      "ct-test",
		Prefix:    "cttestp0",
		TokenHash: "hash",
		CreatedAt: time.Now().UTC(),
	}
	fdb.seedToken(tok)

	router := setupTokenRouterWithUser(fdb, userID)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST /tokens", http.MethodPost, "/api/v1/tokens", `{"name":"ct-test-token"}`},
		{"GET /tokens", http.MethodGet, "/api/v1/tokens", ""},
		{"DELETE /tokens/:id", http.MethodDelete, "/api/v1/tokens/" + tok.ID.String(), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type: expected application/json, got %q", ct)
			}
		})
	}
}
