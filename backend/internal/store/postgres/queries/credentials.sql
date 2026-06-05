-- name: CreateCredential :one
INSERT INTO monitor_credentials (monitor_id, auth_type, name, encrypted_value)
VALUES ($1, $2, $3, $4)
RETURNING id, monitor_id, auth_type, name, created_at, updated_at;

-- name: ListCredentialsByMonitorID :many
SELECT id, monitor_id, auth_type, name, encrypted_value, created_at, updated_at
FROM monitor_credentials
WHERE monitor_id = $1
ORDER BY created_at;

-- name: GetCredential :one
SELECT id, monitor_id, auth_type, name, encrypted_value, created_at, updated_at
FROM monitor_credentials
WHERE id = $1 AND monitor_id = $2;

-- name: UpdateCredential :one
UPDATE monitor_credentials
SET name = $3, encrypted_value = $4, updated_at = now()
WHERE id = $1 AND monitor_id = $2
RETURNING id, monitor_id, auth_type, name, created_at, updated_at;

-- name: DeleteCredential :exec
DELETE FROM monitor_credentials WHERE id = $1 AND monitor_id = $2;

-- name: ListCredentialsByMonitorIDInternal :many
-- Used by scheduler to fetch encrypted values for injection
SELECT id, auth_type, encrypted_value
FROM monitor_credentials
WHERE monitor_id = $1;

-- name: ListAllCredentials :many
-- Used by key rotation
SELECT id, encrypted_value FROM monitor_credentials ORDER BY id;

-- name: UpdateCredentialEncryptedValue :exec
-- Used by key rotation
UPDATE monitor_credentials SET encrypted_value = $2, updated_at = now() WHERE id = $1;
