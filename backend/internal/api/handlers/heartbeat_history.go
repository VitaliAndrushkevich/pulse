package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

// HeartbeatHistoryHandler provides the heartbeat historical event timeline
// endpoint. It mirrors HistoryHandler (monitor history) for parameter parsing,
// defaults, and retention clamping, but returns discrete heartbeat events plus
// a summary (uptime %, missed/fail/success counts) instead of check samples.
type HeartbeatHistoryHandler struct {
	queries *db.Queries
	tsStore *timescale.Store
}

// NewHeartbeatHistoryHandler creates a handler with the given dependencies.
func NewHeartbeatHistoryHandler(queries *db.Queries, tsStore *timescale.Store) *HeartbeatHistoryHandler {
	return &HeartbeatHistoryHandler{queries: queries, tsStore: tsStore}
}

type heartbeatEventResponse struct {
	EventType     string  `json:"event_type"`
	PreviousState *string `json:"previous_state,omitempty"`
	OccurredAt    string  `json:"occurred_at"`
}

type heartbeatHistorySummary struct {
	UptimePercent *float64 `json:"uptime_percent"`
	MissedCount   int64    `json:"missed_count"`
	FailCount     int64    `json:"fail_count"`
	SuccessCount  int64    `json:"success_count"`
	TotalEvents   int64    `json:"total_events"`
}

type heartbeatHistoryResponse struct {
	HeartbeatID string                   `json:"heartbeat_id"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	Truncated   bool                     `json:"truncated"`
	Summary     heartbeatHistorySummary  `json:"summary"`
	Events      []heartbeatEventResponse `json:"events"`
}

// buildHeartbeatSummary computes the summary from per-type event counts.
//
// uptime_percent = success_count / total_events * 100, rounded to one decimal,
// and is nil (serialized as JSON null) when there are no events. The invariant
// success_count + fail_count + missed_count == total_events holds because the
// event_type CHECK constrains events to exactly {success, fail, missed}, so
// total_events is computed as that sum. Both fail and missed reduce uptime
// equally — only success counts as "up".
func buildHeartbeatSummary(counts map[string]int64) heartbeatHistorySummary {
	success := counts["success"]
	fail := counts["fail"]
	missed := counts["missed"]
	total := success + fail + missed

	var uptime *float64
	if total > 0 {
		pct := float64(success) / float64(total) * 100
		// Round to one decimal place.
		pct = float64(int64(pct*10+0.5)) / 10
		uptime = &pct
	}

	return heartbeatHistorySummary{
		UptimePercent: uptime,
		MissedCount:   missed,
		FailCount:     fail,
		SuccessCount:  success,
		TotalEvents:   total,
	}
}

// Register mounts the heartbeat history route on the given router group.
func (h *HeartbeatHistoryHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/heartbeats/:id/history", h.GetHistory)
}

// GetHistory handles GET /heartbeats/:id/history.
// Query params: from, to (RFC3339), step (int, seconds, optional).
// Defaults to last 24 hours if from/to not provided. step is accepted and
// validated for symmetry with monitor history but is inert in v1 (no
// server-side bucketing); when present it is echoed back and ignored.
func (h *HeartbeatHistoryHandler) GetHistory(c *gin.Context) {
	// Parse optional step query parameter (accepted for symmetry, inert in v1).
	if stepStr := c.Query("step"); stepStr != "" {
		parsed, err := strconv.Atoi(stepStr)
		if err != nil || validateStep(parsed) != nil {
			apiError(c, http.StatusBadRequest, "INVALID_STEP", "step must be an integer between 60 and 86400 seconds")
			return
		}
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify heartbeat exists and get retention config.
	hb, err := h.queries.GetHeartbeat(c.Request.Context(), id)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
		return
	}

	// Parse time range — defaults to last 24 hours.
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if fromStr := c.Query("from"); fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "from must be a valid RFC 3339 timestamp")
			return
		}
		from = parsed
	}

	if toStr := c.Query("to"); toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "to must be a valid RFC 3339 timestamp")
			return
		}
		to = parsed
	}

	if !from.Before(to) {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "from must be before to")
		return
	}

	// Retention boundary clamping: if from < now - retention_days, clamp and mark truncated.
	from, truncated := retentionClamp(from, now, hb.HistoryRetentionDays)

	events, err := h.tsStore.QueryHeartbeatEvents(c.Request.Context(), id, from, to)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query heartbeat history")
		return
	}

	counts, err := h.tsStore.CountHeartbeatEventsByType(c.Request.Context(), id, from, to)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query heartbeat history")
		return
	}

	data := make([]heartbeatEventResponse, 0, len(events))
	for _, ev := range events {
		data = append(data, heartbeatEventResponse{
			EventType:     ev.EventType,
			PreviousState: ev.PreviousState,
			OccurredAt:    ev.OccurredAt.Format(time.RFC3339),
		})
	}

	resp := heartbeatHistoryResponse{
		HeartbeatID: id.String(),
		From:        from.Format(time.RFC3339),
		To:          to.Format(time.RFC3339),
		Truncated:   truncated,
		Summary:     buildHeartbeatSummary(counts),
		Events:      data,
	}

	c.JSON(http.StatusOK, resp)
}
