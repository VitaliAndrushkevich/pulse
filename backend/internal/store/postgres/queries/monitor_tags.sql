-- name: DeleteMonitorTags :exec
DELETE FROM monitor_tags WHERE monitor_id = $1;

-- name: InsertMonitorTag :exec
INSERT INTO monitor_tags (monitor_id, key, value)
VALUES ($1, $2, $3);

-- name: ListTagsByMonitor :many
SELECT * FROM monitor_tags
WHERE monitor_id = $1
ORDER BY key, value;

-- name: ListAllTagKeys :many
SELECT DISTINCT key FROM monitor_tags
ORDER BY key;

-- name: ListTagValues :many
SELECT DISTINCT value FROM monitor_tags
WHERE key = $1
ORDER BY value;

-- name: ListMonitorsFiltered :many
SELECT m.* FROM monitors m
WHERE ($1::text = '' OR m.type = $1)
  AND (
    COALESCE(cardinality($2::text[]), 0) = 0
    OR m.id IN (
      SELECT mt.monitor_id FROM monitor_tags mt
      WHERE (mt.key || ':' || mt.value) = ANY($2::text[])
      Group BY mt.monitor_id
      HAVING COUNT(DISTINCT mt.key || ':' || mt.value) = cardinality($2::text[])
    )
  )
ORDER BY m.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountMonitorsFiltered :one
SELECT COUNT(*) FROM monitors m
WHERE ($1::text = '' OR m.type = $1)
  AND (
    COALESCE(cardinality($2::text[]), 0) = 0
    OR m.id IN (
      SELECT mt.monitor_id FROM monitor_tags mt
      WHERE (mt.key || ':' || mt.value) = ANY($2::text[])
      GROUP BY mt.monitor_id
      HAVING COUNT(DISTINCT mt.key || ':' || mt.value) = cardinality($2::text[])
    )
  );
