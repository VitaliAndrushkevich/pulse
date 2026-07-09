## Improvements Catalog

This section consolidates all actionable findings into a prioritized remediation plan. Items are ordered by priority score (ascending — highest priority first), grouped into quick wins, strategic improvements, and the full catalog. Informational findings (Priority 16) are excluded — they require no action.

---

### Quick Wins

Items with **Security or Reliability impact** and **Small effort** (hours). These deliver the highest risk reduction per hour invested.

| # | Priority | ID(s) | Title | Component | Impact |
|---|----------|--------|-------|-----------|--------|
| 1 | 5 | PERF-020 | Right-size database connection pool for worker count | Backend / pgx pool | Reliability |
| 2 | 5 | QUAL-020 | Add graceful HTTP shutdown with drain timeout | Backend / main.go | Reliability |
| 3 | 9 | SEC-013 | Redact WebSocket token from request path logs | Backend / log sanitizer | Security |
| 4 | 9 | QUAL-001 | Add SvelteKit error boundary pages (+error.svelte) | Frontend / routes | Reliability |
| 5 | 9 | QUAL-029 | Fix shutdown sequence: drain HTTP before stopping services | Backend / main.go | Reliability |
| 6 | 10 | SEC-007 | Restrict WebSocket upgrader origins in production | Backend / WS handler | Security |
| 7 | 10 | SEC-008 | Return uniform error on WS auth failure (no info disclosure) | Backend / WS handler | Security |
| 8 | 10 | SEC-012 | Replace log.Fatalf in key rotation with proper error return | Backend / crypto | Security |
| 9 | 10 | PERF-022 | Use TimescaleDB native retention policy instead of DELETE subquery | Backend / timescale store | Reliability |
| 10 | 10 | QUAL-021 | Propagate parent context to notification dispatcher workers | Backend / notification | Reliability |
| 11 | 10 | QUAL-023 | Fix data race in Hub.ClientCount() with Run() loop | Backend / hub | Reliability |

**Estimated total effort:** ~16–24 hours for all 11 items.

---

### Strategic Improvements

Items with **Security or Reliability impact** and **Large effort** (weeks). These require architectural changes but address fundamental security gaps.

| # | Priority | ID(s) | Title | Component | Impact |
|---|----------|--------|-------|-----------|--------|
| 1 | 9 | SEC-004 | Implement token refresh/rotation mechanism | Backend + Frontend / auth | Security |
| 2 | 11 | SEC-006 | Migrate JWT from localStorage to httpOnly cookies with CSP | Frontend + Backend / auth | Security |

**Estimated total effort:** 2–4 weeks across both items.

**SEC-004** is the higher priority — a 24-hour static session token with no refresh mechanism is the single largest authentication risk. SEC-006 provides defense-in-depth but requires coordinated backend cookie support and CSP policy implementation.

---

### Full Improvements Catalog

All actionable findings consolidated as remediation items, ordered by priority score.

#### Priority 5 — High Risk / Almost Certain

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 1 | SEC-011 | Cover notification encrypted data in key rotation | Key rotation re-encrypts secrets and credentials but does not re-encrypt notification channel encrypted fields (SMTP password, webhook headers). A key compromise leaves notification secrets readable. | Backend / crypto + notification | Medium | Security | 5 |
| 2 | PERF-020 | Right-size database connection pool | pgx defaults (~4 connections) are undersized relative to PULSE_SCHEDULER_WORKERS (200). Under load, workers block waiting for connections, causing check timeouts. | Backend / pgx pool config | Small | Reliability | 5 |
| 3 | QUAL-020 | Add graceful HTTP shutdown | HTTP server does not call `Shutdown()` with a drain timeout — SIGTERM immediately kills in-flight requests. Clients see connection resets during rolling deploys. | Backend / main.go | Small | Reliability | 5 |

#### Priority 9 — Medium Risk / Almost Certain

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 4 | SEC-004 | Implement token refresh/rotation | 24-hour static JWT with no refresh. A stolen token is valid for the full session lifetime. Rotation reduces exposure window to minutes. | Backend + Frontend / auth | Large | Security | 9 |
| 5 | SEC-013 | Redact WebSocket token from logs | SanitizeLog does not match the `/ws?token=<value>` pattern. Token appears in access logs, violating defense-in-depth for credential protection. | Backend / log sanitizer | Small | Security | 9 |
| 6 | QUAL-001 | Add SvelteKit error boundary pages | Missing `+error.svelte` at route group and root levels. Unhandled errors show the default SvelteKit error page with no branding or recovery path. | Frontend / routes | Small | Reliability | 9 |
| 7 | QUAL-029 | Fix shutdown sequence ordering | Services are stopped before the HTTP server drains. In-flight requests that depend on already-stopped services (DB, hub) fail with connection errors. | Backend / main.go | Small | Reliability | 9 |

