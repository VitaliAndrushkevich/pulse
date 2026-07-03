-- name: CreateChannel :one
INSERT INTO notification_channels (name, channel_type, config)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetChannel :one
SELECT * FROM notification_channels
WHERE id = $1;

-- name: ListChannels :many
SELECT * FROM notification_channels
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountChannels :one
SELECT COUNT(*) FROM notification_channels;

-- name: UpdateChannel :one
UPDATE notification_channels
SET
    name         = $2,
    channel_type = $3,
    config       = $4,
    updated_at   = now()
WHERE id = $1
RETURNING *;

-- name: DeleteChannel :exec
DELETE FROM notification_channels WHERE id = $1;

-- name: CreateBinding :one
INSERT INTO channel_bindings (channel_id, monitor_id, triggers, reminder_interval_minutes)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetBinding :one
SELECT * FROM channel_bindings
WHERE id = $1;

-- name: ListBindingsByMonitor :many
SELECT * FROM channel_bindings
WHERE monitor_id = $1
ORDER BY created_at DESC;

-- name: ListBindingsByMonitorPaginated :many
SELECT * FROM channel_bindings
WHERE monitor_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountBindingsByMonitor :one
SELECT COUNT(*) FROM channel_bindings
WHERE monitor_id = $1;

-- name: ListBindingsByChannel :many
SELECT * FROM channel_bindings
WHERE channel_id = $1
ORDER BY created_at DESC;

-- name: UpdateBinding :one
UPDATE channel_bindings
SET
    triggers                  = $2,
    reminder_interval_minutes = $3,
    updated_at                = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBinding :exec
DELETE FROM channel_bindings WHERE id = $1;

-- name: InsertDeliveryLog :one
INSERT INTO delivery_logs (channel_id, monitor_id, binding_id, trigger_type, attempt, status, error_detail)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListDeliveryLogsByChannel :many
SELECT * FROM delivery_logs
WHERE channel_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDeliveryLogsByChannel :one
SELECT COUNT(*) FROM delivery_logs
WHERE channel_id = $1;

-- name: ListDeliveryLogsByMonitor :many
SELECT * FROM delivery_logs
WHERE monitor_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDeliveryLogsByMonitor :one
SELECT COUNT(*) FROM delivery_logs
WHERE monitor_id = $1;

-- name: UpsertSMTPSettings :one
INSERT INTO smtp_settings (id, host, port, username, password_enc, from_address, tls_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    host         = EXCLUDED.host,
    port         = EXCLUDED.port,
    username     = EXCLUDED.username,
    password_enc = EXCLUDED.password_enc,
    from_address = EXCLUDED.from_address,
    tls_enabled  = EXCLUDED.tls_enabled,
    updated_at   = now()
RETURNING *;

-- name: GetSMTPSettings :one
SELECT * FROM smtp_settings
LIMIT 1;

-- name: DeleteSMTPSettings :exec
DELETE FROM smtp_settings;
