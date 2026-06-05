-- 005_merge_http_https.down.sql
-- Restore 'https' as a valid monitor type.

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check CHECK (type IN ('http', 'https', 'tcp', 'udp', 'websocket'));
