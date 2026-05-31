-- name: CreateCheckResult :one
INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error)
VALUES ($1, $2, $3, $4, $5, $6)
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
