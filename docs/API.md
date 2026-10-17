# API Usage

Pulse exposes a REST API under `/api/v1`. All endpoints return JSON with the error envelope `{ "error": { "code": "...", "message": "..." } }` on failure.

## Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "your-password"}'
```

Response:

```json
{ "token": "eyJhbGciOi..." }
```

## Create a Monitor

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

## Create a gRPC Monitor

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

## List Monitors

```bash
curl http://localhost:8080/api/v1/monitors?page=1&limit=20 \
  -H "Authorization: Bearer <token>"
```

## WebSocket (Real-time Updates)

```bash
# Connect with wscat or any WebSocket client
wscat -c "ws://localhost:8080/ws?token=<jwt_token>"
```

Messages follow the envelope format:

```json
{ "type": "monitor_status", "payload": { "id": "uuid", "status": "up", "latency_ms": 42 } }
```

## Heartbeats (push monitoring)

Heartbeat monitors are the inverse of active checks: instead of Pulse probing a target,
an external job (cron, backup script, scheduled task) pings Pulse on every successful run.
If a ping does not arrive within `expected_interval_seconds + grace_seconds`, the heartbeat
transitions to `down`.

Create a heartbeat and capture the ping URL (the raw ping token is returned **only once**,
on create and on rotate-token):

```bash
curl -X POST http://localhost:8080/api/v1/heartbeats \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Nightly Backup",
    "expected_interval_seconds": 86400,
    "grace_seconds": 3600
  }'
```

Response (`201`) includes `ping_token`, `ping_url`, and `ping_fail_url`. `ping_url` /
`ping_fail_url` are omitted when `PULSE_BASE_URL` is not set — construct them manually as
`<base>/ping/<ping_token>` and `<base>/ping/<ping_token>/fail`.

Report a successful run from your job (no auth — possession of the token in the URL is the
authorization; GET and POST both work, request bodies are ignored):

```bash
curl http://localhost:8080/ping/<ping_token>
```

Report an explicit failure (transitions the heartbeat to `down` without advancing the last
success time):

```bash
curl http://localhost:8080/ping/<ping_token>/fail
```

An unknown, malformed, or rotated token returns a minimal `404`.

> **Note:** Heartbeat notification bindings (`heartbeat_missed`, `heartbeat_recovered`
> triggers) can be created and stored, but end-to-end alert **delivery** for heartbeats is
> not yet wired. State changes are persisted, broadcast over WebSocket, and reflected in
> Prometheus metrics; alerts do not fire on missed runs yet.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/heartbeats` | List heartbeats (paginated; no raw token) |
| `POST` | `/api/v1/heartbeats` | Create heartbeat (returns ping token once) |
| `GET` | `/api/v1/heartbeats/{id}` | Get heartbeat details (no raw token) |
| `PUT` | `/api/v1/heartbeats/{id}` | Create or update heartbeat (idempotent; never rotates token) |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete heartbeat (cascades tags and bindings) |
| `POST` | `/api/v1/heartbeats/{id}/rotate-token` | Issue a new ping token (invalidates old URLs) |
| `GET` | `/api/v1/heartbeats/{id}/notification-bindings` | List bindings for heartbeat |
| `POST` | `/api/v1/heartbeats/{id}/notification-bindings` | Create binding (`heartbeat_missed`/`heartbeat_recovered`) |
| `PUT` | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Update binding |
| `DELETE` | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Delete binding |
| `GET`/`POST` | `/ping/{token}` | Report a successful run (root-level, no auth) |
| `GET`/`POST` | `/ping/{token}/fail` | Report an explicit failure (root-level, no auth) |

## Endpoints

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

Full API reference: [`backend/api/openapi.yaml`](../backend/api/openapi.yaml)

## Metrics Authentication

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
