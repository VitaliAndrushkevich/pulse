package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckPoint is a single monitor check result stored in PostgreSQL/TimescaleDB.
type CheckPoint struct {
	MonitorID        uuid.UUID
	State            string
	LatencyMs        *int32
	StatusCode       *int32
	Error            *string
	SslDaysRemaining *int32
	CheckedAt        time.Time
}

// HeartbeatEvent is a single discrete heartbeat episode stored in TimescaleDB.
// It is written on every ping success, every explicit fail ping, and every
// missed-run sweep transition.
type HeartbeatEvent struct {
	HeartbeatID   uuid.UUID
	EventType     string  // "success" | "fail" | "missed"
	PreviousState *string // observed state before the event, nil if unknown
	OccurredAt    time.Time
}

// HeartbeatEventWriter records heartbeat timeline events. A nil writer is valid
// and simply skips the write (the DB state update, metric, broadcast, and
// dispatch still happen) — matching how a nil hub/dispatcher is tolerated today.
// It is declared here, in the timescale package, so both the handlers and
// heartbeat packages (which already import timescale) can reference it without
// an import cycle; *Store is the sole production implementer.
type HeartbeatEventWriter interface {
	WriteHeartbeatEvent(ctx context.Context, ev HeartbeatEvent) error
}

// AggregatedPoint represents a time-bucketed aggregation of check results.
type AggregatedPoint struct {
	Timestamp   time.Time
	MinLatency  *int32
	MaxLatency  *int32
	AvgLatency  *int32
	CheckCount  int32
	UptimeRatio float64
}

// Store uses the shared PostgreSQL pool with TimescaleDB extension enabled.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping verifies DB connectivity and checks that the TimescaleDB extension is installed.
func (s *Store) Ping(ctx context.Context) error {
	var one int
	if err := s.pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("timescaledb ping: %w", err)
	}

	var extVersion string
	if err := s.pool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'`).Scan(&extVersion); err != nil {
		return fmt.Errorf("timescaledb extension check: %w", err)
	}

	return nil
}

// WriteCheckResult appends one check result row into the time-series hypertable.
func (s *Store) WriteCheckResult(ctx context.Context, pt CheckPoint) error {
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error, ssl_days_remaining)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pt.MonitorID,
		pt.CheckedAt,
		pt.State,
		pt.LatencyMs,
		pt.StatusCode,
		pt.Error,
		pt.SslDaysRemaining,
	)
	if err != nil {
		return fmt.Errorf("timescaledb write: %w", err)
	}

	return nil
}

