-- Add QUIC monitor type support.
ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors
    ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'http3', 'tcp', 'udp', 'websocket', 'grpc', 'dns', 'icmp', 'smtp', 'quic'));
