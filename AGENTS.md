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
- Prefix all endpoints with `/api/v1`.
- Use stable UUID identifiers for resources.
- `PUT /monitors/{id}` must be idempotent create-or-update.
- List endpoints must be paginated (`page`, `limit`).
- Error response envelope:
  - `{ "error": { "code": "SOME_CODE", "message": "Readable message" } }`
- Include `X-Request-ID` in responses.
- **OpenAPI contract is the source of truth for the API surface.** Any change to endpoints, request/response schemas, parameters, or error codes MUST be reflected in `backend/api/openapi.yaml` in the same commit. Do not merge handler changes without updating the spec.

## WebSocket Protocol
- Endpoint: `/ws?token=<jwt_or_api_token>` (root-level, not under `/api/v1`).
- Auth via query parameter (browsers cannot send Authorization headers on WS).
- Message envelope: `{ "type": "<message_type>", "payload": { ... } }`.
- Message types: `connected` (sent once after upgrade), `monitor_status` (diff/patch after each check).
- `monitor_status` payloads are patches — clients must merge into local state, not replace entire objects.
- Hub drops slow consumers (buffer full) rather than blocking broadcasts.

## Security Requirements
- Never return raw secret values from APIs.
- Encrypt secret values at rest using AES-256-GCM.
- Store API tokens as `bcrypt` hashes; raw token shown only at creation.
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
- Go 1.25, `gin` v1.12, `pgx/v5`, `gorilla/websocket`, `golang-jwt/jwt/v5`.
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
| H: Packaging & Release | ✅ Done (CI deferred) |
| I: Branding & Theming | ✅ Done |

### Completed (A–H):
- PostgreSQL schema + indexes, sqlc query layer, TimescaleDB history store
- Docker Compose infrastructure, fail-fast startup, migration tooling
- AES-256-GCM crypto module, secret write-only API, API token lifecycle, key rotation
- Monitor engine: HTTP/HTTPS, TCP, UDP, WebSocket checkers with full protocol support
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
- Full SvelteKit 5 frontend with TypeScript strict mode and Tailwind CSS 3.4
- API client with Bearer auth, 15s timeout, error envelope parsing, X-Request-ID
- WebSocket client with exponential backoff reconnection (1s–30s, ±25% jitter)
- Reactive stores (Svelte 5 runes): Auth, Monitor (patch-merge), Toast, Connection
- VirtualList with DOM recycling (max 60 nodes), MonitorRow, MonitorForm, Pagination, HistoryChart (uPlot), Toast, ConnectionBadge
- All page routes: Login, Dashboard, Monitor list/detail/create/edit, Settings (secrets)
- Real-time updates: WS patches update dashboard rows and detail view in-place
- 218 frontend tests passing (Vitest + fast-check + @testing-library/svelte) — unit + property-based
- Static frontend embedded via `go:embed` with SPA catch-all routing and cache headers
- Multi-stage Dockerfile (node:22-alpine → golang:1.25-alpine → distroless)
- Production docker-compose with health checks, restart policies, env_file
- `.env.example` with all variables documented
- Complete README with quick start, API examples, architecture docs

### Completed (I: Branding & Theming):
- ECG-inspired logo mark (inline SVG, proportional stroke scaling)
- BrandLockup component (full/compact variants, proportional sizing from `size` prop)
- ThemeSwitcher component (light/dark toggle, localStorage persistence, FOUC prevention)
- CSS custom properties theme system (`:root`/`[data-theme="dark"]` token overrides)
- Tailwind integration: `darkMode: ['selector', '[data-theme="dark"]']`, brand color scale (50–900), semantic aliases
- Self-hosted Inter font (WOFF2, `font-display: swap`)
- Static brand assets (`frontend/static/brand/` — SVG, PNG exports, dark variant, README)
- Favicon, Apple Touch Icon, PWA manifest (`site.webmanifest`)
- Layout integration: responsive lockup in header, ThemeSwitcher in nav, theme-aware token styles
- Login/setup pages with centered BrandLockup
- Property-based tests: stroke proportionality, scaling, WCAG contrast, toggle persistence, icon correctness, token mapping

### Deferred:
- CI quality gates (GitHub Actions) — not required for MVP

## Key Files Reference
| Purpose | Path |
|---------|------|
| Go entrypoint | `backend/cmd/pulse/main.go` |
| API router | `backend/internal/api/router.go` |
| WS hub | `backend/internal/hub/hub.go` |
| WS messages | `backend/internal/hub/messages.go` |
| WS handler | `backend/internal/api/handlers/ws.go` |
| Scheduler | `backend/internal/monitor/scheduler.go` |
| Checkers | `backend/internal/monitor/{http,tcp,udp,websocket}.go` |
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
- `docker-compose.dev.yml`: Go 1.25 container (hot-reload via `go run`) + TimescaleDB 2.17.2-pg16 + frontend dev server
- Backend port: 8080
- Frontend dev container: service `frontend`, base image `node:22-alpine`, port 5173, runs Vite dev server with HMR for local frontend development
- Postgres: `pulse:pulse@localhost:5432/pulse`
- Environment variables: `PULSE_PORT`, `PULSE_DEV`, `PULSE_SECRET_KEY`, `PULSE_JWT_SECRET`, `DATABASE_URL`, `PULSE_SCHEDULER_WORKERS`

## Delivery Constraints
- MVP scope: HTTP/HTTPS, TCP, UDP, WebSocket monitors.
- Future scope only (do not implement unless requested): QUIC, multi-tenant, status page, alert channels, Terraform provider.
- Keep infra local-first with Docker Compose and reproducible startup.

## Development Skills
When working on code, reference these skills for domain guidance:

### Frontend (Svelte)
- **svelte-code-writer**: CLI tools for Svelte documentation lookup and code analysis. Use whenever creating/editing Svelte components (.svelte) or modules (.svelte.ts/.svelte.js).
- **svelte-core-bestpractices**: Guidance on fast, robust, modern Svelte code (reactivity, events, styling, library integration). Load for any Svelte component work.

### Backend (Golang)
- Multiple Golang skills available for database, concurrency, error handling, testing, performance, security, observability, and troubleshooting. Load as needed per task domain.
