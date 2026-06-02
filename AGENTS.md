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

## Security Requirements
- Never return raw secret values from APIs.
- Encrypt secret values at rest using AES-256-GCM.
- Store API tokens as `bcrypt` hashes; raw token shown only at creation.
- Sanitize logs: strip auth headers and known secret fields.

## Performance Requirements
- Design for 500+ monitors from the start.
- Scheduler must use bounded worker pools, not unbounded goroutines.
- WebSocket messages should be diff/patch payloads, not full-state snapshots.
- Frontend monitor collections must use virtualization for large lists.

## Backend Conventions
- Keep core logic in `backend/internal/...` packages.
- Prefer explicit interfaces and small packages over global shared state.
- Use `sqlc` generated queries instead of ORM abstractions.
- Fail fast during startup when dependencies are not reachable.

## Frontend Conventions
- Use TypeScript for all app code.
- Place API client and WS logic in `frontend/src/lib/`.
- Keep stores deterministic and patch-merge oriented.
- Avoid blocking renders; large monitor views must remain virtualized.

## Current Progress

The project is at ~50% of MVP completion. Full milestone breakdown: [docs/MILESTONES.md](docs/MILESTONES.md).

| Milestone | Status |
|-----------|--------|
| A: Foundations | ✅ Done |
| B: Data Layer | ✅ Done |
| C: Security & Secrets | ✅ Done |
| D: Monitor Engine | ✅ Done |
| E: API Surface | ✅ Done |
| F: WebSocket Realtime | 🔲 Todo |
| G: Frontend Product | 🔲 Todo |
| H: Packaging & Release | 🔲 Todo |

Completed (A–D):
- PostgreSQL schema + indexes, sqlc query layer, TimescaleDB history store scaffold
- Docker Compose infrastructure, fail-fast startup, migration tooling
- Basic gin router with X-Request-ID, sanitized logging middleware
- AES-256-GCM crypto module, secret write-only API, API token lifecycle, key rotation
- Monitor engine: Checker interface + Result model, HTTP/HTTPS checker (expected status codes, cert chain validation, SSL expiry threshold), TCP checker, UDP checker (reachability + payload mode), WebSocket checker (gorilla/websocket)
- Bounded worker pool scheduler (configurable via `PULSE_SCHEDULER_WORKERS`, default 50 dev / 200 prod)
- LISTEN/NOTIFY wakeups for instant scheduling on monitor create/update
- Graceful shutdown with context cancellation

Next priority: Milestone F (WebSocket hub and realtime) and Milestone G (Frontend) in parallel.

## Build and Test
Primary commands (expected):
- `make dev`
- `make build`
- `make test`
- `make migrate`
- `make rotate-key`
- `make openapi`

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
