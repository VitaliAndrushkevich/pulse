# Copilot Instructions for Pulse

## Mission
Build Pulse as an API-first uptime monitoring platform with strong reliability, security, and clear operational behavior.

## What To Prioritize
1. Backend API contracts and data models before UI wiring.
2. Scalability for 500+ monitors without UI freezes or unbounded backend concurrency.
3. Security invariants for secret/token handling.
4. Reproducible local/dev deployment via Docker Compose.

## Hard Constraints
- Version all API endpoints under `/api/v1`.
- Keep resource IDs stable UUIDs.
- Implement idempotent `PUT` semantics for monitor resources.
- Do not expose secret values in API responses.
- Ensure all monitor list surfaces are paginated.
- Use diff-only WebSocket updates for monitor state changes.

## Expected Stack
- Backend: Go 1.22+, `gin`, `pgx/v5`, `sqlc`, `golang-migrate`, InfluxDB v2 client, Prometheus client, gorilla/websocket.
- Frontend: SvelteKit + TypeScript, `@sveltejs/adapter-static`, Tailwind, virtualized monitor list, `uplot` for history charts.
- Deployment: single Go binary embedding frontend assets; multi-stage container build; Compose-based local environments.

## API Contract Guidance
- REST first, UI second.
- Keep error shapes consistent:
  - `{ "error": { "code": "...", "message": "..." } }`
- Ensure auth middleware guards protected routes.
- OpenAPI spec is source of truth and must be committed.

## Data and Persistence Guidance
- PostgreSQL tables include: `monitors`, `secrets`, `incidents`, `check_results`, `users`, `api_tokens`.
- InfluxDB stores time-series check results and history.
- Use explicit schema migrations for every data change.

## Security Guidance
- Secret encryption: AES-256-GCM with key from `PULSE_SECRET_KEY`.
- API tokens: store only `bcrypt` hashes.
- Logging middleware must redact `Authorization` and secret-like fields.
- Key rotation path must exist and operate transactionally.

## Runtime and Performance Guidance
- Scheduler uses bounded worker pool and queue by `next_check_at`.
- Prefer `LISTEN/NOTIFY` wakeups for monitor updates.
- Avoid full-state fan-out over WebSockets.
- Frontend monitor views must remain responsive at high monitor counts.

## Quality Gate
When implementing features, verify:
1. API endpoint behavior is covered before frontend merge logic.
2. OpenAPI documentation matches runtime behavior.
3. Sensitive values are absent from responses/logs.
4. Compose-based startup works from clean state.

## Current Progress

The project is at ~25% of MVP completion. See [docs/MILESTONES.md](../docs/MILESTONES.md) for full details.

| Milestone | Status |
|-----------|--------|
| A: Foundations | ✅ Done |
| B: Data Layer | ✅ Done |
| C: Security & Secrets | 🔲 Next up |
| D: Monitor Engine | 🔲 Todo |
| E: API Surface | 🔲 Todo |
| F: WebSocket Realtime | 🔲 Todo |
| G: Frontend Product | 🔲 Todo |
| H: Packaging & Release | 🔲 Todo |

What's working: PostgreSQL schema, sqlc queries, InfluxDB helpers, Docker infrastructure, fail-fast startup, basic router with health endpoints.

What's next: Milestone C (crypto, secrets, auth primitives) → then E (API handlers) and D (scheduler/checkers) in parallel.

## Development Skills
Use these skills for domain-specific guidance:

### Frontend (Svelte)
- **svelte-code-writer**: CLI tools for Svelte 5 documentation and code analysis. Always use when creating/editing .svelte files or Svelte TypeScript modules.
- **svelte-core-bestpractices**: Code quality guidance for Svelte (reactivity, event handling, performance, styling). Load for component development.

### Backend (Golang)
- Comprehensive skills available for database access, concurrency patterns, error handling, testing, performance optimization, security hardening, and troubleshooting. Load per task requirements.
