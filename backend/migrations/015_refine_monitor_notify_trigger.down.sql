-- Revert to the original trigger that fires on any INSERT or UPDATE.
DROP TRIGGER IF EXISTS trg_monitor_notify ON monitors;

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
