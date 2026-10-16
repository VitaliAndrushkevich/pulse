-- 017_heartbeats.up.sql
-- Heartbeat (dead-man's-switch) monitoring: a parallel resource to monitors.
-- A scheduled job pings Pulse at the end of a successful run; Pulse records the
-- arrival, persists the next expected deadline (last_success + interval + grace),
-- and a periodic-sweep evaluator raises an alert when that deadline passes.
-- See .kiro/specs/heartbeat-monitoring/design.md §1.

-- -------------------------------------------------------
-- heartbeats
--
-- Two-field lifecycle/observed split mirrors monitors (migration 001):
--   status = operator-controlled lifecycle (active / paused)
--   state  = observed state derived from pings + evaluator (up / down / paused / new)
-- The ping ingestion credential is stored as a SHA-256 hex digest plus an 8-char
-- lookup prefix (design §6); the raw token is never stored.
-- -------------------------------------------------------
CREATE TABLE heartbeats (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                      TEXT        NOT NULL,
    expected_interval_seconds INTEGER     NOT NULL CHECK (expected_interval_seconds >= 1
                                                          AND expected_interval_seconds <= 31536000), -- max 365d
    grace_seconds             INTEGER     NOT NULL DEFAULT 0 CHECK (grace_seconds >= 0
                                                          AND grace_seconds <= 31536000),
    -- lifecycle status (operator-controlled)
    status                    TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    -- observed state (derived from pings + evaluator)
    state                     TEXT        NOT NULL DEFAULT 'new'  CHECK (state IN ('up', 'down', 'paused', 'new')),
    last_success_at           TIMESTAMPTZ,
    next_expected_deadline    TIMESTAMPTZ,  -- persisted: last_success_at + interval + grace
    -- ping ingestion credential (write-only; raw token never stored)
    token_prefix              TEXT        NOT NULL,
    token_hash                TEXT        NOT NULL,
    token_created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Evaluator priority scan: only rows that can go down.
-- Partial index excludes paused/down/new (new has NULL deadline anyway).
CREATE INDEX idx_heartbeats_next_deadline ON heartbeats (next_expected_deadline)
    WHERE status = 'active' AND state = 'up';

-- Token verification lookup (prefix narrows candidates, then hash compare).
CREATE INDEX idx_heartbeats_token_prefix ON heartbeats (token_prefix);

-- List/filter by lifecycle + recency (mirrors idx_monitors_status_created_at).
CREATE INDEX idx_heartbeats_status_created_at ON heartbeats (status, created_at);

-- -------------------------------------------------------
-- heartbeat_tags
-- Reuses the monitor_tags shape (migration 009) so tags.ValidateTags and the
-- tag autocomplete contract apply unchanged.
-- -------------------------------------------------------
CREATE TABLE heartbeat_tags (
    heartbeat_id UUID NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (heartbeat_id, key, value)
);

CREATE INDEX idx_heartbeat_tags_heartbeat_id ON heartbeat_tags (heartbeat_id);
CREATE INDEX idx_heartbeat_tags_key_value     ON heartbeat_tags (key, value);

-- -------------------------------------------------------
-- heartbeat_bindings
-- Dedicated bindings table mirroring channel_bindings (migration 014) but
-- referencing heartbeats and carrying the heartbeat trigger set. The existing
-- delivery_logs table is reused for heartbeat delivery audit (its monitor_id
-- column is a generic resource id with no FK).
-- -------------------------------------------------------
CREATE TABLE heartbeat_bindings (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id                UUID        NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    heartbeat_id              UUID        NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    triggers                  JSONB       NOT NULL,
    reminder_interval_minutes INT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_channel_heartbeat UNIQUE (channel_id, heartbeat_id)
);

CREATE INDEX idx_heartbeat_bindings_heartbeat_id ON heartbeat_bindings (heartbeat_id);
