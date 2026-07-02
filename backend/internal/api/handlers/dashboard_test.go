package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// --- Fake DB for dashboard handler tests ---

// dashboardQueryMode controls what data the fake DB returns.
type dashboardQueryMode int

const (
	modeSuccess       dashboardQueryMode = iota // all queries succeed with mock data
	modePartialFail                             // some queries fail
	modeEmpty                                   // no data in any table
)

// dashboardFakeDB implements db.DBTX for dashboard handler tests.
type dashboardFakeDB struct {
	mode       dashboardQueryMode
	queryCount int // tracks which query is being executed
}

func newDashboardFakeDB(mode dashboardQueryMode) *dashboardFakeDB {
	return &dashboardFakeDB{mode: mode}
}

func (f *dashboardFakeDB) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (f *dashboardFakeDB) Query(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
	switch f.mode {
	case modeEmpty:
		return &emptyRows{}, nil
	case modePartialFail:
		// Fail queries that contain "incidents" or "heatmap"
		if strings.Contains(sql, "incidents") || strings.Contains(sql, "time_bucket") {
			return nil, errors.New("simulated query failure")
		}
		return f.successRows(sql)
	case modeSuccess:
		return f.successRows(sql)
	}
	return &emptyRows{}, nil
}

func (f *dashboardFakeDB) successRows(sql string) (pgx.Rows, error) {
	// Health + distribution query: SELECT state, COUNT(*) ...
	if strings.Contains(sql, "GROUP BY state") {
		return &stateDistRows{
			data: []stateCountRow{
				{state: "up", count: 8},
				{state: "down", count: 1},
				{state: "unknown", count: 1},
			},
			idx: -1,
		}, nil
	}

	// Active incidents query
	if strings.Contains(sql, "resolved_at IS NULL") {
		return &incidentRows{
			data: []incidentRow{
				{
					monitorID:   uuid.New().String(),
					monitorName: "API Gateway",
					startedAt:   time.Now().Add(-2 * time.Hour),
					cause:       strPtr("Connection refused on port 443"),
				},
			},
			idx: -1,
		}, nil
	}

	// Top latency query
	if strings.Contains(sql, "avg_latency_ms") && strings.Contains(sql, "LIMIT 5") {
		return &latencyRows{
			data: []latencyRow{
				{monitorID: uuid.New().String(), monitorName: "Payment Service", avgLatencyMs: 342},
				{monitorID: uuid.New().String(), monitorName: "Auth Service", avgLatencyMs: 215},
			},
			idx: -1,
		}, nil
	}

	// SSL expiry query
	if strings.Contains(sql, "ssl_days_remaining") {
		return &sslRows{
			data: []sslRow{
				{
					monitorID:     uuid.New().String(),
					monitorName:   "Main Site",
					daysRemaining: 12,
					expiresAt:     time.Now().Add(12 * 24 * time.Hour),
				},
			},
			idx: -1,
		}, nil
	}

	// Heatmap query (time_bucket)
	if strings.Contains(sql, "time_bucket") {
		return &heatmapRows{
			data: []heatmapRow{
				{hourStart: time.Now().Add(-2 * time.Hour).Truncate(time.Hour), upCount: 10, downCount: 0, unknownCount: 0},
				{hourStart: time.Now().Add(-1 * time.Hour).Truncate(time.Hour), upCount: 9, downCount: 1, unknownCount: 0},
			},
			idx: -1,
		}, nil
	}

	// Recent events query (incidents ORDER BY started_at DESC)
	if strings.Contains(sql, "ORDER BY i.started_at DESC") {
		return &recentEventRows{
			data: []recentEventRow{
				{
					monitorID:  uuid.New().String(),
					name:       "DB Replica",
					startedAt:  time.Now().Add(-30 * time.Minute),
					resolvedAt: timePtr(time.Now().Add(-15 * time.Minute)),
					cause:      strPtr("Timeout"),
				},
			},
			idx: -1,
		}, nil
	}

	return &emptyRows{}, nil
}

func (f *dashboardFakeDB) QueryRow(_ context.Context, sql string, _ ...interface{}) pgx.Row {
	switch f.mode {
	case modeEmpty:
		return &scalarRow{value: 0.0}
	case modePartialFail:
		// The uptime query uses QueryRow — let it succeed
		return &scalarRow{value: 99.95}
	case modeSuccess:
		return &scalarRow{value: 99.95}
	}
	return &scalarRow{value: 0.0}
}

