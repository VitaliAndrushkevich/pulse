-- name: GetHeartbeat :one
SELECT * FROM heartbeats
WHERE id = $1;

-- name: GetHeartbeatForUpdate :one
SELECT * FROM heartbeats
WHERE id = $1
FOR UPDATE;

-- name: ListHeartbeats :many
SELECT * FROM heartbeats
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountHeartbeats :one
SELECT COUNT(*) FROM heartbeats;

-- name: CreateHeartbeat :one
INSERT INTO heartbeats (
    name, expected_interval_seconds, grace_seconds,
    status, state, token_prefix, token_hash, history_retention_days
) VALUES (
    $1, $2, $3,
    $4, 'new', $5, $6, $7
)
RETURNING *;

-- name: UpsertHeartbeat :one
-- Idempotent create-or-update for PUT. On conflict it updates only the
-- operator-controlled fields; token material, last_success_at, and state are
-- never touched here (PUT never rotates the token — design §2.2).
INSERT INTO heartbeats (
    id, name, expected_interval_seconds, grace_seconds,
    status, state, token_prefix, token_hash, history_retention_days
) VALUES (
    $1, $2, $3, $4,
    $5, 'new', $6, $7, $8
)
ON CONFLICT (id) DO UPDATE SET
    name                      = EXCLUDED.name,
    expected_interval_seconds = EXCLUDED.expected_interval_seconds,
    grace_seconds             = EXCLUDED.grace_seconds,
    status                    = EXCLUDED.status,
    history_retention_days    = EXCLUDED.history_retention_days,
    updated_at                = now()
RETURNING *;

-- name: UpdateHeartbeatDeadline :one
-- Recomputes next_expected_deadline and sets state after a PUT that changes
-- interval/grace while a last_success_at exists (design §2.2 / §3.4).
UPDATE heartbeats
SET
    state                  = $2,
    last_success_at        = $3,
    next_expected_deadline = $4,
    updated_at             = now()
WHERE id = $1
RETURNING *;

-- name: MarkHeartbeatDown :one
-- Missed-run transition used by the evaluator sweep. The WHERE state='up' guard
-- makes it fire exactly once per missed-run episode (design §2.2 / §5.2); a
-- second sweep on an already-down row affects 0 rows and returns no row.
UPDATE heartbeats
SET state = 'down', updated_at = now()
WHERE id = $1 AND state = 'up'
RETURNING *;

-- name: MarkHeartbeatUp :one
-- Success ping. A single atomic statement that reads the prior state (FOR UPDATE
-- CTE) and writes the new one, so racing success pings each observe a distinct
-- previous_state and recovery fires at most once (design §4.3, MEDIUM-2).
WITH prev AS (
    SELECT state AS previous_state
    FROM heartbeats
    WHERE id = $1
    FOR UPDATE
)
UPDATE heartbeats h
SET
    state                  = 'up',
    last_success_at        = $2,
    next_expected_deadline = $3,
    updated_at             = now()
FROM prev
WHERE h.id = $1
RETURNING h.*, prev.previous_state;

-- name: MarkHeartbeatFail :one
-- Explicit-failure ping. Transitions state → down without advancing
-- last_success_at. Returns the prior state atomically (design §4.3).
WITH prev AS (
    SELECT state AS previous_state
    FROM heartbeats
    WHERE id = $1
    FOR UPDATE
)
UPDATE heartbeats h
SET
    state      = 'down',
    updated_at = now()
FROM prev
WHERE h.id = $1
RETURNING h.*, prev.previous_state;

-- name: RotateHeartbeatToken :one
UPDATE heartbeats
SET
    token_prefix     = $2,
    token_hash       = $3,
    token_created_at = now(),
    updated_at       = now()
WHERE id = $1
RETURNING *;

-- name: GetHeartbeatByTokenPrefix :many
-- Candidate rows for the ping-path token verification (prefix narrows, then
-- constant-time SHA-256 compare in application code — mirrors ListAPITokensByPrefix).
SELECT id, state, status, token_hash, token_prefix,
       expected_interval_seconds, grace_seconds
FROM heartbeats
WHERE token_prefix = $1;

-- name: ListDueHeartbeats :many
-- Evaluator priority scan backed by the partial index idx_heartbeats_next_deadline.
SELECT * FROM heartbeats
WHERE status = 'active'
  AND state = 'up'
  AND next_expected_deadline <= now()
ORDER BY next_expected_deadline ASC
LIMIT $1;

-- name: CountHeartbeatsByState :many
SELECT state, COUNT(*) AS count
FROM heartbeats
GROUP BY state;

-- name: DeleteHeartbeat :exec
DELETE FROM heartbeats WHERE id = $1;
