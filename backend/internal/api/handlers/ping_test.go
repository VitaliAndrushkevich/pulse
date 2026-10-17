package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/heartbeat"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// recordingPingDispatcher captures DispatchHeartbeat calls so tests can assert
// the recovery path fires with the correct previous state.
type recordingPingDispatcher struct {
	calls []pingDispatchCall
}

type pingDispatchCall struct {
	id       uuid.UUID
	newState string
	prevState string
}

func (d *recordingPingDispatcher) DispatchHeartbeat(_ *gin.Context, hb db.Heartbeat, newState, previousState string) {
	d.calls = append(d.calls, pingDispatchCall{id: hb.ID, newState: newState, prevState: previousState})
}

// recordingEventWriter captures WriteHeartbeatEvent calls so tests can assert the
// ping path records exactly one timeline event with the right type/previous_state.
type recordingEventWriter struct {
	mu    sync.Mutex
	calls []timescale.HeartbeatEvent
}

func (w *recordingEventWriter) WriteHeartbeatEvent(_ context.Context, ev timescale.HeartbeatEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, ev)
	return nil
}

func (w *recordingEventWriter) snapshot() []timescale.HeartbeatEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]timescale.HeartbeatEvent, len(w.calls))
	copy(out, w.calls)
	return out
}

// erroringEventWriter always fails, to prove a write error is swallowed and the
// ping still returns 200.
type erroringEventWriter struct{}

func (erroringEventWriter) WriteHeartbeatEvent(_ context.Context, _ timescale.HeartbeatEvent) error {
	return errors.New("boom")
}

func setupPingRouter(fdb *heartbeatFakeDB, disp handlers.PingDispatcher) *gin.Engine {
	return setupPingRouterWithEvents(fdb, disp, nil)
}

func setupPingRouterWithEvents(fdb *heartbeatFakeDB, disp handlers.PingDispatcher, events timescale.HeartbeatEventWriter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// A fresh private registry avoids clashing with other tests' metrics.
	metrics := heartbeat.NewMetrics(prometheus.NewRegistry())
	ph := handlers.NewPingHandler(db.New(fdb), nil, metrics, disp, events)
	ph.RegisterPingRoutes(r)
	return r
}

// seedHeartbeatWithToken creates a heartbeat whose token_prefix/token_hash match
// a freshly generated raw token, and returns the raw token for use in the URL.
func seedHeartbeatWithToken(t *testing.T, fdb *heartbeatFakeDB, status, state string) (db.Heartbeat, string) {
	t.Helper()
	raw, prefix, err := token.GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw: %v", err)
	}
	hb := db.Heartbeat{
		ID:                      uuid.New(),
		Name:                    "nightly-backup",
		ExpectedIntervalSeconds: 3600,
		GraceSeconds:            300,
		Status:                  status,
		State:                   state,
		TokenPrefix:             prefix,
		TokenHash:               token.HashRawSHA256(raw),
		TokenCreatedAt:          time.Now().UTC(),
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
	}
	fdb.seedHeartbeat(hb)
	return hb, raw
}

func TestPingSuccess_RecordsAndReturns200(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "active", "new")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "OK" {
		t.Fatalf("ping success: expected 200 OK, got %d %q", w.Code, w.Body.String())
	}

	fdb.mu.Lock()
	updated := fdb.heartbeats[hb.ID]
	fdb.mu.Unlock()
	if updated.State != "up" {
		t.Fatalf("expected state up after success ping, got %q", updated.State)
	}
	if !updated.LastSuccessAt.Valid {
		t.Fatal("success ping should set last_success_at")
	}
	if !updated.NextExpectedDeadline.Valid {
		t.Fatal("success ping should set next_expected_deadline")
	}
}

func TestPingSuccess_FromDownFiresRecovery(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "active", "down")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	req := httptest.NewRequest(http.MethodPost, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("expected exactly 1 dispatch call, got %d", len(disp.calls))
	}
	call := disp.calls[0]
	if call.id != hb.ID || call.newState != "up" || call.prevState != "down" {
		t.Fatalf("recovery dispatch wrong: %+v (want id=%s new=up prev=down)", call, hb.ID)
	}
}

func TestPingFail_TransitionsDownWithoutAdvancingSuccess(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "active", "up")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw+"/fail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "OK" {
		t.Fatalf("ping fail: expected 200 OK, got %d %q", w.Code, w.Body.String())
	}

	fdb.mu.Lock()
	updated := fdb.heartbeats[hb.ID]
	fdb.mu.Unlock()
	if updated.State != "down" {
		t.Fatalf("expected state down after fail ping, got %q", updated.State)
	}
	if updated.LastSuccessAt.Valid {
		t.Fatal("fail ping must NOT advance last_success_at")
	}
	if len(disp.calls) != 1 || disp.calls[0].newState != "down" || disp.calls[0].prevState != "up" {
		t.Fatalf("fail dispatch wrong: %+v", disp.calls)
	}
}

