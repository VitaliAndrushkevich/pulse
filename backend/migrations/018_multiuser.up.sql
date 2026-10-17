ALTER TABLE users
 ADD COLUMN role TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'read_only')),
 ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true,
 ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN session_version BIGINT NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX users_email_normalized ON users (lower(email));
