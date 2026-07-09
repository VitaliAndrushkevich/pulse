## Future Roadmap

This roadmap recommends strategic investments that go beyond individual finding remediation. Items are organized by timeframe and category, building on the patterns identified across the security, performance, and code quality audit domains.

---

### Short-Term (1–3 Months)

These items address foundational gaps that block further maturity. Most relate directly to high-priority findings but require architectural effort beyond a single-file fix.

#### Architecture

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-01 | **Content Security Policy (CSP)** | Implement a strict CSP with `script-src 'self'`, `style-src 'self' 'unsafe-inline'` (for Tailwind), and `connect-src` restricted to the API origin. This is the primary defense-in-depth control for the JWT-in-localStorage architecture (SEC-001, SEC-006). Deploy as HTTP response headers from the Go server on all HTML responses. | None |
| R-02 | **Graceful shutdown sequence** | Refactor the main shutdown path to follow the correct drain order: (1) stop HTTP listener, (2) drain in-flight HTTP requests with timeout, (3) stop scheduler tick, (4) drain notification workers, (5) close WebSocket hub, (6) close DB pool. Addresses QUAL-020 and QUAL-029 as an architectural unit. | None |
| R-03 | **CI/CD pipeline** | Implement GitHub Actions with: `go test ./...`, `go vet`, `golangci-lint`, `pnpm test`, OpenAPI spec validation, Docker build, and image push to GHCR. This was deferred from MVP and is prerequisite for all automated quality gates. | None |
| R-04 | **Token rotation mechanism** | Implement short-lived access tokens (15 min) with a refresh token stored in an HttpOnly cookie. The refresh endpoint issues new access tokens without requiring re-authentication. Addresses SEC-004 (24h static session). | R-01 (CSP protects the refresh cookie) |

#### Security

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-05 | **Rate limiting middleware** | Add per-IP rate limiting on `/auth/login` (5 attempts/minute), `/auth/setup` (3 attempts/minute), and a global 100 req/s per IP on API endpoints. Use a token bucket algorithm with in-memory store (single-instance deployment). | None |
| R-06 | **Key rotation scope expansion** | Extend the key rotation command to re-encrypt notification channel credentials (SMTP passwords, webhook headers) within the same transaction. Addresses SEC-011 directly. | None |
| R-07 | **Log redaction hardening** | Extend `SanitizeLog` to cover WebSocket upgrade request paths (token in query parameter), and add a structured logging middleware that redacts Authorization headers at the gin middleware level rather than per-handler. Addresses SEC-013. | None |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-08 | **Error boundary pages** | Add SvelteKit `+error.svelte` pages at the root and per-route-group level to prevent white-screen failures. Addresses QUAL-001. | None |
| R-09 | **Bundle size CI guard** | Add a `size-limit` step to CI that fails the build if initial JS exceeds 200 KB gzipped or any route chunk exceeds 30 KB gzipped. Prevents regression of the current healthy bundle state (PERF-003). | R-03 (CI pipeline) |

---

### Medium-Term (3–6 Months)

These items improve operational maturity and prepare the platform for growth beyond single-instance deployment.

#### Architecture

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-10 | **E2E test infrastructure** | Introduce Playwright for critical user paths: login flow, monitor CRUD, real-time status updates via WebSocket, notification channel configuration. Target 10–15 scenarios covering the happy paths that unit tests cannot validate (cross-component, cross-API interactions). | R-03 (CI pipeline) |
| R-11 | **API versioning strategy** | Define a formal deprecation policy for `/api/v1`. Introduce response headers (`Sunset`, `Deprecation`) on endpoints slated for change. Prepare the router to serve `/api/v2` alongside v1 when breaking changes are needed. | None |
| R-12 | **WebSocket auth refactor** | Consolidate WS authentication with the HTTP auth middleware (shared function, not duplicated logic). Move WS token from query parameter to the first message after upgrade (protocol-level auth) to eliminate URL-based token exposure in logs and browser history. Addresses SEC-002, SEC-014. | R-04 (token rotation) |

