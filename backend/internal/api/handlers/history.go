package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
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
	State            string  `json:"state"`
	LatencyMs        *int32  `json:"latency_ms,omitempty"`
	StatusCode       *int32  `json:"status_code,omitempty"`
	Error            *string `json:"error,omitempty"`
	SslDaysRemaining *int32  `json:"ssl_days_remaining,omitempty"`
	CheckedAt        string  `json:"checked_at"`
}

type aggregatedPointResponse struct {
	Timestamp  string  `json:"timestamp"`
	MinLatency *int32  `json:"min_latency_ms"`
	MaxLatency *int32  `json:"max_latency_ms"`
	AvgLatency *int32  `json:"avg_latency_ms"`
	CheckCount int32   `json:"check_count"`
	UptimeRatio float64 `json:"uptime_ratio"`
}

type historyResponse struct {
	MonitorID        string                    `json:"monitor_id"`
	From             string                    `json:"from"`
	To               string                    `json:"to"`
	Step             *int                      `json:"step,omitempty"`
	Truncated        bool                      `json:"truncated"`
	Points           []historyPointResponse    `json:"points,omitempty"`
	AggregatedPoints []aggregatedPointResponse `json:"aggregated_points,omitempty"`
}

// validateStep checks whether the step value is within [60, 86400].
// Returns nil if valid, or an error describing the valid range.
func validateStep(step int) error {
	if step < 60 || step > 86400 {
		return fmt.Errorf("step must be between 60 and 86400, got %d", step)
	}
	return nil
}

// autoStep computes the aggregation step for a given range in seconds.
// It returns ceil(rangeSeconds / 1000) which bounds the response to ≤ 1000 points.
func autoStep(rangeSeconds float64) int {
	return int(math.Ceil(rangeSeconds / 1000))
}

// retentionClamp checks whether `from` is before the retention boundary (now - retentionDays).
// If so, it returns the clamped `from` and truncated=true; otherwise returns the original `from`
// and truncated=false.
func retentionClamp(from, now time.Time, retentionDays int32) (clampedFrom time.Time, truncated bool) {
	boundary := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	if from.Before(boundary) {
		return boundary, true
	}
	return from, false
}

// aggregateBucketResult holds the computed aggregation for a single bucket of check results.
type aggregateBucketResult struct {
	MinLatency  *int32
	MaxLatency  *int32
	AvgLatency  *int32
	CheckCount  int32
	UptimeRatio float64
}

// checkResultInput represents a single check result used for aggregation testing.
type checkResultInput struct {
	LatencyMs *int32
	State     string
}

// computeAggregation computes min/max/avg latency, check count, and uptime ratio
// for a set of check results. This mirrors the SQL aggregation logic.
func computeAggregation(results []checkResultInput) aggregateBucketResult {
	if len(results) == 0 {
		return aggregateBucketResult{}
	}

	var (
		minLat, maxLat        *int32
		latSum                int64
		latCount              int
		upCount               int
	)

	for _, r := range results {
		if r.LatencyMs != nil {
			v := *r.LatencyMs
			if minLat == nil || v < *minLat {
				cp := v
				minLat = &cp
			}
			if maxLat == nil || v > *maxLat {
				cp := v
				maxLat = &cp
			}
			latSum += int64(v)
			latCount++
		}
		if r.State == "up" {
			upCount++
		}
	}

	var avgLat *int32
	if latCount > 0 {
		avg := int32(latSum / int64(latCount))
		avgLat = &avg
	}

	total := int32(len(results))
	uptimeRatio := float64(upCount) / float64(total)

	return aggregateBucketResult{
		MinLatency:  minLat,
		MaxLatency:  maxLat,
		AvgLatency:  avgLat,
		CheckCount:  total,
		UptimeRatio: uptimeRatio,
	}
}

// Register mounts the history route on the given router group.
func (h *HistoryHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/monitors/:id/history", h.GetHistory)
}

// GetHistory handles GET /monitors/:id/history.
// Query params: from, to (RFC3339), step (int, seconds, optional).
// Defaults to last 24 hours if from/to not provided.
func (h *HistoryHandler) GetHistory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Parse optional step query parameter.
	var step int
	var hasStep bool
	if stepStr := c.Query("step"); stepStr != "" {
		parsed, err := strconv.Atoi(stepStr)
		if err != nil || validateStep(parsed) != nil {
			apiError(c, http.StatusBadRequest, "INVALID_STEP", "step must be an integer between 60 and 86400 seconds")
			return
		}
		step = parsed
		hasStep = true
	}

	// Verify monitor exists and get retention config.
	monitor, err := h.queries.GetMonitor(c.Request.Context(), id)
	if err != nil {
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

	// Retention boundary clamping: if from < now - retention_days, clamp and mark truncated.
	from, truncated := retentionClamp(from, now, monitor.HistoryRetentionDays)

	// Auto-step: when step is absent and range > 24h, compute ceil(range_seconds / 1000).
	if !hasStep {
		rangeSeconds := to.Sub(from).Seconds()
		if rangeSeconds > 24*3600 {
			step = autoStep(rangeSeconds)
			hasStep = true
		}
	}

	// Route to raw or aggregated query based on step.
	resp := historyResponse{
		MonitorID: id.String(),
		From:      from.Format(time.RFC3339),
		To:        to.Format(time.RFC3339),
		Truncated: truncated,
	}

	if hasStep {
		resp.Step = &step

		points, err := h.tsStore.QueryHistoryAggregated(c.Request.Context(), id, from, to, step)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query history")
			return
		}

		data := make([]aggregatedPointResponse, 0, len(points))
		for _, pt := range points {
			data = append(data, aggregatedPointResponse{
				Timestamp:   pt.Timestamp.Format(time.RFC3339),
				MinLatency:  pt.MinLatency,
				MaxLatency:  pt.MaxLatency,
				AvgLatency:  pt.AvgLatency,
				CheckCount:  pt.CheckCount,
				UptimeRatio: pt.UptimeRatio,
			})
		}
		resp.AggregatedPoints = data
	} else {
		points, err := h.tsStore.QueryHistory(c.Request.Context(), id, from, to)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query history")
			return
		}

		data := make([]historyPointResponse, 0, len(points))
		for _, pt := range points {
			data = append(data, historyPointResponse{
				State:            pt.State,
				LatencyMs:        pt.LatencyMs,
				StatusCode:       pt.StatusCode,
				Error:            pt.Error,
				SslDaysRemaining: pt.SslDaysRemaining,
				CheckedAt:        pt.CheckedAt.Format(time.RFC3339),
			})
		}
		resp.Points = data
	}

	c.JSON(http.StatusOK, resp)
}
