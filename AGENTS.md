# Project Guidelines

## Product Intent
Pulse is a self-hosted uptime monitoring platform.

Core outcomes:
- API-first backend with stable, versioned resources under `/api/v1`
- Realtime status updates via native WebSocket
- Operational reliability at 500+ monitors
- Single deployable container with embedded frontend assets

## Repository Map
| Dir | What | Toolchain |
|-----|------|-----------|
| `backend/` | Pulse server (`gin` API + scheduler + WS hub + notifications), migrations, OpenAPI spec | Go (see `backend/go.mod`) |
| `frontend/` | SvelteKit static SPA, embedded into the Go binary at build time | pnpm (see `frontend/package.json`) |
| `mcp/` | Separate `pulse-mcp` binary exposing Pulse API to MCP clients | Go (own `mcp/go.mod`, own `mcp/Makefile`) |
| `docs/` | `DEVELOPMENT.md`, `API.md`, `AUDIT-Jul-2026.md`, `PRODUCT-ROADMAP.md`, `TECHNICAL-DEBT.md` | — |
| `.kiro/specs/` | Per-feature requirements/design/tasks (detailed history of every feature) | — |

## Architecture Rules
- Backend is Go (`gin` + scheduler + websocket hub) in one binary.
- Frontend is static SvelteKit build embedded into Go (`go:embed` in `backend/internal/frontend/frontend.go`, SPA catch-all in the router).
- PostgreSQL is source of truth for config/state.
- PostgreSQL with TimescaleDB extension stores time-series monitor history.
- REST and OpenAPI contract must be implemented before UI integration.
- Migrations are embedded (`backend/migrations/migrations.go`) and auto-applied at server startup; `make migrate` is only needed for manual control.

## API Conventions
- Prefix all endpoints with `/api/v1`. Root-level exceptions: `GET /healthz`, `GET /metrics`, `GET /ws`, dev-only `/swagger`.
- Use stable UUID identifiers for resources.
- `PUT /monitors/{id}` must be idempotent create-or-update.
- List endpoints must be paginated (`page`, `limit`).
- Error response envelope:
  - `{ "error": { "code": "SOME_CODE", "message": "Readable message" } }`
- Include `X-Request-ID` in responses.
- **OpenAPI contract is the source of truth for the API surface.** Any change to endpoints, request/response schemas, parameters, or error codes MUST be reflected in `backend/api/openapi.yaml` in the same commit. Do not merge handler changes without updating the spec.
- Known spec drift (fix when touching these routes): `GET /monitors/{id}/credentials/{credentialId}`, `GET /monitors/{id}/delivery-logs`, `GET /api/v1/healthz` are routed but not in the spec.

