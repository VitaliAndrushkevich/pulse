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
	MonitorID  uuid.UUID
	State      string
	LatencyMs  *int32
	StatusCode *int32
	Error      *string
	CheckedAt  time.Time
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
		`INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		pt.MonitorID,
		pt.CheckedAt,
		pt.State,
		pt.LatencyMs,
		pt.StatusCode,
		pt.Error,
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
		`SELECT monitor_id, state, latency_ms, status_code, error, checked_at
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
