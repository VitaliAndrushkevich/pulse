-- name: GetAPIToken :one
SELECT * FROM api_tokens WHERE id = $1;

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListAPITokensByPrefix :many
SELECT * FROM api_tokens
WHERE prefix = $1
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListAPITokensByUser :many
SELECT * FROM api_tokens
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAPITokensByUser :one
SELECT COUNT(*) FROM api_tokens WHERE user_id = $1;

-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, name, prefix, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: RevokeAPIToken :one
UPDATE api_tokens
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: TouchAPIToken :exec
UPDATE api_tokens
SET last_used_at = now()
WHERE id = $1;
