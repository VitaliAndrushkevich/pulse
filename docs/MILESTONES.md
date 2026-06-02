# Pulse — Project Milestones & Current Stage

## Project Overview

Pulse is a self-hosted uptime monitoring platform (API-first, single-container deployment). The MVP targets HTTP/HTTPS, TCP, UDP, and WebSocket monitors with realtime status updates, designed to handle 500+ monitors.

---

## Current Stage: Mid Development (~60% complete)

Milestones A, B, C, D, and E are done. The project has a solid data foundation, full security primitives, a complete monitor execution engine, and a full REST API surface with OpenAPI contract. Next up: WebSocket realtime and frontend.

```
[████████████████████░░░░░░░░░░░░] ~60%
     A ✓   B ✓   C ✓   D ✓   E ✓   F…H todo
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

## Milestone C: Security & Secrets ✅ DONE

**Goal:** Secure at-rest secret handling, single-user auth primitives, API token management.

Delivered:
- ✅ AES-256-GCM encryption/decryption module (`internal/crypto`)
- ✅ Secret write-only API (values never returned in responses)
- ✅ API token create/list/revoke with bcrypt hash storage
- ✅ Log sanitization middleware (strip auth headers and secret fields)
- ✅ Key rotation command (`make rotate-key`) with transactional re-encryption

---

## Milestone D: Monitor Execution Engine ✅ DONE

**Goal:** Reliable check execution for all MVP protocols, scheduler that scales to 500+ monitors.

Delivered:
- ✅ `Checker` interface and shared `Result` model (`internal/monitor/checker.go`)
- ✅ HTTP/HTTPS checker with configurable expected status codes (explicit list or range), SSL certificate chain validation, expiry threshold, custom headers, redirect control
- ✅ TCP checker — dial + latency measurement via context-aware `net.Dialer`
- ✅ UDP checker — reachability mode (default) + payload/response validation mode
- ✅ WebSocket checker — `gorilla/websocket` with optional handshake message validation
- ✅ Bounded worker pool scheduler (configurable: `PULSE_SCHEDULER_WORKERS`, default 50 dev / 200 production)
- ✅ Priority-based polling via `ListActiveMonitorsDue` (ordered by `next_check_at ASC NULLS FIRST`)
- ✅ Dual-write results to TimescaleDB (time-series) and `check_results` table (API)
- ✅ PostgreSQL `LISTEN/NOTIFY` wakeups — zero-delay scheduling on monitor create/update (migration `003_monitor_notify_trigger`)
- ✅ Graceful shutdown via context cancellation; workers drain before exit
- ✅ `gorilla/websocket` dependency added for WebSocket checker

---

## Milestone E: API Surface & Contract ✅ DONE

**Goal:** Complete versioned REST API with idempotent monitor management, OpenAPI contract.

Delivered:
- ✅ Full `gin` router under `/api/v1` with standardized error envelope and X-Request-ID
- ✅ JWT auth (`POST /api/v1/auth/login`) + combined middleware (JWT + API token)
- ✅ Monitor CRUD with idempotent `PUT /monitors/{id}` (ON CONFLICT DO UPDATE)
- ✅ Monitor history endpoint (`GET /monitors/{id}/history`) backed by TimescaleDB (7-day max window)
- ✅ Incidents list endpoints (global + per-monitor, paginated, optional status=open filter)
- ✅ Prometheus `/metrics` endpoint (`pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`)
- ✅ Committed `backend/api/openapi.yaml` (OpenAPI 3.0.3, all endpoints documented)

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
| sqlc query layer | ✅ Complete | CRUD + paginated lists + upsert generated |
| TimescaleDB helpers | ✅ Complete | Write + range query scaffold |
| Fail-fast startup | ✅ Complete | Exits on missing dependencies |
| Docker infrastructure | ✅ Complete | Compose, Dockerfile, health checks |
| Makefile | ✅ Complete | All primary targets defined |
| API router | ✅ Complete | Full CRUD for monitors, secrets, tokens, incidents, history |
| Crypto module | ✅ Complete | AES-256-GCM encrypt/decrypt + key validation |
| Protocol checkers | ✅ Complete | HTTP/HTTPS, TCP, UDP, WebSocket — all compiled and wired |
| Scheduler | ✅ Complete | Bounded worker pool, LISTEN/NOTIFY wakeups, graceful shutdown |
| Auth/JWT | ✅ Complete | Login endpoint + combined JWT/API-token middleware |
| Prometheus metrics | ✅ Complete | /metrics with monitor_up, response_time, monitors_total |
| OpenAPI spec | ✅ Complete | backend/api/openapi.yaml (3.0.3) |
| WebSocket hub | 🔲 Placeholder | Empty file |
| Frontend | ⚠️ Scaffold | Layout + static dashboard, no data integration |
| CI pipeline | 🔲 Not started | — |

---

## Recommended Next Steps (Priority Order)

1. **Milestone E** — API surface (monitor CRUD, JWT auth, OpenAPI contract)
2. **Milestone F** — WebSocket hub and diff/patch pipeline (can parallelize with E)
3. **Milestone G** — Frontend product (depends on E and F)
4. **Milestone H** — Packaging and release (depends on everything)

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
