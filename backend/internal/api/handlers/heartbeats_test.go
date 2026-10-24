package handlers_test

import (
	"context"
	"encoding/json"
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
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// --- Fake db.DBTX for heartbeat handler tests ---
//
// Covers the read / delete / rotate-token / binding / ping query paths that do
// not go through a pgxpool transaction. Transactional POST/PUT success is
// exercised by the DB-backed store integration test (Phase 1); here the handler
// tests focus on validation, auth, DTO shape, token-leak prevention, and the
// atomic ping transitions, which the fake can model faithfully.

type heartbeatFakeDB struct {
	mu         sync.Mutex
	heartbeats map[uuid.UUID]db.Heartbeat
	bindings   map[uuid.UUID]db.HeartbeatBinding
	channels   map[uuid.UUID]bool
	deleted    []uuid.UUID
	clearState []uuid.UUID
}

func newHeartbeatFakeDB() *heartbeatFakeDB {
	return &heartbeatFakeDB{
		heartbeats: make(map[uuid.UUID]db.Heartbeat),
		bindings:   make(map[uuid.UUID]db.HeartbeatBinding),
		channels:   make(map[uuid.UUID]bool),
	}
}

func (f *heartbeatFakeDB) seedHeartbeat(hb db.Heartbeat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeats[hb.ID] = hb
}

func (f *heartbeatFakeDB) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case strings.Contains(sql, "name: DeleteHeartbeat "):
		id := args[0].(uuid.UUID)
		delete(f.heartbeats, id)
		f.deleted = append(f.deleted, id)
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	// Tag/binding exec statements (DeleteHeartbeatTags, InsertHeartbeatTag,
	// DeleteHeartbeatBinding) are no-ops in these read-path tests.
	return pgconn.NewCommandTag("OK"), nil
}

func (f *heartbeatFakeDB) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case strings.Contains(sql, "name: GetHeartbeatByTokenPrefix"):
		prefix := args[0].(string)
		var out []db.GetHeartbeatByTokenPrefixRow
		for _, hb := range f.heartbeats {
			if hb.TokenPrefix == prefix {
				out = append(out, db.GetHeartbeatByTokenPrefixRow{
					ID: hb.ID, State: hb.State, Status: hb.Status,
					TokenHash: hb.TokenHash, TokenPrefix: hb.TokenPrefix,
					ExpectedIntervalSeconds: hb.ExpectedIntervalSeconds,
					GraceSeconds:            hb.GraceSeconds,
				})
			}
		}
		return &hbTokenPrefixRows{rows: out, idx: -1}, nil
	case strings.Contains(sql, "name: ListHeartbeats"):
		var out []db.Heartbeat
		for _, hb := range f.heartbeats {
			out = append(out, hb)
		}
		return &heartbeatRows{rows: out, idx: -1}, nil
	case strings.Contains(sql, "name: ListTagsByHeartbeat"):
		return &heartbeatTagRows{idx: -1}, nil
	case strings.Contains(sql, "name: ListBindingsByHeartbeat"):
		hid := args[0].(uuid.UUID)
		var out []db.HeartbeatBinding
		for _, b := range f.bindings {
			if b.HeartbeatID == hid {
				out = append(out, b)
			}
		}
		return &heartbeatBindingRows{rows: out, idx: -1}, nil
	}
	return &heartbeatRows{idx: -1}, nil
}