#### Priority 10 — Medium Risk / Possible

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 8 | SEC-001 | Move JWT to httpOnly cookie or add XSS mitigations | JWT in localStorage is accessible to any XSS payload. Moving to httpOnly cookies removes client-side access. | Frontend / auth store | Medium | Security | 10 |
| 9 | SEC-002 | Suppress WS token from server access logs | WebSocket URL with `?token=` is logged by reverse proxies and Go's default request logger. Add path rewriting or log filtering. | Backend / WS handler | Medium | Security | 10 |
| 10 | SEC-007 | Restrict WebSocket origin in production | `websocket.Upgrader.CheckOrigin` returns true for all origins. Enables cross-site WebSocket hijacking in production. | Backend / WS handler | Small | Security | 10 |
| 11 | SEC-008 | Uniform WS auth error responses | WS returns different messages for "missing token" vs "invalid token", enabling attackers to enumerate valid token formats. | Backend / WS handler | Small | Security | 10 |
| 12 | SEC-012 | Fix log.Fatalf in key rotation | `log.Fatalf` calls `os.Exit(1)` before deferred transaction rollback executes. Partial re-encryption may leave data in an inconsistent state. | Backend / crypto | Small | Security | 10 |
| 13 | PERF-010 | Add request deduplication for concurrent identical fetches | Multiple components firing the same API call simultaneously create redundant network traffic. A shared-promise pattern deduplicates in-flight requests. | Frontend / api.ts | Medium | Performance | 10 |
| 14 | PERF-011 | Wire AbortController on component unmount | Pending fetch requests are not cancelled when components unmount or navigation occurs, leading to stale state updates and wasted bandwidth. | Frontend / api.ts + routes | Medium | Performance | 10 |
| 15 | PERF-021 | Fix N+1 query in notification fan-out | Scheduler evaluates notifications per-monitor with individual DB queries for bindings. Batch loading bindings eliminates the N+1 pattern. | Backend / notification fanout | Medium | Performance | 10 |
| 16 | PERF-022 | Use TimescaleDB native retention policy | Custom `DELETE FROM check_results WHERE ...` subquery is slower and less reliable than TimescaleDB's built-in `drop_chunks()` retention policy. | Backend / timescale store | Small | Reliability | 10 |
| 17 | QUAL-005 | Wrap remaining hardcoded strings with t() | Several user-visible strings bypass the i18n `t()` function, breaking localization for non-English users. | Frontend / components | Medium | Quality | 10 |
| 18 | QUAL-007 | Add test coverage for notification store and routes | Notification module has 0% test coverage. Critical delivery logic (fan-out, retry, state tracking) lacks regression protection. | Backend + Frontend / notifications | Medium | Quality | 10 |
| 19 | QUAL-021 | Propagate parent context to dispatcher workers | Workers use `context.Background()`, ignoring shutdown signals. In-flight deliveries may run indefinitely after SIGTERM. | Backend / notification | Small | Reliability | 10 |
| 20 | QUAL-023 | Fix Hub.ClientCount() data race | `ClientCount()` reads the clients map without synchronization while `Run()` modifies it. Race detector flags this under concurrent access. | Backend / hub | Small | Reliability | 10 |

#### Priority 11 — Medium Risk / Possible (lower likelihood)

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 21 | SEC-006 | Implement CSP and migrate to httpOnly cookies | Defense-in-depth gap: no Content-Security-Policy header limits XSS blast radius. Combined with localStorage JWT, a single XSS compromises the session. | Frontend + Backend | Large | Security | 11 |
| 22 | PERF-002 | Use stable item identity in VirtualList keys | Index-based keying causes unnecessary DOM recycling when items are inserted or removed mid-list. Use monitor UUID as key. | Frontend / VirtualList | Small | Performance | 11 |
| 23 | PERF-006 | Migrate HistoryChart to reactive data pattern | HistoryChart uses legacy `onMount` API without reacting to prop changes. Data updates require manual chart recreation. | Frontend / HistoryChart | Small | Performance | 11 |
| 24 | PERF-014 | Set immutable Cache-Control on static font assets | Font files served with `no-cache` force revalidation on every page load. Content-hashed fonts should use `max-age=31536000, immutable`. | Frontend / static config | Small | Performance | 11 |
| 25 | QUAL-022 | Propagate per-check context to ICMP resolver | ICMP resolver uses `context.Background()` instead of the per-check timeout context. Checks may hang indefinitely on DNS resolution. | Backend / ICMP checker | Small | Reliability | 11 |

