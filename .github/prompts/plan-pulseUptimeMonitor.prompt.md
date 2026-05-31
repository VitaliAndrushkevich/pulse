# Plan: Pulse — Uptime Monitoring Platform

## Summary
Self-hosted uptime monitoring alternative to Uptime Kuma. Go backend (single binary: API + workers), SvelteKit frontend, PostgreSQL for config/state, InfluxDB for time-series metrics, native WebSockets, Prometheus `/metrics` endpoint. Single-user auth. Docker Compose deployment. MVP monitors: HTTP/HTTPS, TCP, UDP, WebSocket. QUIC monitoring in future scope.

### Core Design Constraints (non-negotiable)
- **500+ monitors**: UI must never freeze. Backend must handle 500+ concurrent check goroutines efficiently.
- **API-first**: REST API is a first-class product. OpenAPI spec is the source of truth. Every feature accessible via API before UI.
- **IaC-ready from day 1**: API versioning (`/api/v1/`), stable resource IDs, idempotent PUT semantics — Terraform provider is a real near-future goal.

---

## Architecture

```
SvelteKit Frontend (embedded in Go binary via //go:embed)
    │  REST API + Native WebSocket
    ▼
Go API Server (gin router)
    ├── REST /api/v1/*
    ├── WebSocket /ws  (gorilla/websocket)
    ├── Prometheus /metrics
    ├── Static /*  (embedded SvelteKit build, catch-all SPA handler)
    └── Monitor Scheduler (bounded goroutine pool)
           │                │
     PostgreSQL          InfluxDB v2
  (monitors, secrets,  (time-series:
   incidents, users,    response times,
   api_tokens)          uptime history)
```

---

## Technology Stack

**Backend**
- Language: Go 1.22+
- Router: `gin` (fast, built-in validation/binding, strong swaggo support)
- WebSocket: `gorilla/websocket` (native, no socket.io)
- DB driver (PG): `pgx/v5` with `sqlc` for type-safe queries
- Migrations: `golang-migrate`
- InfluxDB: official `influxdb-client-go` v2
- Prometheus: `prometheus/client_golang`
- Config: env vars + optional YAML (`viper`)
- OpenAPI docs: `swaggo/swag`
- Crypto: Go native `crypto/aes` + `crypto/cipher` (AES-256-GCM) for secrets, `golang.org/x/crypto/bcrypt` for passwords/tokens

**Frontend**
- Framework: SvelteKit + TypeScript
- Adapter: `@sveltejs/adapter-static` (pure static output, no Node runtime)
- Styling: Tailwind CSS + shadcn-svelte (components)
- Real-time: Native WebSocket (Svelte stores)
- Charts: `uplot` (lightweight response time graphs)
- List rendering: `svelte-virtual-list` (virtual scrolling for 500+ monitors)

**Infrastructure**
- Single Docker container: Go binary embeds compiled SvelteKit static files via `//go:embed`
- Multi-stage Dockerfile: Node stage (build frontend) → Go stage (embed + compile) → distroless final image
- `docker-compose.yml` — 3 services: `pulse` (all-in-one), `postgres`, `influxdb`
- `docker-compose.dev.yml` — separate Go backend + Vite dev server (hot-reload), Vite proxies `/api` and `/ws` to Go
- Caddy as TLS termination in production (automatic HTTPS)
- Makefile for dev commands

---

## Project Structure

```
pulse/
├── backend/
│   ├── cmd/pulse/           # main.go — single binary entry point
│   ├── internal/
│   │   ├── api/             # gin router, handlers, middleware
│   │   │   ├── handlers/    # monitors, incidents, secrets, auth
│   │   │   ├── middleware/  # auth JWT, logging (sanitized), CORS, rate-limit
│   │   │   └── router.go
│   │   ├── crypto/          # AES-256-GCM encrypt/decrypt, key loading
│   │   ├── monitor/         # check engines + scheduler
│   │   │   ├── scheduler.go # bounded worker pool, LISTEN/NOTIFY wakeup
│   │   │   ├── http.go      # HTTP/HTTPS checker (status, latency, SSL expiry)
│   │   │   ├── tcp.go       # TCP checker
│   │   │   ├── udp.go       # UDP checker
│   │   │   └── websocket.go # WebSocket checker
│   │   ├── store/
│   │   │   ├── postgres/    # sqlc-generated queries (monitors, secrets, incidents)
│   │   │   └── influx/      # write/query time-series metrics
│   │   ├── metrics/         # Prometheus gauge/counter definitions
│   │   └── hub/             # WebSocket hub (diff-only broadcast to frontend)
│   ├── migrations/          # SQL migration files (golang-migrate)
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── routes/          # SvelteKit file-based routes
│   │   │   ├── +page.svelte       # dashboard (virtual scroll grid)
│   │   │   ├── monitors/          # list + detail pages
│   │   │   └── settings/          # auth, config
│   │   ├── lib/
│   │   │   ├── api.ts             # REST API client
│   │   │   ├── ws.ts              # WebSocket singleton + reconnect logic
│   │   │   └── stores/            # Svelte stores (monitors, events)
│   │   └── components/            # shared UI components
│   ├── svelte.config.js           # adapter-static config
│   └── package.json
├── Dockerfile                     # multi-stage: node → go → distroless
├── docker-compose.yml             # production (pulse + postgres + influxdb)
├── docker-compose.dev.yml         # development (hot-reload)
└── Makefile
```

