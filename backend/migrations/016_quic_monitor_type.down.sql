-- Remove QUIC monitor type.
DELETE FROM check_results WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'quic');
DELETE FROM monitors WHERE type = 'quic';

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors
    ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'http3', 'tcp', 'udp', 'websocket', 'grpc', 'dns', 'icmp', 'smtp'));
