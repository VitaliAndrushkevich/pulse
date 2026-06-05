package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// MonitorStatsHandler provides uptime statistics and SSL info for a monitor.
type MonitorStatsHandler struct {
	queries *db.Queries
}

// NewMonitorStatsHandler creates a handler with the given query layer.
func NewMonitorStatsHandler(queries *db.Queries) *MonitorStatsHandler {
	return &MonitorStatsHandler{queries: queries}
}

type uptimeWindowStats struct {
	TotalChecks  int64   `json:"total_checks"`
	UpChecks     int64   `json:"up_checks"`
	UptimePercent float64 `json:"uptime_percent"`
	AvgLatencyMs int32   `json:"avg_latency_ms"`
}

type sslInfo struct {
	DaysRemaining int32  `json:"days_remaining"`
	ExpiresAt     string `json:"expires_at"` // approximate date based on days remaining
}

type monitorStatsResponse struct {
	MonitorID string             `json:"monitor_id"`
	Uptime24h uptimeWindowStats  `json:"uptime_24h"`
	Uptime30d uptimeWindowStats  `json:"uptime_30d"`
	SSL       *sslInfo           `json:"ssl,omitempty"`
	LastError *lastErrorInfo     `json:"last_error,omitempty"`
}

type lastErrorInfo struct {
	Error     string `json:"error"`
	CheckedAt string `json:"checked_at"`
}

// Register mounts the stats route on the given router group.
func (h *MonitorStatsHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/monitors/:id/stats", h.GetStats)
}

// GetStats handles GET /monitors/:id/stats.
func (h *MonitorStatsHandler) GetStats(c *gin.Context) {
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

	now := time.Now().UTC()

	// 24h stats
	stats24h, err := h.queries.GetMonitorUptimeStats(c.Request.Context(), db.GetMonitorUptimeStatsParams{
		MonitorID: id,
		CheckedAt: now.Add(-24 * time.Hour),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query uptime stats")
		return
	}

	// 30d stats
	stats30d, err := h.queries.GetMonitorUptimeStats(c.Request.Context(), db.GetMonitorUptimeStatsParams{
		MonitorID: id,
		CheckedAt: now.Add(-30 * 24 * time.Hour),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to query uptime stats")
		return
	}

	resp := monitorStatsResponse{
		MonitorID: id.String(),
		Uptime24h: toUptimeWindowStats(stats24h),
		Uptime30d: toUptimeWindowStats(stats30d),
	}

	// SSL info
	sslDays, err := h.queries.GetLatestSSLDaysRemaining(c.Request.Context(), id)
	if err == nil && sslDays != nil {
		expiresAt := now.Add(time.Duration(*sslDays) * 24 * time.Hour)
		resp.SSL = &sslInfo{
			DaysRemaining: *sslDays,
			ExpiresAt:     expiresAt.Format("2006-01-02"),
		}
	}

	// Last error — get latest check result with error
	latest, err := h.queries.GetLatestCheckResult(c.Request.Context(), id)
	if err == nil && latest.Error != nil && latest.State == "down" {
		resp.LastError = &lastErrorInfo{
			Error:     *latest.Error,
			CheckedAt: latest.CheckedAt.Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, resp)
}

func toUptimeWindowStats(row db.GetMonitorUptimeStatsRow) uptimeWindowStats {
	var uptimePercent float64
	if row.TotalChecks > 0 {
		uptimePercent = float64(row.UpChecks) / float64(row.TotalChecks) * 100
	}
	return uptimeWindowStats{
		TotalChecks:   row.TotalChecks,
		UpChecks:      row.UpChecks,
		UptimePercent: uptimePercent,
		AvgLatencyMs:  row.AvgLatencyMs,
	}
}
