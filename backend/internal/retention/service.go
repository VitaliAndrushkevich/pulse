package retention

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// RetentionService periodically deletes expired check_result rows
// according to each monitor's configured history_retention_days.
type RetentionService struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	interval    time.Duration
	batchSize   int // max monitors per page (100)
	deleteLimit int // max rows per DELETE statement (10,000)
	running     atomic.Bool
	rowsDeleted prometheus.Counter
}

// Config holds constructor parameters for RetentionService.
type Config struct {
	Pool        *pgxpool.Pool
	Queries     *db.Queries
	Interval    time.Duration
	BatchSize   int
	DeleteLimit int
}

// New creates a RetentionService and registers its Prometheus metrics.
func New(cfg Config) (*RetentionService, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("retention: pool is required")
	}
	if cfg.Queries == nil {
		return nil, fmt.Errorf("retention: queries is required")
	}
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("retention: interval must be positive")
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	deleteLimit := cfg.DeleteLimit
	if deleteLimit <= 0 {
		deleteLimit = 10_000
	}

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pulse_retention_rows_deleted_total",
		Help: "Cumulative count of check_result rows removed by the retention service.",
	})
	if err := prometheus.Register(counter); err != nil {
		// If already registered (e.g. in tests), try to reuse.
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			counter = are.ExistingCollector.(prometheus.Counter)
		} else {
			return nil, fmt.Errorf("retention: failed to register metric: %w", err)
		}
	}

	return &RetentionService{
		pool:        cfg.Pool,
		queries:     cfg.Queries,
		interval:    cfg.Interval,
		batchSize:   batchSize,
		deleteLimit: deleteLimit,
		rowsDeleted: counter,
	}, nil
}

// Start runs the retention cleanup loop until ctx is cancelled.
// It executes a cycle immediately on start, then on every tick.
func (s *RetentionService) Start(ctx context.Context) {
	slog.Info("retention service started", "interval", s.interval)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run once immediately at startup.
	s.tryRunCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("retention service stopping", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.tryRunCycle(ctx)
		}
	}
}

// tryRunCycle attempts to run a retention cycle, skipping if one is already in progress.
func (s *RetentionService) tryRunCycle(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		slog.Warn("retention cycle skipped: previous cycle still running")
		return
	}
	defer s.running.Store(false)

	if err := s.runCycle(ctx); err != nil {
		slog.Error("retention cycle failed", "error", err)
	}
}

// runCycle iterates all monitors in pages, deleting expired check_result rows
// for each monitor. It processes monitors in batches of batchSize and caps
// each DELETE at deleteLimit rows.
func (s *RetentionService) runCycle(ctx context.Context) error {
	startTime := time.Now()
	slog.Info("retention cycle started")

	var totalDeleted int64
	offset := int32(0)

	const maxConsecutiveBatchErrors = 3

	consecutiveErrors := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		monitors, err := s.queries.ListMonitors(ctx, db.ListMonitorsParams{
			Limit:  int32(s.batchSize),
			Offset: offset,
		})
		if err != nil {
			consecutiveErrors++
			// Log and skip this batch — retry on the next scheduled cycle.
			slog.Error("retention batch failed: listing monitors",
				"offset", offset,
				"error", err,
				"consecutive_errors", consecutiveErrors,
			)
			if consecutiveErrors >= maxConsecutiveBatchErrors {
				slog.Warn("retention cycle aborting after consecutive batch errors",
					"consecutive_errors", consecutiveErrors,
				)
				break
			}
			offset += int32(s.batchSize)
			continue
		}

		consecutiveErrors = 0

		if len(monitors) == 0 {
			break
		}

		for _, mon := range monitors {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			deleted, err := s.deleteExpiredRows(ctx, mon)
			if err != nil {
				// Log and continue — don't let one monitor's failure block others.
				slog.Error("retention delete failed",
					"monitor_id", mon.ID,
					"error", err,
				)
				continue
			}

			if deleted > 0 {
				totalDeleted += deleted
				s.rowsDeleted.Add(float64(deleted))
				slog.Debug("retention deleted rows",
					"monitor_id", mon.ID,
					"rows_deleted", deleted,
				)
			}
		}

		if len(monitors) < s.batchSize {
			break
		}
		offset += int32(s.batchSize)
	}

	slog.Info("retention cycle completed",
		"total_deleted", totalDeleted,
		"duration", time.Since(startTime),
	)
	return nil
}

// deleteExpiredRows removes check_result rows older than the monitor's retention
// period, capped at deleteLimit rows per call.
func (s *RetentionService) deleteExpiredRows(ctx context.Context, mon db.Monitor) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(mon.HistoryRetentionDays) * 24 * time.Hour)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM check_results
		 WHERE id IN (
		   SELECT id FROM check_results
		   WHERE monitor_id = $1
		     AND checked_at < $2
		   LIMIT $3
		 )`,
		mon.ID, cutoff, s.deleteLimit,
	)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}