#### Scalability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-13 | **Connection pool auto-tuning** | Replace fixed pgx defaults with pool sizing derived from `PULSE_SCHEDULER_WORKERS + PULSE_NOTIFICATION_WORKERS + 20` overhead connections. Add pool metrics (`pool_acquired`, `pool_idle`, `pool_max`) to Prometheus. Addresses PERF-020. | None |
| R-14 | **Notification fan-out batch queries** | Replace the N+1 query pattern in `EvaluateAndDispatch` with a batch query that fetches all bindings for triggered monitors in a single round-trip. Addresses PERF-021. | None |
| R-15 | **TimescaleDB native retention** | Replace the custom DELETE-with-subquery retention logic with TimescaleDB's `add_retention_policy()` on the `check_results` hypertable. Enables background cleanup without application-level scheduling. Addresses PERF-022. | None |

#### Observability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-16 | **Structured logging migration** | Replace `log.Printf` / `log.Fatalf` calls with `slog` (Go 1.21+ stdlib). Use JSON output in production, text in dev. Add request_id, monitor_id, and channel_id as structured fields. Resolves SEC-012 (log.Fatalf in key rotation preventing deferred rollback). | None |
| R-17 | **Platform self-health endpoint** | Extend `/healthz` to return component-level health: DB connectivity, pool utilization, scheduler running, notification dispatcher running, hub client count. Expose as both human-readable JSON and Prometheus gauge metrics. | None |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-18 | **API documentation site** | Generate and host API documentation from `openapi.yaml` using Redoc or Stoplight Elements. Serve at `/docs` in production (currently Swagger UI is dev-only). Add request/response examples for all endpoints. | None |
| R-19 | **Developer onboarding guide** | Write a `docs/CONTRIBUTING.md` covering: local setup (with and without Docker), running tests, adding a new checker protocol, adding a notification channel, adding a frontend route, and the migration workflow. | None |
| R-20 | **Frontend test coverage expansion** | Add integration tests for notification channel management, monitor creation/editing, and WebSocket reconnection behavior. Target the zero-coverage modules identified in QUAL-007. | R-10 (E2E infra for integration scenarios) |

---

### Long-Term (6–12 Months)

These items address scalability beyond the 500-monitor design point and prepare Pulse for multi-instance deployment scenarios.

#### Scalability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-21 | **Horizontal scheduler scaling** | Introduce a distributed lock (PostgreSQL advisory locks or Redis-based) so multiple Pulse instances can share the monitor workload. Each instance claims a partition of monitors via consistent hashing. Enables scaling beyond what a single worker pool can handle. | R-13 (pool tuning), R-15 (native retention) |
| R-22 | **Read replica support** | Add a secondary read-only DB connection pool for list/history queries. Route all `GET` endpoints and dashboard data to the replica, reserving the primary for writes and scheduler operations. Enables scaling read-heavy dashboard traffic independently. | R-13 (pool tuning) |
| R-23 | **Queue-based notification dispatch** | Replace the in-process notification channel buffer with an external queue (PostgreSQL `LISTEN/NOTIFY` or Redis Streams). Enables notification workers to run in a separate process or instance, decoupling delivery latency from check execution. | R-14 (batch queries), R-16 (structured logging) |
| R-24 | **Check result streaming pipeline** | Introduce a lightweight event bus (PostgreSQL logical replication or NATS) between the scheduler and consumers (WebSocket hub, notification evaluator, metrics aggregator). Decouples producers from consumers and enables future integrations (status page, webhooks) without modifying the scheduler. | R-21 (horizontal scaling) |

#### Observability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-25 | **Distributed tracing** | Instrument the request lifecycle with OpenTelemetry spans: HTTP handler → DB query → notification dispatch → delivery. Correlate trace IDs across the scheduler, hub, and notification subsystems. Export to Jaeger or OTLP-compatible backend. | R-16 (structured logging — trace IDs in logs) |
| R-26 | **Self-alerting** | Configure Pulse to monitor itself: create internal monitors for `/healthz`, DB connection health, and notification delivery success rate. Alert via the notification subsystem when platform health degrades. A "watchdog" pattern that proves the monitoring pipeline works end-to-end. | R-17 (self-health endpoint) |
| R-27 | **Operational dashboard** | Create a Grafana dashboard (or embedded admin page) visualizing: scheduler throughput, check latency percentiles, WebSocket client count, notification delivery success rate, DB pool utilization, and memory/CPU usage. Sourced from existing Prometheus metrics plus the additions from R-13 and R-17. | R-17 (health endpoint), R-13 (pool metrics) |

