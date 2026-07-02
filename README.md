# Pulse

Pulse is a self-hosted uptime monitoring platform. It ships as a single binary with an embedded web UI, backed by PostgreSQL and TimescaleDB for time-series storage. Designed for reliability at 500+ monitors with bounded worker pools, real-time WebSocket updates, and an API-first architecture.

> **Vibecoded with [Kiro](https://kiro.dev)** — an AI-powered IDE that turns ideas into working software through structured specs, steering files, and iterative development.

## Key Features

- **Multi-protocol monitoring** — HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP
- **Single deployable container** — Go binary with embedded SvelteKit frontend
- **Real-time updates** — WebSocket diff/patch messages for instant UI sync
- **API-first** — full REST API with OpenAPI 3.0.3 spec
- **Dashboard widgets** — StatusRing, HealthScore, UptimeHeatmap, ResponseSparklines, SSLWarnings, IncidentsPanel, EventsFeed
- **Internationalization** — 13 languages with RTL support (Arabic), lazy-loaded locale bundles
- **Prometheus metrics** — built-in `/metrics` endpoint
- **Security** — AES-256-GCM secret encryption, JWT + API token auth, per-monitor credentials
- **Scalable** — bounded worker pools, designed for 500+ concurrent monitors
- **Light/Dark/System theming** — CSS custom properties with tri-state cycling, OS preference tracking, WCAG AA contrast
- **Brand identity** — ECG-inspired logo mark with responsive lockup, self-hosted Inter typography

## Architecture

Pulse runs as a single Go process serving both the API and the frontend:

```
┌─────────────────────────────────────────────────┐
│                 Pulse Binary                    │
│                                                 │
│  ┌──────────┐  ┌───────────┐  ┌──────────────┐  │
│  │ gin HTTP │  │ Scheduler │  │ WebSocket Hub│  │
│  │  Router  │  │  + Workers│  │  (fan-out)   │  │
│  └────┬─────┘  └─────┬─────┘  └──────┬───────┘  │
│       │              │               │          │
│  ┌────┴──────────────┴───────────────┴────────┐ │
│  │           PostgreSQL + TimescaleDB         │ │
│  └────────────────────────────────────────────┘ │
│                                                 │
│  ┌────────────────────────────────────────────┐ │
│  │        Embedded SvelteKit Frontend         │ │
│  └────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

### Package Layout

| Package | Purpose |
|---------|---------|
| `backend/cmd/pulse/main.go` | Application entrypoint |
| `backend/internal/api/` | HTTP handlers and gin router |
| `backend/internal/monitor/` | Scheduler, worker pool, protocol checkers |
| `backend/internal/hub/` | WebSocket hub with fan-out broadcast |
| `backend/internal/store/` | Database layer (postgres + timescale) |
| `backend/internal/frontend/` | Embedded SPA assets (`go:embed`) |
| `frontend/` | SvelteKit source code |
| `frontend/static/brand/` | Logo SVGs, PNG exports, usage guidelines |
| `backend/api/openapi.yaml` | OpenAPI 3.0.3 specification |
| `backend/migrations/` | SQL migration files |

### Data Flow

1. **Scheduler** dispatches checks to a bounded worker pool
2. **Workers** execute protocol-specific checks (HTTP, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP)
3. **Results** are persisted to TimescaleDB and broadcast to the **WebSocket Hub**
4. **Hub** sends diff/patch messages to connected clients
5. **Frontend** merges patches into local state for real-time UI updates

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (v20.10+)
- [Docker Compose](https://docs.docker.com/compose/install/) v2

That's it. The container image includes everything needed to run Pulse.

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/VitaliAndrushkevich/pulse.git
cd pulse

# 2. Configure environment
cp .env.example .env
# Edit .env — you MUST change PULSE_SECRET_KEY and PULSE_JWT_SECRET

# 3. Start Pulse
docker compose up -d
```

Pulse is now running at [http://localhost:8080](http://localhost:8080).

On first launch you'll be guided through initial setup to create your admin account.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PULSE_PORT` | HTTP port the server listens on | `8080` |
| `PULSE_DEV` | Enable dev mode (Swagger UI, debug logging) | `false` |
| `PULSE_SCHEDULER_WORKERS` | Number of concurrent check workers | `200` |
| `PULSE_SECRET_KEY` | AES-256-GCM key for secrets at rest (base64, 32 bytes) | **required** |
| `PULSE_JWT_SECRET` | Secret for signing JWT tokens | **required** |
| `PULSE_JWT_EXPIRY` | JWT token expiry duration (Go duration) | `24h` |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable` |

Generate secrets:

```bash
# AES-256 key (32 bytes, base64)
openssl rand -base64 32

# JWT secret (hex string)
openssl rand -hex 32
```

## API Usage

Pulse exposes a REST API under `/api/v1`. All endpoints return JSON with the error envelope `{ "error": { "code": "...", "message": "..." } }` on failure.

### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "your-password"}'
```

Response:

```json
{ "token": "eyJhbGciOi..." }
```

### Create a Monitor

```bash
curl -X POST http://localhost:8080/api/v1/monitors \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My API",
    "type": "http",
    "target": "https://api.example.com/health",
    "interval_seconds": 60
  }'
```

### Create a gRPC Monitor

```bash
curl -X POST http://localhost:8080/api/v1/monitors \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gRPC Health Check",
    "type": "grpc",
    "target": "grpc.example.com:443",
    "interval_seconds": 30,
    "timeout_seconds": 10,
    "settings": {
      "service_method": "grpc.health.v1.Health/Check",
      "tls_mode": "tls",
      "ssl_expiry_threshold": 30
    }
  }'
```

### List Monitors

```bash
curl http://localhost:8080/api/v1/monitors?page=1&limit=20 \
  -H "Authorization: Bearer <token>"
```

### WebSocket (Real-time Updates)

```bash
# Connect with wscat or any WebSocket client
wscat -c "ws://localhost:8080/ws?token=<jwt_token>"
```

Messages follow the envelope format:

```json
{ "type": "monitor_status", "payload": { "id": "uuid", "status": "up", "latency_ms": 42 } }
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/monitors` | List monitors (paginated) |
| `POST` | `/api/v1/monitors` | Create monitor |
| `GET` | `/api/v1/monitors/{id}` | Get monitor details |
| `PUT` | `/api/v1/monitors/{id}` | Create or update monitor (idempotent) |
| `DELETE` | `/api/v1/monitors/{id}` | Delete monitor |
| `GET` | `/api/v1/monitors/{id}/history` | Check history (TimescaleDB, 7-day window) |
| `POST` | `/api/v1/monitors/{id}/credentials` | Create monitor credential |
| `GET` | `/api/v1/monitors/{id}/credentials` | List monitor credentials (values redacted) |
| `PUT` | `/api/v1/monitors/{id}/credentials/{credentialId}` | Update credential |
| `DELETE` | `/api/v1/monitors/{id}/credentials/{credentialId}` | Delete credential |
| `GET` | `/api/v1/incidents` | List incidents (paginated) |
| `GET` | `/api/v1/monitors/{id}/incidents` | Per-monitor incidents |
| `POST` | `/api/v1/secrets` | Create a secret |
| `GET` | `/api/v1/secrets` | List secrets (values redacted) |
| `POST` | `/api/v1/tokens` | Create API token |
| `GET` | `/healthz` | Health check |
| `GET` | `/metrics` | Prometheus metrics |

Full API reference: [`backend/api/openapi.yaml`](backend/api/openapi.yaml)

## Supported Languages

Pulse ships with 13 locale bundles. The UI language is selectable per-user from Settings.

| Language | Code | Direction |
|----------|------|-----------|
| English | `en` | LTR |
| العربية (Arabic) | `ar` | RTL |
| Беларуская (Belarusian) | `be` | LTR |
| Deutsch (German) | `de` | LTR |
| Español (Spanish) | `es` | LTR |
| Français (French) | `fr` | LTR |
| Italiano (Italian) | `it` | LTR |
| 日本語 (Japanese) | `ja` | LTR |
| 한국어 (Korean) | `ko` | LTR |
| Português (Portuguese) | `pt` | LTR |
| Русский (Russian) | `ru` | LTR |
| Türkçe (Turkish) | `tr` | LTR |
| 中文 (Chinese) | `zh` | LTR |

Non-English locales are lazy-loaded on demand. Fallback chain: active locale → English → key string.

## Development

### Prerequisites (Development)

- Go 1.25+
- Node.js 22+
- pnpm 9+ (install via `corepack enable` or [pnpm.io/installation](https://pnpm.io/installation))
- PostgreSQL 16 with TimescaleDB 2.17+
- Make

### Make Targets

| Target | Description |
|--------|-------------|
| `make dev` | Full stack via Docker Compose (Pulse + TimescaleDB) |
| `make dev-local` | Lightweight compose (backend + postgres only) |
| `make run` | `go run ./cmd/pulse` (requires local postgres) |
| `make build` | Build Go binary |
| `make build-frontend` | Build frontend and copy to embed path |
| `make build-all` | Production build: frontend + Go binary with embedded assets |
| `make test` | Run Go tests (`go test ./...`) |
| `make migrate` | Run database migrations up |
| `make migrate-down` | Roll back last migration |
| `make rotate-key` | AES key rotation with transactional re-encryption |
| `make openapi` | Validate OpenAPI spec |

### Local Setup

```bash
# Start database
docker compose up postgres -d

# Run migrations
make migrate

# Start backend (hot-reload with go run)
make run

# In a separate terminal — start frontend dev server
cd frontend && pnpm install && pnpm dev
```

#### Frontend Dev Container

For containerized frontend development with hot module replacement (HMR), use the `frontend` service in `docker-compose.dev.yml`:

```bash
docker compose -f docker-compose.dev.yml up frontend
```

This starts the Vite dev server on port **5173** with HMR enabled — source file changes are reflected in the browser instantly.

### Running Tests

```bash
# Backend tests
make test

# Frontend tests (unit + property-based via fast-check)
cd frontend && pnpm test
```

### Brand Assets

Logo files and brand guidelines are in `frontend/static/brand/`. To regenerate PNG exports from the SVG source:

```bash
cd frontend && node scripts/generate-brand-pngs.mjs
```

To regenerate favicon and PWA icons:

```bash
cd frontend && node scripts/generate-icons.mjs
```

## Docker Compose Override

For local customization (different ports, extra services), create a `docker-compose.override.yml`:

```yaml
services:
  pulse:
    ports:
      - "3000:8080"
    environment:
      PULSE_DEV: "true"
```

Docker Compose automatically merges this with the base file.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, Gin, pgx/v5, sqlc, gorilla/websocket, golang-jwt/jwt/v5 |
| Frontend | Svelte 5, SvelteKit, TypeScript strict, Tailwind CSS 3.4, uPlot |
| Database | PostgreSQL 16 + TimescaleDB 2.17 |
| Protocols | HTTP/HTTPS, HTTP/3 (QUIC), TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP |
| Observability | Prometheus client_golang |
| Container | Multi-stage Dockerfile (distroless runtime) |

## License

Licensed under the [Apache License 2.0](LICENSE).