func (f *heartbeatFakeDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	// Match on the stable sqlc "-- name: Xxx" markers embedded in each query.
	case strings.Contains(sql, "name: MarkHeartbeatUp"):
		id := args[0].(uuid.UUID)
		hb := f.heartbeats[id]
		prev := hb.State
		hb.State = "up"
		hb.LastSuccessAt = args[1].(pgtype.Timestamptz)
		hb.NextExpectedDeadline = args[2].(pgtype.Timestamptz)
		f.heartbeats[id] = hb
		return &markUpRow{hb: hb, prev: prev}
	case strings.Contains(sql, "name: MarkHeartbeatFail"):
		id := args[0].(uuid.UUID)
		hb := f.heartbeats[id]
		prev := hb.State
		hb.State = "down"
		f.heartbeats[id] = hb
		return &markFailRow{hb: hb, prev: prev}
	case strings.Contains(sql, "name: RotateHeartbeatToken"):
		id := args[0].(uuid.UUID)
		hb, ok := f.heartbeats[id]
		if !ok {
			return &heartbeatRow{err: pgx.ErrNoRows}
		}
		hb.TokenPrefix = args[1].(string)
		hb.TokenHash = args[2].(string)
		hb.TokenCreatedAt = time.Now().UTC()
		f.heartbeats[id] = hb
		return &heartbeatRow{hb: hb}
	case strings.Contains(sql, "name: CountHeartbeats") && !strings.Contains(sql, "ByState"):
		return &hbScalarRow{count: int64(len(f.heartbeats))}
	case strings.Contains(sql, "name: CountBindingsByHeartbeat"):
		hid := args[0].(uuid.UUID)
		var n int64
		for _, b := range f.bindings {
			if b.HeartbeatID == hid {
				n++
			}
		}
		return &hbScalarRow{count: n}
	case strings.Contains(sql, "name: GetHeartbeat ") || strings.Contains(sql, "name: GetHeartbeatForUpdate"):
		id := args[0].(uuid.UUID)
		hb, ok := f.heartbeats[id]
		if !ok {
			return &heartbeatRow{err: pgx.ErrNoRows}
		}
		return &heartbeatRow{hb: hb}
	case strings.Contains(sql, "name: GetHeartbeatBinding"):
		id := args[0].(uuid.UUID)
		b, ok := f.bindings[id]
		if !ok {
			return &heartbeatBindingRow{err: pgx.ErrNoRows}
		}
		return &heartbeatBindingRow{b: b}
	case strings.Contains(sql, "name: GetChannel"):
		id := args[0].(uuid.UUID)
		if !f.channels[id] {
			return &hbChannelRow{err: pgx.ErrNoRows}
		}
		return &hbChannelRow{id: id}
	case strings.Contains(sql, "name: CreateHeartbeatBinding"):
		b := db.HeartbeatBinding{
			ID:          uuid.New(),
			ChannelID:   args[0].(uuid.UUID),
			HeartbeatID: args[1].(uuid.UUID),
			Triggers:    args[2].(json.RawMessage),
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		f.bindings[b.ID] = b
		return &heartbeatBindingRow{b: b}
	case strings.Contains(sql, "name: UpdateHeartbeatBinding"):
		id := args[0].(uuid.UUID)
		b := f.bindings[id]
		b.Triggers = args[1].(json.RawMessage)
		f.bindings[id] = b
		return &heartbeatBindingRow{b: b}
	}
	return &heartbeatRow{err: pgx.ErrNoRows}
}

// --- fake pgx.Row / pgx.Rows implementations ---

func scanHeartbeat(hb db.Heartbeat, dest []interface{}) error {
	*dest[0].(*uuid.UUID) = hb.ID
	*dest[1].(*string) = hb.Name
	*dest[2].(*int32) = hb.ExpectedIntervalSeconds
	*dest[3].(*int32) = hb.GraceSeconds
	*dest[4].(*string) = hb.Status
	*dest[5].(*string) = hb.State
	*dest[6].(*pgtype.Timestamptz) = hb.LastSuccessAt
	*dest[7].(*pgtype.Timestamptz) = hb.NextExpectedDeadline
	*dest[8].(*string) = hb.TokenPrefix
	*dest[9].(*string) = hb.TokenHash
	*dest[10].(*time.Time) = hb.TokenCreatedAt
	*dest[11].(*time.Time) = hb.CreatedAt
	*dest[12].(*time.Time) = hb.UpdatedAt
	*dest[13].(*int32) = hb.HistoryRetentionDays
	return nil
}

type heartbeatRow struct {
	hb  db.Heartbeat
	err error
}

func (r *heartbeatRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	return scanHeartbeat(r.hb, dest)
}

type heartbeatRows struct {
	rows []db.Heartbeat
	idx  int
}

