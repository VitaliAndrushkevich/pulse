-- 018_heartbeat_events.up.sql
-- Append-only historical event log for heartbeats (dead-man's-switch resource).
-- One row is written on every ping success, every explicit fail ping, and every
-- missed-run sweep transition. Mirrors check_results (migrations 001/004) but
-- models DISCRETE heartbeat episodes, not scheduled check samples.

CREATE TABLE heartbeat_events (
    id            UUID        NOT NULL DEFAULT gen_random_uuid(),
    heartbeat_id  UUID        NOT NULL REFERENCES heartbeats (id) ON DELETE CASCADE,
    occurred_at   TIMESTAMPTZ NOT NULL,
    event_type    TEXT        NOT NULL CHECK (event_type IN ('success', 'fail', 'missed')),
    -- Optional transition context: the observed state the heartbeat held before
    -- this event (one of up/down/paused/new). Lets the UI label recoveries vs.
    -- repeated downs without a second query. Nullable because the first ever
    -- event for a heartbeat has no prior observed state captured.
    previous_state TEXT       CHECK (previous_state IN ('up', 'down', 'paused', 'new')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, occurred_at)
);

CREATE INDEX idx_heartbeat_events_hb_id_occurred_at
    ON heartbeat_events (heartbeat_id, occurred_at DESC);

-- The timescaledb extension is created in migration 004
-- (004_timescaledb_check_results.up.sql), which runs earlier; it is intentionally
-- NOT repeated here, so no CREATE EXTENSION is needed before create_hypertable.
SELECT create_hypertable('heartbeat_events', 'occurred_at', if_not_exists => TRUE);

-- Per-heartbeat history retention, mirroring monitors.history_retention_days
-- (migration 011): NOT NULL DEFAULT 30, bounded to [1, 365].
ALTER TABLE heartbeats
    ADD COLUMN history_retention_days INTEGER NOT NULL DEFAULT 30
    CONSTRAINT chk_heartbeat_retention_range
        CHECK (history_retention_days >= 1 AND history_retention_days <= 365);
