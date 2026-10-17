# Project Guidelines

## Product Intent
Pulse is a self-hosted uptime monitoring platform.

Core outcomes:
- API-first backend with stable, versioned resources under `/api/v1`
- Realtime status updates via native WebSocket
- Operational reliability at 500+ monitors
- Single deployable container with embedded frontend assets

## Architecture Rules
- Backend is Go (`gin` + scheduler + websocket hub) in one binary.
- Frontend is static SvelteKit build embedded into Go.
- PostgreSQL is source of truth for config/state.
- PostgreSQL with TimescaleDB extension stores time-series monitor history.
- REST and OpenAPI contract must be implemented before UI integration.

## API Conventions
- Prefix all endpoints with `/api/v1`. Root-level exceptions: `GET /healthz`, `GET /metrics`, `GET /ws`, dev-only `/swagger`, and the heartbeat ping ingestion routes `GET|POST /ping/{token}` and `GET|POST /ping/{token}/fail`.
- Use stable UUID identifiers for resources.
- `PUT /monitors/{id}` and `PUT /heartbeats/{id}` must be idempotent create-or-update. PUT on a heartbeat never rotates its ping token.
- Heartbeat ping routes are authorized solely by possession of the unguessable token in the URL path (no JWT/API token/session; `security: []` in the spec). Unknown/malformed/rotated tokens return a minimal `404` after a constant-time compare.
- List endpoints must be paginated (`page`, `limit`).
- Error response envelope:
  - `{ "error": { "code": "SOME_CODE", "message": "Readable message" } }`
- Include `X-Request-ID` in responses.
- **OpenAPI contract is the source of truth for the API surface.** Any change to endpoints, request/response schemas, parameters, or error codes MUST be reflected in `backend/api/openapi.yaml` in the same commit. Do not merge handler changes without updating the spec.

