-- Migration: Add NOTIFY trigger on monitors table for scheduler wakeups.
-- When a monitor is inserted or updated (status/interval/target changes),
-- fire a notification on the 'monitor_changes' channel so the scheduler
-- can immediately re-poll without waiting for its next tick.

CREATE OR REPLACE FUNCTION notify_monitor_change()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('monitor_changes', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_monitor_notify
    AFTER INSERT OR UPDATE ON monitors
    FOR EACH ROW
    EXECUTE FUNCTION notify_monitor_change();