---

## Implementation Phases

### Phase 1 — Project Scaffolding
1. Init Go module `github.com/VitaliAndrushkevich/pulse`, set up `cmd/pulse/main.go`
2. Init SvelteKit project in `frontend/` with TypeScript + Tailwind + shadcn-svelte + `adapter-static`
3. Write `docker-compose.yml` (pulse + postgres + influxdb) and `docker-compose.dev.yml`
4. Write `Makefile` with targets: `dev`, `build`, `migrate`, `test`, `rotate-key`
5. Set up `golang-migrate` and write initial PostgreSQL schema migration

### Phase 2 — Data Layer
6. Define PostgreSQL schema: `monitors`, `secrets`, `incidents`, `check_results`, `users`, `api_tokens`
   *(depends on Phase 1)*
7. Generate `sqlc` queries for all CRUD operations
8. Set up InfluxDB bucket and write/query helpers in `internal/store/influx/`
9. Write DB connection init in `main.go` (fail-fast on startup)

### Phase 2.5 — Crypto & Secret Store
10. Implement `internal/crypto/`: AES-256-GCM encrypt/decrypt, key loading from `PULSE_SECRET_KEY` env var
    *(depends on Phase 2)*
11. Implement secret CRUD handlers — write-only API (values never returned in responses)
12. Implement API token CRUD — `bcrypt` hash storage, raw token shown once on creation
13. Add log sanitization middleware (strips `Authorization`, secret body fields)
14. Add `make rotate-key` target — re-encrypts all `secrets` rows in a single transaction

### Phase 3 — Monitor Engines
15. Define `Checker` interface: `Check(ctx, config) Result` — all types implement this
    *(depends on Phase 2)*
16. Implement `HTTP/HTTPS` checker: status code, response time, SSL cert expiry days
17. Implement `TCP` checker: dial + measure latency
18. Implement `UDP` checker: send/receive with configurable payload
19. Implement `WebSocket` checker: connect + optional handshake message
20. Implement `Scheduler`: bounded worker pool (default 200 workers), priority queue by `next_check_at`, PostgreSQL `LISTEN/NOTIFY` wakeup on monitor create/update
    *(depends on 15–19)*

### Phase 4 — REST API
21. Set up `gin` router with versioned prefix `/api/v1` and `//go:embed` static catch-all
    *(depends on Phase 2)*
22. Implement JWT-based single-user auth (login endpoint, middleware)
23. Implement monitor CRUD: `GET/POST /monitors`, `GET/PUT/DELETE /monitors/{id}` (PUT is create-or-update)
24. Implement history endpoint: `GET /monitors/{id}/history` — queries InfluxDB
25. Implement incidents endpoint: `GET /incidents`
26. Add Prometheus `/metrics`: `pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`
27. Add OpenAPI annotations + `swag generate` Makefile target → committed `openapi.yaml`

### Phase 5 — WebSocket Hub
28. Implement `internal/hub/` — fan-out broadcaster
    *(depends on Phase 3)*
29. Scheduler results → diff computation → hub → connected clients (patch only changed fields)
30. Expose authenticated `/ws` endpoint

### Phase 6 — Frontend
31. Build dashboard: monitor status grid with virtual scrolling (up/down/degraded, real-time)
    *(can start in parallel with Phases 3–5 using mock data)*
32. Build monitor list + create/edit form (secrets referenced by UUID, not shown raw)
33. Build monitor detail: response time chart (uplot), incidents log
34. Wire WebSocket store — merge incoming patches into Svelte store reactively
35. Build login page + JWT handling (httpOnly cookie)

### Phase 7 — Polish & Packaging
36. Switch SvelteKit to `@sveltejs/adapter-static`, verify `frontend/build/` output
37. Add `//go:embed all:frontend/build` to Go binary + Gin catch-all SPA handler
38. Write multi-stage `Dockerfile`: Node build → Go embed+compile → distroless final
39. Finalize `docker-compose.yml` with healthchecks and volume mounts
40. Finalize `docker-compose.dev.yml` with Vite proxy config for `/api` and `/ws`
41. Write `README.md` with quick-start instructions

---

## Key Files to Create

