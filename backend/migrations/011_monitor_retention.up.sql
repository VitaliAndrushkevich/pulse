-- Add per-monitor history retention configuration.
ALTER TABLE monitors
    ADD COLUMN history_retention_days INTEGER NOT NULL DEFAULT 30
    CONSTRAINT chk_retention_range CHECK (history_retention_days >= 1 AND history_retention_days <= 365);
