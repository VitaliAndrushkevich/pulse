-- 008_grpc_monitor_type.down.sql
-- Remove 'grpc' from the monitors type constraint.
-- Delete any existing gRPC monitors first to satisfy the restored constraint.

DELETE FROM monitors WHERE type = 'grpc';

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'tcp', 'udp', 'websocket'));
