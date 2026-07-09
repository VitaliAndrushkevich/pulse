-- Migration: Refine monitor NOTIFY trigger to fire only on configuration changes.
-- Previously the trigger fired on every UPDATE (including state/last_checked_at
-- updates from the scheduler), creating a noisy feedback loop:
-- check → update state → NOTIFY → wakeup → re-check schedule.
--
-- Now the trigger fires only when configuration columns change (the columns that
-- actually affect scheduling decisions).

DROP TRIGGER IF EXISTS trg_monitor_notify ON monitors;

CREATE OR REPLACE FUNCTION notify_monitor_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('monitor_changes', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_monitor_notify
    AFTER INSERT OR UPDATE OF name, type, target, interval_seconds, timeout_seconds, status, settings
    ON monitors
    FOR EACH ROW
    EXECUTE FUNCTION notify_monitor_change();