#### Security

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-28 | **Audit logging** | Record administrative actions (login, monitor CRUD, channel CRUD, key rotation, settings changes) in a dedicated `audit_log` table with actor, action, resource, timestamp, and IP address. Expose via API for review. Prepares for compliance requirements if Pulse is used in regulated environments. | R-16 (structured logging) |
| R-29 | **RBAC preparation** | Design a role-based access control model (admin, operator, viewer) with resource-level permissions. Implement as a middleware layer that reads roles from JWT claims. While Pulse is currently single-admin, this prepares for multi-user deployment without requiring a full rewrite. | R-04 (token rotation — roles in JWT) |
| R-30 | **Secret scanning in CI** | Add `gitleaks` or `trufflehog` to the CI pipeline to prevent accidental commits of secrets, API keys, or encryption keys. Addresses the defense-in-depth gap identified in SEC-005 (source map and environment variable exposure risks). | R-03 (CI pipeline) |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-31 | **Plugin architecture for checkers** | Refactor the checker registry to support external protocol plugins (loaded via Go plugin or subprocess protocol). Enables community contributions of new check types without modifying the core binary. | R-11 (API versioning) |
| R-32 | **Development workflow automation** | Add `make lint`, `make fmt`, `make check-all` commands that run the full pre-commit validation suite locally. Add pre-commit hooks (via `lefthook` or `husky`) that run linting and type checking before push. | R-03 (CI pipeline) |
| R-33 | **Storybook for UI components** | Set up Storybook (or Histoire for Svelte 5) to document and visually test reusable components: VirtualList, MonitorRow, MonitorForm, HistoryChart, Toast, ConnectionBadge, BrandLockup, ThemeSwitcher. Enables isolated development and visual regression testing. | R-10 (E2E test infra) |

---

### Roadmap Summary

| Timeframe | Architecture | Scalability | Observability | Security | Developer Experience |
|-----------|:---:|:---:|:---:|:---:|:---:|
| **Short-term** (1–3 mo) | R-01, R-02, R-03, R-04 | — | — | R-05, R-06, R-07 | R-08, R-09 |
| **Medium-term** (3–6 mo) | R-10, R-11, R-12 | R-13, R-14, R-15 | R-16, R-17 | — | R-18, R-19, R-20 |
| **Long-term** (6–12 mo) | — | R-21, R-22, R-23, R-24 | R-25, R-26, R-27 | R-28, R-29, R-30 | R-31, R-32, R-33 |

### Dependency Chain (Critical Path)

```
R-03 (CI) ──→ R-09 (bundle guard)
         ──→ R-10 (E2E tests) ──→ R-20 (coverage) ──→ R-33 (Storybook)
         ──→ R-30 (secret scanning)
         ──→ R-32 (workflow automation)

R-01 (CSP) ──→ R-04 (token rotation) ──→ R-12 (WS auth refactor)
                                      ──→ R-29 (RBAC)

R-13 (pool tuning) ──→ R-21 (horizontal scaling) ──→ R-24 (event bus)
                   ──→ R-22 (read replicas)

R-16 (structured logging) ──→ R-25 (tracing) ──→ R-28 (audit logging)
R-17 (self-health) ──→ R-26 (self-alerting) ──→ R-27 (dashboard)
```

### Prioritization Guidance

**Highest impact, lowest effort** (start here):
1. **R-03** — CI/CD pipeline unlocks automated quality gates for everything else
2. **R-05** — Rate limiting is a single middleware addition protecting against brute-force
3. **R-02** — Graceful shutdown is a focused refactor of existing code
4. **R-08** — Error boundaries are a few Svelte files preventing white-screen failures

**Highest strategic value** (invest early):
1. **R-01 + R-04** — CSP + token rotation fundamentally improves the security posture
2. **R-16** — Structured logging is prerequisite for tracing, audit logging, and debuggability
3. **R-21** — Horizontal scaling removes the single-instance ceiling for large deployments
