-- SMTP monitors previously always skipped TLS certificate verification.
-- Verification is now on by default and controlled by settings.skip_tls_verify.
-- Keep existing monitors behaving as before by opting them in explicitly.
UPDATE monitors
SET settings = settings || '{"skip_tls_verify": true}'::jsonb
WHERE type = 'smtp'
  AND NOT (settings ? 'skip_tls_verify');