- `backend/cmd/pulse/main.go` — entry point, wires all components
- `backend/internal/crypto/crypto.go` — AES-256-GCM helpers
- `backend/internal/monitor/scheduler.go` — bounded worker pool
- `backend/internal/api/router.go` — gin router + embed handler
- `backend/internal/metrics/prometheus.go` — metric definitions
- `backend/internal/hub/hub.go` — WebSocket broadcaster
- `backend/migrations/001_initial.sql` — schema
- `frontend/src/lib/ws.ts` — WebSocket singleton + reconnect logic
- `frontend/src/lib/stores/monitors.ts` — reactive monitor state
- `Dockerfile`
- `docker-compose.yml`

---

## Verification

1. `make dev` starts all services cleanly
2. `POST /api/v1/auth/login` returns JWT; protected routes reject unauthenticated requests
3. `POST /api/v1/auth/tokens` returns raw API token once; subsequent reads return only the hash indicator
4. Create an HTTP monitor via REST API referencing a secret; verify check fires and result appears in InfluxDB
5. `GET /api/v1/monitors/{id}` returns `[REDACTED]` for credential values
6. `GET /metrics` returns correct Prometheus output including `pulse_monitor_up`
7. Open frontend dashboard with 500 mock monitors — no white page, virtual scroll works
8. Status updates arrive in real-time via WebSocket when a monitor changes state
9. TCP + UDP + WebSocket monitors execute checks successfully
10. `GET /api/v1/monitors/{id}/history` returns time-series data from InfluxDB
11. `docker-compose up` fresh pull works end-to-end on a clean machine

---

## Decisions

- **Single binary** (API + scheduler in one process) — can split later for scale
- **Single container** — Go embeds SvelteKit static build via `//go:embed`; one port, one image
- **Gin over chi** — built-in binding/validation, larger ecosystem, better swaggo support
- **sqlc over GORM** — type-safe, no magic, better performance
- **pgx over database/sql** — better PostgreSQL feature support and performance
- **uplot over chart.js** — 5x lighter, sufficient for time-series line charts
- **QUIC** — future scope: monitor target protocol for HTTP/3 services
- **Alerts** — future scope: only Prometheus `/metrics` in MVP
- **Multi-tenant** — future scope: single-user JWT for now
- **Status page** — future scope: public read-only page
- **Terraform provider** — near-future: API is designed for it (stable UUIDs, idempotent PUT, versioned)
- **No Vault / external secret engine** — goal is DB-at-rest protection only; `PULSE_SECRET_KEY` env var is sufficient

### 500+ Monitors: Design Decisions
- **Scheduler**: bounded worker pool (not one goroutine per monitor). Configurable max concurrency (default 200). Priority queue dispatch.
- **WebSocket hub**: diff-only updates — clients hold local state and merge patches. Never broadcast full state.
- **REST API**: ALL list endpoints paginated (`?page=&limit=`). No unbounded array responses.
- **Frontend**: `svelte-virtual-list` for monitor list — renders only visible DOM nodes. Dashboard grid paginated.
- **Database**: composite indexes on `(status, created_at)`, `(next_check_at)`. `LISTEN/NOTIFY` for instant scheduler wakeup.

### Credential Security: Design Decisions
- **Encrypted secret store**: `secrets` table — values encrypted AES-256-GCM before write, decrypted only at check time
- **Master key**: `PULSE_SECRET_KEY` env var (32-byte base64) — never stored in DB
- **Write-only API**: credential values never returned in any API response (`[REDACTED]`)
- **Secret references**: monitors reference secrets by UUID — one secret reused across monitors, rotated in one operation
- **No secrets in logs**: middleware strips `Authorization` and known secret body fields
- **API tokens**: `bcrypt` hash in DB — raw token shown once on creation (GitHub PAT pattern)
- **TLS**: Caddy reverse proxy in `docker-compose.yml` with automatic HTTPS (Let's Encrypt)

### API-First: Design Decisions
- OpenAPI 3.x spec auto-generated via `swaggo/swag`, committed as `openapi.yaml`
- All resource IDs are stable UUIDs
- `PUT /api/v1/monitors/{id}` is create-or-update (Terraform-friendly idempotency)
- Consistent error schema: `{ "error": { "code": "MONITOR_NOT_FOUND", "message": "..." } }`
- `/api/v1/` versioned path from day 1
- Rate limiting middleware (token bucket) via `gin-contrib`
- `X-Request-ID` propagated through all responses

---

## Out of Scope (MVP)

- QUIC / HTTP3 monitor type
- Alert channels (Slack, Discord, Telegram, Email, PagerDuty)
- Terraform provider (API is ready for it)
- Multi-tenant / organizations
- Public status page
- Kubernetes / Helm chart
- ICMP (ping) monitor type
- HashiCorp Vault / external secret engines
