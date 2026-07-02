-- Proto sources: stores FileDescriptorSet binary data per monitor for gRPC schema-aware payloads.
CREATE TABLE proto_sources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id      UUID NOT NULL UNIQUE REFERENCES monitors(id) ON DELETE CASCADE,
    source_type     TEXT NOT NULL CHECK (source_type IN ('upload', 'reflection')),
    descriptor_bytes BYTEA NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_proto_sources_monitor_id ON proto_sources(monitor_id);