// --- Helpers ---

func strPtr(s string) *string { return &s }

func timePtr(t time.Time) *time.Time { return &t }

// --- Row types for fake query results ---

// scalarRow implements pgx.Row for single-value queries (e.g., AVG uptime).
type scalarRow struct {
	value float64
}

func (r *scalarRow) Scan(dest ...interface{}) error {
	if len(dest) > 0 {
		*dest[0].(*float64) = r.value
	}
	return nil
}

// emptyRows implements pgx.Rows with zero results.
type emptyRows struct{}

func (r *emptyRows) Close()                                        {}
func (r *emptyRows) Err() error                                    { return nil }
func (r *emptyRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *emptyRows) RawValues() [][]byte                           { return nil }
func (r *emptyRows) Conn() *pgx.Conn                              { return nil }
func (r *emptyRows) Next() bool                                    { return false }
func (r *emptyRows) Scan(_ ...interface{}) error                   { return nil }
func (r *emptyRows) Values() ([]interface{}, error)                { return nil, nil }

// stateDistRows returns state distribution data.
type stateCountRow struct {
	state string
	count int
}

type stateDistRows struct {
	data []stateCountRow
	idx  int
}

func (r *stateDistRows) Close()                                        {}
func (r *stateDistRows) Err() error                                    { return nil }
func (r *stateDistRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *stateDistRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stateDistRows) RawValues() [][]byte                           { return nil }
func (r *stateDistRows) Conn() *pgx.Conn                              { return nil }
func (r *stateDistRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *stateDistRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *stateDistRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*string) = row.state
	*dest[1].(*int) = row.count
	return nil
}

// incidentRows returns active incident data.
type incidentRow struct {
	monitorID   string
	monitorName string
	startedAt   time.Time
	cause       *string
}

type incidentRows struct {
	data []incidentRow
	idx  int
}

func (r *incidentRows) Close()                                        {}
func (r *incidentRows) Err() error                                    { return nil }
func (r *incidentRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *incidentRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *incidentRows) RawValues() [][]byte                           { return nil }
func (r *incidentRows) Conn() *pgx.Conn                              { return nil }
func (r *incidentRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *incidentRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *incidentRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*string) = row.monitorID
	*dest[1].(*string) = row.monitorName
	*dest[2].(*time.Time) = row.startedAt
	*dest[3].(**string) = row.cause
	return nil
}

// latencyRows returns top latency data.
type latencyRow struct {
	monitorID    string
	monitorName  string
	avgLatencyMs int
}

type latencyRows struct {
	data []latencyRow
	idx  int
}

func (r *latencyRows) Close()                                        {}
func (r *latencyRows) Err() error                                    { return nil }
func (r *latencyRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *latencyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *latencyRows) RawValues() [][]byte                           { return nil }
func (r *latencyRows) Conn() *pgx.Conn                              { return nil }
func (r *latencyRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *latencyRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *latencyRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*string) = row.monitorID
	*dest[1].(*string) = row.monitorName
	*dest[2].(*int) = row.avgLatencyMs
	return nil
}

// sslRows returns SSL expiry data.
type sslRow struct {
	monitorID     string
	monitorName   string
	daysRemaining int
	expiresAt     time.Time
}

type sslRows struct {
	data []sslRow
	idx  int
}

func (r *sslRows) Close()                                        {}
func (r *sslRows) Err() error                                    { return nil }
func (r *sslRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *sslRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *sslRows) RawValues() [][]byte                           { return nil }
func (r *sslRows) Conn() *pgx.Conn                              { return nil }
func (r *sslRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *sslRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *sslRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*string) = row.monitorID
	*dest[1].(*string) = row.monitorName
	*dest[2].(*int) = row.daysRemaining
	*dest[3].(*time.Time) = row.expiresAt
	return nil
}

// heatmapRows returns heatmap data.
type heatmapRow struct {
	hourStart    time.Time
	upCount      int
	downCount    int
	unknownCount int
}

type heatmapRows struct {
	data []heatmapRow
	idx  int
}

