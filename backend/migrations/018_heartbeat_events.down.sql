-- 018_heartbeat_events.down.sql
ALTER TABLE heartbeats DROP COLUMN IF EXISTS history_retention_days;
DROP TABLE IF EXISTS heartbeat_events;
