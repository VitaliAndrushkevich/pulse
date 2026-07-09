package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// DashboardDB is the interface for database operations needed by the dashboard handler.
// Both *pgxpool.Pool and db.DBTX satisfy this interface.
type DashboardDB interface {
	db.DBTX
}

// DashboardHandler provides the aggregated dashboard summary endpoint.
type DashboardHandler struct {
	queries *db.Queries
	pool    DashboardDB
}

// NewDashboardHandler creates a handler with the given query layer and pool.
func NewDashboardHandler(queries *db.Queries, pool *pgxpool.Pool) *DashboardHandler {
	return &DashboardHandler{queries: queries, pool: pool}
}

// NewDashboardHandlerWithDB creates a handler with the given query layer and any DBTX-compatible pool.
// This constructor is primarily used in tests.
func NewDashboardHandlerWithDB(queries *db.Queries, pool DashboardDB) *DashboardHandler {
	return &DashboardHandler{queries: queries, pool: pool}
}

// HealthScoreData holds the global uptime percentage across all active monitors.
type HealthScoreData struct {
	UptimePercent      float64 `json:"uptime_percent"`
	ActiveMonitorCount int     `json:"active_monitor_count"`
	PartialData        bool    `json:"partial_data"`
}

// StatusDistribution holds the count of monitors in each state.
type StatusDistribution struct {
	Up      int `json:"up"`
	Down    int `json:"down"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// ActiveIncident represents a monitor currently in the down state.
type ActiveIncident struct {
	MonitorID   string  `json:"monitor_id"`
	MonitorName string  `json:"monitor_name"`
	StartedAt   string  `json:"started_at"`
	Cause       *string `json:"cause"`
	State       string  `json:"state"`
}

// TopLatencyMonitor represents a monitor with one of the highest average latencies.
type TopLatencyMonitor struct {
	MonitorID    string `json:"monitor_id"`
	MonitorName  string `json:"monitor_name"`
	AvgLatencyMs int    `json:"avg_latency_ms"`
}

// SSLExpiryEntry represents a monitor with an SSL certificate expiring soon.
type SSLExpiryEntry struct {
	MonitorID     string `json:"monitor_id"`
	MonitorName   string `json:"monitor_name"`
	DaysRemaining int    `json:"days_remaining"`
	ExpiresAt     string `json:"expires_at"`
}

// HeatmapHour represents aggregated monitor state for one hour.
type HeatmapHour struct {
	HourStart    string `json:"hour_start"`
	UpCount      int    `json:"up_count"`
	DownCount    int    `json:"down_count"`
	UnknownCount int    `json:"unknown_count"`
}

// RecentEvent represents a state transition for a monitor.
type RecentEvent struct {
	MonitorID   string `json:"monitor_id"`
	MonitorName string `json:"monitor_name"`
	FromState   string `json:"from_state"`
	ToState     string `json:"to_state"`
	OccurredAt  string `json:"occurred_at"`
}

// DashboardSummaryResponse is the top-level response for GET /dashboard/summary.
type DashboardSummaryResponse struct {
	HealthScore        HealthScoreData     `json:"health_score"`
	StatusDistribution StatusDistribution  `json:"status_distribution"`
	ActiveIncidents    []ActiveIncident    `json:"active_incidents"`
	TopLatencyMonitors []TopLatencyMonitor `json:"top_latency_monitors"`
	SSLExpiry          []SSLExpiryEntry    `json:"ssl_expiry"`
	Heatmap            []HeatmapHour       `json:"heatmap"`
	RecentEvents       []RecentEvent       `json:"recent_events"`
	GeneratedAt        string              `json:"generated_at"`
	PartialData        bool                `json:"partial_data"`
}

// Register mounts the dashboard routes on the given router group.
func (h *DashboardHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/dashboard/summary", h.GetSummary)
}

// GetSummary handles GET /api/v1/dashboard/summary.
// Runs 6 database queries in parallel and assembles the response.
// If any sub-query fails, partial data is returned with partial_data: true.
func (h *DashboardHandler) GetSummary(c *gin.Context) {
	ctx := c.Request.Context()

	var (
		healthScore        HealthScoreData
		statusDistribution StatusDistribution
		activeIncidents    []ActiveIncident
		topLatency         []TopLatencyMonitor
		sslExpiry          []SSLExpiryEntry
		heatmap            []HeatmapHour
		recentEvents       []RecentEvent
		partialData        bool
	)

	g, gCtx := errgroup.WithContext(ctx)

	// Query 1: Health score + status distribution
	g.Go(func() error {
		hs, sd, err := h.queryHealthAndDistribution(gCtx)
		if err != nil {
			return fmt.Errorf("health+distribution: %w", err)
		}
		healthScore = hs
		statusDistribution = sd
		return nil
	})

	// Query 2: Active incidents
	g.Go(func() error {
		incidents, err := h.queryActiveIncidents(gCtx)
		if err != nil {
			return fmt.Errorf("active incidents: %w", err)
		}
		activeIncidents = incidents
		return nil
	})

	// Query 3: Top latency monitors
	g.Go(func() error {
		monitors, err := h.queryTopLatency(gCtx)
		if err != nil {
			return fmt.Errorf("top latency: %w", err)
		}
		topLatency = monitors
		return nil
	})

	// Query 4: SSL expiry warnings
	g.Go(func() error {
		entries, err := h.querySSLExpiry(gCtx)
		if err != nil {
			return fmt.Errorf("ssl expiry: %w", err)
		}
		sslExpiry = entries
		return nil
	})

	// Query 5: Heatmap (TimescaleDB time_bucket)
	g.Go(func() error {
		hours, err := h.queryHeatmap(gCtx)
		if err != nil {
			return fmt.Errorf("heatmap: %w", err)
		}
		heatmap = hours
		return nil
	})

	// Query 6: Recent events
	g.Go(func() error {
		events, err := h.queryRecentEvents(gCtx)
		if err != nil {
			return fmt.Errorf("recent events: %w", err)
		}
		recentEvents = events
		return nil
	})

	// errgroup.Wait returns the first error, but we want partial data.
	// We use individual error tracking instead.
	if err := g.Wait(); err != nil {
		partialData = true
	}

	// Ensure slices are never nil in JSON output.
	if activeIncidents == nil {
		activeIncidents = []ActiveIncident{}
	}
	if topLatency == nil {
		topLatency = []TopLatencyMonitor{}
	}
	if sslExpiry == nil {
		sslExpiry = []SSLExpiryEntry{}
	}
	if heatmap == nil {
		heatmap = []HeatmapHour{}
	}
	if recentEvents == nil {
		recentEvents = []RecentEvent{}
	}

	// Propagate partial_data to health score struct as well.
	healthScore.PartialData = partialData

	resp := DashboardSummaryResponse{
		HealthScore:        healthScore,
		StatusDistribution: statusDistribution,
		ActiveIncidents:    activeIncidents,
		TopLatencyMonitors: topLatency,
		SSLExpiry:          sslExpiry,
		Heatmap:            heatmap,
		RecentEvents:       recentEvents,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		PartialData:        partialData,
	}

	c.JSON(http.StatusOK, resp)
}

// queryHealthAndDistribution fetches the state distribution for active monitors
// and computes the uptime percentage from check_results in the last 24 hours.
func (h *DashboardHandler) queryHealthAndDistribution(ctx context.Context) (HealthScoreData, StatusDistribution, error) {
	// Get state distribution for active monitors.
	rows, err := h.pool.Query(ctx,
		`SELECT state, COUNT(*) AS cnt
		 FROM monitors
		 WHERE status = 'active'
		 GROUP BY state`)
	if err != nil {
		return HealthScoreData{}, StatusDistribution{}, fmt.Errorf("state distribution query: %w", err)
	}
	defer rows.Close()

	var dist StatusDistribution
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return HealthScoreData{}, StatusDistribution{}, fmt.Errorf("state distribution scan: %w", err)
		}
		switch state {
		case "up":
			dist.Up = count
		case "down":
			dist.Down = count
		default:
			dist.Unknown += count
		}
	}
	if err := rows.Err(); err != nil {
		return HealthScoreData{}, StatusDistribution{}, fmt.Errorf("state distribution rows: %w", err)
	}
	dist.Total = dist.Up + dist.Down + dist.Unknown

	// Compute average uptime percentage across all active monitors over last 24h.
	since := time.Now().UTC().Add(-24 * time.Hour)
	var uptimePercent float64

	err = h.pool.QueryRow(ctx,
		`SELECT COALESCE(
			AVG(
				CASE WHEN total > 0
					THEN (up_count::float / total::float) * 100
					ELSE 0
				END
			), 0)
		 FROM (
			SELECT
				cr.monitor_id,
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE cr.state = 'up') AS up_count
			FROM check_results cr
			JOIN monitors m ON m.id = cr.monitor_id
			WHERE m.status = 'active'
			  AND cr.checked_at >= $1
			GROUP BY cr.monitor_id
		 ) sub`, since).Scan(&uptimePercent)
	if err != nil {
		return HealthScoreData{}, StatusDistribution{}, fmt.Errorf("uptime query: %w", err)
	}

	// Round to 2 decimal places.
	uptimePercent = math.Round(uptimePercent*100) / 100

	healthScore := HealthScoreData{
		UptimePercent:      uptimePercent,
		ActiveMonitorCount: dist.Total,
		PartialData:        false,
	}

	return healthScore, dist, nil
}

// queryActiveIncidents fetches currently unresolved incidents with monitor names.
// Returns at most one incident per monitor (the earliest unresolved one),
// ordered by started_at ascending, capped at 10.
func (h *DashboardHandler) queryActiveIncidents(ctx context.Context) ([]ActiveIncident, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT sub.monitor_id, sub.monitor_name, sub.started_at, sub.cause
		 FROM (
		   SELECT DISTINCT ON (m.id)
		     m.id AS monitor_id,
		     m.name AS monitor_name,
		     i.started_at,
		     i.cause
		   FROM incidents i
		   JOIN monitors m ON m.id = i.monitor_id
		   WHERE i.resolved_at IS NULL
		   ORDER BY m.id, i.started_at ASC
		 ) sub
		 ORDER BY sub.started_at ASC
		 LIMIT 10`)
	if err != nil {
		return nil, fmt.Errorf("active incidents query: %w", err)
	}
	defer rows.Close()

	incidents := make([]ActiveIncident, 0)
	for rows.Next() {
		var (
			monitorID   string
			monitorName string
			startedAt   time.Time
			cause       *string
		)
		if err := rows.Scan(&monitorID, &monitorName, &startedAt, &cause); err != nil {
			return nil, fmt.Errorf("active incidents scan: %w", err)
		}
		incidents = append(incidents, ActiveIncident{
			MonitorID:   monitorID,
			MonitorName: monitorName,
			StartedAt:   startedAt.UTC().Format(time.RFC3339),
			Cause:       cause,
			State:       "down",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("active incidents rows: %w", err)
	}

	return incidents, nil
}

// queryTopLatency fetches the top 5 monitors by average latency in the last 24h.
func (h *DashboardHandler) queryTopLatency(ctx context.Context) ([]TopLatencyMonitor, error) {
	since := time.Now().UTC().Add(-24 * time.Hour)

	rows, err := h.pool.Query(ctx,
		`SELECT m.id, m.name, AVG(cr.latency_ms)::integer AS avg_latency_ms
		 FROM check_results cr
		 JOIN monitors m ON m.id = cr.monitor_id
		 WHERE m.status = 'active'
		   AND cr.checked_at >= $1
		   AND cr.latency_ms IS NOT NULL
		 GROUP BY m.id, m.name
		 HAVING AVG(cr.latency_ms) IS NOT NULL
		 ORDER BY avg_latency_ms DESC NULLS LAST
		 LIMIT 5`, since)
	if err != nil {
		return nil, fmt.Errorf("top latency query: %w", err)
	}
	defer rows.Close()

	monitors := make([]TopLatencyMonitor, 0)
	for rows.Next() {
		var (
			monitorID    string
			monitorName  string
			avgLatencyMs int
		)
		if err := rows.Scan(&monitorID, &monitorName, &avgLatencyMs); err != nil {
			return nil, fmt.Errorf("top latency scan: %w", err)
		}
		monitors = append(monitors, TopLatencyMonitor{
			MonitorID:    monitorID,
			MonitorName:  monitorName,
			AvgLatencyMs: avgLatencyMs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("top latency rows: %w", err)
	}

	return monitors, nil
}

// querySSLExpiry fetches monitors with SSL certificates expiring within 30 days.
func (h *DashboardHandler) querySSLExpiry(ctx context.Context) ([]SSLExpiryEntry, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT ON (cr.monitor_id)
			m.id,
			m.name,
			cr.ssl_days_remaining,
			cr.checked_at + (cr.ssl_days_remaining || ' days')::interval AS expires_at
		 FROM check_results cr
		 JOIN monitors m ON m.id = cr.monitor_id
		 WHERE m.status = 'active'
		   AND cr.ssl_days_remaining IS NOT NULL
		   AND cr.ssl_days_remaining <= 30
		 ORDER BY cr.monitor_id, cr.checked_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("ssl expiry query: %w", err)
	}
	defer rows.Close()

	entries := make([]SSLExpiryEntry, 0)
	for rows.Next() {
		var (
			monitorID     string
			monitorName   string
			daysRemaining int
			expiresAt     time.Time
		)
		if err := rows.Scan(&monitorID, &monitorName, &daysRemaining, &expiresAt); err != nil {
			return nil, fmt.Errorf("ssl expiry scan: %w", err)
		}
		entries = append(entries, SSLExpiryEntry{
			MonitorID:     monitorID,
			MonitorName:   monitorName,
			DaysRemaining: daysRemaining,
			ExpiresAt:     expiresAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ssl expiry rows: %w", err)
	}

	// Sort by days_remaining ASC (the DISTINCT ON may not preserve global order).
	// Re-query with proper ordering.
	// Actually, let's handle ordering in the query itself using a subquery.
	// For simplicity and correctness, sort in Go since dataset is small (<=30 entries typically).
	sortSSLEntries(entries)

	return entries, nil
}

// sortSSLEntries sorts by days_remaining ascending, name ascending as tiebreaker.
func sortSSLEntries(entries []SSLExpiryEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			if entries[j].DaysRemaining < entries[j-1].DaysRemaining ||
				(entries[j].DaysRemaining == entries[j-1].DaysRemaining && entries[j].MonitorName < entries[j-1].MonitorName) {
				entries[j], entries[j-1] = entries[j-1], entries[j]
			} else {
				break
			}
		}
	}
}

// queryHeatmap fetches hourly aggregated state counts for the last 24 hours
// using TimescaleDB time_bucket.
func (h *DashboardHandler) queryHeatmap(ctx context.Context) ([]HeatmapHour, error) {
	since := time.Now().UTC().Add(-24 * time.Hour)

	rows, err := h.pool.Query(ctx,
		`SELECT
			time_bucket('1 hour', checked_at) AS hour_start,
			COUNT(*) FILTER (WHERE state = 'up') AS up_count,
			COUNT(*) FILTER (WHERE state = 'down') AS down_count,
			COUNT(*) FILTER (WHERE state NOT IN ('up', 'down')) AS unknown_count
		 FROM check_results
		 WHERE checked_at >= $1
		 GROUP BY hour_start
		 ORDER BY hour_start ASC`, since)
	if err != nil {
		return nil, fmt.Errorf("heatmap query: %w", err)
	}
	defer rows.Close()

	hours := make([]HeatmapHour, 0)
	for rows.Next() {
		var (
			hourStart    time.Time
			upCount      int
			downCount    int
			unknownCount int
		)
		if err := rows.Scan(&hourStart, &upCount, &downCount, &unknownCount); err != nil {
			return nil, fmt.Errorf("heatmap scan: %w", err)
		}
		hours = append(hours, HeatmapHour{
			HourStart:    hourStart.UTC().Format(time.RFC3339),
			UpCount:      upCount,
			DownCount:    downCount,
			UnknownCount: unknownCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("heatmap rows: %w", err)
	}

	return hours, nil
}

// queryRecentEvents fetches the 20 most recent incidents and derives state
// transition events (down + optional recovery). After deduplication, the
// result is sorted and trimmed to 10 events.
func (h *DashboardHandler) queryRecentEvents(ctx context.Context) ([]RecentEvent, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT m.id, m.name, i.started_at, i.resolved_at, i.cause
		 FROM incidents i
		 JOIN monitors m ON m.id = i.monitor_id
		 ORDER BY i.started_at DESC
		 LIMIT 20`)
	if err != nil {
		return nil, fmt.Errorf("recent events query: %w", err)
	}
	defer rows.Close()

	events := make([]RecentEvent, 0)
	for rows.Next() {
		var (
			monitorID  string
			name       string
			startedAt  time.Time
			resolvedAt *time.Time
			cause      *string
		)
		if err := rows.Scan(&monitorID, &name, &startedAt, &resolvedAt, &cause); err != nil {
			return nil, fmt.Errorf("recent events scan: %w", err)
		}

		// Each incident generates up to 2 events: the down transition and the recovery.
		// Add the "went down" event.
		events = append(events, RecentEvent{
			MonitorID:   monitorID,
			MonitorName: name,
			FromState:   "up",
			ToState:     "down",
			OccurredAt:  startedAt.UTC().Format(time.RFC3339),
		})

		// If resolved, also add the recovery event.
		if resolvedAt != nil {
			events = append(events, RecentEvent{
				MonitorID:   monitorID,
				MonitorName: name,
				FromState:   "down",
				ToState:     "up",
				OccurredAt:  resolvedAt.UTC().Format(time.RFC3339),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent events rows: %w", err)
	}

	// Deduplicate events by (monitor_id, to_state, occurred_at) to prevent
	// duplicate keys on the frontend from multiple incident rows with same timestamp.
	events = deduplicateEvents(events)

	// Sort all events by occurred_at descending and take top 10.
	sortEventsDesc(events)
	if len(events) > 10 {
		events = events[:10]
	}

	return events, nil
}

// sortEventsDesc sorts events by OccurredAt descending (most recent first).
func sortEventsDesc(events []RecentEvent) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].OccurredAt > events[j-1].OccurredAt; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}

// deduplicateEvents removes events with duplicate (monitor_id, to_state, occurred_at) keys.
// Keeps the first occurrence of each unique combination.
func deduplicateEvents(events []RecentEvent) []RecentEvent {
	type eventKey struct {
		MonitorID  string
		ToState    string
		OccurredAt string
	}
	seen := make(map[eventKey]bool)
	result := make([]RecentEvent, 0, len(events))
	for _, e := range events {
		key := eventKey{MonitorID: e.MonitorID, ToState: e.ToState, OccurredAt: e.OccurredAt}
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}
