-- 009_monitor_tags.up.sql
-- Normalized tag storage for monitors (key-value pairs).

CREATE TABLE monitor_tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    UNIQUE (monitor_id, key, value)
);

CREATE INDEX idx_monitor_tags_monitor_id ON monitor_tags(monitor_id);
CREATE INDEX idx_monitor_tags_key_value ON monitor_tags(key, value);
CREATE INDEX idx_monitor_tags_key ON monitor_tags(key);
