-- name: CreateCheckResult :one
INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error, ssl_days_remaining)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListCheckResultsByMonitor :many
SELECT * FROM check_results
WHERE monitor_id = $1
ORDER BY checked_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCheckResultsByMonitor :one
SELECT COUNT(*) FROM check_results WHERE monitor_id = $1;

-- name: GetLatestCheckResult :one
SELECT * FROM check_results
WHERE monitor_id = $1
ORDER BY checked_at DESC
LIMIT 1;

-- name: DeleteOldCheckResults :exec
DELETE FROM check_results
WHERE monitor_id = $1
  AND checked_at < $2;

-- name: GetMonitorUptimeStats :one
SELECT
  COUNT(*) AS total_checks,
  COUNT(*) FILTER (WHERE state = 'up') AS up_checks,
  COALESCE(AVG(latency_ms) FILTER (WHERE state = 'up'), 0)::INTEGER AS avg_latency_ms
FROM check_results
WHERE monitor_id = $1
  AND checked_at >= $2;

-- name: GetLatestSSLDaysRemaining :one
SELECT ssl_days_remaining FROM check_results
WHERE monitor_id = $1
  AND ssl_days_remaining IS NOT NULL
ORDER BY checked_at DESC
LIMIT 1;
