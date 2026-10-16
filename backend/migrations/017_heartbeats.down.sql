-- 017_heartbeats.down.sql
-- Reverse of 017_heartbeats.up.sql. Drops in reverse dependency order:
-- heartbeat_bindings (FK → heartbeats, notification_channels), then
-- heartbeat_tags (FK → heartbeats), then heartbeats.
-- delivery_logs is left untouched (it was never altered by the up migration).

DROP TABLE IF EXISTS heartbeat_bindings;
DROP TABLE IF EXISTS heartbeat_tags;
DROP TABLE IF EXISTS heartbeats;