// QueryHistory returns check results for one monitor in [start, end), ordered by time ascending.
func (s *Store) QueryHistory(ctx context.Context, monitorID uuid.UUID, start, end time.Time) ([]CheckPoint, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT monitor_id, state, latency_ms, status_code, error, ssl_days_remaining, checked_at
		 FROM check_results
		 WHERE monitor_id = $1
		   AND checked_at >= $2
		   AND checked_at < $3
		 ORDER BY checked_at ASC`,
		monitorID,
		start,
		end,
	)
	if err != nil {
		return nil, fmt.Errorf("timescaledb query: %w", err)
	}
	defer rows.Close()

	points := make([]CheckPoint, 0)
	for rows.Next() {
		var point CheckPoint
		if err := rows.Scan(
			&point.MonitorID,
			&point.State,
			&point.LatencyMs,
			&point.StatusCode,
			&point.Error,
			&point.SslDaysRemaining,
			&point.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("timescaledb scan: %w", err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb rows: %w", err)
	}

	return points, nil
}

// QueryHistoryAggregated returns downsampled check results for one monitor in [start, end),
// aggregated into time buckets of the specified step duration. Each bucket includes
// min/max/avg latency, check count, and uptime ratio.
func (s *Store) QueryHistoryAggregated(
	ctx context.Context,
	monitorID uuid.UUID,
	start, end time.Time,
	stepSeconds int,
) ([]AggregatedPoint, error) {
	interval := fmt.Sprintf("%d seconds", stepSeconds)

	rows, err := s.pool.Query(
		ctx,
		`SELECT
			time_bucket($4::interval, checked_at) AS bucket,
			MIN(latency_ms) AS min_latency_ms,
			MAX(latency_ms) AS max_latency_ms,
			AVG(latency_ms)::integer AS avg_latency_ms,
			COUNT(*) AS check_count,
			COUNT(*) FILTER (WHERE state = 'up')::float / COUNT(*) AS uptime_ratio
		 FROM check_results
		 WHERE monitor_id = $1
		   AND checked_at >= $2
		   AND checked_at < $3
		 GROUP BY bucket
		 ORDER BY bucket ASC`,
		monitorID,
		start,
		end,
		interval,
	)
	if err != nil {
		return nil, fmt.Errorf("timescaledb aggregated query: %w", err)
	}
	defer rows.Close()

	points := make([]AggregatedPoint, 0)
	for rows.Next() {
		var point AggregatedPoint
		if err := rows.Scan(
			&point.Timestamp,
			&point.MinLatency,
			&point.MaxLatency,
			&point.AvgLatency,
			&point.CheckCount,
			&point.UptimeRatio,
		); err != nil {
			return nil, fmt.Errorf("timescaledb aggregated scan: %w", err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb aggregated rows: %w", err)
	}

	return points, nil
}

// WriteHeartbeatEvent appends one event row into the heartbeat_events hypertable.
func (s *Store) WriteHeartbeatEvent(ctx context.Context, ev HeartbeatEvent) error {
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO heartbeat_events (heartbeat_id, occurred_at, event_type, previous_state)
		 VALUES ($1, $2, $3, $4)`,
		ev.HeartbeatID,
		ev.OccurredAt,
		ev.EventType,
		ev.PreviousState,
	)
	if err != nil {
		return fmt.Errorf("timescaledb heartbeat event write: %w", err)
	}

	return nil
}

// QueryHeartbeatEvents returns events for one heartbeat in [start, end),
// ordered by occurred_at ascending. Mirrors QueryHistory.
func (s *Store) QueryHeartbeatEvents(ctx context.Context, heartbeatID uuid.UUID, start, end time.Time) ([]HeartbeatEvent, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT heartbeat_id, event_type, previous_state, occurred_at
		 FROM heartbeat_events
		 WHERE heartbeat_id = $1
		   AND occurred_at >= $2
		   AND occurred_at < $3
		 ORDER BY occurred_at ASC`,
		heartbeatID,
		start,
		end,
	)
	if err != nil {
		return nil, fmt.Errorf("timescaledb heartbeat events query: %w", err)
	}
	defer rows.Close()

	events := make([]HeartbeatEvent, 0)
	for rows.Next() {
		var ev HeartbeatEvent
		if err := rows.Scan(
			&ev.HeartbeatID,
			&ev.EventType,
			&ev.PreviousState,
			&ev.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("timescaledb heartbeat events scan: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb heartbeat events rows: %w", err)
	}

	return events, nil
}

// CountHeartbeatEventsByType returns per-type counts for one heartbeat in
// [start, end). Backs the summary (missed-run count, uptime) in a single
// round-trip so the handler can read counts["missed"] without materializing
// every row twice.
func (s *Store) CountHeartbeatEventsByType(ctx context.Context, heartbeatID uuid.UUID, start, end time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT event_type, COUNT(*)
		 FROM heartbeat_events
		 WHERE heartbeat_id = $1
		   AND occurred_at >= $2
		   AND occurred_at < $3
		 GROUP BY event_type`,
		heartbeatID,
		start,
		end,
	)
	if err != nil {
		return nil, fmt.Errorf("timescaledb heartbeat events count query: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var (
			eventType string
			count     int64
		)
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("timescaledb heartbeat events count scan: %w", err)
		}
		counts[eventType] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb heartbeat events count rows: %w", err)
	}

	return counts, nil
}
