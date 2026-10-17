package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// setupHeartbeatHistoryRouter wires the heartbeat history handler against the
// shared fake DB with a nil Timescale store. All branches exercised here
// (step/id/from/to validation and 404) return before the store is touched, so
// the nil store is never dereferenced. The store-backed happy path is covered
// by the DB-gated timescale round-trip tests (FEAT-001).
func setupHeartbeatHistoryRouter(fdb *heartbeatFakeDB) *gin.Engine {
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
	h := handlers.NewHeartbeatHistoryHandler(queries, nil)
	h.Register(v1)
	return r
}

func historyErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal error envelope: %v (body=%s)", err, body)
	}
	return env.Error.Code
}

func TestHeartbeatHistory_InvalidID(t *testing.T) {
	router := setupHeartbeatHistoryRouter(newHeartbeatFakeDB())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/not-a-uuid/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
	if code := historyErrorCode(t, w.Body.Bytes()); code != "INVALID_ID" {
		t.Fatalf("expected INVALID_ID, got %s", code)
	}
}

func TestHeartbeatHistory_InvalidStep(t *testing.T) {
	router := setupHeartbeatHistoryRouter(newHeartbeatFakeDB())

	cases := []string{"abc", "59", "86401", "0", "-1"}
	for _, step := range cases {
		// Use a valid-looking UUID so step validation (which runs first) is the
		// branch under test.
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/heartbeats/"+uuid.New().String()+"/history?step="+url.QueryEscape(step), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("step=%q: expected 400, got %d (body=%s)", step, w.Code, w.Body.String())
		}
		if code := historyErrorCode(t, w.Body.Bytes()); code != "INVALID_STEP" {
			t.Fatalf("step=%q: expected INVALID_STEP, got %s", step, code)
		}
	}
}

func TestHeartbeatHistory_NotFound(t *testing.T) {
	router := setupHeartbeatHistoryRouter(newHeartbeatFakeDB())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/"+uuid.New().String()+"/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	if code := historyErrorCode(t, w.Body.Bytes()); code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %s", code)
	}
}

func TestHeartbeatHistory_InvalidFromTo(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	hb.HistoryRetentionDays = 30
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatHistoryRouter(fdb)

	cases := []struct {
		name  string
		query string
	}{
		{"bad from", "?from=not-a-time"},
		{"bad to", "?to=not-a-time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/heartbeats/"+hb.ID.String()+"/history"+tc.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
			}
			if code := historyErrorCode(t, w.Body.Bytes()); code != "VALIDATION_ERROR" {
				t.Fatalf("expected VALIDATION_ERROR, got %s", code)
			}
		})
	}
}

func TestHeartbeatHistory_FromNotBeforeTo(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb := seedTestHeartbeat("active", "up")
	hb.HistoryRetentionDays = 30
	fdb.seedHeartbeat(hb)
	router := setupHeartbeatHistoryRouter(fdb)

	// from == to and from > to must both be rejected.
	t0 := "2026-07-01T00:00:00Z"
	t1 := "2026-07-02T00:00:00Z"
	cases := []struct {
		name  string
		query string
	}{
		{"equal", "?from=" + t0 + "&to=" + t0},
		{"reversed", "?from=" + t1 + "&to=" + t0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/heartbeats/"+hb.ID.String()+"/history"+tc.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
			}
			if code := historyErrorCode(t, w.Body.Bytes()); code != "VALIDATION_ERROR" {
				t.Fatalf("expected VALIDATION_ERROR, got %s", code)
			}
		})
	}
}

// NOTE: retention clamping and summary math are pure functions tested in the
// in-package test file heartbeat_history_internal_test.go. The store-backed
// happy path (events + summary over a clamped window) is validated by the
// DB-gated timescale round-trip tests (FEAT-001).