## WebSocket Protocol
- Endpoint: `/ws?token=<jwt_or_api_token>` (root-level, not under `/api/v1`).
- Auth via query parameter (browsers cannot send Authorization headers on WS).
- Message envelope: `{ "type": "<message_type>", "payload": { ... } }`.
- Message types: `connected` (sent once after upgrade), `monitor_status` (diff/patch after each check), `heartbeat_status` (diff/patch when a heartbeat's state changes on ping/fail/missed-run).
- `monitor_status` and `heartbeat_status` payloads are patches — clients must merge into local state, not replace entire objects.
- Origin validation: in production, the `Origin` header must match `PULSE_BASE_URL` (scheme+host). In dev mode (`PULSE_DEV=true`) or when `PULSE_BASE_URL` is empty, all origins are allowed.
- Hub drops slow consumers (buffer full) rather than blocking broadcasts.

## Security Requirements
- Never return raw secret values from APIs.
- Encrypt secret values at rest using AES-256-GCM.
- Store API tokens as `bcrypt` hashes; raw token shown only at creation.
- Heartbeat ping tokens are stored as SHA-256 hashes plus an 8-char lookup prefix; the raw ping token is returned only on heartbeat create and rotate-token. GET/LIST never expose the raw token or hash. Verification is a constant-time compare; only the prefix is ever logged.
- Sanitize logs: strip auth headers and known secret fields.
- Combined auth: endpoints accept both JWT (session) and Bearer API token (programmatic).
- WS auth uses constant-time comparison with dummy bcrypt hash on failure (timing-safe).

## Performance Requirements
- Design for 500+ monitors from the start.
- Scheduler must use bounded worker pools, not unbounded goroutines.
- WebSocket messages should be diff/patch payloads, not full-state snapshots.
- Frontend monitor collections must use virtualization for large lists.
- Hub broadcast channel is buffered (256); messages dropped (not blocked) when full.

## Backend Conventions
- Keep core logic in `backend/internal/...` packages.
- Prefer explicit interfaces and small packages over global shared state.
- Use `sqlc` generated queries instead of ORM abstractions.
- Fail fast during startup when dependencies are not reachable.
- Go 1.26, `gin` v1.12, `pgx/v5`, `gorilla/websocket`, `golang-jwt/jwt/v5`.
- Prometheus metrics via `prometheus/client_golang`.
- Migrations via `golang-migrate/v4`.

## Frontend Conventions
- Svelte 5 with SvelteKit, TypeScript strict, Tailwind CSS 3.4.
- **pnpm** is the package manager for the `frontend/` directory — use `pnpm` for dependency installation (`pnpm install`), script execution (`pnpm <script>`), and lockfile management (`pnpm-lock.yaml`).
- Static adapter (`@sveltejs/adapter-static`) — output embedded into Go binary.
- Place API client in `frontend/src/lib/api.ts`.
- Place WebSocket client in `frontend/src/lib/ws.ts`.
- Place stores in `frontend/src/lib/stores/` — deterministic, patch-merge oriented.
- Reusable components in `frontend/src/components/`.
- Avoid blocking renders; large monitor views must remain virtualized.
- **Theming:** Use CSS custom properties from `app.css` (e.g., `var(--color-brand-primary)`) instead of hardcoded Tailwind color classes. Tailwind brand utilities (`bg-brand-500`, `text-brand-600`) resolve to CSS variables automatically.
- **Dark mode:** Controlled via `data-theme` attribute on `<html>`. Never use Tailwind's `dark:` prefix — use `[data-theme="dark"]` selector strategy already configured.
- **i18n / Localization:** All user-visible strings MUST use the `t()` function from `$lib/i18n`. Never hardcode display strings in Svelte templates or component scripts. When adding new UI text (labels, buttons, errors, toasts, empty states), add the corresponding key to `frontend/src/locales/en.json` first, then reference it via `t('section.key')`. Other locale files (`ru.json`, `es.json`, etc.) should also receive the new key — use the English value as a placeholder if no translation is available yet.

## Current Progress

The project is at MVP completion. Full milestone breakdown: [docs/MILESTONES.md](docs/MILESTONES.md).

| Milestone | Status |
|-----------|--------|
| A: Foundations | ✅ Done |
| B: Data Layer | ✅ Done |
| C: Security & Secrets | ✅ Done |
| D: Monitor Engine | ✅ Done |
| E: API Surface | ✅ Done |
| F: WebSocket Realtime | ✅ Done |
| G: Frontend Product | ✅ Done |
| H: Packaging & Release | ✅ Done |
| I: Branding & Theming | ✅ Done |
| J: Notifications | ✅ Done |
| K: Heartbeat (push) Monitoring (PF-01 first useful version) | ✅ Done |

### Completed (A–H):
- PostgreSQL schema + indexes, sqlc query layer, TimescaleDB history store
- Docker Compose infrastructure, fail-fast startup, migration tooling
- AES-256-GCM crypto module, secret write-only API, API token lifecycle, key rotation
- Monitor engine: HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP, QUIC checkers with full protocol support
- Bounded worker pool scheduler (`PULSE_SCHEDULER_WORKERS`, default 50 dev / 200 prod)
- LISTEN/NOTIFY wakeups for instant scheduling on monitor create/update
- Graceful shutdown with context cancellation
- Full `gin` router under `/api/v1` with JWT + API token combined auth
- Monitor CRUD (POST, GET, PUT idempotent, DELETE), history (TimescaleDB 7-day window), incidents (global + per-monitor, paginated)
- Secrets and tokens management (write-only, bcrypt hashing)
- Prometheus `/metrics` endpoint (`pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`)
- OpenAPI 3.0.3 spec committed at `backend/api/openapi.yaml`
- Swagger UI served in dev mode (`PULSE_DEV=true`)
- WebSocket hub with fan-out, ping/pong keepalive, slow-consumer eviction
- Typed message envelope with `monitor_status` (diff/patch) and `connected` message types
- Authenticated `/ws` endpoint with query-param token validation (JWT + API token)
- Scheduler → Hub broadcast (check results sent to hub after execution)
- Notification subsystem: dispatcher with bounded worker pool, email (SMTP) and webhook channels, trigger-based fan-out, exponential backoff retry, delivery logging, reminder scheduler
- Heartbeat (push) monitoring (PF-01 first useful version): `heartbeats` / `heartbeat_tags` / `heartbeat_bindings` tables (migration 017), admin CRUD + idempotent PUT + rotate-token, root-level token-scoped ping ingestion (`/ping/{token}`[/fail]) with SHA-256-hashed tokens, bounded missed-run sweep evaluator (`PULSE_HEARTBEAT_WORKERS`, default 50), once-per-episode down transition, missed-run/recovery notifications through the existing dispatcher, `heartbeat_status` WS broadcast, metrics `pulse_heartbeat_up` / `pulse_heartbeats_total`, binding triggers `heartbeat_missed` / `heartbeat_recovered`, and a localized frontend UI (list/detail/create/edit + nav). Deferred non-goals: run-lifecycle events, timezone-aware cron, concurrency/late-arrival semantics, a dedicated MCP heartbeat tool, restore-validity verification, external receiver, per-ping history
- Full SvelteKit 5 frontend with TypeScript strict mode and Tailwind CSS 3.4
- API client with Bearer auth, 15s timeout, error envelope parsing, X-Request-ID
- WebSocket client with exponential backoff reconnection (1s–30s, ±25% jitter)
- Reactive stores (Svelte 5 runes): Auth, Monitor (patch-merge), Toast, Connection
- VirtualList with DOM recycling (max 60 nodes), MonitorRow, MonitorForm, Pagination, HistoryChart (uPlot), Toast, ConnectionBadge
- All page routes: Login, Dashboard, Monitor list/detail/create/edit, Settings (secrets)
- Real-time updates: WS patches update dashboard rows and detail view in-place
- 218 frontend tests passing (Vitest + fast-check + @testing-library/svelte) — unit + property-based
- Static frontend embedded via `go:embed` with SPA catch-all routing and cache headers
- Multi-stage Dockerfile (node:lts-alpine → golang:1.26-alpine → distroless)
- Production docker-compose with health checks, restart policies, inline environment with shell variable expansion
- `.env.example` with all variables documented
- Complete README with quick start, API examples, architecture docs

### Completed (I: Branding & Theming):
- ECG-inspired logo mark (inline SVG, proportional stroke scaling)
- BrandLockup component (full/compact variants, proportional sizing from `size` prop)
- ThemeSwitcher component (light/dark/system tri-state cycling, localStorage persistence under `pulse-theme-mode`, FOUC prevention)
- CSS custom properties theme system (`:root`/`[data-theme="dark"]` token overrides)
- Tailwind integration: `darkMode: ['selector', '[data-theme="dark"]']`, brand color scale (50–900), semantic aliases
- Self-hosted Inter font (WOFF2, `font-display: swap`)
- Static brand assets (`frontend/static/brand/` — SVG, PNG exports, dark variant, README)
- Favicon, Apple Touch Icon, PWA manifest (`site.webmanifest`)
- Layout integration: responsive lockup in header, ThemeSwitcher in nav, theme-aware token styles
- Login/setup pages with centered BrandLockup
- Property-based tests: stroke proportionality, scaling, WCAG contrast, cycle persistence, icon correctness, token mapping

### Completed (J: Notifications):
- Notification dispatcher with bounded worker pool (`PULSE_NOTIFICATION_WORKERS`, default 50, buffer 256)
- Email (SMTP) channel: TLS support, PLAIN auth, branded HTML templates, encrypted password at rest
- Webhook channel: configurable method/URL/headers, Go `text/template` body rendering, encrypted headers at rest, 1 MB body limit
- Trigger-based fan-out: `monitor_down`, `monitor_up`, `degraded`, `ssl_expiring`, `n_failures_in_row`
- State deduplication via `StateTracker` (prevents repeated alerts for same ongoing condition)
- Exponential backoff retry queue (30s → 60s → 120s, max 4 attempts total)
- Retryable vs non-retryable error classification (`DeliveryError` type)
- Delivery logging: all attempts recorded in `delivery_logs` table (success/failure + error detail, max 1024 chars)
- Reminder scheduler: periodic re-notification (30–1440 min) while condition persists, auto-deactivates on recovery
- Notification channel CRUD API (`/api/v1/notifications/channels`)
- Notification binding CRUD API (`/api/v1/monitors/{id}/notification-bindings`)
- SMTP settings management API (GET/PUT/DELETE + test connection)
- Template variable reference endpoint (`/api/v1/notifications/template-variables`)
- Test notification delivery endpoint (`/api/v1/notifications/channels/{id}/test`)
- Graceful shutdown: drain timeout, in-flight tracking, reject-on-stopping
- Prometheus metrics: `pulse_notification_deliveries_total`, `pulse_notification_dropped_total`, `pulse_notification_in_flight`, `pulse_notification_retry_queue_size`
- Panic recovery in workers (never crashes the dispatcher)
- Scheduler → Dispatcher integration: non-blocking enqueue after each check result

### Completed (CI):
- GitHub Actions PR workflow (`.github/workflows/pull_request_opened.yml`): backend tests (Go race detector, TimescaleDB service), MCP server checks (tests + build, path-filtered to `mcp/**`), frontend checks (type check, locale validation, unit tests, build), concurrency group per PR
- GitHub Actions Release workflow (`.github/workflows/release.yml`): triggered on `v*` tag push, cross-compiles pulse + pulse-mcp for linux/darwin (amd64/arm64), builds and pushes multi-arch Docker image to GHCR with semver tags, attaches binaries + SHA-256 checksums to GitHub Release

## Key Files Reference
| Purpose | Path |
|---------|------|
| CI PR checks | `.github/workflows/pull_request_opened.yml` |
| Release workflow | `.github/workflows/release.yml` |
| Release drafter | `.github/workflows/release-drafter.yml` |
| Go entrypoint | `backend/cmd/pulse/main.go` |
| API router | `backend/internal/api/router.go` |
| WS hub | `backend/internal/hub/hub.go` |
| WS messages | `backend/internal/hub/messages.go` |
| WS handler | `backend/internal/api/handlers/ws.go` |
| Scheduler | `backend/internal/monitor/scheduler.go` |
| Target normalization | `backend/internal/target/normalize.go` |
| Heartbeat evaluator (missed-run sweep) | `backend/internal/heartbeat/evaluator.go` |
| Heartbeat notification dispatch | `backend/internal/heartbeat/dispatch.go` |
| Heartbeat metrics | `backend/internal/heartbeat/metrics.go` |
| Heartbeat deadline recompute helper | `backend/internal/heartbeat/deadline.go` |
| Heartbeat admin CRUD handler | `backend/internal/api/handlers/heartbeats.go` |
| Heartbeat ping ingestion handler | `backend/internal/api/handlers/ping.go` |
| Heartbeat notification-binding handler | `backend/internal/api/handlers/notification_bindings_heartbeat.go` |
| Heartbeat migration | `backend/migrations/017_heartbeats.{up,down}.sql` |
| Heartbeat sqlc queries | `backend/internal/store/postgres/queries/{heartbeats,heartbeat_tags,heartbeat_bindings}.sql` |
| Heartbeat store (frontend, patch-merge) | `frontend/src/lib/stores/heartbeats.svelte.ts` |
| Heartbeat components | `frontend/src/components/{HeartbeatRow,HeartbeatForm,HeartbeatBindings,PingExamples}.svelte` |
| Heartbeat routes | `frontend/src/routes/heartbeats/` |
| Checkers | `backend/internal/monitor/{http,http3,tcp,udp,websocket,grpc,dns,icmp,smtp,quic}.go` |
| Notification dispatcher | `backend/internal/notification/dispatcher.go` |
| Notification types & metrics | `backend/internal/notification/types.go` |
| Notification fan-out | `backend/internal/notification/fanout.go` |
| Notification state tracker | `backend/internal/notification/state_tracker.go` |
| Notification retry queue | `backend/internal/notification/retry.go` |
| Notification reminders | `backend/internal/notification/reminder.go` |
| SMTP email client | `backend/internal/notification/smtp/client.go` |
| SMTP email template | `backend/internal/notification/smtp/template.go` |
| Webhook client | `backend/internal/notification/webhook/client.go` |
| Webhook template validation | `backend/internal/notification/webhook/template.go` |
| Notification channel handlers | `backend/internal/api/handlers/notification_channels.go` |
| Notification binding handlers | `backend/internal/api/handlers/notification_bindings.go` |
| SMTP settings handlers | `backend/internal/api/handlers/smtp_settings.go` |
| sqlc queries | `backend/internal/store/postgres/` |
| TimescaleDB | `backend/internal/store/timescale/` |
| Migrations | `backend/migrations/` |
| OpenAPI spec | `backend/api/openapi.yaml` |
| Frontend API client | `frontend/src/lib/api.ts` |
| Frontend WS client | `frontend/src/lib/ws.ts` |
| Frontend stores | `frontend/src/lib/stores/` |
| Frontend routes | `frontend/src/routes/` |
| Brand lockup component | `frontend/src/components/BrandLockup.svelte` |
| Theme switcher component | `frontend/src/components/ThemeSwitcher.svelte` |
| Theme tokens (CSS) | `frontend/src/app.css` |
| Static brand assets | `frontend/static/brand/` |
| Brand PNG generator | `frontend/scripts/generate-brand-pngs.mjs` |
| Icon PNG generator | `frontend/scripts/generate-icons.mjs` |
| Locale validation script | `frontend/scripts/validate-locales.ts` |
| i18n module | `frontend/src/lib/i18n/` |
| Translation files | `frontend/src/locales/*.json` |

## Build and Test
Primary commands:
- `make dev` — full stack via docker-compose (Pulse + TimescaleDB + frontend dev server)
- `make dev-local` — lightweight compose (backend + postgres only)
- `make run` — `go run ./cmd/pulse` (requires local postgres)
- `make build` — `go build ./cmd/pulse`
- `make test` — `go test ./...`
- `pnpm test` — run frontend unit tests via Vitest (execute from `frontend/` directory)
- `pnpm dev` — run Vite frontend dev server locally with HMR (execute from `frontend/` directory)
- `make migrate` — run migrations up
- `make migrate-down` — roll back last migration
- `make rotate-key` — AES key rotation with transactional re-encryption
- `make openapi` — validate OpenAPI spec

## Infrastructure
- `docker-compose.dev.yml`: Go 1.26 container (hot-reload via `go run`) + TimescaleDB 2.28.3-pg17 + frontend dev server
- Backend port: 8080
- Frontend dev container: service `frontend`, base image `node:lts-alpine`, port 5173, runs Vite dev server with HMR for local frontend development
- Postgres: `pulse:pulse@localhost:5432/pulse`
- Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PULSE_PORT` | HTTP server listen port | `8080` |
| `PULSE_DEV` | Enable development mode (Swagger UI, verbose logs) | *(empty — disabled)* |
| `PULSE_SECRET_KEY` | AES-256-GCM encryption key for secrets at rest | *(required)* |
| `PULSE_JWT_SECRET` | HMAC secret for JWT token signing | *(required)* |
| `DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `PULSE_SCHEDULER_WORKERS` | Bounded worker pool size for monitor checks | `50` |
| `PULSE_HEARTBEAT_WORKERS` | Bounded worker pool size for the heartbeat missed-run sweep evaluator | `50` |
| `PULSE_BASE_URL` | Public URL for email links, webhooks, and WebSocket origin validation | *(empty)* |
| `PULSE_METRICS_USER` | Basic auth username for `/metrics` endpoint | *(empty — no auth)* |
| `PULSE_METRICS_PASSWORD` | Basic auth password for `/metrics` endpoint | *(empty — no auth)* |
| `PULSE_RESET_ADMIN` | Re-enables setup flow for admin credential reset | *(empty — disabled)* |

- ICMP monitoring requires `CAP_NET_RAW` (granted via `cap_add: [NET_RAW]` in docker-compose). Falls back to unprivileged UDP ICMP on Linux 3.0+ when available.

## Delivery Constraints
- Supported protocols: HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP, QUIC.
- Heartbeat (push) monitoring (PF-01 first useful version) is implemented as a resource parallel to monitors: admin CRUD under `/api/v1/heartbeats`, root-level token-scoped ping ingestion (`/ping/{token}`, `/ping/{token}/fail`), a bounded missed-run sweep evaluator, missed-run/recovery notifications through the existing dispatcher, WebSocket `heartbeat_status`, Prometheus metrics, and a full frontend UI (list/detail/create/edit + nav entry). See the "Heartbeats" section for the subsystem detail. A dedicated heartbeat MCP tool is deferred (non-goal) — no existing MCP tool consumes heartbeat endpoints, so none require co-change.
- Future scope only (do not implement unless requested): multi-tenant, status page, Terraform provider. Heartbeat follow-ups deferred as non-goals: run-lifecycle events (run ids / duration), timezone-aware cron schedules, concurrent-run / duplicate / late-arrival / clock-boundary semantics, restore-validity verification, an independently hosted external receiver, and per-ping history/charts.
- Keep infra local-first with Docker Compose and reproducible startup.

## MCP Server
- The MCP server (`mcp/`) is a separate Go binary that proxies Pulse API endpoints to MCP-compatible AI clients.
- **Co-change rule:** When any endpoint consumed by the MCP server is changed in `backend/api/openapi.yaml` or its handler implementation, the corresponding MCP tool handler(s) in `mcp/internal/tools/` and PulseClient method(s) in `mcp/internal/pulseapi/` MUST be updated in the same commit. Affected endpoints:
  - `GET /monitors`
  - `GET /monitors/{id}`
  - `GET /monitors/{id}/stats`
  - `GET /monitors/{id}/history`
  - `GET /incidents`
  - `GET /monitors/{id}/incidents`
  - `POST /monitors`
  - `DELETE /monitors/{id}`
  - `PUT /monitors/{id}`
  - `GET /dashboard/summary`
  - `GET /tags`
  - `GET /tags/{key}`
- **Breaking response-shape changes** (renamed/removed fields, type changes, restructured envelopes) additionally require updating the relevant tool's output projection logic and its property tests in `mcp/internal/tools/`.

## Development Skills
When working on code, reference these skills for domain guidance:

### Frontend (Svelte)
- **svelte-code-writer**: CLI tools for Svelte documentation lookup and code analysis. Use whenever creating/editing Svelte components (.svelte) or modules (.svelte.ts/.svelte.js).
- **svelte-core-bestpractices**: Guidance on fast, robust, modern Svelte code (reactivity, events, styling, library integration). Load for any Svelte component work.

### Backend (Golang)
- Multiple Golang skills available for database, concurrency, error handling, testing, performance, security, observability, and troubleshooting. Load as needed per task domain.

## Notifications

### Architecture
The notification subsystem (`backend/internal/notification/`) is an asynchronous, non-blocking pipeline integrated into the scheduler. When a check completes, the scheduler evaluates trigger conditions and fan-outs delivery jobs to a bounded worker pool — independent from the monitoring workers.

### Channel Types
- **Email (SMTP)**: Branded HTML emails via configurable SMTP server. Instance-wide SMTP settings stored encrypted in DB. Supports TLS, PLAIN auth.
- **Webhook**: HTTP callbacks with Go `text/template` body rendering. Custom headers (encrypted at rest). Configurable method (GET/POST/PUT/PATCH/DELETE).

### Trigger Conditions
Bindings connect a notification channel to a monitor with trigger rules:
- `monitor_down` — fires once on transition to "down" state
- `monitor_up` — fires once on recovery (down → up)
- `degraded` — response time exceeds `threshold_ms` (1–60000 ms)
- `ssl_expiring` — SSL certificate expires within `days_before` (1–365 days)
- `n_failures_in_row` — consecutive failures reach `count` (1–100)

State deduplication prevents repeated notifications for the same ongoing condition.

### Delivery Pipeline
1. Scheduler completes a check → calls `dispatchNotifications`
2. Fan-out: `EvaluateAndDispatch` evaluates trigger conditions against bindings
3. Matching triggers → independent `DeliveryJob` per binding enqueued (non-blocking `select/default`)
4. Worker pool processes jobs with panic recovery and metric tracking
5. Delivery log recorded in `delivery_logs` table (success/failure with error detail)
6. Failed retryable deliveries → exponential backoff retry queue (30s → 60s → 120s, max 4 attempts)
7. Non-retryable errors (template parse, oversized body, decryption) → immediate permanent failure

### Reminders
The `ReminderScheduler` re-enqueues notifications at configurable intervals (30–1440 min) while a triggering condition persists. Deactivates automatically on recovery.

### Template Variables (Webhook)
Available in webhook body templates via Go `text/template` syntax:
- `.Monitor.ID`, `.Monitor.Name`, `.Monitor.URL`, `.Monitor.Target`
- `.Status`, `.PreviousStatus`, `.ResponseTime`
- `.Incident.ID`, `.Incident.StartedAt`, `.Incident.Duration`
- `.Timestamp`, `.BaseURL`

### API Endpoints
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/notifications/channels` | Create notification channel (email/webhook) |
| `GET` | `/api/v1/notifications/channels` | List channels |
| `GET` | `/api/v1/notifications/channels/{id}` | Get channel details |
| `PUT` | `/api/v1/notifications/channels/{id}` | Update channel |
| `DELETE` | `/api/v1/notifications/channels/{id}` | Delete channel |
| `POST` | `/api/v1/notifications/channels/{id}/test` | Send test notification |
| `GET` | `/api/v1/notifications/channels/{id}/delivery-logs` | List delivery logs |
| `GET` | `/api/v1/notifications/template-variables` | Get available template variables |
| `GET` | `/api/v1/notifications/smtp-settings` | Get SMTP config (password redacted) |
| `PUT` | `/api/v1/notifications/smtp-settings` | Create/update SMTP settings |
| `DELETE` | `/api/v1/notifications/smtp-settings` | Remove SMTP settings |
| `POST` | `/api/v1/notifications/smtp-settings/test` | Test SMTP connection |
| `POST` | `/api/v1/monitors/{id}/notification-bindings` | Create binding (channel → monitor + triggers) |
| `GET` | `/api/v1/monitors/{id}/notification-bindings` | List bindings for monitor |
| `PUT` | `/api/v1/monitors/{id}/notification-bindings/{bindingId}` | Update binding triggers |
| `DELETE` | `/api/v1/monitors/{id}/notification-bindings/{bindingId}` | Delete binding |

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `PULSE_NOTIFICATION_WORKERS` | Concurrent notification delivery workers | `50` |
| `PULSE_NOTIFICATION_DRAIN_TIMEOUT` | Graceful shutdown drain timeout (Go duration) | `30s` |
| `PULSE_LOG_LEVEL` | Log verbosity for notification delivery (`warn`, `info`, `debug`) | `warn` |
| `PULSE_BASE_URL` | Public URL for links in email notifications and WebSocket origin validation | *(empty — links omitted)* |

### Prometheus Metrics
- `pulse_notification_deliveries_total{channel_type, outcome}` — delivery attempts by channel and result
- `pulse_notification_dropped_total{channel_type}` — notifications dropped (buffer full or shutting down)
- `pulse_notification_in_flight` — deliveries currently in progress
- `pulse_notification_retry_queue_size` — notifications waiting in retry queue

## Heartbeats

Heartbeat (push) monitoring is the inverse of the probe scheduler: instead of Pulse calling out to a target, the scheduled job calls *in* to Pulse at the end of a successful run (a "dead man's switch"). Pulse records the arrival, persists when the next ping is expected, and raises an alert when that deadline passes with no ping. This closes the blind spot where a host stays online while a cron job (backup, invoice run, cert renewal, nightly import) silently stops firing. It is a sibling resource to monitors, not a monitor type.

Scope: this is the PF-01 first useful version — a heartbeat resource with an expected interval, grace period, last success, and a scoped ingestion token, plus success/explicit-failure pings, missed-run detection, and notifications. Run-lifecycle events (run ids / per-run duration), timezone-aware cron schedules, concurrent-run / duplicate / late-arrival / clock-boundary semantics, a dedicated MCP heartbeat tool, restore-validity verification, an independently hosted external receiver, and per-ping history/charts are explicit non-goals and are not shipped.

### Resource and Status Model

A heartbeat carries two separate fields, mirroring the monitors convention:
- **Lifecycle `status`** (`active` | `paused`) — the configuration intent. A `paused` heartbeat is skipped entirely by the missed-run evaluator.
- **Observed `state`** (`up` | `down` | `paused` | `new`) — what the UI shows. `new` = created but never pinged (no deadline exists yet, so it can never be marked down); `up` = last ping was a success and the deadline has not passed; `down` = the deadline passed with no ping, or an explicit-failure ping arrived; `paused` = surfaced when lifecycle `status = paused`.

Other fields: stable UUID `id`, `name`, `expected_interval_seconds`, `grace_seconds`, `last_success_at` (nullable), `next_expected_deadline` (nullable, persisted = `last_success_at + interval + grace`), `tags`, timestamps, and write-only ping-token material (`token_prefix` for display, `token_hash` for verification).

### Ingestion vs. Administration Separation

Administration (create/list/read/update/delete, rotate-token, bindings) lives under the authenticated `/api/v1/heartbeats` group and requires combined JWT or Bearer API-token auth, exactly like monitors. Ingestion lives at the **root-level** `/ping/{token}` and `/ping/{token}/fail` routes (registered directly on the engine like `/ws`, not under `/api/v1`, and `/ping` is added to the `apiPrefixes` SPA guard). Ping routes carry **no** JWT, API token, or session — authorization is possession of the unguessable token in the URL path, which authorizes reporting for exactly one heartbeat and confers no administrative capability. This hard separation means a ping URL distributed to a cron host can never be used for admin access or secret disclosure.

### Ping Semantics and Deadline Recompute

- **Success ping** (`GET|POST /ping/{token}`): sets `last_success_at = now`, recomputes and persists `next_expected_deadline = now + interval + grace`, transitions `state` to `up`, and fires the recovery notification path if the prior state was `down`. Returns a minimal `200 OK`.
- **Explicit-failure ping** (`GET|POST /ping/{token}/fail`): transitions `state` to `down` and fires the missed/failure path; it does **not** advance `last_success_at`. Returns a minimal `200 OK`.
- Both `GET` and `POST` are accepted (cron wrappers and CI runners vary; any body is ignored). A paused heartbeat still records a success ping's `last_success_at` but is not evaluated for missed runs.
- Unknown/malformed/rotated tokens get a constant-time dummy compare and a minimal `404`, revealing nothing about whether the prefix matched. Only `token_prefix` is logged, never the raw token.
- Notification dispatch and WS broadcast are fired non-blocking after the write, so ingestion latency never depends on delivery or fan-out.

### Periodic-Sweep Evaluator

Missed-run detection reuses the scheduler's bounded-worker-pool pattern (`backend/internal/heartbeat/evaluator.go`, modeled on `monitor/scheduler.go`): a `time.Ticker` loop (default 1s tick) queries a bounded batch (default 500) of due heartbeats via `ListDueHeartbeats`, backed by a **partial index** on `next_expected_deadline WHERE status='active' AND state='up'` so paused, `new`, and already-`down` rows are excluded cheaply at scale. Jobs are dispatched to a fixed worker pool sized by `PULSE_HEARTBEAT_WORKERS` (default 50) — no unbounded goroutines. `markDown` calls `MarkHeartbeatDown` (`WHERE id=$1 AND state='up'`), and only on an actual row update does it set the `pulse_heartbeat_up` gauge to 0, broadcast `heartbeat_status`, and dispatch the missed-run notification — so a heartbeat is marked down **once per episode**, never re-firing on every tick while it stays down. Shutdown is `context`-cancellation: the loop stops, closes the jobs channel, and drains in-flight work without leaking goroutines.

### Ping-Token Storage (justified bcrypt deviation)

API tokens and passwords use bcrypt (see Security Requirements). Heartbeat ping tokens deliberately use **SHA-256** (hex) plus an 8-char lookup prefix instead. The deviation is scoped and justified: a ping token is a high-entropy 256-bit random value (not a low-entropy human secret), so bcrypt's work factor adds no meaningful brute-force resistance, while ping ingestion is a hot path that must stay fast under bursts at 500+ heartbeats — a per-request bcrypt compare would be a needless cost. The raw token is generated via `token.GenerateRaw()` (`crypto/rand` → base64url) and returned exactly once on create and rotate-token; `GET`/`LIST` expose only `token_prefix`. Verification is a constant-time hex compare with a dummy-compare on miss (timing-safe). This deviation applies to ping tokens only; bcrypt remains the rule for API tokens and passwords.

### Notification Integration

Heartbeats reuse the entire notification subsystem rather than running a parallel alerting path. A dedicated `heartbeat_bindings` table (same shape as monitor bindings) connects a channel to a heartbeat with heartbeat-specific triggers, validated by `ValidateHeartbeatTriggers` (allowed set `{heartbeat_missed, heartbeat_recovered}`; monitor triggers such as `degraded`/`ssl_expiring`/`n_failures_in_row` are rejected with `400 VALIDATION_ERROR`). At dispatch, `translateHeartbeatTriggers` maps `heartbeat_missed → monitor_down` and `heartbeat_recovered → monitor_up`, so the existing `StateTracker` (keyed by the heartbeat UUID), fan-out, exponential-backoff retry queue (30s → 60s → 120s, max 4), and `delivery_logs` all work unchanged. The dispatch helper (`backend/internal/heartbeat/dispatch.go`) builds `[]notification.BindingWithChannel` with `MonitorID = heartbeatID` (satisfying `delivery_logs.monitor_id NOT NULL`) and a pre-filled payload builder, then calls `EvaluateAndDispatchWithPayload`. Down alerts fire once per missed-run episode; recovery fires once per `down → up` transition; dedup flags clear on recovery. Deleting a heartbeat cascades its bindings and clears its `StateTracker` state.

### API Endpoints
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/heartbeats` | Create heartbeat (returns raw ping token + ping URLs **once**) |
| `GET` | `/api/v1/heartbeats` | List heartbeats (paginated; never returns raw token/hash) |
| `GET` | `/api/v1/heartbeats/{id}` | Get heartbeat details (`token_prefix` only) |
| `PUT` | `/api/v1/heartbeats/{id}` | Idempotent create-or-update (never rotates the token) |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete heartbeat, cascade bindings, clear state |
| `POST` | `/api/v1/heartbeats/{id}/rotate-token` | Rotate ping token (returns new raw token **once**) |
| `POST` | `/api/v1/heartbeats/{id}/notification-bindings` | Create binding (channel → heartbeat + heartbeat triggers) |
| `GET` | `/api/v1/heartbeats/{id}/notification-bindings` | List bindings for heartbeat (paginated) |
| `PUT` | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Update binding triggers |
| `DELETE` | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Delete binding |
| `GET`/`POST` | `/ping/{token}` | Success ping (root-level, token-in-path, no admin auth) |
| `GET`/`POST` | `/ping/{token}/fail` | Explicit-failure ping (root-level, token-in-path, no admin auth) |

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `PULSE_HEARTBEAT_WORKERS` | Bounded worker-pool size for the missed-run sweep evaluator | `50` |

### Prometheus Metrics
- `pulse_heartbeat_up{heartbeat_id, heartbeat_name}` — per-heartbeat up (1) / down (0) gauge (no token material in labels)
- `pulse_heartbeats_total` — total number of heartbeats

## Localization (i18n)

### Architecture
- i18n module: `frontend/src/lib/i18n/` — config, locale store, resolution, types.
- Translation files: `frontend/src/locales/{code}.json` (one per language).
- Fallback chain: active locale → English (`en.json`) → key string.
- Non-English locales are lazy-loaded via dynamic imports.
- RTL support: locales with `dir: 'rtl'` in `config.ts` automatically set `document.documentElement.dir`.

### Supported Locales (13)
| Code | Language | Direction |
|------|----------|-----------|
| en | English | LTR |
| ar | العربية (Arabic) | **RTL** |
| be | Беларуская (Belarusian) | LTR |
| de | Deutsch (German) | LTR |
| es | Español (Spanish) | LTR |
| fr | Français (French) | LTR |
| it | Italiano (Italian) | LTR |
| ja | 日本語 (Japanese) | LTR |
| ko | 한국어 (Korean) | LTR |
| pt | Português (Portuguese) | LTR |
| ru | Русский (Russian) | LTR |
| tr | Türkçe (Turkish) | LTR |
| zh | 中文 (Chinese) | LTR |

### Adding Strings to New Features (MANDATORY CHECKLIST)
When adding any new UI feature with user-visible text:

1. **Add keys to `en.json` first** — English is the source of truth.
2. **Use `t('section.key')` in components** — never hardcode display strings.
3. **Update ALL locale files** — add the new key to every `*.json` file in `frontend/src/locales/`. Use the English value as a placeholder if no translation is available yet.
4. **Run locale validation**: `pnpm --filter frontend run validate-locales` (if available) to detect missing keys.
5. **Verify RTL** — for layout-affecting strings (long labels, directional icons like `←`), ensure Arabic locale renders correctly. Use `dir`-aware CSS (`margin-inline-start` instead of `margin-left`).

### Adding a New Language
1. Create `frontend/src/locales/{code}.json` with all keys translated.
2. Register in `frontend/src/lib/i18n/config.ts` → `SUPPORTED_LOCALES` array.
   - Add `dir: 'rtl'` if the language is right-to-left.
3. Validate JSON: `python3 -c "import json; json.load(open('frontend/src/locales/{code}.json'))"`.
4. The language will appear automatically in the Settings → Language selector.

### Key Conventions
- Nest keys by page/section: `dashboard.title`, `monitors.form.name`, `settings.tokens.createButton`.
- Use `common.*` for shared strings (Save, Cancel, Delete, etc.).
- Interpolation: `{variable}` syntax — e.g. `"Page {page} of {totalPages}"`.
- Never use locale-specific quotes that break JSON (e.g. `""` — use `「」` or escaped `\"`).

### Files Reference
| Purpose | Path |
|---------|------|
| i18n config (locale list) | `frontend/src/lib/i18n/config.ts` |
| Locale store (t function) | `frontend/src/lib/i18n/locale.svelte.ts` |
| English (source of truth) | `frontend/src/locales/en.json` |
| All locales | `frontend/src/locales/*.json` |
| Language selector UI | `frontend/src/components/LanguageSelector.svelte` |
