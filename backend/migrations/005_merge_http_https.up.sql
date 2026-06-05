-- 005_merge_http_https.up.sql
-- Merge 'https' monitor type into 'http'.
-- The HTTPChecker already determines TLS from the URL scheme in the target field.

-- Convert all existing HTTPS monitors to HTTP
UPDATE monitors SET type = 'http' WHERE type = 'https';

-- Drop the old CHECK constraint and add a new one without 'https'
ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check CHECK (type IN ('http', 'tcp', 'udp', 'websocket'));
