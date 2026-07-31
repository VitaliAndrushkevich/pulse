# Pulse

Pulse is a self-hosted uptime monitoring platform. It ships as a single binary with an embedded web UI, backed by PostgreSQL and TimescaleDB for time-series storage. Designed for reliability at 500+ monitors with bounded worker pools, real-time WebSocket updates, and an API-first architecture.

*Pulse is an independent take on uptime monitoring — built with its own perspective on architecture and workflows. If you have questions, feature requests, or feedback, feel free to open an issue or reach out. Contributions of any kind are very welcome.*

> **Vibecoded with [Kiro](https://kiro.dev)** — an AI-powered IDE that turns ideas into working software through structured specs, steering files, and iterative development. Also there were some steps which were implemented with Copilot.

## Key Features

- **Multi-protocol monitoring** — HTTP/HTTPS, HTTP/3, QUIC, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP
- **Notifications** — Email (SMTP) and webhook channels with trigger-based rules, deduplication, retry with exponential backoff, and reminders
- **Single deployable container** — Go binary with embedded SvelteKit frontend
- **Real-time updates** — WebSocket diff/patch messages for instant UI sync
- **API-first** — full REST API with OpenAPI 3.0.3 spec
- **Dashboard widgets** — StatusRing, HealthScore, UptimeHeatmap, ResponseSparklines, SSLWarnings, IncidentsPanel, EventsFeed
- **Internationalization** — 13 languages with RTL support (Arabic), lazy-loaded locale bundles
- **MCP server** — manage monitors and get the status via AI assistants (Kiro, Copilot, Claude, Cursor) through the Model Context Protocol
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
| `mcp/` | MCP server binary — AI integration via Model Context Protocol |

### Data Flow

1. **Scheduler** dispatches checks to a bounded worker pool
2. **Workers** execute protocol-specific checks (HTTP, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP)
3. **Results** are persisted to TimescaleDB and broadcast to the **WebSocket Hub**
4. **Hub** sends diff/patch messages to connected clients
5. **Frontend** merges patches into local state for real-time UI updates
6. **Notification Dispatcher** evaluates trigger conditions and delivers alerts via email/webhook

## Quick Start

Grab the [`docker-compose.yml`](docker-compose.yml) and replace the placeholder secrets with real values:

```bash
# 1. Generate production secrets
openssl rand -base64 32  # → use as PULSE_SECRET_KEY
openssl rand -hex 32     # → use as PULSE_JWT_SECRET

# 2. Replace placeholder values in docker-compose.yml with generated secrets

# 3. Start Pulse
docker compose up -d
```

For a full list of available environment variables, see [`.env.example`](.env.example).

> The examples use Docker Compose, but Pulse runs on any OCI-compatible runtime — Podman, nerdctl, Rancher Desktop, or Kubernetes. Adapting the compose file to these tools is straightforward; refer to their respective documentation for setup instructions.

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

For API examples, endpoint reference, and authentication details, see [`docs/API.md`](docs/API.md).

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

## MCP Server (AI Integration)

Pulse includes an MCP server for managing monitors through AI assistants (Kiro, Claude, Codex, Cursor). For setup instructions and available tools, see [`mcp/README.md`](mcp/README.md).

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

For local setup, make targets, testing, CI/CD pipelines, and brand asset generation, see [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).

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
| Backend | Go 1.26, Gin, pgx/v5, sqlc, gorilla/websocket, golang-jwt/jwt/v5 |
| Frontend | Svelte 5, SvelteKit, TypeScript strict, Tailwind CSS 3.4, uPlot |
| Database | PostgreSQL 16 + TimescaleDB 2.17 |
| Protocols | HTTP/HTTPS, HTTP/3 (QUIC), TCP, UDP, WebSocket, gRPC, DNS, ICMP (raw sockets), SMTP |
| Observability | Prometheus client_golang |
| Container | Multi-stage Dockerfile (distroless runtime) |

## License

Licensed under the [Apache License 2.0](LICENSE).
