-- 001_initial.down.sql
-- Rolls back 001_initial.up.sql (drop in reverse dependency order).

DROP TABLE IF EXISTS check_results;
DROP TABLE IF EXISTS incidents;
DROP TABLE IF EXISTS monitors;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS timescaledb;