func (r *heartbeatRows) Close()                                       {}
func (r *heartbeatRows) Err() error                                   { return nil }
func (r *heartbeatRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *heartbeatRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *heartbeatRows) RawValues() [][]byte                          { return nil }
func (r *heartbeatRows) Conn() *pgx.Conn                              { return nil }
func (r *heartbeatRows) TypeMap() *pgtype.Map                         { return pgtype.NewMap() }
func (r *heartbeatRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *heartbeatRows) Next() bool                                   { r.idx++; return r.idx < len(r.rows) }
func (r *heartbeatRows) Scan(dest ...interface{}) error               { return scanHeartbeat(r.rows[r.idx], dest) }

type hbTokenPrefixRows struct {
	rows []db.GetHeartbeatByTokenPrefixRow
	idx  int
}

func (r *hbTokenPrefixRows) Close()                                       {}
func (r *hbTokenPrefixRows) Err() error                                   { return nil }
func (r *hbTokenPrefixRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *hbTokenPrefixRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *hbTokenPrefixRows) RawValues() [][]byte                          { return nil }
func (r *hbTokenPrefixRows) Conn() *pgx.Conn                              { return nil }
func (r *hbTokenPrefixRows) TypeMap() *pgtype.Map                         { return pgtype.NewMap() }
func (r *hbTokenPrefixRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *hbTokenPrefixRows) Next() bool                                   { r.idx++; return r.idx < len(r.rows) }
func (r *hbTokenPrefixRows) Scan(dest ...interface{}) error {
	row := r.rows[r.idx]
	*dest[0].(*uuid.UUID) = row.ID
	*dest[1].(*string) = row.State
	*dest[2].(*string) = row.Status
	*dest[3].(*string) = row.TokenHash
	*dest[4].(*string) = row.TokenPrefix
	*dest[5].(*int32) = row.ExpectedIntervalSeconds
	*dest[6].(*int32) = row.GraceSeconds
	return nil
}

type heartbeatTagRows struct{ idx int }

func (r *heartbeatTagRows) Close()                                       {}
func (r *heartbeatTagRows) Err() error                                   { return nil }
func (r *heartbeatTagRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *heartbeatTagRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *heartbeatTagRows) RawValues() [][]byte                          { return nil }
func (r *heartbeatTagRows) Conn() *pgx.Conn                              { return nil }
func (r *heartbeatTagRows) TypeMap() *pgtype.Map                         { return pgtype.NewMap() }
func (r *heartbeatTagRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *heartbeatTagRows) Next() bool                                   { return false }
func (r *heartbeatTagRows) Scan(_ ...interface{}) error                  { return nil }

func scanBinding(b db.HeartbeatBinding, dest []interface{}) error {
	*dest[0].(*uuid.UUID) = b.ID
	*dest[1].(*uuid.UUID) = b.ChannelID
	*dest[2].(*uuid.UUID) = b.HeartbeatID
	*dest[3].(*json.RawMessage) = b.Triggers
	*dest[4].(**int32) = b.ReminderIntervalMinutes
	*dest[5].(*time.Time) = b.CreatedAt
	*dest[6].(*time.Time) = b.UpdatedAt
	return nil
}

type heartbeatBindingRow struct {
	b   db.HeartbeatBinding
	err error
}

func (r *heartbeatBindingRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	return scanBinding(r.b, dest)
}

type heartbeatBindingRows struct {
	rows []db.HeartbeatBinding
	idx  int
}

func (r *heartbeatBindingRows) Close()                                       {}
func (r *heartbeatBindingRows) Err() error                                   { return nil }
func (r *heartbeatBindingRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *heartbeatBindingRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *heartbeatBindingRows) RawValues() [][]byte                          { return nil }
func (r *heartbeatBindingRows) Conn() *pgx.Conn                              { return nil }
func (r *heartbeatBindingRows) TypeMap() *pgtype.Map                         { return pgtype.NewMap() }
func (r *heartbeatBindingRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *heartbeatBindingRows) Next() bool                                   { r.idx++; return r.idx < len(r.rows) }
func (r *heartbeatBindingRows) Scan(dest ...interface{}) error {
	return scanBinding(r.rows[r.idx], dest)
}

type markUpRow struct {
	hb   db.Heartbeat
	prev string
}