func (r *heatmapRows) Close()                                        {}
func (r *heatmapRows) Err() error                                    { return nil }
func (r *heatmapRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *heatmapRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *heatmapRows) RawValues() [][]byte                           { return nil }
func (r *heatmapRows) Conn() *pgx.Conn                              { return nil }
func (r *heatmapRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *heatmapRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *heatmapRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*time.Time) = row.hourStart
	*dest[1].(*int) = row.upCount
	*dest[2].(*int) = row.downCount
	*dest[3].(*int) = row.unknownCount
	return nil
}

// recentEventRows returns recent events data.
type recentEventRow struct {
	monitorID  string
	name       string
	startedAt  time.Time
	resolvedAt *time.Time
	cause      *string
}

type recentEventRows struct {
	data []recentEventRow
	idx  int
}

func (r *recentEventRows) Close()                                        {}
func (r *recentEventRows) Err() error                                    { return nil }
func (r *recentEventRows) CommandTag() pgconn.CommandTag                 { return pgconn.NewCommandTag("") }
func (r *recentEventRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *recentEventRows) RawValues() [][]byte                           { return nil }
func (r *recentEventRows) Conn() *pgx.Conn                              { return nil }
func (r *recentEventRows) Values() ([]interface{}, error)                { return nil, nil }

func (r *recentEventRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *recentEventRows) Scan(dest ...interface{}) error {
	row := r.data[r.idx]
	*dest[0].(*string) = row.monitorID
	*dest[1].(*string) = row.name
	*dest[2].(*time.Time) = row.startedAt
	*dest[3].(**time.Time) = row.resolvedAt
	*dest[4].(**string) = row.cause
	return nil
}

// --- Test setup helpers ---

var testJWTSecret = []byte("test-jwt-secret-for-dashboard-tests")

// setupDashboardRouter creates a gin router with injected user_id (bypasses auth).
func setupDashboardRouter(fdb *dashboardFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	userID := uuid.New()

	// Inject user_id directly (simulates auth middleware passing).
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Next()
	})

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")
	h := handlers.NewDashboardHandlerWithDB(queries, fdb)
	h.Register(v1)

	return r
}

// setupDashboardRouterWithJWTAuth creates a gin router with real JWT auth middleware.
func setupDashboardRouterWithJWTAuth(fdb *dashboardFakeDB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	queries := db.New(fdb)
	v1 := r.Group("/api/v1")

	protected := v1.Group("")
	protected.Use(middleware.JWTAuth(testJWTSecret))

	h := handlers.NewDashboardHandlerWithDB(queries, fdb)
	h.Register(protected)

	return r
}

// --- Test cases ---

