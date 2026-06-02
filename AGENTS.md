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
- Static adapter (`@sveltejs/adapter-static`) — output embedded into Go binary.
- Place API client in `frontend/src/lib/api.ts`.
- Place WebSocket client in `frontend/src/lib/ws.ts`.
- Place stores in `frontend/src/lib/stores/` — deterministic, patch-merge oriented.
- Reusable components in `frontend/src/components/`.
- Avoid blocking renders; large monitor views must remain virtualized.

## Current Progress

The project is at ~65% of MVP completion. Full milestone breakdown: [docs/MILESTONES.md](docs/MILESTONES.md).

| Milestone | Status |
|-----------|--------|
| A: Foundations | ✅ Done |
| B: Data Layer | ✅ Done |
| C: Security & Secrets | ✅ Done |
| D: Monitor Engine | ✅ Done |
| E: API Surface | ✅ Done |
| F: WebSocket Realtime | 🟡 In Progress |
| G: Frontend Product | ⚠️ Scaffold Only |
| H: Packaging & Release | 🔲 Todo |

### Completed (A–E):
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

### In Progress (F — WebSocket Realtime):
- ✅ WebSocket hub with fan-out, ping/pong keepalive, slow-consumer eviction (`internal/hub/`)
- ✅ Typed message envelope with `monitor_status` (diff/patch) and `connected` message types
- ✅ Authenticated `/ws` endpoint with query-param token validation (JWT + API token)
- ✅ Router wires Hub and WS handler
- 🔲 Scheduler → Hub integration (broadcast check results to hub after execution)
- 🔲 Reconnect/backoff documentation for UI clients

### Scaffold Only (G — Frontend):
- ✅ SvelteKit + TypeScript + Tailwind CSS project structure
- ✅ Layout with navigation (Dashboard, Monitors, Settings)
- ✅ Static dashboard page with placeholder stat cards
- ✅ Route stubs: `/`, `/monitors`, `/settings`
- 🔲 API client implementation (`src/lib/api.ts` — placeholder)
- 🔲 WebSocket client implementation (`src/lib/ws.ts` — placeholder)
- 🔲 Monitor store with patch-merge logic (`src/lib/stores/monitors.ts` — placeholder)
- 🔲 Monitor list with virtualization
- 🔲 Monitor create/edit forms
- 🔲 Monitor detail view with history chart (uplot)
- 🔲 Login flow with JWT cookie handling

### Next Priority:
1. Wire scheduler → hub broadcast (complete F)
2. Implement frontend API client and WS client (start G)
3. Build monitor dashboard with live data

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

## Build and Test
Primary commands:
- `make dev` — full stack via docker-compose (Pulse + TimescaleDB)
- `make dev-local` — lightweight compose (backend + postgres only)
- `make run` — `go run ./cmd/pulse` (requires local postgres)
- `make build` — `go build ./cmd/pulse`
- `make test` — `go test ./...`
- `make migrate` — run migrations up
- `make migrate-down` — roll back last migration
- `make rotate-key` — AES key rotation with transactional re-encryption
- `make openapi` — validate OpenAPI spec

## Infrastructure
- `docker-compose.dev.yml`: Go 1.25 container (hot-reload via `go run`) + TimescaleDB 2.17.2-pg16
- Backend port: 8080
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
