# Pulse

Pulse is a self-hosted uptime monitoring platform. It ships as a single binary with an embedded web UI, backed by PostgreSQL and TimescaleDB for time-series storage. Designed for reliability at 500+ monitors with bounded worker pools, real-time WebSocket updates, and an API-first architecture.

> **Vibecoded with [Kiro](https://kiro.dev)** — an AI-powered IDE that turns ideas into working software through structured specs, steering files, and iterative development.

## Key Features

- **Multi-protocol monitoring** — HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP
- **Notifications** — Email (SMTP) and webhook channels with trigger-based rules, deduplication, retry with exponential backoff, and reminders
- **Single deployable container** — Go binary with embedded SvelteKit frontend
- **Real-time updates** — WebSocket diff/patch messages for instant UI sync
- **API-first** — full REST API with OpenAPI 3.0.3 spec
- **Dashboard widgets** — StatusRing, HealthScore, UptimeHeatmap, ResponseSparklines, SSLWarnings, IncidentsPanel, EventsFeed
- **Internationalization** — 13 languages with RTL support (Arabic), lazy-loaded locale bundles
- **Prometheus metrics** — built-in `/metrics` endpoint with optional Basic Auth protection
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
| `backend/internal/notification/` | Notification dispatcher, SMTP/webhook delivery, retry, reminders |
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
6. **Notification Dispatcher** evaluates trigger conditions and delivers alerts via email/webhook

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
| `PULSE_BASE_URL` | Public URL for email links and WebSocket origin validation (e.g. `https://pulse.example.com`) | *(empty — links omitted, WS allows all origins)* |
| `PULSE_NOTIFICATION_WORKERS` | Number of concurrent notification delivery workers | `50` |
| `PULSE_NOTIFICATION_DRAIN_TIMEOUT` | Max time to drain in-flight notifications on shutdown (Go duration) | `30s` |
| `PULSE_LOG_LEVEL` | Log verbosity for notification delivery (`warn`, `info`, `debug`) | `warn` |
| `PULSE_METRICS_USER` | Basic Auth username for `/metrics` endpoint (empty = no auth) | *(empty)* |
| `PULSE_METRICS_PASSWORD` | Basic Auth password for `/metrics` endpoint (empty = no auth) | *(empty)* |
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
| `POST` | `/api/v1/monitors/{id}/proto-source` | Upload proto files for gRPC monitor |
| `POST` | `/api/v1/monitors/{id}/proto-source/reflect` | Trigger Server Reflection for gRPC monitor |
| `GET` | `/api/v1/monitors/{id}/proto-source` | Get proto source metadata |
| `DELETE` | `/api/v1/monitors/{id}/proto-source` | Delete proto source |
| `POST` | `/api/v1/grpc/reflect` | Ad-hoc Server Reflection (no monitor required) |
| `POST` | `/api/v1/grpc/parse-proto` | Ad-hoc proto file parsing (no monitor required) |
| `POST` | `/api/v1/notifications/channels` | Create notification channel |
| `GET` | `/api/v1/notifications/channels` | List notification channels |
| `GET` | `/api/v1/notifications/channels/{id}` | Get channel details |
| `PUT` | `/api/v1/notifications/channels/{id}` | Update channel |
| `DELETE` | `/api/v1/notifications/channels/{id}` | Delete channel |
| `POST` | `/api/v1/notifications/channels/{id}/test` | Send test notification |
| `GET` | `/api/v1/notifications/channels/{id}/delivery-logs` | Delivery log for channel |
| `GET` | `/api/v1/notifications/template-variables` | Available template variables |
| `GET` | `/api/v1/notifications/smtp-settings` | Get SMTP config |
| `PUT` | `/api/v1/notifications/smtp-settings` | Create/update SMTP settings |
| `DELETE` | `/api/v1/notifications/smtp-settings` | Remove SMTP settings |
| `POST` | `/api/v1/notifications/smtp-settings/test` | Test SMTP connection |
| `POST` | `/api/v1/monitors/{id}/notification-bindings` | Create notification binding |
| `GET` | `/api/v1/monitors/{id}/notification-bindings` | List bindings for monitor |
| `PUT` | `/api/v1/monitors/{id}/notification-bindings/{bindingId}` | Update binding |
| `DELETE` | `/api/v1/monitors/{id}/notification-bindings/{bindingId}` | Delete binding |
| `GET` | `/healthz` | Health check |
| `GET` | `/metrics` | Prometheus metrics (optional Basic Auth) |

Full API reference: [`backend/api/openapi.yaml`](backend/api/openapi.yaml)

### Metrics Authentication

The `/metrics` endpoint can be protected with HTTP Basic Auth by setting `PULSE_METRICS_USER` and `PULSE_METRICS_PASSWORD`. When both variables are set, Prometheus must include credentials in its scrape config:

```yaml
scrape_configs:
  - job_name: pulse
    basic_auth:
      username: prometheus
      password: your-secret
    static_configs:
      - targets: ['localhost:8080']
```

When either variable is empty, `/metrics` is served without authentication.

## Notifications

Pulse includes a built-in notification system that alerts you when monitors change state. Notifications are delivered asynchronously via a bounded worker pool, separate from the monitoring engine.

### Supported Channels

- **Email (SMTP)** — Branded HTML emails with monitor status, response time, and incident links. Configure SMTP settings in the UI (Settings → SMTP) or via the API.
- **Webhook** — HTTP callbacks to any URL. Customizable request method, headers (encrypted at rest), and body template using Go `text/template` syntax.

### Trigger Conditions

Create notification bindings to control when alerts fire:

| Trigger | Description | Parameters |
|---------|-------------|------------|
| `monitor_down` | Monitor transitions to "down" | — |
| `monitor_up` | Monitor recovers (down → up) | — |
| `degraded` | Response time exceeds threshold | `threshold_ms` (1–60000) |
| `ssl_expiring` | SSL certificate expiring soon | `days_before` (1–365) |
| `n_failures_in_row` | Consecutive failures reach count | `count` (1–100) |

Notifications are deduplicated — a trigger fires once per state transition, not on every check.

### Reminders

Bindings support optional reminders (`reminder_interval_minutes`: 30–1440) that re-send notifications at configurable intervals while a condition persists.

### Retry & Delivery Logs

Failed deliveries are retried with exponential backoff (30s → 60s → 120s, max 4 attempts). Non-retryable errors (malformed templates, oversized payloads) fail immediately. All delivery attempts are recorded in the delivery log, accessible per-channel via the API.

### Webhook Template Variables

Use these variables in webhook body templates:

```
{{.Monitor.Name}}       — Monitor display name
{{.Monitor.Target}}     — Target URL/host
{{.Status}}             — Current state ("up" or "down")
{{.PreviousStatus}}     — State before this check
{{.ResponseTime}}       — Response time in milliseconds
{{.Incident.ID}}        — Incident UUID
{{.Incident.StartedAt}} — Incident start time
{{.Incident.Duration}}  — Incident duration
{{.Timestamp}}          — Event timestamp
{{.BaseURL}}            — Pulse public URL (from PULSE_BASE_URL)
```

### Example: Create a Webhook Channel

```bash
curl -X POST http://localhost:8080/api/v1/notifications/channels \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Slack Alerts",
    "type": "webhook",
    "config": {
      "url": "https://hooks.slack.com/services/T.../B.../xxx",
      "method": "POST",
      "body_template": "{\"text\": \"{{.Monitor.Name}} is {{.Status}}\"}"
    }
  }'
```

### Example: Bind a Channel to a Monitor

```bash
curl -X POST http://localhost:8080/api/v1/monitors/<monitor-id>/notification-bindings \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "<channel-uuid>",
    "triggers": [
      {"type": "monitor_down"},
      {"type": "monitor_up"}
    ],
    "reminder_interval_minutes": 30
  }'
```

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

## ICMP Monitoring

The ICMP checker uses raw sockets (no `ping` binary required). This needs the `NET_RAW` capability:

- **Docker**: The included `docker-compose.yml` already grants `cap_add: [NET_RAW]`.
- **Bare metal**: Run with `CAP_NET_RAW` (`setcap cap_net_raw+ep ./pulse`) or as root, or on Linux 3.0+ with `sysctl net.ipv4.ping_group_range` covering the process GID (unprivileged UDP ICMP fallback).
- **Kubernetes**: Add `NET_RAW` to the container's `securityContext.capabilities.add`.

If neither privileged raw sockets nor unprivileged UDP ICMP are available, ICMP monitors will report an error on check execution.

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
| Protocols | HTTP/HTTPS, HTTP/3 (QUIC), TCP, UDP, WebSocket, gRPC, DNS, ICMP (raw sockets), SMTP |
| Observability | Prometheus client_golang |
| Container | Multi-stage Dockerfile (distroless runtime) |

## License

Licensed under the [Apache License 2.0](LICENSE).
