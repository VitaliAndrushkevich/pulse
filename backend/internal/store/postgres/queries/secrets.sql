-- name: GetSecret :one
SELECT * FROM secrets WHERE id = $1;

-- name: ListSecrets :many
SELECT id, name, created_at, updated_at FROM secrets
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSecrets :one
SELECT COUNT(*) FROM secrets;

-- name: CreateSecret :one
INSERT INTO secrets (name, encrypted_value)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateSecret :one
UPDATE secrets
SET
    name            = $2,
    encrypted_value = $3,
    updated_at      = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSecret :exec
DELETE FROM secrets WHERE id = $1;
