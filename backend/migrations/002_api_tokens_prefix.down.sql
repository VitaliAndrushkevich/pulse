-- 002_api_tokens_prefix.down.sql
-- Revert prefix column addition.

DROP INDEX IF EXISTS idx_api_tokens_prefix;

ALTER TABLE api_tokens DROP COLUMN IF EXISTS prefix;
