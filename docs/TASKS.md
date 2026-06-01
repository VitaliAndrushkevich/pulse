# Pulse MVP Task Board

This task board converts [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) into execution-ready tickets.

## How To Use
- Status values: `todo`, `in_progress`, `blocked`, `review`, `done`
- Priority values: `P0` critical, `P1` high, `P2` normal
- Keep API-first order: backend/API tasks must be `done` before dependent UI tasks close
- Track dependencies using task IDs

## Milestone A: Foundations

### TASK-001: Bootstrap backend module
- Status: `done`
- Priority: `P0`
- Depends on: none
- Scope:
  - Initialize Go module `github.com/VitaliAndrushkevich/pulse` in `backend/`
  - Create `backend/cmd/pulse/main.go` entrypoint
  - Add baseline package layout under `backend/internal/`
- Done when:
  - `go mod tidy` succeeds
  - `go run ./cmd/pulse` starts with placeholder server

### TASK-002: Bootstrap frontend app
- Status: `done`
- Priority: `P1`
- Depends on: none
- Scope:
  - Initialize SvelteKit + TypeScript in `frontend/`
  - Add Tailwind and adapter static setup
  - Add baseline route structure
- Done when:
  - `npm run build` in `frontend/` succeeds
  - Static output is generated

### TASK-003: Compose and Make targets
- Status: `done`
- Priority: `P0`
- Depends on: TASK-001
- Scope:
  - Add `docker-compose.yml` and `docker-compose.dev.yml`
  - Add `Makefile` with `dev`, `build`, `migrate`, `test`, `rotate-key`, `openapi`
- Done when:
  - `make dev` brings up services without manual edits

### TASK-004: Migration tooling setup
- Status: `done`
- Priority: `P0`
- Depends on: TASK-001, TASK-003
- Scope:
  - Configure `golang-migrate`
  - Add `backend/migrations/001_initial.sql` scaffold
- Done when:
  - `make migrate` applies and rolls back locally

## Milestone B: Data Layer

### TASK-005: PostgreSQL schema implementation
- Status: `done`
- Priority: `P0`
- Depends on: TASK-004
- Scope:
  - Implement schema for `monitors`, `secrets`, `incidents`, `check_results`, `users`, `api_tokens`
  - Add UUID PKs and required FK constraints
  - Add indexes: `(status, created_at)`, `(next_check_at)` and access-path indexes
- Done when:
  - Migration runs cleanly on empty DB
  - Schema matches plan requirements

### TASK-006: sqlc query layer
- Status: `done`
- Priority: `P0`
- Depends on: TASK-005
- Scope:
  - Add `sqlc.yaml`
  - Create CRUD and list queries with pagination for main resources
  - Generate typed query package under `backend/internal/store/postgres/`
- Done when:
  - Query generation passes in CI/local
  - All list queries support `limit` and `offset` or cursor

### TASK-007: TimescaleDB helpers
- Status: `done`
- Priority: `P1`
- Depends on: TASK-001, TASK-003
- Scope:
  - Add write/query adapters in `backend/internal/store/timescale/`
  - Define schema conventions for monitor history on `check_results` hypertable
- Done when:
  - Test write and range query return expected points

### TASK-008: Fail-fast dependency init
- Status: `done`
- Priority: `P0`
- Depends on: TASK-006, TASK-007
- Scope:
  - Wire Postgres and TimescaleDB startup checks in `main.go`
  - Exit startup on dependency connectivity failure
- Done when:
  - Process exits non-zero when either DB is unavailable
- Notes:
  - `internal/store/postgres/pool.go` adds `Connect` (pgxpool + ping).
  - `main.go` pings Postgres and validates TimescaleDB extension within a 10s startup timeout and
    calls `log.Fatalf` (exit 1) on failure. Verified exit code 1 when Postgres
    is unreachable and a clean boot under `docker compose up`.

## Milestone C: Security and Secrets

### TASK-009: AES-256-GCM crypto module
- Status: `done`
- Priority: `P0`
- Depends on: TASK-005
- Scope:
  - Implement encryption/decryption helpers in `backend/internal/crypto/crypto.go`
  - Load and validate `PULSE_SECRET_KEY` (32-byte base64)
- Done when:
  - Unit tests cover round-trip and invalid key handling
- Notes:
  - `LoadKey` reads env var, base64-decodes, validates 32-byte length.
  - `Encrypt` uses AES-256-GCM with random nonce prepended to output.
  - `Decrypt` splits nonce and authenticates before decrypting.
  - 12 unit tests pass covering round-trip, nonce uniqueness, tamper
    detection, truncation, and all key validation error paths.
  - Uses only Go standard library (no new dependencies).

### TASK-010: Secret write-only API
- Status: `done`
- Priority: `P0`
- Depends on: TASK-006, TASK-009
- Scope:
  - Implement secret CRUD handlers
  - Ensure responses never include raw values
- Done when:
  - API responses return redacted placeholders only