// TestDashboardSummary_Success tests successful aggregate response with all
// sub-queries returning data. Verifies JSON structure and 200 status code.
//
// Validates: Requirements 1.1, 8.3
func TestDashboardSummary_Success(t *testing.T) {
	fdb := newDashboardFakeDB(modeSuccess)
	router := setupDashboardRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// Verify top-level response structure.
	requiredFields := []string{
		"health_score",
		"status_distribution",
		"active_incidents",
		"top_latency_monitors",
		"ssl_expiry",
		"heatmap",
		"recent_events",
		"generated_at",
		"partial_data",
	}
	for _, field := range requiredFields {
		if _, ok := resp[field]; !ok {
			t.Errorf("response missing required field: %s", field)
		}
	}

	// partial_data should be false on success.
	if resp["partial_data"] != false {
		t.Errorf("expected partial_data=false, got %v", resp["partial_data"])
	}

	// Verify health_score sub-object.
	hs, ok := resp["health_score"].(map[string]interface{})
	if !ok {
		t.Fatal("health_score is not an object")
	}
	if hs["uptime_percent"] != 99.95 {
		t.Errorf("expected uptime_percent=99.95, got %v", hs["uptime_percent"])
	}
	if hs["active_monitor_count"] != float64(10) {
		t.Errorf("expected active_monitor_count=10, got %v", hs["active_monitor_count"])
	}
	if hs["partial_data"] != false {
		t.Errorf("expected health_score.partial_data=false, got %v", hs["partial_data"])
	}

	// Verify status_distribution.
	sd, ok := resp["status_distribution"].(map[string]interface{})
	if !ok {
		t.Fatal("status_distribution is not an object")
	}
	if sd["up"] != float64(8) {
		t.Errorf("expected up=8, got %v", sd["up"])
	}
	if sd["down"] != float64(1) {
		t.Errorf("expected down=1, got %v", sd["down"])
	}
	if sd["unknown"] != float64(1) {
		t.Errorf("expected unknown=1, got %v", sd["unknown"])
	}
	if sd["total"] != float64(10) {
		t.Errorf("expected total=10, got %v", sd["total"])
	}

	// Verify active_incidents is a non-empty array.
	incidents, ok := resp["active_incidents"].([]interface{})
	if !ok {
		t.Fatal("active_incidents is not an array")
	}
	if len(incidents) != 1 {
		t.Errorf("expected 1 incident, got %d", len(incidents))
	}
	if len(incidents) > 0 {
		inc := incidents[0].(map[string]interface{})
		if inc["monitor_name"] != "API Gateway" {
			t.Errorf("expected monitor_name='API Gateway', got %v", inc["monitor_name"])
		}
		if inc["state"] != "down" {
			t.Errorf("expected state='down', got %v", inc["state"])
		}
	}

	// Verify top_latency_monitors is populated.
	latency, ok := resp["top_latency_monitors"].([]interface{})
	if !ok {
		t.Fatal("top_latency_monitors is not an array")
	}
	if len(latency) != 2 {
		t.Errorf("expected 2 latency monitors, got %d", len(latency))
	}

	// Verify ssl_expiry is populated.
	ssl, ok := resp["ssl_expiry"].([]interface{})
	if !ok {
		t.Fatal("ssl_expiry is not an array")
	}
	if len(ssl) != 1 {
		t.Errorf("expected 1 ssl entry, got %d", len(ssl))
	}

	// Verify heatmap is populated.
	hm, ok := resp["heatmap"].([]interface{})
	if !ok {
		t.Fatal("heatmap is not an array")
	}
	if len(hm) != 2 {
		t.Errorf("expected 2 heatmap entries, got %d", len(hm))
	}

	// Verify recent_events is populated.
	events, ok := resp["recent_events"].([]interface{})
	if !ok {
		t.Fatal("recent_events is not an array")
	}
	if len(events) == 0 {
		t.Error("expected at least 1 recent event")
	}

	// Verify generated_at is a valid timestamp.
	genAt, ok := resp["generated_at"].(string)
	if !ok || genAt == "" {
		t.Error("generated_at should be a non-empty string")
	}
	if _, err := time.Parse(time.RFC3339, genAt); err != nil {
		t.Errorf("generated_at is not valid RFC3339: %v", err)
	}

	// Verify Content-Type is JSON.
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// TestDashboardSummary_PartialFailure tests that when sub-queries fail,
// partial results are still returned with partial_data: true.
//
// Validates: Requirements 1.5, 8.3
func TestDashboardSummary_PartialFailure(t *testing.T) {
	fdb := newDashboardFakeDB(modePartialFail)
	router := setupDashboardRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still return 200 (partial results, not a 500).
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with partial data, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// partial_data must be true when sub-queries fail.
	if resp["partial_data"] != true {
		t.Errorf("expected partial_data=true, got %v", resp["partial_data"])
	}

	// health_score.partial_data must also be true.
	hs, ok := resp["health_score"].(map[string]interface{})
	if !ok {
		t.Fatal("health_score is not an object")
	}
	if hs["partial_data"] != true {
		t.Errorf("expected health_score.partial_data=true, got %v", hs["partial_data"])
	}

	// Successful sub-queries should still populate their data.
	// Status distribution should be populated (its query succeeded).
	sd, ok := resp["status_distribution"].(map[string]interface{})
	if !ok {
		t.Fatal("status_distribution is not an object")
	}
	if sd["total"] != float64(10) {
		t.Errorf("expected total=10, got %v", sd["total"])
	}

	// Top latency should be populated (its query succeeded).
	latency, ok := resp["top_latency_monitors"].([]interface{})
	if !ok {
		t.Fatal("top_latency_monitors is not an array")
	}
	if len(latency) != 2 {
		t.Errorf("expected 2 latency monitors, got %d", len(latency))
	}

	// Failed sub-queries return empty arrays (not nil).
	// Heatmap query failed, so it should be empty.
	hm, ok := resp["heatmap"].([]interface{})
	if !ok {
		t.Fatal("heatmap is not an array")
	}
	if len(hm) != 0 {
		t.Errorf("expected 0 heatmap entries (query failed), got %d", len(hm))
	}

	// All arrays must be present (never null in JSON).
	arrayFields := []string{"active_incidents", "top_latency_monitors", "ssl_expiry", "heatmap", "recent_events"}
	for _, field := range arrayFields {
		val := resp[field]
		if val == nil {
			t.Errorf("%s should be an empty array, not null", field)
		}
	}
}

// TestDashboardSummary_Unauthorized tests that a request without a valid token
// returns 401 Unauthorized.
//
// Validates: Requirements 8.3
func TestDashboardSummary_Unauthorized(t *testing.T) {
	fdb := newDashboardFakeDB(modeSuccess)
	router := setupDashboardRouterWithJWTAuth(fdb)

	tests := []struct {
		name       string
		authHeader string
	}{
		{"no auth header", ""},
		{"empty bearer", "Bearer "},
		{"invalid token", "Bearer invalid-token-value"},
		{"malformed header", "Basic dXNlcjpwYXNz"},
		{"expired JWT", generateExpiredJWT()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}

			// Verify error envelope.
			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			errObj, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error envelope in response")
			}
			if errObj["code"] != "UNAUTHORIZED" {
				t.Errorf("expected code=UNAUTHORIZED, got %v", errObj["code"])
			}
		})
	}
}