#### Priority 13 — Low Risk / Almost Certain

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 26 | PERF-008 | Cache computed styles in HistoryChart | `getComputedStyle()` called on every `createChart` invocation forces style recalculation. Cache theme colors and invalidate only on theme change. | Frontend / HistoryChart | Small | Performance | 13 |
| 27 | PERF-013-B | Enable precompress in static adapter | `precompress: false` means the server must compress on-the-fly. Pre-built gzip/brotli assets reduce TTFB and server CPU. | Frontend / svelte.config | Small | Performance | 13 |
| 28 | QUAL-004 | Replace Tailwind dark: prefix with data-theme selector | Several components use `dark:` prefix violating the project's `[data-theme="dark"]` convention. Causes inconsistent theme behavior. | Frontend / components | Small | Quality | 13 |
| 29 | QUAL-009 | Add error reporting in auth store catch blocks | Silent catch blocks swallow authentication errors. Users see no feedback when login/refresh fails silently. | Frontend / auth store | Small | Quality | 13 |

#### Priority 14 — Low Risk / Likely or Unlikely

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 30 | SEC-003 | Add client-side JWT expiry check | Frontend uses token until server rejects it. A pre-flight expiry check prevents unnecessary failed requests and improves UX. | Frontend / auth store | Small | Security | 14 |
| 31 | SEC-005 | Suppress source maps in production build | No explicit `sourcemap: false` in vite config. If source maps are generated, they expose application logic to attackers. | Frontend / vite.config | Small | Security | 14 |
| 32 | SEC-009 | Add dummy bcrypt on empty Authorization header | Combined auth skips bcrypt comparison when Authorization header is empty, creating a timing difference detectable by attackers. | Backend / auth middleware | Small | Security | 14 |
| 33 | SEC-010 | Sanitize server-generated chart tooltip content | `{@html}` renders tooltip content from server data. While currently safe (server-generated), a future API change could introduce stored XSS. | Frontend / HistoryChart | Small | Security | 14 |
| 34 | SEC-014 | Unify WS auth with combinedAuth middleware | WS auth duplicates the combined auth logic independently. Divergence risk: fixes to REST auth may not propagate to WS path. | Backend / WS handler | Medium | Security | 14 |
| 35 | SEC-015 | Route proto source handler through sqlc layer | Proto source handler uses direct `pool.Query()` bypassing the sqlc abstraction. Reduces consistency guarantees and auditability. | Backend / monitor | Small | Quality | 14 |
| 36 | PERF-005 | Reduce route node 5 bundle size | Route node at 39 KB gzipped exceeds the 20 KB threshold. Consider code-splitting heavy components or lazy-loading sub-routes. | Frontend / routes | Medium | Performance | 14 |
| 37 | PERF-012 | Deduplicate reconnect refetch requests | WebSocket reconnection triggers multiple simultaneous monitor fetches. Consolidate into a single fetch with shared promise. | Frontend / WS + stores | Small | Performance | 14 |
| 38 | PERF-013 | Add bundle size CI guard | No automated check prevents bundle size regressions. A CI step comparing against size budgets catches growth early. | Frontend / CI | Small | Quality | 14 |
| 39 | PERF-023 | Fix Hub ClientCount() mutex consistency | ClientCount() reads a counter not atomically updated by the Run() loop, creating stale-read potential under concurrency. | Backend / hub | Small | Reliability | 14 |
| 40 | PERF-024 | Pre-allocate Prometheus labels in scheduler | Scheduler rebuilds label strings on every tick. Pre-allocate label sets at registration time for zero-alloc metric updates. | Backend / scheduler | Small | Performance | 14 |
| 41 | QUAL-002 | Add type guards for WebSocket message assertions | Unchecked type assertions in WS message handler may panic on malformed messages in unexpected states. | Backend / hub messages | Small | Quality | 14 |
| 42 | QUAL-006 | Use CSS custom properties for semantic colors | Hardcoded Tailwind color classes bypass the theme system. Use `var(--color-*)` tokens for consistent theme switching. | Frontend / components | Medium | Quality | 14 |
| 43 | QUAL-008 | Separate data fetching from presentation components | Several route components mix `fetch` calls with rendering logic, making them harder to test and reuse. | Frontend / routes | Medium | Quality | 14 |
| 44 | QUAL-011 | Add keyboard navigation to tab components | Tab widgets lack `role="tablist"` and arrow key support, failing WCAG 2.1 AA keyboard accessibility requirements. | Frontend / components | Small | Quality | 14 |
| 45 | QUAL-024 | Fix notification processJob half-handling | Error from processJob is logged but the job is also marked as failed — double handling pattern. Choose one error path. | Backend / notification | Small | Quality | 14 |
| 46 | QUAL-025 | Route timescale/retention queries through sqlc | Direct SQL queries in timescale and retention stores bypass the sqlc layer, reducing type safety and query auditability. | Backend / store | Medium | Quality | 14 |
| 47 | QUAL-026 | Propagate context to proto registry compilation | Proto registry uses `context.Background()` for compilation. Should accept the server startup context for cancellation support. | Backend / monitor | Small | Quality | 14 |
| 48 | QUAL-027 | Use %w verb in reflect.go error wrapping | Missing `%w` verb breaks `errors.Is`/`errors.As` chains, making error inspection unreliable for callers. | Backend / monitor | Small | Quality | 14 |