func (r *markUpRow) Scan(dest ...interface{}) error {
	_ = scanHeartbeat(r.hb, dest[:14])
	*dest[14].(*string) = r.prev
	return nil
}

type markFailRow struct {
	hb   db.Heartbeat
	prev string
}

func (r *markFailRow) Scan(dest ...interface{}) error {
	_ = scanHeartbeat(r.hb, dest[:14])
	*dest[14].(*string) = r.prev
	return nil
}

type hbScalarRow struct{ count int64 }

func (r *hbScalarRow) Scan(dest ...interface{}) error {
	*dest[0].(*int64) = r.count
	return nil
}

// hbChannelRow scans the minimal shape used by GetChannel verification. GetChannel
// scans many columns; only existence matters here, so we return ErrNoRows for a
// missing channel and a zero-valued row otherwise. Since the binding handler only
// checks the error, the exact column scan is not asserted.
type hbChannelRow struct {
	id  uuid.UUID
	err error
}

func (r *hbChannelRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	// Best-effort: set the first UUID column (id) and leave the rest zero.
	if len(dest) > 0 {
		if p, ok := dest[0].(*uuid.UUID); ok {
			*p = r.id
		}
	}
	return nil
}

// --- Router setup ---

func setupHeartbeatRouter(fdb *heartbeatFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	})

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")

	// nil pool: transactional create/PUT paths are not exercised here. GET/LIST/
	// DELETE/rotate-token and all pre-pool validation branches do not touch it.
	hbHandler := handlers.NewHeartbeatHandler(queries, nil, "https://pulse.example.com", &recordingClearer{fdb: fdb})
	hbHandler.Register(v1)

	bindingHandler := handlers.NewHeartbeatBindingHandler(queries)
	bindingHandler.Register(v1)

	return r
}

type recordingClearer struct{ fdb *heartbeatFakeDB }

func (c *recordingClearer) ClearState(id uuid.UUID) {
	c.fdb.mu.Lock()
	defer c.fdb.mu.Unlock()
	c.fdb.clearState = append(c.fdb.clearState, id)
}

func seedTestHeartbeat(status, state string) db.Heartbeat {
	return db.Heartbeat{
		ID:                      uuid.New(),
		Name:                    "nightly-backup",
		ExpectedIntervalSeconds: 86400,
		GraceSeconds:            3600,
		Status:                  status,
		State:                   state,
		TokenPrefix:             "a1B2c3D4",
		TokenHash:               "deadbeef",
		TokenCreatedAt:          time.Now().UTC(),
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
	}
}

// --- Tests ---

func TestHeartbeatGet_NeverLeaksTokenOrHash(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/"+hb.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "ping_token") || strings.Contains(body, "token_hash") || strings.Contains(body, hb.TokenHash) {
		t.Fatalf("GET response leaked raw token or hash: %s", body)
	}
	if !strings.Contains(body, "token_prefix") {
		t.Fatalf("GET response should expose token_prefix: %s", body)
	}
}

func TestHeartbeatList_NeverLeaksTokenOrHash(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	fdb.seedHeartbeat(seedTestHeartbeat("active", "up"))
	fdb.seedHeartbeat(seedTestHeartbeat("paused", "new"))
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats?page=1&limit=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("LIST: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "ping_token") || strings.Contains(body, "token_hash") || strings.Contains(body, "deadbeef") {
		t.Fatalf("LIST response leaked raw token or hash: %s", body)
	}
}

func TestHeartbeatGet_PausedStatusMapsToPausedState(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("paused", "up") // stored state up, status paused
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/"+hb.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["state"] != "paused" {
		t.Fatalf("paused heartbeat should present state=paused, got %v", resp["state"])
	}
}

func TestHeartbeatGet_InvalidID(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "INVALID_ID")
}

func TestHeartbeatGet_NotFound(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusNotFound, "NOT_FOUND")
}

