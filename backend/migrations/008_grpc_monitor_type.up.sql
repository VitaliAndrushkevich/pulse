-- 008_grpc_monitor_type.up.sql
-- Add 'grpc' to the monitors type constraint.

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'tcp', 'udp', 'websocket', 'grpc'));
