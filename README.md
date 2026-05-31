# Pulse

Pulse is a self-hosted uptime monitoring platform designed as an API-first alternative to Uptime Kuma.

## Current Status

Initial Milestone A bootstrap is in progress. The backend entrypoint, basic API router, and compose/make workflows are now wired.

Core implementation work is tracked in:

- [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md)
- [docs/TASKS.md](docs/TASKS.md)

## Architecture Snapshot

- Backend: Go single binary (API, scheduler, websocket hub)
- Frontend: SvelteKit static build embedded in backend binary
- Storage: PostgreSQL for config and state, InfluxDB for time-series history
- Observability: Prometheus metrics at /metrics

## MVP Scope

- Monitor types: HTTP/HTTPS, TCP, UDP, WebSocket
- Single-user authentication
- Docker Compose based local and production-style startup

## Repository Layout

- [backend](backend): Go application code and migrations
- [frontend](frontend): SvelteKit frontend
- [docs](docs): planning, tasking, architecture, and operational docs
- [.github](.github): Copilot prompt and instruction files

## Documentation Policy

Documentation must be updated together with code changes.

When a change impacts behavior, architecture, API contracts, setup steps, or operations, update related docs in the same pull request.

Minimum expected updates:

- API changes: update OpenAPI and relevant docs
- Architecture changes: update architecture or design docs
- New workflows or commands: update README and runbook docs
- Task progress: update [docs/TASKS.md](docs/TASKS.md) task status

## Next Steps

1. Complete frontend bootstrap and SvelteKit initialization
2. Finalize migration tooling integration (`make migrate`)
3. Start Phase 2 data schema and sqlc setup

## Quick Start (Bootstrap)

1. Run backend locally:
   - `make run`
2. Build backend binary:
   - `make build`
3. Start full stack containers:
   - `make dev`
4. Start local development container profile:
   - `make dev-local`

The backend fails fast on startup: it exits with a non-zero code if PostgreSQL
or InfluxDB cannot be reached. `make dev` provisions both via Docker Compose
(including InfluxDB org/bucket/token), so `make run` against a local stack
expects those dependencies to be up.

Key environment variables:

- `PULSE_PORT` (default `8080`)
- `DATABASE_URL` (default `postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable`)
- `INFLUXDB_URL` (default `http://influxdb:8086`)
- `INFLUXDB_TOKEN`, `INFLUXDB_ORG` (default `pulse`), `INFLUXDB_BUCKET` (default `pulse`)

Health endpoints:

- `GET /healthz`
- `GET /api/v1/healthz`
