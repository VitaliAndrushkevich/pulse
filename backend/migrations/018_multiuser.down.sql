DROP INDEX users_email_normalized;
ALTER TABLE users DROP COLUMN session_version, DROP COLUMN must_change_password, DROP COLUMN enabled, DROP COLUMN role;