## WebSocket Protocol
- Endpoint: `/ws?token=<jwt_or_api_token>` (root-level, not under `/api/v1`).
- Auth via query parameter (browsers cannot send Authorization headers on WS).
- Message envelope: `{ "type": "<message_type>", "payload": { ... } }`.
- Message types (`backend/internal/hub/messages.go`): `connected` (sent once after upgrade), `monitor_status` (diff/patch after each check), `monitor_tags_changed` (tag set of a monitor changed), `heartbeat_status` (diff/patch when a heartbeat's state changes on ping/fail/missed-run).
- `monitor_status` and `heartbeat_status` payloads are patches — clients must merge into local state, not replace entire objects.
- Origin validation: in production, the `Origin` header must match `PULSE_BASE_URL` (scheme+host). In dev mode (`PULSE_DEV=true`) or when `PULSE_BASE_URL` is empty, all origins are allowed.
- Hub drops slow consumers (buffer full) rather than blocking broadcasts.

## Security Requirements
- Never return raw secret values from APIs.
- Encrypt secret values at rest using AES-256-GCM (`PULSE_SECRET_KEY` = base64 of exactly 32 bytes).
- Monitor auth secrets are per-monitor credentials (`/monitors/{id}/credentials`). The legacy `/api/v1/secrets` API is removed from the router; `backend/internal/api/handlers/secrets.go` is dead code — do not re-wire it.
- Store API tokens as `bcrypt` hashes; raw token shown only at creation.
- Sanitize logs: strip auth headers and known secret fields.
- Combined auth: endpoints accept both JWT (session) and Bearer API token (programmatic) — `combinedAuth` in `backend/internal/api/router.go`.
- WS auth uses constant-time comparison with dummy bcrypt hash on failure (timing-safe).
- `make rotate-key` re-encrypts legacy secrets and monitor credentials only; SMTP password and webhook headers are NOT rotated yet (TD-008 in `docs/TECHNICAL-DEBT.md`).

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
- Go 1.26, `gin` v1.12, `pgx/v5`, `gorilla/websocket`, `golang-jwt/jwt/v5`. Exact versions: `backend/go.mod`.
- Prometheus metrics via `prometheus/client_golang`.
- Migrations via `golang-migrate/v4`.
- Property-based tests use `pgregory.net/rapid` (backend and MCP).

## Frontend Conventions
- Svelte 5 with SvelteKit 2, Vite 8 + Svelte Vite plugin 7, TypeScript strict, Tailwind CSS 3.4. Node 22.12+ or a newer supported LTS; exact versions in `frontend/package.json`. Tests: Vitest + fast-check + @testing-library/svelte.
- **pnpm** is the package manager for the `frontend/` directory — use `pnpm` for dependency installation (`pnpm install`), script execution (`pnpm <script>`), and lockfile management (`pnpm-lock.yaml`). Run `pnpm install` before any script (a fresh checkout has no `node_modules`).
- Static adapter (`@sveltejs/adapter-static`) — output embedded into Go binary.
- Place API client in `frontend/src/lib/api.ts`.
- Place WebSocket client in `frontend/src/lib/ws.ts`.
- Place stores in `frontend/src/lib/stores/` — deterministic, patch-merge oriented.
- Reusable components in `frontend/src/components/`.
- Avoid blocking renders; large monitor views must remain virtualized.
- **Theming:** Use CSS custom properties from `app.css` (e.g., `var(--color-brand-primary)`) instead of hardcoded Tailwind color classes. Tailwind brand utilities (`bg-brand-500`, `text-brand-600`) resolve to CSS variables automatically.
- **Dark mode:** Controlled via `data-theme` attribute on `<html>`. Never use Tailwind's `dark:` prefix — use `[data-theme="dark"]` selector strategy already configured.
- **Visual identity:** Follow [UI design](docs/UI-DESIGN.md): Pulse blue accents, blue-white light surfaces, navy dark surfaces, flat panels, compact corners, and monospace operational values. Reserve green/amber/red for status. Keep filled-action colors separate from inline-action text colors so both remain readable in dark mode. Older `indigo-*` utilities are mapped to the brand scale; use `brand-*` for new work.
- **Navigation shell:** The desktop rail in `+layout.svelte` collapses from full labels to icons; `pulse-sidebar-collapsed` persists the preference. Preserve accessible names, hover/focus labels, the skip link, and parent-route active states. Live status, theme, and Logout belong in the rail footer; mobile keeps labeled navigation and visible controls. Use logical CSS properties for RTL.
- **Page layout:** Use `components/PageHeader.svelte` for the eyebrow, title, optional description, and optional action snippet. Dashboard, Monitors, and Settings share this header. Let the workspace own page padding; avoid an additional centered/padded page wrapper. Use `pulse-panel`, `pulse-eyebrow`, `pulse-tag`, and `pulse-protocol` for shared visual primitives and avoid redundant nested cards.
- **Select controls:** Use `components/SelectField.svelte` rather than new bare native selects. Pass the existing `<option>` snippet, a stable `id` with a matching `<label>`, and `bind:value`/`onchange` as needed. It keeps a hidden native select for typed values and form integration, and exposes a themed combobox/listbox with keyboard navigation, typeahead, outside dismissal, and viewport-aware top-layer menus. The native fallback is internal to the component. Keep fields and adjacent submit buttons aligned at a 40px minimum control height; let form rows stack on narrow screens.
- **UI verification:** Run the frontend checks below, including locale coverage in `ar` and `be`. For shared control or layout changes inspect populated/empty views, both themes, narrow viewports, RTL, keyboard focus, and dropdown boundaries. Branding tests read the actual CSS palette for dark foreground contrast and Tailwind fallback alignment. Preserve fixed row heights in virtualized lists.
- **i18n / Localization:** All user-visible strings MUST use the `t()` function from `$lib/i18n`. Never hardcode display strings in Svelte templates or component scripts. When adding new UI text (labels, buttons, errors, toasts, empty states), add the corresponding key to `frontend/src/locales/en.json` first, then reference it via `t('section.key')`. Other locale files (`ru.json`, `es.json`, etc.) should also receive the new key — use the English value as a placeholder if no translation is available yet. See [Localization](#localization-i18n).

## MCP Server
- The MCP server (`mcp/`) is a separate Go binary that proxies Pulse API endpoints to MCP-compatible AI clients. Entrypoint `mcp/main.go`; tools registered in `mcp/internal/server/register.go`.
- Tools: read — `list-monitors`, `get-monitor`, `monitor-stats`, `monitor-history`, `downtime-summary`, `list-incidents`, `dashboard-summary`, `list-tags`; write (only with `PULSE_MCP_ACCESS_MODE=read-write`) — `create-monitor`, `delete-monitor`, `pause-monitor`, `resume-monitor`.
- Transports: `stdio` (default) or `http`.
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
- Adding a monitor type also touches MCP type maps (`mcp/internal/tools/create_monitor.go`, `mcp/internal/tools/list_monitors.go`).

## Feature Inventory
MVP is complete. Detail and history live in `.kiro/specs/<name>/`; roadmap in `docs/PRODUCT-ROADMAP.md`; known gaps in `docs/TECHNICAL-DEBT.md`.

| Area | Implemented | Spec |
|------|-------------|------|
| Monitor engine | 10 types: http, http3, tcp, udp, websocket, grpc, dns, icmp, smtp, quic; bounded scheduler; LISTEN/NOTIFY wakeups on create/update | `grpc-monitor` |
| gRPC payloads | Proto upload/reflection per monitor, ad-hoc `/grpc/reflect`, `/grpc/parse-proto` | `grpc-proto-payload` |
| Monitors API | CRUD, idempotent PUT, filtering/ordering/pagination, stats (uptime %, SSL info), incidents (global + per-monitor) | — |
| Tags | `/tags`, `/tags/{key}`, `monitor_tags_changed` WS message, tag labels on Prometheus metrics (capped by `PULSE_MAX_METRIC_TAG_KEYS`) | `monitor-tagging` |
| Credentials | Per-monitor encrypted credentials (replaced legacy secrets API) | `secrets-and-auth-rework` |
| History & retention | History query with `from`/`to`/`step` (default last 24h); per-monitor retention 1–365 days (default 30); retention service | `monitor-history-explorer` |
| Dashboard | `/dashboard/summary` + widgets in `frontend/src/components/dashboard/` | `dashboard-refactor` |
| Status timeline | Check bar with failure details | `status-timeline-failure-details` |
| Notifications | Email (SMTP) + webhook channels, bindings, triggers, retry, delivery logs. Reminders are NOT wired end-to-end (TD-004) | `notification-channels` |
| Auth | JWT + API tokens, first-run setup, password change, `PULSE_RESET_ADMIN` | `password-reset` |
| Realtime | WS hub, diff/patch messages | — |
| Frontend | Routes: `/`, `/login`, `/setup`, `/monitors` (+ create, `[id]`, `[id]/edit`), `/heartbeats` (+ create, `[id]`, `[id]/edit`), `/notifications` (+ create, `[id]/edit`), `/settings` (tokens, password, language) | `frontend-product` |
| i18n / branding | 13 locales incl. RTL; lockup, theme switcher, CSS tokens | `i18n-localization`, `pulse-branding` |
| Heartbeat monitoring | Push/dead-man's-switch resource parallel to monitors: `/api/v1/heartbeats` admin CRUD + idempotent PUT + rotate-token (optional `history_retention_days`, 1–365, default 30; `INVALID_RETENTION_PERIOD` on out-of-range), root-level token-scoped ping ingestion (`/ping/{token}`, `/ping/{token}/fail`), bounded missed-run sweep evaluator (`PULSE_HEARTBEAT_WORKERS`), missed-run/recovery notifications via the existing dispatcher, `heartbeat_status` WS message, `pulse_heartbeat_up`/`pulse_heartbeats_total` metrics, append-only `heartbeat_events` history (success/fail/missed episodes in a TimescaleDB hypertable, migration `018_heartbeat_events`, pruned by the retention service per `history_retention_days`), frontend UI (list/detail/create/edit + nav) | `heartbeat-monitoring` |
| MCP server | 12 tools, stdio/http | `mcp-server` |
| Packaging / CI | Dockerfile, compose files, 6 GitHub workflows | `packaging-release`, `pnpm-frontend-dev` |

**Heartbeat (push) monitoring** is implemented (PF-01 first useful version, migrations `017_heartbeats` + `018_heartbeat_events`). Per-event history is now persisted: every ping success, explicit fail, and missed-run transition writes a row to the `heartbeat_events` hypertable (with the heartbeat's `previous_state` for recovery labeling), retained for `history_retention_days` and pruned by the retention service. The write path is wired (ping/sweep → `HeartbeatEventWriter`); a read/timeline API and UI for these events are not yet exposed. A dedicated heartbeat MCP tool is a deferred non-goal — no existing MCP tool consumes heartbeat endpoints, so none require co-change. Other deferred non-goals: run-lifecycle events (run ids / duration), timezone-aware cron schedules, concurrent-run / duplicate / late-arrival / clock-boundary semantics, restore-validity verification, and an independently hosted external receiver.

## Development Workflow

### Run locally
| Command | What it starts |
|---------|----------------|
| `make dev-local` | `docker-compose.dev.yml`: backend via `go run` (8080), Vite dev server with HMR (5173), TimescaleDB (5432). Normal dev loop. |
| `make dev` | `docker-compose.yml`: published `ghcr.io/vitaliandrushkevich/pulse:main` image + TimescaleDB (prod-like; no local build, DB port not exposed). |
| `make run` | `go run ./cmd/pulse` with dev env; needs Postgres on `localhost:5432` (e.g. `docker compose -f docker-compose.dev.yml up postgres -d`). |
| `cd frontend && pnpm dev` | Vite dev server only; proxies `/api` and ws to `VITE_API_PROXY_TARGET` (default `http://localhost:8080`). |
| `make dev-down` / `make dev-local-down` | Stop stacks. Uses `down -v`: **deletes DB volumes**. |

### Checks before finishing (mirror PR CI in `.github/workflows/pull_request_opened.yml`)
| Changed | Run |
|---------|-----|
| `backend/**` | `cd backend && go test -race -count=1 ./... && go build -o /dev/null ./cmd/pulse` (`make test` is only `go test ./...`: no race detector, cached, no build) |
| `frontend/**` | `cd frontend && pnpm install && pnpm check && pnpm validate-locales && pnpm test && pnpm build` |
| `mcp/**` | `cd mcp && go test -race -count=1 ./... && go build -o /dev/null ./main.go` (or `make test` / `make build` in `mcp/`) |
| `backend/api/openapi.yaml` | Keep handlers in sync; check MCP co-change list. `make openapi` only prints the spec version — it is not a validator. |
| SQL queries / migrations | `cd backend && sqlc generate` then confirm `sqlc diff` is clean |

Optional suites (not run in CI):
- TimescaleDB integration: `cd backend && PULSE_RUN_DB_TESTS=1 go test ./internal/store/timescale/...` (needs Postgres on 5432).
- gRPC integration: `cd backend && go test -tags integration ./internal/monitor/...`.

### Add a migration + sqlc query
1. Add `backend/migrations/NNN_name.up.sql` and `NNN_name.down.sql` (next sequential number; embedded and auto-applied on startup).
2. Add/modify queries in `backend/internal/store/postgres/queries/*.sql`.
3. `cd backend && sqlc generate` (config `backend/sqlc.yaml`; output `backend/internal/store/postgres/`). Never hand-edit generated `*.sql.go`.
4. Time-series data goes through `backend/internal/store/timescale/`, not sqlc.

### Add a monitor type / checker
1. Implement the checker in `backend/internal/monitor/<type>.go` and register it in `backend/internal/monitor/checker.go`.
2. Target normalization: `backend/internal/target/normalize.go`.
3. Migration updating the `monitors_type_check` constraint (see `backend/migrations/016_quic_monitor_type.up.sql`).
4. Handler validation in `backend/internal/api/handlers/monitors.go`; type enums + settings schema in `backend/api/openapi.yaml`.
5. Frontend: `MonitorType` in `frontend/src/lib/types.ts`, `frontend/src/lib/validation.ts`, `frontend/src/components/MonitorForm.svelte`, `frontend/src/components/FilterBar.svelte`, `frontend/src/routes/monitors/+page.svelte`, plus locale keys.
6. MCP type maps (see MCP section). Update the supported-protocols list in Delivery Constraints.

### Add an endpoint
1. Handler in `backend/internal/api/handlers/` with a `Register(group)` method; register it in `backend/internal/api/router.go` (`protected` group unless public by design).
2. Use the error envelope and pagination conventions above.
3. Update `backend/api/openapi.yaml` in the same commit.
4. If the endpoint is in the MCP co-change list, update `mcp/internal/pulseapi/` and `mcp/internal/tools/` in the same commit.
5. Frontend calls go through `frontend/src/lib/api.ts`.

## Commands Reference
- Root `Makefile`: `dev`, `dev-local`, `dev-down`, `dev-local-down`, `run`, `build` (backend only, version via ldflags), `build-frontend` (pnpm build → `backend/internal/frontend/dist/`), `build-all` (binary with embedded UI), `test`, `migrate`, `migrate-down`, `rotate-key` (needs `PULSE_SECRET_KEY` + `PULSE_SECRET_KEY_NEW`), `openapi` (prints spec version).
- `mcp/Makefile`: `build` (→ `mcp/bin/pulse-mcp`), `test`, `lint` (requires `golangci-lint`), `clean`.
- `frontend/package.json` scripts: `dev`, `build` (runs `validate-locales` first), `validate-locales`, `preview`, `check`, `test`. There are no lint/format scripts.

## Environment Variables
Source of truth: `backend/internal/config/config.go`, `backend/cmd/pulse/main.go`, `mcp/internal/config/config.go`. Template: `.env.example`.

| Variable | Default | Notes |
|----------|---------|-------|
| `PULSE_PORT` | `8080` | HTTP listen port |
| `PULSE_DEV` | disabled | Exactly `true` enables Swagger UI (`/swagger`) and allows all WS origins |
| `PULSE_SECRET_KEY` | **required** | AES-256-GCM key: base64 of exactly 32 bytes, else fatal |
| `PULSE_JWT_SECRET` | **required** | HMAC secret for JWT signing |
| `PULSE_JWT_EXPIRY` | `24h` | Go duration |
| `DATABASE_URL` | `postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable` | Also used by `backend/cmd/migrate`, `backend/cmd/rotate` |
| `PULSE_SCHEDULER_WORKERS` | `50` | Check worker pool size; `.env.example` recommends 200 for prod |
| `PULSE_HEARTBEAT_WORKERS` | `50` | Bounded worker-pool size for the heartbeat missed-run sweep evaluator |
| `PULSE_BASE_URL` | empty | Public URL for email links and WS Origin validation (empty → links omitted, all origins allowed) |
| `PULSE_METRICS_USER` / `PULSE_METRICS_PASSWORD` | empty | Basic Auth on `/metrics` only when both are set |
| `PULSE_RESET_ADMIN` | disabled | `true` re-enables the setup flow for admin credential reset |
| `PULSE_OPENAPI_DIR` | `api` | Dir with `openapi.yaml` served by Swagger in dev |
| `PULSE_RETENTION_CHECK_INTERVAL` | `1h` | Retention sweep interval, range 1m–168h |
| `PULSE_MAX_METRIC_TAG_KEYS` | `10` | Max tag keys promoted to Prometheus labels |
| `PULSE_NOTIFICATION_WORKERS` | `10` | Notification delivery workers (`.env.example` sets 50); queue buffer fixed at 256 |
| `PULSE_NOTIFICATION_DRAIN_TIMEOUT` | `30s` | Graceful shutdown drain timeout |
| `PULSE_LOG_LEVEL` | `warn` | `info`/`debug` → verbose notification delivery logs |
| `PULSE_SECRET_KEY_NEW` | — | New key for `make rotate-key` |
| `MIGRATIONS_PATH` | `file://migrations` | `backend/cmd/migrate` only |
| `PULSE_RUN_DB_TESTS` | unset | `1` runs TimescaleDB integration tests |
| `VITE_API_PROXY_TARGET` | `http://localhost:8080` | Vite dev proxy target |
| `PULSE_MCP_API_TOKEN` | **required** (MCP) | Pulse API token used by `pulse-mcp` |
| `PULSE_MCP_API_BASE_URL` | `http://localhost:8080/api/v1` | |
| `PULSE_MCP_ACCESS_MODE` | `read-only` | `read-write` enables write tools |
| `PULSE_MCP_TRANSPORT` | `stdio` | or `http` |
| `PULSE_MCP_HTTP_ADDR` | `:9090` | http transport listen address |
| `PULSE_MCP_REQUEST_TIMEOUT` | `15s` | |

When adding an env var, update this table, `.env.example`, and README in the same change.

## Infrastructure & CI
- Compose: `docker-compose.dev.yml` (backend `golang:1.26` + `node:lts-alpine` frontend + TimescaleDB pg17), `docker-compose.yml` (published image + TimescaleDB). Postgres creds: `pulse:pulse@localhost:5432/pulse`.
- `Dockerfile`: `node:lts-alpine` → `golang:1.26-alpine` → distroless.
- ICMP monitoring requires `CAP_NET_RAW` (granted via `cap_add: [NET_RAW]` in docker-compose). Falls back to unprivileged UDP ICMP on Linux 3.0+ when available.

| Workflow | Trigger | Does |
|----------|---------|------|
| `.github/workflows/pull_request_opened.yml` | PR → `main` | backend race tests + build; MCP tests + build (path-filtered `mcp/**`); frontend check, locales, tests, build |
| `.github/workflows/main-push.yml` | push → `main` | MCP verify; multi-arch image `:latest` |
| `.github/workflows/pull_request_merged.yml` | push → `main` | Image `:main` + `:<sha>` |
| `.github/workflows/release-drafter.yml` | push → `main` | Updates draft release |
| `.github/workflows/release.yml` | tag `v*` | Cross-compiled `pulse` + `pulse-mcp` binaries + checksums; multi-arch semver images |
| `.github/workflows/release-published.yml` | release published | Semver + `:latest` images |

## Delivery Constraints
- Supported protocols: HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP, QUIC.
- Future scope only (do not implement unless requested): multi-tenant, status page, Terraform provider.
- Keep infra local-first with Docker Compose and reproducible startup.

## Notifications
Asynchronous, non-blocking pipeline in `backend/internal/notification/`, with its own bounded worker pool independent from monitoring workers. Routes: `/api/v1/notifications/{channels,template-variables,smtp-settings}`, `/api/v1/monitors/{id}/notification-bindings`, `/api/v1/monitors/{id}/delivery-logs` — full contract in `backend/api/openapi.yaml`.

- **Channels:** Email (SMTP; instance-wide SMTP settings encrypted in DB; TLS, PLAIN auth; branded HTML) and Webhook (method GET/POST/PUT/PATCH/DELETE; Go `text/template` body; headers encrypted at rest; 1 MB rendered body limit).
- **Triggers** (per binding): `monitor_down` (once on transition to down), `monitor_up` (once on recovery), `degraded` (`threshold_ms` 1–60000), `ssl_expiring` (`days_before` 1–365), `n_failures_in_row` (`count` 1–100). State deduplication prevents repeated alerts for the same ongoing condition.
- **Pipeline:** scheduler check → `EvaluateAndDispatch` fan-out → one `DeliveryJob` per matching binding, enqueued non-blocking (`select/default`, dropped when full) → worker with panic recovery → row in `delivery_logs` (error detail ≤1024 chars) → retryable failures go to backoff retry (30s → 60s → 120s, max 4 attempts total); non-retryable errors (template parse, oversized body, decryption) fail permanently.
- **Reminders:** bindings accept a 30–1440 min reminder interval and `ReminderScheduler` exists, but nothing calls `ActivateReminder` in production — reminders do not fire yet (TD-004).
- **Webhook template variables:** `.Monitor.ID`, `.Monitor.Name`, `.Monitor.URL`, `.Monitor.Target`, `.Status`, `.PreviousStatus`, `.ResponseTime`, `.Incident.ID`, `.Incident.StartedAt`, `.Incident.Duration`, `.Timestamp`, `.BaseURL`.
- **Metrics:** `pulse_notification_deliveries_total{channel_type, outcome}`, `pulse_notification_dropped_total{channel_type}`, `pulse_notification_in_flight`, `pulse_notification_retry_queue_size`. Monitor metrics: `pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`.
- Other known gaps: per-binding condition isolation (TD-003), persisted jobs/retries (TD-005).

## Localization (i18n)

### Architecture
- i18n module: `frontend/src/lib/i18n/` — config, locale store, resolution, types.
- Translation files: `frontend/src/locales/{code}.json` (one per language).
- Fallback chain: active locale → English (`en.json`) → key string.
- Non-English locales are lazy-loaded via dynamic imports.
- RTL support: locales with `dir: 'rtl'` in `config.ts` automatically set `document.documentElement.dir`.

### Supported Locales
Source of truth: `SUPPORTED_LOCALES` in `frontend/src/lib/i18n/config.ts`.

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
4. **Run locale validation**: `cd frontend && pnpm validate-locales`. Caveat: the script only warns on missing keys (fails on missing files / nesting > 4 levels) and its locale list omits `ar` and `be` — check those two files manually.
5. **Verify RTL** — for layout-affecting strings (long labels, directional icons like `←`), ensure Arabic locale renders correctly. Use `dir`-aware CSS (`margin-inline-start` instead of `margin-left`).

### Adding a New Language
1. Create `frontend/src/locales/{code}.json` with all keys translated.
2. Register in `frontend/src/lib/i18n/config.ts` → `SUPPORTED_LOCALES` array.
   - Add `dir: 'rtl'` if the language is right-to-left.
3. Add the code to the locale list in `frontend/scripts/validate-locales.ts`.
4. Validate JSON: `python3 -c "import json; json.load(open('frontend/src/locales/{code}.json'))"`.
5. The language will appear automatically in the Settings → Language selector.

### Key Conventions
- Nest keys by page/section: `dashboard.title`, `monitors.form.name`, `settings.tokens.createButton`.
- Use `common.*` for shared strings (Save, Cancel, Delete, etc.).
- Interpolation: `{variable}` syntax — e.g. `"Page {page} of {totalPages}"`.
- Never use locale-specific quotes that break JSON (e.g. `""` — use `「」` or escaped `\"`).

## Key Files Reference
| Purpose | Path |
|---------|------|
| Go entrypoint | `backend/cmd/pulse/main.go` |
| Migrate / key-rotation commands | `backend/cmd/migrate/main.go`, `backend/cmd/rotate/main.go` |
| Config (env loading) | `backend/internal/config/config.go` |
| API router (+ `combinedAuth`) | `backend/internal/api/router.go` |
| Auth / JWT / logging middleware | `backend/internal/api/middleware/` |
| HTTP handlers | `backend/internal/api/handlers/` |
| Monitor + tags handlers | `backend/internal/api/handlers/monitors.go`, `backend/internal/api/handlers/tags.go` |
| Credential handlers / crypto | `backend/internal/api/handlers/credentials.go`, `backend/internal/api/handlers/credential_crypto.go` |
| Notification channel + SMTP settings handlers | `backend/internal/api/handlers/notification_channels.go`, `backend/internal/api/handlers/smtp_settings.go` |
| Notification binding + delivery-log handlers | `backend/internal/api/handlers/notification_bindings.go` |
| WS handler / hub / messages | `backend/internal/api/handlers/ws.go`, `backend/internal/hub/hub.go`, `backend/internal/hub/messages.go` |
| Scheduler / checker registry | `backend/internal/monitor/scheduler.go`, `backend/internal/monitor/checker.go` |
| Checkers (one file per type) | `backend/internal/monitor/` |
| LISTEN/NOTIFY wakeups | `backend/internal/monitor/notify.go` |
| Tag-aware Prometheus metrics | `backend/internal/monitor/dynamic_metrics.go` |
| Target normalization | `backend/internal/target/normalize.go` |
| Crypto / API tokens | `backend/internal/crypto/crypto.go`, `backend/internal/token/token.go` |
| Proto handling / retention / tags | `backend/internal/proto/`, `backend/internal/retention/`, `backend/internal/tags/` |
| Notification core (dispatcher, fan-out, state, retry, reminders) | `backend/internal/notification/` |
| SMTP / webhook clients | `backend/internal/notification/smtp/`, `backend/internal/notification/webhook/` |
| sqlc config / queries / generated code | `backend/sqlc.yaml`, `backend/internal/store/postgres/queries/`, `backend/internal/store/postgres/` |
| TimescaleDB store | `backend/internal/store/timescale/` |
| Migrations | `backend/migrations/` |
| Embedded frontend | `backend/internal/frontend/frontend.go` |
| OpenAPI spec | `backend/api/openapi.yaml` |
| MCP entrypoint / config / tool registration | `mcp/main.go`, `mcp/internal/config/config.go`, `mcp/internal/server/register.go` |
| MCP tools / Pulse API client | `mcp/internal/tools/`, `mcp/internal/pulseapi/` |
| Frontend API / WS client | `frontend/src/lib/api.ts`, `frontend/src/lib/ws.ts` |
| Frontend stores / routes / components | `frontend/src/lib/stores/`, `frontend/src/routes/`, `frontend/src/components/` |
| Vite config (dev proxy) | `frontend/vite.config.ts` |
| Brand lockup / theme switcher | `frontend/src/components/BrandLockup.svelte`, `frontend/src/components/ThemeSwitcher.svelte` |
| UI design / shared header / select | `docs/UI-DESIGN.md`, `frontend/src/components/PageHeader.svelte`, `frontend/src/components/SelectField.svelte` |
| Theme tokens (CSS) | `frontend/src/app.css` |
| Static brand assets / generators | `frontend/static/brand/`, `frontend/scripts/generate-brand-pngs.mjs`, `frontend/scripts/generate-icons.mjs` |
| i18n module / locale store | `frontend/src/lib/i18n/`, `frontend/src/lib/i18n/config.ts`, `frontend/src/lib/i18n/locale.svelte.ts` |
| Translations / validation | `frontend/src/locales/`, `frontend/scripts/validate-locales.ts`, `frontend/scripts/generate-locales.mjs` |
| Language selector UI | `frontend/src/components/LanguageSelector.svelte` |
| Compose / Docker / env template | `docker-compose.yml`, `docker-compose.dev.yml`, `Dockerfile`, `.env.example` |
| CI workflows | `.github/workflows/` |
| Docs / feature specs | `docs/`, `.kiro/specs/` |

## Agent Tooling
- Skills are pinned in `skills-lock.json` (Go `golang-*` skills, `svelte-code-writer`, `svelte-core-bestpractices`) and indexed in `.atl/skill-registry.md`. The skills directory under `.agents/` is gitignored and may need restoring from the lock file.
  - **svelte-code-writer**: Svelte docs lookup and code analysis — use whenever creating/editing `.svelte`, `.svelte.ts`, `.svelte.js` files.
  - **svelte-core-bestpractices**: reactivity, events, styling, library integration — load for any Svelte component work.
  - **golang-***: database, concurrency, error handling, testing, performance, security, observability, troubleshooting — load per task domain.
- Kiro hooks in `.kiro/hooks/`: `locale-sync-check` (frontend edits → `t()` + all locales), `openapi-mcp-sync` (spec edits → MCP tools), `update-swagger-documentation` (backend edits → OpenAPI), `update-docs-on-change` (source/config edits → docs).
