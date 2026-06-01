# Pulse — Project Milestones & Current Stage

## Project Overview

Pulse is a self-hosted uptime monitoring platform (API-first, single-container deployment). The MVP targets HTTP/HTTPS, TCP, UDP, and WebSocket monitors with realtime status updates, designed to handle 500+ monitors.

---

## Current Stage: Early Development (~20-25% complete)

Milestones A and B are done. The project has a solid data foundation but no business logic, API handlers, or frontend integration yet.

```
[████████░░░░░░░░░░░░░░░░░░░░░░░░] ~25%
     A ✓   B ✓   C…H todo
```

---

## Milestone A: Foundations ✅ DONE

**Goal:** Repository bootstrap, local developer workflow, baseline deployment pipeline.

What's delivered:
- Go backend module with `cmd/pulse/main.go` entrypoint
- SvelteKit + TypeScript + Tailwind frontend scaffold
- `docker-compose.yml` (Pulse + TimescaleDB/PostgreSQL 16 with health checks)
- `docker-compose.dev.yml` for local development
- `Makefile` with `dev`, `build`, `test`, `migrate`, `run` targets
- Migration tooling via `golang-migrate` (`cmd/migrate`)
- Multi-stage Dockerfile (Go build → distroless runtime)

---

## Milestone B: Data Layer ✅ DONE

**Goal:** Durable config/state in PostgreSQL, time-series writes/reads through TimescaleDB.

What's delivered:
- Full PostgreSQL schema: `users`, `api_tokens`, `secrets`, `monitors`, `incidents`, `check_results`
- Proper indexes for scheduler priority queue and query patterns
- `sqlc`-generated typed query layer (CRUD + paginated lists for all resources)
- TimescaleDB write/query helpers (`WriteCheckResult`, `QueryHistory`)
- Fail-fast startup: process exits non-zero when Postgres is unreachable or TimescaleDB extension is unavailable
- `X-Request-ID` middleware and `/healthz` endpoints

---

## Milestone C: Security & Secrets 🚧 IN PROGRESS (next up)

**Goal:** Secure at-rest secret handling, single-user auth primitives, API token management.

Planned deliverables:
- ✅ AES-256-GCM encryption/decryption module (`internal/crypto`)
- Secret write-only API (values never returned in responses)
- API token create/list/revoke with bcrypt hash storage
- Log sanitization middleware (strip auth headers and secret fields)
- Key rotation command (`make rotate-key`) with transactional re-encryption

---

## Milestone D: Monitor Execution Engine 🔲 TODO

**Goal:** Reliable check execution for all MVP protocols, scheduler that scales to 500+ monitors.

Planned deliverables:
- `Checker` interface and shared `Result` model
- Protocol checkers: HTTP/HTTPS, TCP, UDP, WebSocket
- Bounded worker pool scheduler (default concurrency 200)
- Priority queue by `next_check_at`
- PostgreSQL `LISTEN/NOTIFY` wakeups on monitor changes

---

## Milestone E: API Surface & Contract 🔲 TODO

**Goal:** Complete versioned REST API with idempotent monitor management, OpenAPI contract.

Planned deliverables:
- Full `gin` router under `/api/v1` with standardized error envelope
- JWT auth endpoints and middleware
- Monitor CRUD with idempotent `PUT /monitors/{id}`
- History and incidents read endpoints (paginated)
- Prometheus `/metrics` endpoint
- Generated and committed `openapi.yaml`

---

## Milestone F: Realtime Channel (WebSocket) 🔲 TODO

**Goal:** Diff-based state updates over WebSocket for live dashboard.

Planned deliverables:
- WebSocket hub (`internal/hub`) with fan-out to connected clients
- Diff/patch computation from scheduler state changes
- Authenticated `/ws` endpoint
- Reconnect and backoff documentation for UI clients

---

## Milestone G: Frontend Product 🔲 TODO

**Goal:** Responsive dashboard and monitor workflows at 500+ monitor scale.

Planned deliverables:
- Monitor status dashboard with virtualized list/grid rendering
- Monitor create/edit forms (secret references by UUID only)
- Monitor detail view with `uplot` history chart and incidents list
- WebSocket store merge logic (`src/lib/stores/`)
- Login flow with JWT cookie handling

---

## Milestone H: Packaging & Release Readiness 🔲 TODO

**Goal:** Single-binary, single-container production artifact.

Planned deliverables:
- Embed static frontend via `//go:embed` with SPA catch-all routing
- Multi-stage Dockerfile (node build → Go build → distroless)
- Production-ready compose with health checks and volumes
- README quick start and operational documentation
- CI workflow for build/test/lint/OpenAPI drift checks

---

## What Exists Today (Inventory)

| Layer | Status | Notes |
|-------|--------|-------|
| PostgreSQL schema | ✅ Complete | All MVP tables, indexes, constraints |
| sqlc query layer | ✅ Complete | CRUD + paginated lists generated |
| TimescaleDB helpers | ✅ Complete | Write + range query scaffold |
| Fail-fast startup | ✅ Complete | Exits on missing dependencies |
| Docker infrastructure | ✅ Complete | Compose, Dockerfile, health checks |
| Makefile | ✅ Complete | All primary targets defined |
| API router | ⚠️ Scaffold | Only `/healthz` endpoints, no business handlers |
| Crypto module | ✅ Complete | AES-256-GCM encrypt/decrypt + key validation |
| Protocol checkers | 🔲 Placeholder | Empty files for HTTP, TCP, UDP, WS |
| Scheduler | 🔲 Placeholder | Empty file |
| WebSocket hub | 🔲 Placeholder | Empty file |
| Metrics | 🔲 Placeholder | Empty file |
| Frontend | ⚠️ Scaffold | Layout + static dashboard, no data integration |
| Auth/JWT | 🔲 Not started | — |
| OpenAPI spec | 🔲 Not started | — |
| CI pipeline | 🔲 Not started | — |

---

## Recommended Next Steps (Priority Order)

1. **Milestone C** — Security is a prerequisite for all API work
2. **Milestone E** — API handlers (can partially overlap with C)
3. **Milestone D** — Monitor engine (independent of API, can parallelize)
4. **Milestone F** — WebSocket (depends on D)
5. **Milestone G** — Frontend (depends on E and F)
6. **Milestone H** — Packaging (depends on everything)

---

## Verification Checklist (MVP Sign-Off)

- [ ] `make dev` starts all services cleanly
- [ ] Auth login and protected route rejection
- [ ] API token raw value shown once only
- [ ] Monitor checks write visible history in TimescaleDB
- [ ] Secrets redacted in all API responses
- [ ] `/metrics` exposes required Prometheus series
- [ ] Frontend handles 500 monitor mock load without freezing
- [ ] Realtime status updates via WebSocket patches
- [ ] TCP, UDP, and WebSocket checks execute successfully
- [ ] History endpoint returns time-series data
- [ ] Fresh `docker-compose up` works end-to-end on clean machine
