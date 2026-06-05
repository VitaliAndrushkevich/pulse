-- Add ssl_days_remaining to check_results for certificate expiration tracking.
ALTER TABLE check_results ADD COLUMN ssl_days_remaining INTEGER;
