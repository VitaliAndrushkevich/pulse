-- Refuse rollback while native channels exist; never silently remove channels.
BEGIN;
ALTER TABLE notification_channels
    DROP CONSTRAINT notification_channels_channel_type_check;
ALTER TABLE notification_channels
    ADD CONSTRAINT notification_channels_channel_type_check
    CHECK (channel_type IN ('email', 'webhook'));
COMMIT;
