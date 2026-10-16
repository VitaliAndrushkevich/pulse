package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/VitaliAndrushkevich/pulse/internal/heartbeat"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// PingDispatcher is the seam the ping handler uses to fire the heartbeat
// notification path after a success or failure ping. The concrete heartbeat
// dispatch helper is wired in Phase 4; a nil dispatcher is valid and simply
// skips notification dispatch (the DB write and WS broadcast still happen).
type PingDispatcher interface {
	// DispatchHeartbeat fires the notification path for an observed heartbeat
	// state transition. newState is "up" (success) or "down" (explicit fail);
	// previousState is the state read atomically with the write.
	DispatchHeartbeat(c *gin.Context, hb db.Heartbeat, newState, previousState string)
}

// PingHandler serves the root-level, unauthenticated heartbeat ping endpoints.
// Authorization is possession of the unguessable token in the URL — never a
// JWT/API-token/session. The handler holds only *db.Queries (no pool): the
// success/fail writes are single atomic UPDATE … RETURNING statements that read
// the prior state together with the write (design §4.1, §4.3).
type PingHandler struct {
	queries    *db.Queries
	hub        *hub.Hub         // WebSocket broadcast hub (may be nil)
	metrics    *heartbeat.Metrics // Prometheus gauges (may be nil)
	dispatcher PingDispatcher   // notification seam (may be nil until Phase 4)
}

// NewPingHandler creates a ping handler. hub, metrics, and dispatcher may be nil.
func NewPingHandler(queries *db.Queries, wsHub *hub.Hub, metrics *heartbeat.Metrics, dispatcher PingDispatcher) *PingHandler {
	return &PingHandler{queries: queries, hub: wsHub, metrics: metrics, dispatcher: dispatcher}
}

// RegisterPingRoutes registers the four root-level ping routes directly on the
// gin engine (outside /api/v1, like /ws). Both GET and POST are accepted because
// cron wrappers and CI runners vary; request bodies are ignored (design §4.1).
func (h *PingHandler) RegisterPingRoutes(r gin.IRoutes) {
	r.GET("/ping/:token", h.Success)
	r.POST("/ping/:token", h.Success)
	r.GET("/ping/:token/fail", h.Fail)
	r.POST("/ping/:token/fail", h.Fail)
}

// Success handles GET|POST /ping/:token — a job reporting a successful run.
func (h *PingHandler) Success(c *gin.Context) {
	hb, ok := h.verifyToken(c)
	if !ok {
		return
	}

	now := time.Now().UTC()
	deadline := heartbeat.NextDeadline(now, int(hb.ExpectedIntervalSeconds), int(hb.GraceSeconds))

	updated, err := h.queries.MarkHeartbeatUp(c.Request.Context(), db.MarkHeartbeatUpParams{
		ID:                   hb.ID,
		LastSuccessAt:        pgtype.Timestamptz{Time: now, Valid: true},
		NextExpectedDeadline: pgtype.Timestamptz{Time: deadline, Valid: true},
	})
	if err != nil {
		log.Printf("ping: mark up failed heartbeat_prefix=%s: %v", hb.TokenPrefix, err)
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to record ping")
		return
	}

	row := markUpRowToHeartbeat(updated)

	// Non-blocking side effects after the write (design §4.3). A paused heartbeat
	// still records success; the evaluator independently skips paused rows, so no
	// special-casing is needed here.
	h.afterTransition(c, row, "up", updated.PreviousState)

	c.String(http.StatusOK, "OK")
}

// Fail handles GET|POST /ping/:token/fail — a job reporting an explicit failure.
func (h *PingHandler) Fail(c *gin.Context) {
	hb, ok := h.verifyToken(c)
	if !ok {
		return
	}

	updated, err := h.queries.MarkHeartbeatFail(c.Request.Context(), hb.ID)
	if err != nil {
		log.Printf("ping: mark fail failed heartbeat_prefix=%s: %v", hb.TokenPrefix, err)
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to record ping")
		return
	}

	row := markFailRowToHeartbeat(updated)
	h.afterTransition(c, row, "down", updated.PreviousState)

	c.String(http.StatusOK, "OK")
}

