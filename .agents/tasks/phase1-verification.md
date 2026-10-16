# Phase 1 (Data layer) — Verification Record

Worktree: `/Users/vandrushkevich/workdir/personal/pulse/.worktrees/heartbeat`
Branch: `feat/heartbeat-impl`
Date: 2026-10-16

## Environment

- Go: `go1.25.1 darwin/arm64` (module requires `go 1.26.0`; no `toolchain` directive, so 1.25.1 compiles it without issue).
- sqlc: `v1.31.1` (`/opt/homebrew/bin/sqlc`).
- Docker daemon: **NOT running** — no local Postgres/TimescaleDB available.
- Local Postgres on `localhost:5432`: **port closed / connection refused**.

## Commands run and results

| Command | Result | Notes |
|---------|--------|-------|
| `make -C <worktree> build` | **PASS (exit 0)** | `go build ./cmd/pulse` succeeds with the new generated code. |
| `sqlc generate` (in `backend/`) | **PASS (exit 0)** | Produced `heartbeats.sql.go`, `heartbeat_tags.sql.go`, `heartbeat_bindings.sql.go`, and updated `models.go`. Re-running is idempotent (no further diff). sqlc parses the migration DDL as its schema input, so the migration SQL is syntactically valid and consistent with the queries. |
| `go test ./internal/store/postgres/` | **PASS (exit 0)** | Package compiles; the new `heartbeats_integration_test.go` compiles and runs. Its tests **SKIP** because `PULSE_RUN_DB_TESTS != 1` and no Postgres is reachable. |
| `make -C <worktree> test` | **PASS (exit 0)** | Full `go test ./...` green. All existing suites pass; heartbeat integration tests skip (see below). |
| `make -C <worktree> migrate` | **FAILED (exit 1) — expected** | `dial tcp [::1]:5432: connect: connection refused`. No Postgres running in this environment. |
| `make -C <worktree> migrate-down` | **NOT RUN** | Same blocker — Postgres unreachable. |

## Honest status of DB-dependent verification

The following were **NOT executed** because no local Postgres/TimescaleDB was reachable (Docker daemon down, port 5432 closed):

- `make migrate` / `make migrate-down` round-trip (017 up/down applied against a live DB).
- The `heartbeats_integration_test.go` cases:
  - `TestMigration017_UpDown` (migration down-then-up round-trip)
  - `TestCreateAndGetHeartbeat`
  - `TestMarkHeartbeatUp_ReturnsPriorState`
  - `TestMarkHeartbeatDown_FiresOnce`
  - `TestListDueHeartbeats_PartialIndexExclusions`
  - `TestDeleteHeartbeat_CascadesTagsAndBindings`

These tests are gated on `PULSE_RUN_DB_TESTS=1` (mirroring `internal/store/timescale/store_test.go`) and `t.Skip` when unset / when the DB is unavailable. They **compile and run** (verified via `go test`), but they **did not assert against a real database** in this environment. **I am not claiming the integration tests passed.**

### To run the DB-backed verification locally

```sh
# start Postgres/TimescaleDB
docker compose -f docker-compose.dev.yml up postgres -d

# apply + roll back migration 017 (and all others)
make migrate
make migrate-down
make migrate   # re-apply

# run the heartbeat integration tests against the live DB
cd backend && PULSE_RUN_DB_TESTS=1 go test ./internal/store/postgres/ -run Heartbeat -v
cd backend && PULSE_RUN_DB_TESTS=1 go test ./internal/store/postgres/ -run Migration017 -v
```

## Static evidence gathered without a DB

- `sqlc generate` consumes `backend/migrations/*.sql` as its schema; it accepted the 017 DDL and generated typed queries that reference the new columns/tables, so the migration SQL is well-formed PostgreSQL and the query set is schema-consistent.
- The generated `MarkHeartbeatUp` / `MarkHeartbeatFail` rows carry a `PreviousState` field (CTE round-trip), and `MarkHeartbeatDown` carries the `WHERE id=$1 AND state='up'` guard — confirmed by reading the generated `heartbeats.sql.go`.