- Notes:
  - `internal/api/handlers/secrets.go` implements full CRUD under `/api/v1/secrets`.
  - Router refactored to accept `Deps` struct (Queries + SecretKey).
  - `main.go` loads `PULSE_SECRET_KEY`, creates `*db.Queries`, passes both to router.
  - Values encrypted with AES-256-GCM (base64-encoded) before DB write.
  - Responses only expose `id`, `name`, `created_at`, `updated_at` — never raw or encrypted values.
  - Pagination on list endpoint (`page`/`limit`, default 20, max 100).
  - Error responses use standard envelope `{ "error": { "code", "message" } }`.
  - 8 unit tests pass covering redaction, encryption round-trip, CRUD, and validation.

### TASK-011: API token lifecycle
- Status: `done`
- Priority: `P0`
- Depends on: TASK-006
- Scope:
  - Implement token create/list/revoke endpoints
  - Store only `bcrypt` hash and metadata
  - Return raw token only at creation
- Done when:
  - Created token cannot be retrieved in raw form afterward
- Notes:
  - Schema migration `002_api_tokens_prefix` adds `prefix` column and partial index for efficient lookup.
  - Pure `internal/token` package: `Generate()` produces 32-byte crypto/rand token (43-char base64url), 8-char prefix, bcrypt hash (cost 10). `ValidateHash()` uses constant-time bcrypt comparison.
  - `TokenHandler` (Create, List, Revoke) follows `SecretHandler` pattern with strict pagination validation (400 on invalid input).
  - `BearerAuth` middleware: prefix-based lookup, bcrypt compare, dummy comparison on failure for uniform timing.
  - Protected route group in router: both TokenHandler and SecretHandler behind BearerAuth.
  - Idempotent revocation via `COALESCE(revoked_at, now())` in SQL.
  - Integration tests cover full lifecycle, ordering, field completeness, idempotence, X-Request-ID, and Content-Type.

### TASK-012: Sanitized logging middleware
- Status: `done`
- Priority: `P0`
- Depends on: TASK-001
- Scope:
  - Add middleware that strips/masks `Authorization` and secret-like fields
- Done when:
  - Logs contain no auth/secret values in integration checks
- Notes:
  - `internal/api/middleware/logging.go` implements `SanitizedLogger()` using `gin.LoggerWithFormatter`.
  - `sanitizeHeaders()` clones request headers and replaces Authorization values with `[REDACTED]`.
  - Integrated as global middleware in `router.go`.
  - Unit tests verify redaction, header preservation, and no mutation of originals.

### TASK-013: Key rotation workflow
- Status: `done`
- Priority: `P1`
- Depends on: TASK-009, TASK-010
- Scope:
  - Implement `make rotate-key` command
  - Re-encrypt all rows transactionally
- Done when:
  - Rotation succeeds atomically or rolls back fully on failure
- Notes:
  - `cmd/rotate/main.go` reads `PULSE_SECRET_KEY` (old) and `PULSE_SECRET_KEY_NEW` (new).
  - Connects to Postgres, begins a transaction, fetches all secrets via `ListAllSecrets`.
  - For each secret: base64-decode → decrypt with old key → encrypt with new key → base64-encode → update row.
  - Commits atomically on success; any failure triggers full rollback.
  - Added `ListAllSecrets` sqlc query (unpaginated, ordered by id).
  - Makefile `rotate-key` target wired to `go run ./cmd/rotate`.
  - Unit test verifies round-trip: old-key encrypt → decrypt → new-key encrypt → new-key decrypt succeeds, old-key decrypt fails.

## Milestone D: Monitor Engine

### TASK-014: Checker interface and result model
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-006
- Scope:
  - Define shared `Checker` interface and protocol-neutral result fields
- Done when:
  - All checker implementations compile against shared interface

### TASK-015: HTTP/HTTPS checker
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-014
- Scope:
  - Status code, latency, SSL expiry days
- Done when:
  - Checker records expected result fields for healthy and failing targets

### TASK-016: TCP checker
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Dial target and measure latency
- Done when:
  - Checker captures success/failure and latency

### TASK-017: UDP checker
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Send payload and await expected response with timeout
- Done when:
  - Checker handles timeout and malformed response scenarios

### TASK-018: WebSocket checker
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Connect and optional handshake message validation
- Done when:
  - Checker reports connect and handshake outcomes

### TASK-019: Scheduler core
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-015, TASK-016, TASK-017, TASK-018
- Scope:
  - Bounded worker pool (default 200)
  - Priority queue by `next_check_at`
  - Persistent scheduling loop
- Done when:
  - No unbounded goroutine growth under 500 monitor load test

### TASK-020: LISTEN/NOTIFY wakeups
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-019
- Scope:
  - Trigger scheduler wakeup on monitor create/update
- Done when:
  - New monitor starts scheduling without polling delay

## Milestone E: API and Contract

### TASK-021: Router and versioned API surface
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-008
- Scope:
  - Configure `gin` router and `/api/v1` group
  - Add request ID propagation and standardized errors
- Done when:
  - All API routes are namespaced under `/api/v1`

