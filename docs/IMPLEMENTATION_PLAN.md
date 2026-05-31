# Pulse Implementation Plan (MVP)

## Why This Plan Exists
This plan translates [.github/prompts/plan-pulseUptimeMonitor.prompt.md](../.github/prompts/plan-pulseUptimeMonitor.prompt.md) into an execution-ready roadmap with clear deliverables, dependencies, and acceptance criteria.

## Delivery Strategy
- Milestone model: 7 milestones matching architecture phases
- Scope: MVP only (HTTP/HTTPS, TCP, UDP, WebSocket checks)
- API-first rule: backend API endpoints and OpenAPI contract must land before frontend features are considered done
- Performance rule: all list APIs are paginated and UI list rendering is virtualized before load testing

## Milestone 0: Foundations (Week 1)
### Goals
- Repository bootstrap and local developer workflow
- Baseline deployment and migration pipeline

### Tasks
1. Initialize Go backend module at `backend/` with `cmd/pulse/main.go`.
2. Initialize SvelteKit app at `frontend/` with TypeScript, Tailwind, `@sveltejs/adapter-static`.
3. Add `docker-compose.yml` and `docker-compose.dev.yml`.
4. Add `Makefile` targets: `dev`, `build`, `migrate`, `test`, `rotate-key`, `openapi`.
5. Add migration tooling (`golang-migrate`) and initial migration scaffold.

### Exit Criteria
- `make dev` starts backend, postgres, and influxdb successfully.
- `make migrate` applies schema without manual edits.

## Milestone 1: Data Plane (Week 2)
### Goals
- Durable config/state in PostgreSQL
- Time-series writes and reads through InfluxDB helper layer

### Tasks
1. Implement PostgreSQL schema for `monitors`, `secrets`, `incidents`, `check_results`, `users`, `api_tokens`.
2. Add required indexes for scheduler and monitor querying.
3. Generate SQL access layer with `sqlc`.
4. Implement InfluxDB write/query helpers in `backend/internal/store/influx/`.
5. Wire fail-fast DB initialization in `main.go`.

### Exit Criteria
- Backend refuses startup when PostgreSQL or InfluxDB is unavailable.
- CRUD queries compile and pass integration smoke tests.

## Milestone 2: Security and Secret Management (Week 3)
### Goals
- Secure at-rest secret handling
- Single-user auth and token management primitives

### Tasks
1. Implement AES-256-GCM encryption/decryption in `backend/internal/crypto/crypto.go`.
2. Implement secret API handlers with write-only response policy.
3. Implement API token creation/list/delete with `bcrypt` hash storage.
4. Add request/response log sanitization middleware.
5. Implement key rotation command (`make rotate-key`) as single transaction re-encryption.

### Exit Criteria
- Secret values never appear in API responses or logs.
- Raw API token is shown only once at creation.

## Milestone 3: Monitor Execution Engine (Week 4)
### Goals
- Reliable check execution for all MVP protocols
- Scheduler that scales to 500+ monitors

### Tasks
1. Define `Checker` interface and shared `Result` model.
2. Implement protocol checkers: HTTP/HTTPS, TCP, UDP, WebSocket.
3. Implement bounded worker scheduler (default concurrency 200).
4. Implement priority queue by `next_check_at`.
5. Add PostgreSQL `LISTEN/NOTIFY` wakeups on monitor changes.

### Exit Criteria
- 500 monitor test run completes without unbounded goroutine growth.
- Scheduler latency remains stable under concurrent monitor updates.

## Milestone 4: API Surface and Contract (Week 5)
### Goals
- Complete versioned REST API with idempotent monitor management
- Prometheus metrics and OpenAPI source-of-truth committed

### Tasks
1. Implement `gin` router under `/api/v1`.
2. Add auth endpoints and middleware (JWT).
3. Add monitor CRUD with idempotent `PUT /api/v1/monitors/{id}`.
4. Add history and incidents read endpoints.
5. Expose `/metrics` and core counters/gauges.
6. Annotate handlers and generate OpenAPI artifact (`openapi.yaml`).

