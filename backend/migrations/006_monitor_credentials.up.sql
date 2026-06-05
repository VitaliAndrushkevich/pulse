CREATE TABLE monitor_credentials (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id      UUID        NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
    auth_type       TEXT        NOT NULL CHECK (auth_type IN ('bearer', 'basic', 'header')),
    name            TEXT        NOT NULL,
    encrypted_value TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_monitor_credentials_monitor_id ON monitor_credentials (monitor_id);