// TestDashboardSummary_EmptyMonitors tests that when no monitors exist,
// the response returns appropriate defaults (0 health score, empty arrays).
//
// Validates: Requirements 1.1, 8.3
func TestDashboardSummary_EmptyMonitors(t *testing.T) {
	fdb := newDashboardFakeDB(modeEmpty)
	router := setupDashboardRouter(fdb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// partial_data should be false (no failures, just empty).
	if resp["partial_data"] != false {
		t.Errorf("expected partial_data=false, got %v", resp["partial_data"])
	}

	// Health score should show 0 uptime and 0 monitors.
	hs, ok := resp["health_score"].(map[string]interface{})
	if !ok {
		t.Fatal("health_score is not an object")
	}
	if hs["uptime_percent"] != float64(0) {
		t.Errorf("expected uptime_percent=0, got %v", hs["uptime_percent"])
	}
	if hs["active_monitor_count"] != float64(0) {
		t.Errorf("expected active_monitor_count=0, got %v", hs["active_monitor_count"])
	}

	// Status distribution should be all zeros.
	sd, ok := resp["status_distribution"].(map[string]interface{})
	if !ok {
		t.Fatal("status_distribution is not an object")
	}
	if sd["up"] != float64(0) {
		t.Errorf("expected up=0, got %v", sd["up"])
	}
	if sd["down"] != float64(0) {
		t.Errorf("expected down=0, got %v", sd["down"])
	}
	if sd["unknown"] != float64(0) {
		t.Errorf("expected unknown=0, got %v", sd["unknown"])
	}
	if sd["total"] != float64(0) {
		t.Errorf("expected total=0, got %v", sd["total"])
	}

	// All array fields should be empty arrays (not null).
	arrayFields := []string{
		"active_incidents",
		"top_latency_monitors",
		"ssl_expiry",
		"heatmap",
		"recent_events",
	}
	for _, field := range arrayFields {
		arr, ok := resp[field].([]interface{})
		if !ok {
			t.Errorf("%s should be an array, got %T", field, resp[field])
			continue
		}
		if len(arr) != 0 {
			t.Errorf("expected %s to be empty, got %d items", field, len(arr))
		}
	}

	// generated_at should still be a valid timestamp.
	genAt, ok := resp["generated_at"].(string)
	if !ok || genAt == "" {
		t.Error("generated_at should be a non-empty string")
	}
}

// --- Helper functions ---

// generateExpiredJWT creates a JWT token that has already expired.
func generateExpiredJWT() string {
	token, _, _ := middleware.GenerateJWT(
		testJWTSecret,
		uuid.New(),
		"test@example.com",
		-1*time.Hour, // expired 1 hour ago
	)
	return "Bearer " + token
}