### Exit Criteria
- Protected routes reject unauthenticated calls.
- OpenAPI is generated and committed with CI validation.

## Milestone 5: Realtime Channel (Week 6)
### Goals
- Diff-based state updates over WebSocket

### Tasks
1. Implement WebSocket hub in `backend/internal/hub/hub.go`.
2. Feed scheduler state changes through diff computation.
3. Expose authenticated `/ws` endpoint.
4. Add reconnect and backoff guidance to API docs for UI clients.

### Exit Criteria
- WebSocket pushes patch-only updates for monitor state changes.
- Multiple concurrent clients receive consistent updates.

## Milestone 6: Frontend Product (Weeks 6-7)
### Goals
- Responsive dashboard and monitor workflows at 500+ monitor scale

### Tasks
1. Implement monitor status dashboard with virtualized list/grid.
2. Implement monitor create/edit flows with secret UUID references.
3. Implement monitor detail with `uplot` history graph and incidents list.
4. Implement WebSocket store merge logic in `frontend/src/lib/stores/`.
5. Implement login flow with JWT cookie handling.

### Exit Criteria
- 500 monitor mock load does not freeze UI.
- Live updates are reflected without full-state redraw.

## Milestone 7: Packaging and Release Readiness (Week 8)
### Goals
- Single-binary and single-container production artifact

### Tasks
1. Embed static frontend (`//go:embed all:frontend/build`) and SPA catch-all routing.
2. Create multi-stage `Dockerfile` (node build -> go build -> distroless runtime).
3. Finalize production and dev compose files with health checks and volumes.
4. Write `README.md` quick start and operational notes.
5. Add CI workflow for build/test/lint/openapi drift checks.

### Exit Criteria
- `docker-compose up` runs end-to-end on clean environment.
- Image boots and serves API, WebSocket, metrics, and frontend from one container.

## Cross-Cutting Technical Rules
- Stable UUID resource IDs for all primary resources.
- Monitor state vocabulary for MVP is `up` / `down` / `unknown` (per-check results
  are `up` / `down`). A `degraded` state is out of MVP scope; introducing it
  requires a schema change to the `monitors.state` / `check_results.state` CHECK
  constraints plus checker semantics, and should be a deliberate future ticket.
- Error envelope format:
  - `{ "error": { "code": "...", "message": "..." } }`
- Pagination mandatory on all list endpoints (`page`, `limit`, optional cursor later).
- `X-Request-ID` generated/propagated for every response.
- No unbounded memory growth paths in API or UI data pipelines.

## Suggested Initial Backlog (First 10 Tickets)
1. Repo skeleton + Go/Svelte bootstrap
2. Compose files + Makefile
3. Initial SQL migration + sqlc config
4. PostgreSQL connection and health startup gates
5. Influx store helpers
6. AES crypto module + secret key loader
7. Secret CRUD write-only handlers
8. HTTP checker + common checker interface
9. Scheduler worker pool + queue
10. Monitor CRUD endpoints + OpenAPI generation

## Risks and Mitigations
- Worker saturation at high monitor count: enforce bounded pool and queue metrics.
- Influx query cost growth: enforce retention policy and bounded history windows.
- API drift from docs: CI fails when generated OpenAPI differs from committed file.
- UI jank with large datasets: virtualized rendering and incremental patch merges only.

## Definition of Done (MVP)
- All verification checks listed in [.github/prompts/plan-pulseUptimeMonitor.prompt.md](../.github/prompts/plan-pulseUptimeMonitor.prompt.md) pass.
- API endpoints exist before corresponding UI features are marked complete.
- Security constraints met: no secret leakage in logs/responses and bcrypt token storage.
- End-to-end startup validated through Docker Compose on a fresh machine.
