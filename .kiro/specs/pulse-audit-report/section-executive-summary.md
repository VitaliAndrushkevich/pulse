## Executive Summary

This audit evaluated the Pulse uptime monitoring platform across three domains — security, performance, and code quality — covering both the SvelteKit 5 frontend (primary focus) and the Go backend (secondary focus).

### Finding Summary

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 3 |
| Medium | 22 |
| Low | 28 |
| Informational | 17 |
| **Total** | **70** |

Of the 70 total findings, **53 are actionable** (require remediation). The remaining 17 are informational observations confirming correct behavior or noting areas for future consideration.

### Distribution by Domain

| Domain | High | Medium | Low | Informational | Total |
|--------|------|--------|-----|---------------|-------|
| Security | 1 | 8 | 9 | 2 | 20 |
| Performance | 1 | 8 | 10 | 9 | 28 |
| Code Quality | 1 | 6 | 9 | 3 | 19 |

### Overall Risk Posture: **Medium**

No critical vulnerabilities were identified. The three High-severity findings are all exploitable under production conditions (Almost Certain likelihood) but do not represent immediate data breach or unauthorized access risks. The platform demonstrates sound security fundamentals — encrypted secrets at rest, parameterized queries, proper authentication coverage — with gaps concentrated in operational resilience and defense-in-depth measures.

### Top-3 Priority Items Requiring Immediate Attention

| # | ID | Severity | Domain | Title | Effort |
|---|-----|----------|--------|-------|--------|
| 1 | SEC-011 | High | Security | Key rotation does not cover notification channel encrypted data | Medium |
| 2 | PERF-020 | High | Performance | Database connection pool undersized for worker count | Small |
| 3 | QUAL-020 | High | Reliability | HTTP server lacks graceful shutdown — in-flight requests aborted on SIGTERM | Small |

**SEC-011** represents an incomplete encryption key rotation — notification channel credentials (SMTP passwords, webhook headers) are not re-encrypted during `make rotate-key`, leaving them encrypted with the old key and unreadable after rotation completes.

**PERF-020** uses pgx pool defaults (4 connections) against a scheduler configured for 200 concurrent workers, creating connection contention under load that will degrade check scheduling reliability.

**QUAL-020** calls `server.Close()` on SIGTERM instead of `server.Shutdown(ctx)`, immediately terminating all in-flight HTTP requests and WebSocket connections without draining — causing data loss for any active API operation or real-time client.

---

## Methodology

### Scope

| Layer | Coverage | Path |
|-------|----------|------|
| Frontend (primary) | SvelteKit 5 application — components, routes, API client, WebSocket client, stores, locales | `frontend/src/` |
| Backend (secondary) | Go binary — API handlers, scheduler, WebSocket hub, notification dispatcher, crypto, data layer | `backend/internal/` |
| Configuration | Docker Compose, environment variables, Dockerfile | `docker-compose*.yml`, `.env.example`, `Dockerfile` |
| Dependencies | Frontend packages, Go modules | `package.json`, `go.mod` |
| API Contract | REST endpoint surface | `backend/api/openapi.yaml` |

### Techniques

| Technique | Purpose | Application |
|-----------|---------|-------------|
| Static code analysis | Identify dangerous patterns, type safety issues, error handling gaps | Full codebase |
| Pattern grep | Find `{@html}`, `any` types, `console.log`, hardcoded secrets, raw SQL | Frontend + Backend |
| Data flow tracing | Trace token lifecycle, secret exposure paths, input-to-render flows | Auth, crypto, API client |
| Architecture review | Evaluate design decisions against requirements and scale targets | System-wide |
| Dependency version audit | Identify outdated packages and known vulnerabilities | `package.json`, `go.mod` |
| Convention compliance | Compare code against AGENTS.md project rules | Full codebase |
| AST-level code reading | Verify type safety, error propagation, resource cleanup | Frontend + Backend |

### Tools

- File traversal and directory structure mapping
- Regex pattern search across source files
- AST-level code reading with function/class extraction
- Line-level file inspection for evidence collection

### Scope Limitations

The following were **not** performed due to the read-only, static nature of this audit:

- **No dynamic testing** — no runtime profiling, load testing, or penetration testing was executed
- **No production environment access** — findings about runtime behavior (frame rates, query latency, memory growth) are assessed from code structure and configuration, not measured
- **No dependency vulnerability scanning** — CVE assessment is based on version comparison against known advisories, not automated tooling output (e.g., `npm audit`, `govulncheck`)
- **No browser testing** — accessibility findings are based on code inspection of ARIA attributes and semantic HTML, not assistive technology validation

### Exclusions

The following areas were explicitly excluded from this audit:

- **Third-party library internals** — only their usage patterns are audited, not source code
- **MCP server** (`mcp/`) — separate binary with independent lifecycle and deployment
- **Infrastructure beyond Docker Compose** — no cloud provider, Kubernetes, or CI/CD pipeline review
- **Test file quality** — test files were reviewed for coverage gaps but not audited for correctness or best practices
- **Generated code** — `sqlc` output in `backend/internal/store/postgres/` was reviewed for usage patterns only