func TestHeartbeatCreate_ValidationError(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	// Missing expected_interval_seconds.
	body := `{"name":"job"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestHeartbeatCreate_IntervalOutOfRange(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	body := `{"name":"job","expected_interval_seconds":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestHeartbeatCreate_InvalidTags(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	// Reserved "pulse:" prefix on a tag key triggers INVALID_TAGS.
	body := `{"name":"job","expected_interval_seconds":60,"tags":[{"key":"pulse:x","value":"y"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "INVALID_TAGS")
}

func TestHeartbeatPut_InvalidID(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	body := `{"name":"job","expected_interval_seconds":60}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/heartbeats/not-a-uuid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "INVALID_ID")
}

func TestHeartbeatDelete_Returns204AndClearsState(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/heartbeats/"+hb.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if len(fdb.deleted) != 1 || fdb.deleted[0] != hb.ID {
		t.Fatalf("expected delete of %s, got %v", hb.ID, fdb.deleted)
	}
	if len(fdb.clearState) != 1 || fdb.clearState[0] != hb.ID {
		t.Fatalf("expected ClearState(%s), got %v", hb.ID, fdb.clearState)
	}
}

func TestHeartbeatDelete_InvalidID(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/heartbeats/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "INVALID_ID")
}

func TestHeartbeatRotateToken_ReturnsRawTokenOnce(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+hb.ID.String()+"/rotate-token", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("rotate: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		PingToken   string `json:"ping_token"`
		PingURL     string `json:"ping_url"`
		PingFailURL string `json:"ping_fail_url"`
		TokenPrefix string `json:"token_prefix"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PingToken == "" {
		t.Fatal("rotate response must include a non-empty ping_token")
	}
	if !strings.HasPrefix(resp.PingURL, "https://pulse.example.com/ping/") {
		t.Fatalf("ping_url not built from base URL: %q", resp.PingURL)
	}
	if !strings.HasSuffix(resp.PingFailURL, "/fail") {
		t.Fatalf("ping_fail_url must end in /fail: %q", resp.PingFailURL)
	}
	// The prefix in the response must match the first 8 chars of the raw token.
	if resp.TokenPrefix != resp.PingToken[:8] {
		t.Fatalf("token_prefix %q does not match raw token prefix %q", resp.TokenPrefix, resp.PingToken[:8])
	}
}

func TestHeartbeatRotateToken_NotFound(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	router := setupHeartbeatRouter(fdb)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+uuid.New().String()+"/rotate-token", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusNotFound, "NOT_FOUND")
}

// --- Binding trigger validation ---

func TestHeartbeatBindingCreate_RejectsMonitorTriggers(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	channelID := uuid.New()
	fdb.channels[channelID] = true

	// monitor_down is a monitor trigger — must be rejected for heartbeat bindings.
	body := `{"channel_id":"` + channelID.String() + `","triggers":[{"type":"monitor_down"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+hb.ID.String()+"/notification-bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %q", resp.Error.Code)
	}
	if len(resp.Error.Details) == 0 {
		t.Fatal("expected field-level details rejecting the monitor trigger")
	}
}

func TestHeartbeatBindingCreate_MissingChannelID(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatRouter(fdb)

	body := `{"triggers":[{"type":"heartbeat_missed"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+hb.ID.String()+"/notification-bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestHeartbeatBindingCreate_AcceptsHeartbeatTriggers(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	channelID := uuid.New()
	fdb.channels[channelID] = true
	router := setupHeartbeatRouter(fdb)

	body := `{"channel_id":"` + channelID.String() + `","triggers":[{"type":"heartbeat_missed"},{"type":"heartbeat_recovered"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+hb.ID.String()+"/notification-bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHeartbeatBindingCreate_ReminderOutOfRange(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	fdb.seedHeartbeat(hb)
	channelID := uuid.New()
	fdb.channels[channelID] = true
	router := setupHeartbeatRouter(fdb)

	body := `{"channel_id":"` + channelID.String() + `","triggers":[{"type":"heartbeat_missed"}],"reminder_interval_minutes":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeats/"+hb.ID.String()+"/notification-bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHeartbeatErrorCode(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

// assertHeartbeatErrorCode decodes the standard error envelope and asserts status + code.
func assertHeartbeatErrorCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error envelope: %v (body=%s)", err, w.Body.String())
	}
	if resp.Error.Code != code {
		t.Fatalf("expected error code %q, got %q (body=%s)", code, resp.Error.Code, w.Body.String())
	}
}
