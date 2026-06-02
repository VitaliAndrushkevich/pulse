-- 001_initial.up.sql
-- Full schema for Pulse MVP (TASK-005).

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- -------------------------------------------------------
-- users
-- -------------------------------------------------------
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- -------------------------------------------------------
-- api_tokens  (bcrypt hash stored, raw token shown once)
-- -------------------------------------------------------
CREATE TABLE api_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    token_hash  TEXT        NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_tokens_user_id ON api_tokens (user_id);

-- -------------------------------------------------------
-- secrets  (AES-256-GCM encrypted at rest, never returned raw)
-- -------------------------------------------------------
CREATE TABLE secrets (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    encrypted_value TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- -------------------------------------------------------
-- monitors
-- -------------------------------------------------------
CREATE TABLE monitors (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    type             TEXT        NOT NULL CHECK (type IN ('http', 'https', 'tcp', 'udp', 'websocket')),
    target           TEXT        NOT NULL,
    interval_seconds INTEGER     NOT NULL DEFAULT 60 CHECK (interval_seconds > 0),
    timeout_seconds  INTEGER     NOT NULL DEFAULT 10 CHECK (timeout_seconds > 0),
    -- lifecycle status (active / paused by operator)
    status           TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    -- last known probe state
    state            TEXT        NOT NULL DEFAULT 'unknown' CHECK (state IN ('up', 'down', 'unknown')),
    last_checked_at  TIMESTAMPTZ,
    next_check_at    TIMESTAMPTZ,
    -- protocol-specific settings (e.g. expected_status, headers, payload)
    settings         JSONB       NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Scheduler priority queue: always scans by next_check_at
CREATE INDEX idx_monitors_next_check_at ON monitors (next_check_at)
    WHERE status = 'active';

-- List/filter by state + recency
CREATE INDEX idx_monitors_status_created_at ON monitors (status, created_at);

-- -------------------------------------------------------
-- incidents
-- -------------------------------------------------------
CREATE TABLE incidents (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id  UUID        NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
    started_at  TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    cause       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_incidents_monitor_id_started_at ON incidents (monitor_id, started_at DESC);
-- Open incident lookup
CREATE INDEX idx_incidents_unresolved ON incidents (monitor_id)
    WHERE resolved_at IS NULL;

-- -------------------------------------------------------
-- check_results  (time-series monitor history in TimescaleDB)
-- -------------------------------------------------------
CREATE TABLE check_results (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    monitor_id  UUID        NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
    checked_at  TIMESTAMPTZ NOT NULL,
    state       TEXT        NOT NULL CHECK (state IN ('up', 'down')),
    latency_ms  INTEGER,
    status_code INTEGER,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, checked_at)
);

CREATE INDEX idx_check_results_monitor_id_checked_at ON check_results (monitor_id, checked_at DESC);

SELECT create_hypertable('check_results', 'checked_at', if_not_exists => TRUE);

