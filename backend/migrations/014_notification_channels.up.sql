-- 014_notification_channels.up.sql
-- Notification channels: email/webhook alerting with per-monitor bindings and delivery audit log.

-- -------------------------------------------------------
-- notification_channels
-- -------------------------------------------------------
CREATE TABLE notification_channels (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(100) NOT NULL,
    channel_type VARCHAR(20)  NOT NULL CHECK (channel_type IN ('email', 'webhook')),
    config       JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- -------------------------------------------------------
-- channel_bindings
-- -------------------------------------------------------
CREATE TABLE channel_bindings (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id                UUID        NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    monitor_id                UUID        NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    triggers                  JSONB       NOT NULL,
    reminder_interval_minutes INT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_channel_monitor UNIQUE (channel_id, monitor_id)
);

CREATE INDEX idx_channel_bindings_monitor_id ON channel_bindings(monitor_id);

-- -------------------------------------------------------
-- delivery_logs
-- Note: monitor_id intentionally has no FK — audit trail persists after monitor deletion.
-- binding_id is nullable for the same reason.
-- -------------------------------------------------------
CREATE TABLE delivery_logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id   UUID        NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    monitor_id   UUID        NOT NULL,
    binding_id   UUID,
    trigger_type VARCHAR(50) NOT NULL,
    attempt      INT         NOT NULL DEFAULT 1,
    status       VARCHAR(20) NOT NULL CHECK (status IN ('success', 'failure')),
    error_detail VARCHAR(1024),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_logs_channel_created ON delivery_logs(channel_id, created_at);
CREATE INDEX idx_delivery_logs_monitor ON delivery_logs(monitor_id);

-- -------------------------------------------------------
-- smtp_settings (singleton row, managed via UI/API)
-- -------------------------------------------------------
CREATE TABLE smtp_settings (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    host         VARCHAR(255) NOT NULL,
    port         INT          NOT NULL CHECK (port BETWEEN 1 AND 65535),
    username     VARCHAR(255),
    password_enc BYTEA,
    from_address VARCHAR(254) NOT NULL,
    tls_enabled  BOOLEAN      NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
