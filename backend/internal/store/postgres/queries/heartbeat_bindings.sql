-- name: CreateHeartbeatBinding :one
INSERT INTO heartbeat_bindings (channel_id, heartbeat_id, triggers, reminder_interval_minutes)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetHeartbeatBinding :one
SELECT * FROM heartbeat_bindings
WHERE id = $1;

-- name: ListBindingsByHeartbeat :many
SELECT * FROM heartbeat_bindings
WHERE heartbeat_id = $1
ORDER BY created_at DESC;

-- name: ListBindingsByHeartbeatPaginated :many
SELECT * FROM heartbeat_bindings
WHERE heartbeat_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountBindingsByHeartbeat :one
SELECT COUNT(*) FROM heartbeat_bindings
WHERE heartbeat_id = $1;

-- name: UpdateHeartbeatBinding :one
UPDATE heartbeat_bindings
SET
    triggers                  = $2,
    reminder_interval_minutes = $3,
    updated_at                = now()
WHERE id = $1
RETURNING *;

-- name: DeleteHeartbeatBinding :exec
DELETE FROM heartbeat_bindings WHERE id = $1;
