-- Remove HTTP/3 monitor type support.
ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors
    ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'tcp', 'udp', 'websocket', 'grpc'));


