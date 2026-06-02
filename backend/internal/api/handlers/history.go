package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

// HistoryHandler provides the monitor history endpoint.
type HistoryHandler struct {
	queries *db.Queries
	tsStore *timescale.Store
}

// NewHistoryHandler creates a handler with the given dependencies.
func NewHistoryHandler(queries *db.Queries, tsStore *timescale.Store) *HistoryHandler {
	return &HistoryHandler{queries: queries, tsStore: tsStore}
}

type historyPointResponse struct {
	State      string  `json:"state"`
	LatencyMs  *int32  `json:"latency_ms,omitempty"`
	StatusCode *int32  `json:"status_code,omitempty"`
	Error      *string `json:"error,omitempty"`
	CheckedAt  string  `json:"checked_at"`
}

type historyResponse struct {
	MonitorID string                 `json:"monitor_id"`
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Points    []historyPointResponse `json:"points"`
}

// Register mounts the history route on the given router group.
func (h *HistoryHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/monitors/:id/history", h.GetHistory)
}

// GetHistory handles GET /monitors/:id/history.
// Query params: from, to (RFC3339). Defaults to last 24 hours.
func (h *HistoryHandler) GetHistory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Verify monitor exists.
	if _, err := h.queries.GetMonitor(c.Request.Context(), id); err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
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

	// Cap maximum window to 7 days to bound response size.
	maxWindow := 7 * 24 * time.Hour
	if to.Sub(from) > maxWindow {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "time window cannot exceed 7 days")
		return
	}

	points, err := h.tsStore.QueryHistory(c.Request.Context(), id, from, to)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query history")
		return
	}

	data := make([]historyPointResponse, 0, len(points))
	for _, pt := range points {
		data = append(data, historyPointResponse{
			State:      pt.State,
			LatencyMs:  pt.LatencyMs,
			StatusCode: pt.StatusCode,
			Error:      pt.Error,
			CheckedAt:  pt.CheckedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, historyResponse{
		MonitorID: id.String(),
		From:      from.Format(time.RFC3339),
		To:        to.Format(time.RFC3339),
		Points:    data,
	})
}