// verifyToken resolves and authenticates the ping token from the URL path. On any
// failure (too short, no prefix match, hash mismatch) it writes a minimal 404
// after a dummy constant-time compare for uniform timing, and returns ok=false.
// It logs only the token prefix, never the raw token (design §4.2, R12.2).
func (h *PingHandler) verifyToken(c *gin.Context) (db.GetHeartbeatByTokenPrefixRow, bool) {
	raw := c.Param("token")
	if len(raw) < token.PrefixLen {
		token.DummyCompareSHA256(raw)
		h.notFound(c)
		return db.GetHeartbeatByTokenPrefixRow{}, false
	}

	prefix := raw[:token.PrefixLen]
	candidates, err := h.queries.GetHeartbeatByTokenPrefix(c.Request.Context(), prefix)
	if err != nil || len(candidates) == 0 {
		token.DummyCompareSHA256(raw)
		h.notFound(c)
		return db.GetHeartbeatByTokenPrefixRow{}, false
	}

	for i := range candidates {
		if token.CompareSHA256Hex(raw, candidates[i].TokenHash) {
			return candidates[i], true
		}
	}

	token.DummyCompareSHA256(raw)
	h.notFound(c)
	return db.GetHeartbeatByTokenPrefixRow{}, false
}

// afterTransition runs the non-blocking metric update, WS broadcast, and
// notification dispatch for an observed state change.
func (h *PingHandler) afterTransition(c *gin.Context, row db.Heartbeat, newState, previousState string) {
	if h.metrics != nil {
		h.metrics.SetUp(row.ID.String(), row.Name, newState == "up")
	}

	if h.hub != nil {
		var lastSuccess, nextDeadline *time.Time
		if row.LastSuccessAt.Valid {
			t := row.LastSuccessAt.Time
			lastSuccess = &t
		}
		if row.NextExpectedDeadline.Valid {
			t := row.NextExpectedDeadline.Time
			nextDeadline = &t
		}
		// The broadcast reflects the stored state; the paused presentation mapping
		// is applied by the API DTO, not the realtime patch.
		h.hub.Broadcast(hub.NewHeartbeatStatusMessage(row.ID.String(), row.State, lastSuccess, nextDeadline))
	}

	if h.dispatcher != nil {
		h.dispatcher.DispatchHeartbeat(c, row, newState, previousState)
	}
}

// notFound writes the minimal, non-revealing 404 used for every ping auth failure.
func (h *PingHandler) notFound(c *gin.Context) {
	c.String(http.StatusNotFound, "not found")
}

// markUpRowToHeartbeat projects a MarkHeartbeatUp result onto the db.Heartbeat
// shape used by the hub message and dispatch seam.
func markUpRowToHeartbeat(r db.MarkHeartbeatUpRow) db.Heartbeat {
	return db.Heartbeat{
		ID:                      r.ID,
		Name:                    r.Name,
		ExpectedIntervalSeconds: r.ExpectedIntervalSeconds,
		GraceSeconds:            r.GraceSeconds,
		Status:                  r.Status,
		State:                   r.State,
		LastSuccessAt:           r.LastSuccessAt,
		NextExpectedDeadline:    r.NextExpectedDeadline,
		TokenPrefix:             r.TokenPrefix,
		TokenHash:               r.TokenHash,
		TokenCreatedAt:          r.TokenCreatedAt,
		CreatedAt:               r.CreatedAt,
		UpdatedAt:               r.UpdatedAt,
	}
}

// markFailRowToHeartbeat projects a MarkHeartbeatFail result onto db.Heartbeat.
func markFailRowToHeartbeat(r db.MarkHeartbeatFailRow) db.Heartbeat {
	return db.Heartbeat{
		ID:                      r.ID,
		Name:                    r.Name,
		ExpectedIntervalSeconds: r.ExpectedIntervalSeconds,
		GraceSeconds:            r.GraceSeconds,
		Status:                  r.Status,
		State:                   r.State,
		LastSuccessAt:           r.LastSuccessAt,
		NextExpectedDeadline:    r.NextExpectedDeadline,
		TokenPrefix:             r.TokenPrefix,
		TokenHash:               r.TokenHash,
		TokenCreatedAt:          r.TokenCreatedAt,
		CreatedAt:               r.CreatedAt,
		UpdatedAt:               r.UpdatedAt,
	}
}