#### Priority 15 — Low Risk / Possible

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 49 | SEC-WS-LEN | Enforce maximum token length on WS query parameter | No length limit on the token query parameter. Extremely long values could cause memory allocation issues in URL parsing. | Backend / WS handler | Small | Security | 15 |
| 50 | PERF-WS-TAB | Add cross-tab WebSocket coordination | Multiple browser tabs each maintain independent WS connections and reconnection timers. A SharedWorker or BroadcastChannel could consolidate. | Frontend / WS client | Medium | Performance | 15 |
| 51 | QUAL-028 | Make Hub.Run() goroutine context-aware | Hub.Run() uses an infinite loop without selecting on context cancellation. Shutdown relies on channel close rather than context propagation. | Backend / hub | Small | Reliability | 15 |

---

### Dependency Upgrade Recommendations

#### Frontend Dependencies

| Package | Current | Latest Stable | Status | Recommendation |
|---------|---------|---------------|--------|----------------|
| `vite` | ^5.4.10 | 6.x | 1 major behind | Upgrade to Vite 6. Vite 5 is in maintenance mode; Vite 6 brings improved dev server performance and ESM-only output. No known CVEs in Vite 5, but active security patches target Vite 6 only. |
| `vitest` | ^2.1.8 | 3.x | 1 major behind | Upgrade to Vitest 3. Aligns with Vite 6, includes improved snapshot isolation and workspace support. |
| `svelte` | ^5.0.0 | 5.x (latest) | Current | No action needed. |
| `@sveltejs/kit` | ^2.20.0 | 2.x (latest) | Current | No action needed. |
| `tailwindcss` | ^3.4.14 | 4.x | 1 major behind | Evaluate Tailwind CSS 4 migration when ecosystem support stabilizes. Breaking changes in config format and plugin API. Non-urgent — v3 is still maintained. |

#### Backend Dependencies

| Package | Current | Latest Stable | Status | Recommendation |
|---------|---------|---------------|--------|----------------|
| `github.com/gin-gonic/gin` | v1.12.0 | v1.12.x | Current | No action needed. |
| `github.com/jackc/pgx/v5` | v5.9.2 | v5.x (latest) | Current | No action needed. |
| `github.com/gorilla/websocket` | v1.5.3 | v1.5.x | Current | No action needed. Note: gorilla/websocket is in maintenance mode. Consider `nhooyr.io/websocket` or `coder/websocket` for active development, but no urgency. |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | v4.x | Current | No action needed. |
| `golang.org/x/crypto` | v0.48.0 | Latest | Current | No action needed. Keep updated — this package frequently receives CVE patches. |
| `github.com/quic-go/quic-go` | v0.59.0 | v0.x | Current | No action needed. Pre-v1 — track releases for breaking changes. |
| `go` | 1.25.0 | 1.25.x | Current | No action needed. |

#### Known CVE Notes

No critical CVEs were identified in the current dependency set at the time of this audit. The project uses recent versions of all major dependencies. Key observations:

1. **golang.org/x/crypto** — frequently patched; ensure automated dependency updates (Dependabot/Renovate) are configured to catch security releases within 48 hours.
2. **gorilla/websocket** — archived/maintenance-only as of 2024. No new CVE patches will be issued. Current version (v1.5.3) has no known vulnerabilities, but any future discovery will not be patched upstream.
3. **Vite 5** — no known CVEs, but security patches are now prioritized for Vite 6. The maintenance window for Vite 5 is limited.

**Recommendation:** Configure Dependabot or Renovate for both `frontend/package.json` and `backend/go.mod` with weekly security-only update checks. This provides automated CVE detection without manual version tracking.

---

### Summary

| Category | Count |
|----------|-------|
| Quick Wins (Security/Reliability + Small effort) | 11 |
| Strategic Improvements (Security/Reliability + Large effort) | 2 |
| Total actionable improvements | 51 |
| Dependency upgrades recommended | 3 (Vite 6, Vitest 3, Tailwind 4 evaluation) |

**Recommended execution order:**
1. **Quick Wins** (1–2 sprints) — address all 11 items for immediate risk reduction
2. **Priority 10 Medium-effort items** (2–3 sprints) — SEC-001, PERF-021, QUAL-007
3. **Strategic: SEC-004 token rotation** (dedicated sprint) — largest auth risk
4. **Strategic: SEC-006 CSP + httpOnly** (dedicated sprint) — defense-in-depth completion
5. **Remaining Priority 13–15 items** — backlog, address opportunistically
