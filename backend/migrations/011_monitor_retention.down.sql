-- Remove per-monitor history retention configuration.
ALTER TABLE monitors DROP COLUMN IF EXISTS history_retention_days;
