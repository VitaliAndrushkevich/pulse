-- 002_api_tokens_prefix.up.sql
-- Add prefix column for efficient token lookup by first 8 chars of base64url token.

ALTER TABLE api_tokens ADD COLUMN prefix TEXT NOT NULL DEFAULT '';

-- Partial index: only index non-revoked tokens since revoked tokens won't be used for auth.
CREATE INDEX idx_api_tokens_prefix ON api_tokens (prefix) WHERE revoked_at IS NULL;
