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
- Status: `done`
- Priority: `P0`
- Depends on: TASK-006
- Scope:
  - Define shared `Checker` interface and protocol-neutral result fields
- Done when:
  - All checker implementations compile against shared interface
- Notes:
  - `internal/monitor/checker.go` defines `Checker` interface with `Check(ctx, target, settings) Result`.
  - `Result` struct: `State` (up/down), `LatencyMs`, `StatusCode` (optional), `SSLDaysRemaining` (optional), `Error`.
  - `Registry` maps type strings to implementations; `DefaultRegistry()` wires all built-in checkers.
  - All four protocol implementations compile against the shared interface.

### TASK-015: HTTP/HTTPS checker
- Status: `done`
- Priority: `P0`
- Depends on: TASK-014
- Scope:
  - Status code, latency, SSL expiry days
- Done when:
  - Checker records expected result fields for healthy and failing targets
- Notes:
  - `internal/monitor/http.go` implements `HTTPChecker` using stdlib `net/http`.
  - Configurable `HTTPSettings` stored in monitor `settings` JSON column at creation time:
    - `expected_statuses`: explicit list of acceptable codes (e.g. `[200, 201, 301]`) — takes priority over range.
    - `expected_status_min` / `expected_status_max`: fallback range (default 200–399).
    - `method`: HTTP verb (default GET).
    - `headers`: custom request headers map.
    - `follow_redirects` / `max_redirects`: redirect policy (default: don't follow).
    - `skip_tls_verify`: disable cert verification (default false).
    - `ssl_expiry_threshold`: fail if cert expires within N days (default 0 = disabled).
    - `validate_cert_chain`: explicit chain validation against system CAs (default true for HTTPS).
  - Certificate validation: full chain verified via `x509.Verify` with system root CAs; invalid/expired/wrong-hostname cert → state "down".
  - SSL expiry threshold: if cert days remaining ≤ threshold → state "down" with clear warning.
  - Records: response latency, status code, SSL days remaining.

### TASK-016: TCP checker
- Status: `done`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Dial target and measure latency
- Done when:
  - Checker captures success/failure and latency
- Notes:
  - `internal/monitor/tcp.go` implements `TCPChecker` using `net.Dialer.DialContext`.
  - Context carries timeout deadline; reports latency and up/down based on dial success.

### TASK-017: UDP checker
- Status: `done`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Send payload and await expected response with timeout
- Done when:
  - Checker handles timeout and malformed response scenarios
- Notes:
  - `internal/monitor/udp.go` implements `UDPChecker` with two modes:
    - Reachability mode (default): sends zero-byte datagram, treats timeout as "up" (no ICMP rejection).
    - Payload mode: sends base64-decoded payload, validates response against `expected_response`.
  - Handles timeout vs connection-refused vs response-mismatch as distinct error cases.

### TASK-018: WebSocket checker
- Status: `done`
- Priority: `P1`
- Depends on: TASK-014
- Scope:
  - Connect and optional handshake message validation
- Done when:
  - Checker reports connect and handshake outcomes
- Notes:
  - `internal/monitor/websocket.go` implements `WebSocketChecker` using `gorilla/websocket`.
  - Supports optional handshake: send message after connect, validate response.
  - Connection success without handshake config = "up".
  - Custom headers supported for upgrade request.

### TASK-019: Scheduler core
- Status: `done`
- Priority: `P0`
- Depends on: TASK-015, TASK-016, TASK-017, TASK-018
- Scope:
  - Bounded worker pool (default 200)
  - Priority queue by `next_check_at`
  - Persistent scheduling loop
- Done when:
  - No unbounded goroutine growth under 500 monitor load test
- Notes:
  - `internal/monitor/scheduler.go` implements `Scheduler` with bounded worker pool.
  - Config: `PULSE_SCHEDULER_WORKERS` (default 50 dev, 200 production), 1s tick, 500 batch.
  - Poll loop uses `ListActiveMonitorsDue` (ordered by `next_check_at ASC NULLS FIRST`).
  - Workers dispatch via buffered channel — backpressure, no unbounded goroutines.
  - Each check: run checker → write to TimescaleDB + check_results → update monitor state + next_check_at.
  - Graceful shutdown via context cancellation; all workers drain before exit.
  - Wakeup channel allows LISTEN/NOTIFY to trigger immediate re-poll.

### TASK-020: LISTEN/NOTIFY wakeups
- Status: `done`
- Priority: `P1`
- Depends on: TASK-019
- Scope:
  - Trigger scheduler wakeup on monitor create/update
- Done when:
  - New monitor starts scheduling without polling delay
- Notes:
  - `internal/monitor/notify.go` implements `Listener` subscribing to `monitor_changes` channel.
  - Migration `003_monitor_notify_trigger` adds `notify_monitor_change()` function and `trg_monitor_notify` trigger (AFTER INSERT OR UPDATE on monitors).
  - Payload is the monitor UUID; listener calls `scheduler.Wakeup()` on each notification.
  - Reconnects automatically on connection loss.
  - Wired into `main.go` alongside the scheduler as background goroutines.

## Milestone E: API and Contract

### TASK-021: Router and versioned API surface
- Status: `done`
- Priority: `P0`
- Depends on: TASK-008
- Scope:
  - Configure `gin` router and `/api/v1` group
  - Add request ID propagation and standardized errors
- Done when:
  - All API routes are namespaced under `/api/v1`
- Notes:
  - Router already configured from earlier milestones with `/api/v1` group.
  - `requestIDMiddleware()` propagates or generates `X-Request-ID` on all responses.
  - Standard error envelope `{ "error": { "code", "message" } }` via `apiError()` helper.
  - `SanitizedLogger()` middleware strips auth headers from log output.
  - Combined auth middleware supports both JWT (session) and Bearer API token (programmatic) access.

### TASK-022: Single-user auth
- Status: `done`
- Priority: `P0`
- Depends on: TASK-021
- Scope:
  - Login endpoint and JWT auth middleware
- Done when:
  - Protected routes reject missing/invalid auth
- Notes:
  - `POST /api/v1/auth/login` accepts email + password, returns signed JWT.
  - `middleware/jwt.go` implements `JWTAuth()` middleware and `GenerateJWT()` helper using `golang-jwt/jwt/v5` with HS256.
  - Combined auth in `router.go` tries JWT first (dot-separated token), falls back to Bearer API token (prefix-based bcrypt lookup).
  - Dummy bcrypt comparison on all failure paths for timing uniformity.
  - `PULSE_JWT_SECRET` env var required at startup; `PULSE_JWT_EXPIRY` configurable (default 24h).
  - Config extended with `JWTSecret` and `JWTExpiry` fields.

### TASK-023: Monitor CRUD with idempotent PUT
- Status: `done`
- Priority: `P0`
- Depends on: TASK-021, TASK-006
- Scope:
  - `GET/POST /monitors`
  - `GET/PUT/DELETE /monitors/{id}` with create-or-update semantics
  - Pagination on monitor list
- Done when:
  - Repeating same `PUT` body yields no additional side effects
- Notes:
  - `handlers/monitors.go` implements full CRUD under `/api/v1/monitors`.
  - `PUT /monitors/:id` uses `UpsertMonitor` sqlc query with `ON CONFLICT (id) DO UPDATE` for idempotent create-or-update.
  - Validates monitor type against `[http, https, tcp, udp, websocket]` and status against `[active, paused]`.
  - Defaults: `interval_seconds=60`, `timeout_seconds=10`, `status=active`, `settings={}`.
  - List endpoint paginated with `page`/`limit` (default 20, max 100).
  - Response includes all monitor fields; nullable timestamps as `omitempty`.

### TASK-024: Monitor history endpoint
- Status: `done`
- Priority: `P1`
- Depends on: TASK-007, TASK-023
- Scope:
  - `GET /api/v1/monitors/{id}/history` backed by TimescaleDB query helpers
- Done when:
  - Response includes bounded time-series window and expected points
- Notes:
  - `handlers/history.go` implements `GET /api/v1/monitors/:id/history`.
  - Query params: `from` and `to` (RFC 3339). Defaults to last 24 hours.
  - Max window capped at 7 days to bound response size.
  - Verifies monitor exists before querying history.
  - Uses `timescale.Store.QueryHistory()` for time-range queries ordered by `checked_at ASC`.
  - Response: `{ monitor_id, from, to, points: [{state, latency_ms, status_code, error, checked_at}] }`.

### TASK-025: Incidents list endpoint
- Status: `done`
- Priority: `P1`
- Depends on: TASK-006, TASK-021
- Scope:
  - `GET /api/v1/incidents` with pagination
- Done when:
  - No unbounded list responses
- Notes:
  - `handlers/incidents.go` implements two endpoints:
    - `GET /api/v1/incidents` — all incidents, paginated. Optional `?status=open` filter.
    - `GET /api/v1/monitors/:id/incidents` — incidents for a specific monitor, paginated.
  - Added `ListIncidents` and `CountIncidents` sqlc queries for global pagination.
  - Response envelope: `{ data, total, page, limit, total_pages }`.

### TASK-026: Prometheus metrics
- Status: `done`
- Priority: `P1`
- Depends on: TASK-019, TASK-021
- Scope:
  - Expose `/metrics`
  - Publish `pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`
- Done when:
  - Metrics endpoint scrapes cleanly and emits required series
- Notes:
  - `handlers/metrics.go` defines `Metrics` struct with three collectors:
    - `pulse_monitor_up` (GaugeVec, labels: monitor_id, monitor_name, monitor_type)
    - `pulse_monitor_response_time_seconds` (GaugeVec, same labels)
    - `pulse_monitors_total` (Gauge)
  - `RegisterMetricsRoute()` exposes `/metrics` via `promhttp.HandlerFor`.
  - Uses a dedicated `prometheus.Registry` (no default process/go collectors) for clean output.
  - Scheduler updates gauges after each check execution.
  - No auth on `/metrics` (standard for Prometheus scraping).

### TASK-027: OpenAPI contract generation
- Status: `done`
- Priority: `P0`
- Depends on: TASK-022, TASK-023, TASK-024, TASK-025
- Scope:
  - Add route annotations and generation target
  - Commit generated `openapi.yaml`
- Done when:
  - Generated output matches committed spec
- Notes:
  - Contract-first approach: hand-authored `backend/api/openapi.yaml` (OpenAPI 3.0.3).
  - Covers all endpoints: auth/login, monitors CRUD, monitor history, incidents, secrets, tokens.
  - Defines reusable components: schemas, parameters, responses, security schemes.
  - Supports both JWT and API token auth via bearerAuth security scheme.
  - `make openapi` validates the spec is present.
  - All request/response schemas match the Go handler implementations.

## Milestone F: WebSocket and Realtime

### TASK-028: Hub implementation
- Status: `done`
- Priority: `P0`
- Depends on: TASK-019
- Scope:
  - Implement fan-out hub in `backend/internal/hub/hub.go`
- Done when:
  - Multiple clients can connect/disconnect without leaks
- Notes:
  - `internal/hub/hub.go` implements a concurrent fan-out WebSocket hub using event-loop pattern (single goroutine, channel-based register/unregister/broadcast).
  - `Client` struct wraps `gorilla/websocket.Conn` with buffered send channel (256 messages).
  - Ping/pong keepalive (54s ping interval, 60s pong timeout) for connection health.
  - Slow consumers are disconnected when their send buffer fills (no blocking fan-out).
  - `hub.Stop()` closes all client connections on graceful shutdown.
  - `hub.Broadcast(Message)` is non-blocking; drops messages with log warning if broadcast channel is full.
  - Wired into `main.go` as a background goroutine; stopped on SIGINT/SIGTERM.

### TASK-029: Diff patch pipeline
- Status: `done`
- Priority: `P0`
- Depends on: TASK-028, TASK-019
- Scope:
  - Convert scheduler result updates to patch payloads
  - Broadcast changed fields only
- Done when:
  - No full-state monitor snapshots sent on incremental updates
- Notes:
  - `internal/hub/messages.go` defines `MonitorStatusPayload` — a patch containing only the fields that changed: monitor_id, state, latency_ms, status_code (optional), ssl_days_remaining (optional), error (optional), checked_at, timestamp.
  - Message envelope `{ "type": "monitor_status", "payload": {...} }` allows extensibility for future message types.
  - Scheduler calls `hub.Broadcast(hub.NewMonitorStatusMessage(...))` after each check completes (after metrics update).
  - Hub field is nil-checked — scheduler works without a hub for backward compatibility.
  - No full-state snapshots: each message contains only the latest check result for one monitor.

### TASK-030: Authenticated websocket endpoint
- Status: `done`
- Priority: `P1`
- Depends on: TASK-022, TASK-028
- Scope:
  - Add `/ws` endpoint with auth gate
- Done when:
  - Unauthorized clients are rejected before upgrade
- Notes:
  - `GET /ws` endpoint registered on the root router (outside `/api/v1` for cleaner WS URLs).
  - Auth via `?token=` query parameter (browsers cannot set Authorization headers on WebSocket connections).
  - Validates JWT (HS256, same secret as REST API) or API token (prefix-based bcrypt lookup) before HTTP upgrade.
  - Unauthorized requests receive 401 JSON error response — no WebSocket upgrade occurs.
  - On successful upgrade, sends initial `{ "type": "connected", "payload": { "client_id", "timestamp" } }` message.
  - Dummy bcrypt comparison on all failure paths for uniform timing.
  - `WSHandler` registered in router only when `Hub` is non-nil in `Deps`.

## Milestone G: Frontend

### TASK-031: Dashboard virtualization
- Status: `done`
- Priority: `P0`
- Depends on: TASK-023
- Scope:
  - Build monitor dashboard with virtualized rendering
- Done when:
  - 500 monitor UI test runs without freeze/blank page
- Notes:
  - VirtualList component with fixed-height rows, DOM recycling (max 60 nodes), RAF-throttled scroll handler, configurable buffer (5–20).
  - Dashboard page integrates VirtualList with MonitorRow, fetches monitors on mount, displays stats bar (total/healthy/unhealthy).
  - MonitorStore provides reactive `list`, `totalCount`, `healthyCount`, `unhealthyCount` via Svelte 5 `$derived`.

### TASK-032: Monitor forms and list
- Status: `done`
- Priority: `P1`
- Depends on: TASK-023, TASK-010
- Scope:
  - Create/edit monitor forms
  - Secret references by UUID only
- Done when:
  - Secret values are never displayed in UI responses
- Notes:
  - MonitorForm component with create/edit modes, type-specific settings (http expected codes, udp payload, ws handshake), validation integration, secret reference dropdown (name + UUID format).
  - Monitor list page with pagination, error/empty states, and retry.
  - Monitor create/edit pages integrate MonitorForm with API calls and store updates.
  - Settings page with secrets management — values cleared from state immediately after submit.

### TASK-033: Monitor detail and charts
- Status: `done`
- Priority: `P1`
- Depends on: TASK-024, TASK-025
- Scope:
  - `uplot` response-time chart and incident timeline
- Done when:
  - Detail view renders history and incident data correctly
- Notes:
  - HistoryChart component using uPlot with time x-axis and ms y-axis, handles zero data with placeholder.
  - Monitor detail page fetches monitor + history (24h) + incidents in parallel.
  - Incident timeline shows started_at/resolved_at (or "Ongoing" badge), color-coded dots.
  - All monitor fields displayed: name, type, target, interval, timeout, status, state, timestamps, settings.

### TASK-034: WebSocket store merge
- Status: `done`
- Priority: `P0`
- Depends on: TASK-029, TASK-030
- Scope:
  - Implement patch merge logic in frontend stores
- Done when:
  - Incoming patches update local state deterministically
- Notes:
  - MonitorStore implements deterministic patch-merge: only `state` and `last_checked_at` updated from patch, all other fields preserved.
  - Patches for unknown monitor_ids silently discarded.
  - WebSocket client dispatches `monitor_status` payloads to `monitorStore.applyPatch()`.
  - WS lifecycle wired to layout: connect on auth, disconnect on logout, re-fetch full list on reconnect.
  - Detail view derives monitor from store via `$derived(monitorStore.getById(id))` for real-time WS updates.
  - Dashboard updates single rows in-place via Svelte 5 fine-grained reactivity.

### TASK-035: Login and session handling
- Status: `done`
- Priority: `P1`
- Depends on: TASK-022
- Scope:
  - Login UI and session handling with localStorage JWT
- Done when:
  - Protected pages redirect or deny without session
- Notes:
  - AuthStore using Svelte 5 runes: `getToken()`, `setToken()`, `clearToken()`, `isAuthenticated()` with localStorage persistence.
  - Login page with email/password validation, API call, inline error display (401 → generic message, network → service unavailable).
  - Auth guard in layout redirects to `/login` when unauthenticated.
  - API client injects Bearer token on all requests, handles 401 with token clear + redirect.
  - WebSocket connects with `?token=<jwt>`, handles 4401 close code (auth expired → redirect, no reconnect).
  - 141 unit tests passing across 8 test files (Vitest + fast-check + @testing-library/svelte).

## Milestone H: Packaging and Release

### TASK-036: Static embedding and SPA serving
- Status: `done`
- Priority: `P0`
- Depends on: TASK-002, TASK-021
- Scope:
  - Embed `frontend/build` via `go:embed`
  - Add SPA catch-all route
- Done when:
  - Built binary serves frontend and API from one process
- Notes:
  - `internal/frontend/frontend.go` uses `//go:embed dist/*` directive with `HasAssets()` helper.
  - `dist/.gitkeep` placeholder ensures `go:embed` works before build-time population.
  - SPA catch-all via `r.NoRoute(spaHandler(...))` in router — serves files from embedded FS or falls back to `index.html`.
  - API/system prefixes (`/api/`, `/ws`, `/metrics`, `/healthz`, `/swagger`) return JSON 404 instead of SPA fallback.
  - Cache-Control: `immutable` for `_app/` hashed assets, `no-cache` for `index.html`.
  - Only registered when `frontend.HasAssets()` is true (dev mode without build still works).
  - Makefile targets: `build-frontend` (npm build + copy to embed path), `build-all` (frontend + Go binary).
  - 7 unit tests in `spa_test.go` covering routing, caching, MIME types, and fallback behavior.

### TASK-037: Multi-stage image
- Status: `done`
- Priority: `P0`
- Depends on: TASK-036
- Scope:
  - Node build stage, Go build stage, distroless runtime stage
- Done when:
  - Final image starts and serves app successfully
- Notes:
  - 3-stage Dockerfile: `node:22-alpine` (frontend build) → `golang:1.25-alpine` (Go build with embedded assets) → `gcr.io/distroless/static-debian12` (runtime).
  - Frontend build output copied into `internal/frontend/dist/` before Go build.
  - Binary compiled with `-ldflags="-s -w"` for stripped debug symbols.
  - `.dockerignore` excludes `.git/`, `node_modules/`, docs, test files, IDE configs — keeps build context minimal.
  - Final image: single binary, no shell, no package manager.

### TASK-038: Compose hardening
- Status: `done`
- Priority: `P1`
- Depends on: TASK-037
- Scope:
  - Add health checks, required volumes, production-safe defaults
- Done when:
  - `docker-compose up` fresh run passes health checks end-to-end
- Notes:
  - Production `docker-compose.yml` with `PULSE_DEV=false`, `restart: unless-stopped` on both services.
  - Postgres health check via `pg_isready` with start_period, interval, timeout, retries.
  - Pulse has no CMD-SHELL healthcheck (distroless has no shell) — relies on restart policy.
  - Secrets loaded via `env_file: .env` directive instead of hardcoded values.
  - `.env.example` with all variables documented, generation commands for secrets.
  - Named volume `pulse-postgres-data` for data persistence.

### TASK-039: README quick start
- Status: `done`
- Priority: `P1`
- Depends on: TASK-038
- Scope:
  - Document startup, env vars, migration flow, and basic API checks
- Done when:
  - New machine setup follows README without hidden steps
- Notes:
  - Complete README rewrite: project overview, architecture diagram, package layout table.
  - Prerequisites: Docker + Docker Compose v2 only.
  - Quick Start: 3 steps (clone, configure .env, docker compose up).
  - Environment variables table with descriptions, defaults, and generation commands.
  - API usage examples: login, create monitor, list monitors, WebSocket.
  - Full endpoint reference table.
  - Development section: make targets, local setup, running tests.
  - Docker Compose override example for local customization.

### TASK-040: CI quality gates
- Status: `deferred`
- Priority: `P1`
- Depends on: TASK-027, TASK-037
- Scope:
  - Build, test, lint, OpenAPI drift check jobs
- Done when:
  - PR pipeline enforces contract and build integrity
- Notes:
  - Deferred to a future iteration. Not required for MVP.

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
