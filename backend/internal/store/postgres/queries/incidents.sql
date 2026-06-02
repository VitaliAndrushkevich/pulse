-- name: GetIncident :one
SELECT * FROM incidents WHERE id = $1;

-- name: ListIncidentsByMonitor :many
SELECT * FROM incidents
WHERE monitor_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: CountIncidentsByMonitor :one
SELECT COUNT(*) FROM incidents WHERE monitor_id = $1;

-- name: GetOpenIncidentByMonitor :one
SELECT * FROM incidents
WHERE monitor_id = $1
  AND resolved_at IS NULL
LIMIT 1;

-- name: CreateIncident :one
INSERT INTO incidents (monitor_id, started_at, cause)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ResolveIncident :one
UPDATE incidents
SET resolved_at = $2
WHERE id = $1
RETURNING *;

-- name: ListOpenIncidents :many
SELECT * FROM incidents
WHERE resolved_at IS NULL
ORDER BY started_at DESC
LIMIT $1 OFFSET $2;

-- name: ListIncidents :many
SELECT * FROM incidents
ORDER BY started_at DESC
LIMIT $1 OFFSET $2;

-- name: CountIncidents :one
SELECT COUNT(*) FROM incidents;
