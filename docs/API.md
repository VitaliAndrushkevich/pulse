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