func TestPing_UnknownTokenReturns404(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	// Seed a valid heartbeat so the prefix table is non-empty, then ping with a
	// different, well-formed token that will not match any prefix.
	seedHeartbeatWithToken(t, fdb, "active", "up")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	bogus, _, err := token.GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ping/"+bogus, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token: expected 404, got %d", w.Code)
	}
	if len(disp.calls) != 0 {
		t.Fatalf("no dispatch should occur for an unknown token, got %d", len(disp.calls))
	}
}

func TestPing_WrongHashSamePrefixReturns404(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	_, raw := seedHeartbeatWithToken(t, fdb, "active", "up")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	// Same 8-char prefix, tampered remainder → prefix matches, hash does not.
	tampered := raw[:8] + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	req := httptest.NewRequest(http.MethodGet, "/ping/"+tampered, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("tampered token: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if len(disp.calls) != 0 {
		t.Fatalf("no dispatch should occur for a hash mismatch, got %d", len(disp.calls))
	}
}

func TestPing_ShortTokenReturns404(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	req := httptest.NewRequest(http.MethodGet, "/ping/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("short token: expected 404, got %d", w.Code)
	}
}

func TestPingSuccess_PausedHeartbeatStillRecords(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "paused", "up")
	disp := &recordingPingDispatcher{}
	router := setupPingRouter(fdb, disp)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("paused ping: expected 200, got %d", w.Code)
	}
	fdb.mu.Lock()
	updated := fdb.heartbeats[hb.ID]
	fdb.mu.Unlock()
	if !updated.LastSuccessAt.Valid {
		t.Fatal("paused heartbeat success ping should still record last_success_at")
	}
}

func TestPingSuccess_WritesSuccessEvent(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "active", "down")
	events := &recordingEventWriter{}
	router := setupPingRouterWithEvents(fdb, &recordingPingDispatcher{}, events)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	calls := events.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 event write, got %d", len(calls))
	}
	ev := calls[0]
	if ev.HeartbeatID != hb.ID {
		t.Errorf("event HeartbeatID = %s; want %s", ev.HeartbeatID, hb.ID)
	}
	if ev.EventType != "success" {
		t.Errorf("event EventType = %q; want success", ev.EventType)
	}
	if ev.PreviousState == nil || *ev.PreviousState != "down" {
		t.Errorf("event PreviousState = %v; want down", ev.PreviousState)
	}
	if ev.OccurredAt.IsZero() {
		t.Error("event OccurredAt should be set")
	}
}

func TestPingFail_WritesFailEvent(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	hb, raw := seedHeartbeatWithToken(t, fdb, "active", "up")
	events := &recordingEventWriter{}
	router := setupPingRouterWithEvents(fdb, &recordingPingDispatcher{}, events)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw+"/fail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	calls := events.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 event write, got %d", len(calls))
	}
	ev := calls[0]
	if ev.HeartbeatID != hb.ID || ev.EventType != "fail" {
		t.Errorf("event = %+v; want id=%s type=fail", ev, hb.ID)
	}
	if ev.PreviousState == nil || *ev.PreviousState != "up" {
		t.Errorf("event PreviousState = %v; want up", ev.PreviousState)
	}
}

func TestPingSuccess_NilEventWriterIsNoOp(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	_, raw := seedHeartbeatWithToken(t, fdb, "active", "up")
	// Explicit nil writer — the ping must still succeed with no panic.
	router := setupPingRouterWithEvents(fdb, &recordingPingDispatcher{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "OK" {
		t.Fatalf("nil event writer: expected 200 OK, got %d %q", w.Code, w.Body.String())
	}
}

func TestPingSuccess_EventWriterErrorStillReturns200(t *testing.T) {
	fdb := newHeartbeatFakeDB()
	_, raw := seedHeartbeatWithToken(t, fdb, "active", "up")
	router := setupPingRouterWithEvents(fdb, &recordingPingDispatcher{}, erroringEventWriter{})

	req := httptest.NewRequest(http.MethodGet, "/ping/"+raw, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "OK" {
		t.Fatalf("event writer error should be swallowed: expected 200 OK, got %d %q", w.Code, w.Body.String())
	}
}
