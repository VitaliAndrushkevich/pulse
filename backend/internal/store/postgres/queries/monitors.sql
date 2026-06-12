-- name: GetMonitor :one
SELECT * FROM monitors
WHERE id = $1;

-- name: ListMonitors :many
SELECT * FROM monitors
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountMonitors :one
SELECT COUNT(*) FROM monitors;

-- name: ListActiveMonitorsDue :many
SELECT * FROM monitors
WHERE status = 'active'
  AND (next_check_at IS NULL OR next_check_at <= now())
ORDER BY next_check_at ASC NULLS FIRST
LIMIT $1;

-- name: ListActiveMonitorsDueWithTags :many
SELECT m.id, m.name, m.type, m.target, m.interval_seconds, m.timeout_seconds,
       m.status, m.state, m.last_checked_at, m.next_check_at, m.settings,
       m.created_at, m.updated_at, m.history_retention_days,
       COALESCE(
         json_agg(json_build_object('key', mt.key, 'value', mt.value))
         FILTER (WHERE mt.key IS NOT NULL),
         '[]'::json
       ) AS tags_json
FROM monitors m
LEFT JOIN monitor_tags mt ON mt.monitor_id = m.id
WHERE m.status = 'active'
  AND (m.next_check_at IS NULL OR m.next_check_at <= now())
GROUP BY m.id
ORDER BY m.next_check_at ASC NULLS FIRST
LIMIT $1;

-- name: CreateMonitor :one
INSERT INTO monitors (
    name, type, target, interval_seconds, timeout_seconds,
    status, state, settings, next_check_at, history_retention_days
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
)
RETURNING *;

-- name: UpdateMonitor :one
UPDATE monitors
SET
    name                 = $2,
    type                 = $3,
    target               = $4,
    interval_seconds     = $5,
    timeout_seconds      = $6,
    status               = $7,
    settings             = $8,
    history_retention_days = $9,
    updated_at           = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMonitorState :one
UPDATE monitors
SET
    state           = $2,
    last_checked_at = $3,
    next_check_at   = $4,
    updated_at      = now()
WHERE id = $1
RETURNING *;

-- name: UpsertMonitor :one
INSERT INTO monitors (
    id, name, type, target, interval_seconds, timeout_seconds,
    status, state, settings, next_check_at, history_retention_days
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, 'unknown', $8, now(), $9
)
ON CONFLICT (id) DO UPDATE SET
    name                 = EXCLUDED.name,
    type                 = EXCLUDED.type,
    target               = EXCLUDED.target,
    interval_seconds     = EXCLUDED.interval_seconds,
    timeout_seconds      = EXCLUDED.timeout_seconds,
    status               = EXCLUDED.status,
    settings             = EXCLUDED.settings,
    history_retention_days = EXCLUDED.history_retention_days,
    updated_at           = now()
RETURNING *;

-- name: GetMonitorForUpdate :one
SELECT * FROM monitors
WHERE id = $1
FOR UPDATE;

-- name: DeleteMonitor :exec
DELETE FROM monitors WHERE id = $1;
