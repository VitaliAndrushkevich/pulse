-- 014_notification_channels.down.sql
-- Reverse notification channels schema in dependency order.

DROP TABLE IF EXISTS smtp_settings;
DROP TABLE IF EXISTS delivery_logs;
DROP TABLE IF EXISTS channel_bindings;
DROP TABLE IF EXISTS notification_channels;
