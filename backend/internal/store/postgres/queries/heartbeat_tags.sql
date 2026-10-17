-- name: DeleteHeartbeatTags :exec
DELETE FROM heartbeat_tags WHERE heartbeat_id = $1;

-- name: InsertHeartbeatTag :exec
INSERT INTO heartbeat_tags (heartbeat_id, key, value)
VALUES ($1, $2, $3);

-- name: ListTagsByHeartbeat :many
SELECT * FROM heartbeat_tags
WHERE heartbeat_id = $1
ORDER BY key, value;
