-- name: UpsertProtoSource :one
INSERT INTO proto_sources (monitor_id, source_type, descriptor_bytes, metadata, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (monitor_id) DO UPDATE SET
    source_type = EXCLUDED.source_type,
    descriptor_bytes = EXCLUDED.descriptor_bytes,
    metadata = EXCLUDED.metadata,
    updated_at = now()
RETURNING *;

-- name: GetProtoSource :one
SELECT * FROM proto_sources WHERE monitor_id = $1;

-- name: DeleteProtoSource :exec
DELETE FROM proto_sources WHERE monitor_id = $1;

-- name: ProtoSourceExists :one
SELECT EXISTS(SELECT 1 FROM proto_sources WHERE monitor_id = $1);
