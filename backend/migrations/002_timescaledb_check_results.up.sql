-- 002_timescaledb_check_results.up.sql
-- Upgrade existing installations from plain PostgreSQL table to TimescaleDB hypertable.

CREATE EXTENSION IF NOT EXISTS timescaledb;
SELECT create_hypertable('check_results', 'checked_at', if_not_exists => TRUE);

