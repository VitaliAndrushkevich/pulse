-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1);

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateUserPassword :one
UPDATE users
SET
    password_hash = $2,
    must_change_password = false,
    session_version = session_version + 1,
    updated_at    = now()
WHERE id = $1 AND session_version = $3 AND enabled
RETURNING *;

-- name: UpdateUserEmailAndPassword :one
UPDATE users
SET
    email         = $2,
    enabled = true,
    password_hash = $3,
    must_change_password = false,
    session_version = session_version + 1,
    updated_at    = now()
WHERE id = $1
RETURNING *;

-- name: GetFirstUser :one
SELECT * FROM users WHERE role = 'admin' ORDER BY created_at, id LIMIT 1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: CreateManagedUser :one
INSERT INTO users (email, password_hash, role, must_change_password)
VALUES (lower(sqlc.arg(email)::text), sqlc.arg(password_hash), sqlc.arg(role), true) RETURNING *;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at, id LIMIT $1 OFFSET $2;

-- name: UpdateManagedUser :one
UPDATE users SET role = $2, enabled = $3,
 session_version = session_version + CASE WHEN (enabled AND NOT $3) OR role <> $2 THEN 1 ELSE 0 END,
 updated_at = now() WHERE id = $1 RETURNING *;

-- name: ResetUserPassword :one
UPDATE users SET password_hash = $2, must_change_password = true,
 session_version = session_version + 1, updated_at = now() WHERE id = $1 RETURNING *;

-- name: LockUsers :exec
LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE role = 'admin' AND enabled;