### TASK-022: Single-user auth
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-021
- Scope:
  - Login endpoint and JWT auth middleware
- Done when:
  - Protected routes reject missing/invalid auth

### TASK-023: Monitor CRUD with idempotent PUT
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-021, TASK-006
- Scope:
  - `GET/POST /monitors`
  - `GET/PUT/DELETE /monitors/{id}` with create-or-update semantics
  - Pagination on monitor list
- Done when:
  - Repeating same `PUT` body yields no additional side effects

### TASK-024: Monitor history endpoint
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-007, TASK-023
- Scope:
  - `GET /api/v1/monitors/{id}/history` backed by TimescaleDB query helpers
- Done when:
  - Response includes bounded time-series window and expected points

### TASK-025: Incidents list endpoint
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-006, TASK-021
- Scope:
  - `GET /api/v1/incidents` with pagination
- Done when:
  - No unbounded list responses

### TASK-026: Prometheus metrics
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-019, TASK-021
- Scope:
  - Expose `/metrics`
  - Publish `pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`
- Done when:
  - Metrics endpoint scrapes cleanly and emits required series

### TASK-027: OpenAPI contract generation
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-022, TASK-023, TASK-024, TASK-025
- Scope:
  - Add route annotations and generation target
  - Commit generated `openapi.yaml`
- Done when:
  - Generated output matches committed spec

## Milestone F: WebSocket and Realtime

### TASK-028: Hub implementation
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-019
- Scope:
  - Implement fan-out hub in `backend/internal/hub/hub.go`
- Done when:
  - Multiple clients can connect/disconnect without leaks

### TASK-029: Diff patch pipeline
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-028, TASK-019
- Scope:
  - Convert scheduler result updates to patch payloads
  - Broadcast changed fields only
- Done when:
  - No full-state monitor snapshots sent on incremental updates

### TASK-030: Authenticated websocket endpoint
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-022, TASK-028
- Scope:
  - Add `/ws` endpoint with auth gate
- Done when:
  - Unauthorized clients are rejected before upgrade

## Milestone G: Frontend

### TASK-031: Dashboard virtualization
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-023
- Scope:
  - Build monitor dashboard with virtualized rendering
- Done when:
  - 500 monitor UI test runs without freeze/blank page

### TASK-032: Monitor forms and list
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-023, TASK-010
- Scope:
  - Create/edit monitor forms
  - Secret references by UUID only
- Done when:
  - Secret values are never displayed in UI responses

### TASK-033: Monitor detail and charts
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-024, TASK-025
- Scope:
  - `uplot` response-time chart and incident timeline
- Done when:
  - Detail view renders history and incident data correctly

### TASK-034: WebSocket store merge
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-029, TASK-030
- Scope:
  - Implement patch merge logic in frontend stores
- Done when:
  - Incoming patches update local state deterministically

### TASK-035: Login and session handling
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-022
- Scope:
  - Login UI and session handling with httpOnly cookie expectations
- Done when:
  - Protected pages redirect or deny without session

## Milestone H: Packaging and Release

### TASK-036: Static embedding and SPA serving
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-002, TASK-021
- Scope:
  - Embed `frontend/build` via `go:embed`
  - Add SPA catch-all route
- Done when:
  - Built binary serves frontend and API from one process

### TASK-037: Multi-stage image
- Status: `todo`
- Priority: `P0`
- Depends on: TASK-036
- Scope:
  - Node build stage, Go build stage, distroless runtime stage
- Done when:
  - Final image starts and serves app successfully

### TASK-038: Compose hardening
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-037
- Scope:
  - Add health checks, required volumes, production-safe defaults
- Done when:
  - `docker-compose up` fresh run passes health checks end-to-end

### TASK-039: README quick start
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-038
- Scope:
  - Document startup, env vars, migration flow, and basic API checks
- Done when:
  - New machine setup follows README without hidden steps

### TASK-040: CI quality gates
- Status: `todo`
- Priority: `P1`
- Depends on: TASK-027, TASK-037
- Scope:
  - Build, test, lint, OpenAPI drift check jobs
- Done when:
  - PR pipeline enforces contract and build integrity

## Verification Matrix (Must Pass Before MVP Sign-Off)
- VM-001: `make dev` starts all services
- VM-002: Auth login and protected route rejection behavior
- VM-003: API token raw value shown once only
- VM-004: Monitor check writes visible history in TimescaleDB
- VM-005: Secrets redacted in monitor API responses
- VM-006: `/metrics` exposes required series
- VM-007: Frontend handles 500 monitor mock load without freezing
- VM-008: Realtime status updates via websocket patches
- VM-009: TCP, UDP, and WebSocket checks execute successfully
- VM-010: History endpoint returns time-series data
- VM-011: Fresh `docker-compose up` works end-to-end

## Suggested First Sprint (10 Working Items)
1. TASK-001
2. TASK-003
3. TASK-004
4. TASK-005
5. TASK-006
6. TASK-007
7. TASK-008
8. TASK-009
9. TASK-010
10. TASK-021
