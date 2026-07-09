# Pulse Security and Performance Audit

## Table of Contents

- [1. Executive Summary](#1-executive-summary)
- [2. Methodology](#2-methodology)
- [3. Security Findings](#3-security-findings)
  - [SEC-001: JWT Stored in localStorage](#sec-001-jwt-stored-in-localstorage--vulnerable-to-xss-exfiltration)
  - [SEC-002: WebSocket Token in URL Query Parameter](#sec-002-websocket-token-transmitted-via-url-query-parameter--log-and-history-exposure)
  - [SEC-003: No Client-Side JWT Expiry Validation](#sec-003-no-client-side-jwt-expiry-validation--stale-token-used-until-server-rejection)
  - [SEC-004: No Token Refresh or Rotation](#sec-004-no-token-refresh-or-rotation-mechanism--24-hour-static-session)
  - [SEC-005: No Explicit Source Map Suppression](#sec-005-no-explicit-source-map-suppression-in-production-build-configuration)
  - [SEC-006: JWT in localStorage with CSP Gap](#sec-006-jwt-in-localstorage--accepted-risk-with-csp-defense-in-depth-gap)
  - [SEC-007: WebSocket Upgrader Allows All Origins](#sec-007-websocket-upgrader-allows-all-origins-no-production-restriction)
  - [SEC-008: WS Auth Information Disclosure](#sec-008-websocket-auth-failure-reveals-missing-vs-invalid-token-information-disclosure)
  - [SEC-009: Combined Auth Missing Dummy Bcrypt](#sec-009-combined-auth-missing-dummy-bcrypt-on-empty-authorization-header)
  - [SEC-010: {@html} Used for Chart Tooltip](#sec-010-html-used-for-chart-tooltip-content)
  - [SEC-011: Key Rotation Missing Notification Data](#sec-011-key-rotation-does-not-cover-notification-channel-encrypted-data)
  - [SEC-012: Key Rotation log.Fatalf Issue](#sec-012-key-rotation-uses-logfatalf--deferred-transaction-rollback-never-executes)
  - [SEC-013: Log Sanitization Gap](#sec-013-log-sanitization-does-not-redact-websocket-token-from-request-path)
  - [SEC-014: WS Auth Duplicates combinedAuth](#sec-014-websocket-auth-not-using-shared-combinedauth-middleware-code-duplication-risk)
  - [SEC-015: Proto Source Handler Bypasses sqlc](#sec-015-proto-source-handler-bypasses-sqlc-layer-with-direct-pool-query)
- [4. Performance Findings](#4-performance-findings)
  - [PERF-001 through PERF-028](#perf-001-virtuallist-dom-node-cap-correctly-enforced)
- [5. Code Quality Findings](#5-code-quality-findings)
  - [QUAL-001 through QUAL-029](#qual-001-missing-sveltekit-error-boundary-pages)
- [6. Improvements Catalog](#6-improvements-catalog)
- [7. Future Roadmap](#7-future-roadmap)
- [8. Appendices](#8-appendices)
  - [8.1 Requirements Coverage Matrix](#81-requirements-coverage-matrix)
  - [8.2 Finding ID Index](#82-finding-id-index)

---

## 1. Executive Summary

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

## 2. Methodology

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

---

## 3. Security Findings


### SEC-001: JWT Stored in localStorage — Vulnerable to XSS Exfiltration

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Token Storage |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The JWT authentication token is stored in `localStorage` under the key `pulse_jwt`. localStorage is accessible to any JavaScript executing in the page context, including injected scripts via XSS. If an attacker achieves script execution, they can trivially exfiltrate the token with `localStorage.getItem('pulse_jwt')` and use it from any location until expiry. Classified as Medium because Svelte's default escaping provides strong XSS protection and this is a self-hosted single-admin platform, reducing the attack surface.

**Evidence:**
`frontend/src/lib/stores/auth.svelte.ts:16-30`
```typescript
const STORAGE_KEY = 'pulse_jwt';

function readTokenFromStorage(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeTokenToStorage(token: string): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Silently fail — storage quota or access issue
  }
}
```

**Impact:** If XSS is achieved, attacker gains full session takeover with a token valid for up to 24 hours. Token can be used from external origins since there's no binding to browser fingerprint or IP.

**Remediation:** Migrate token storage to an HTTP-only, Secure, SameSite=Strict cookie set by the backend on successful login. This removes the token from JavaScript-accessible storage entirely. The security property restored is that client-side script execution cannot exfiltrate session credentials.

---

### SEC-002: WebSocket Token Transmitted via URL Query Parameter — Log and History Exposure

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Token Transmission |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The WebSocket connection transmits the JWT via URL query parameter (`/ws?token=<jwt>`). While standard for browser WebSocket authentication, this introduces exposure vectors: server access logs record the full request path including the raw token, browser developer tools show the URL, and intermediate proxies may log it.

**Evidence:**
`frontend/src/lib/ws.ts:210-215`
```typescript
function buildWsUrl(basePath: string, token: string): string {
  if (basePath.startsWith('ws://') || basePath.startsWith('wss://')) {
    const separator = basePath.includes('?') ? '&' : '?';
    return `${basePath}${separator}token=${encodeURIComponent(token)}`;
  }
  // ...
}
```

`backend/internal/api/middleware/logging.go:37-42`
```go
return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s\n",
    param.TimeStamp.Format(time.RFC3339),
    param.StatusCode,
    param.Latency,
    param.Method,
    param.Path, // <-- includes ?token=<jwt> for WS requests
)
```

**Impact:** JWT tokens are written to server log files in plaintext. Anyone with log access gains full session tokens. For a self-hosted platform, this expands the attack surface beyond the application itself.

**Remediation:** Modify `SanitizedLogger` to redact the `token` query parameter from `param.Path` before logging. Alternatively, consider a short-lived single-use ticket exchange pattern. The security property restored is that long-lived credentials are not persisted in log storage.

---

### SEC-003: No Client-Side JWT Expiry Validation — Stale Token Used Until Server Rejection

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Token Validation |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The frontend performs zero client-side JWT validation. There is no decoding of the JWT payload, no check of the `exp` claim, and no proactive session expiry handling. The only expiry detection occurs when a REST API call returns 401 or the WS server sends close code 4401. An expired token remains in localStorage until the user's next API request fails.

**Evidence:**
`frontend/src/lib/stores/auth.svelte.ts:51-54`
```typescript
export function getToken(): string | null {
  return token;
}
```

`frontend/src/lib/api.ts:125-128`
```typescript
const token = getToken();
if (token) {
  headers['Authorization'] = `Bearer ${token}`;
}
```

**Impact:** Degraded user experience — users may work in the UI for minutes after their token has expired, only to encounter errors on their next action. No security vulnerability since the server correctly validates expiry.

**Remediation:** Add a lightweight client-side expiry check: decode the JWT payload (base64url without signature verification), extract the `exp` claim, and compare against `Date.now()/1000`. The security property restored is defense-in-depth (expired tokens are never transmitted).

---

### SEC-004: No Token Refresh or Rotation Mechanism — 24-Hour Static Session

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Session Management |
| **Effort** | Large (weeks) |
| **Priority** | 9 |

**Description:** The system has no token refresh, rotation, or sliding window mechanism. A JWT with 24-hour expiry is issued at login and used for all requests until expiry forces re-authentication. A stolen token remains valid for up to 24 hours with no mechanism to detect or revoke it (JWTs are stateless). Per requirements, session lifetime exceeding 8 hours without re-validation is classified as Medium risk.

**Evidence:**
`.env.example:75-77`
```bash
# JWT token expiry duration (Go duration format)
# Examples: 24h, 12h, 1h30m
PULSE_JWT_EXPIRY=24h
```

No refresh endpoint exists in `frontend/src/lib/api.ts` — searched for "refresh", "rotate", "renew" with zero results.

**Impact:** Extended window of exploitation for stolen tokens. No ability to revoke individual JWT sessions without changing the signing secret (which invalidates ALL sessions). Combined with localStorage storage (SEC-001), a single XSS gives 24-hour access with no server-side kill switch.

**Remediation:** Implement a refresh token mechanism: issue short-lived access tokens (15 min) with a long-lived refresh token stored in an HTTP-only cookie. Add a `POST /auth/refresh` endpoint. The security properties restored are: reduced exploitation window, server-side session revocation, and token theft detection via refresh token reuse.

---

### SEC-005: No Explicit Source Map Suppression in Production Build Configuration

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Data Exposure — Source Maps |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The Vite production build relies on Vite 5's default behavior (`build.sourcemap: false`) to exclude source maps. This is implicit — there is no explicit `build: { sourcemap: false }` in `vite.config.ts`. If overridden by a plugin or environment variable, source maps would be served to all users without server-side access control.

**Evidence:**
`frontend/vite.config.ts:1-12`
```typescript
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    proxy: {
      '/api': process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
      '/ws': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
        ws: true,
      },
    },
  },
});
```

**Impact:** If source maps were accidentally included, any user could reconstruct the full original TypeScript source, revealing business logic and potential vulnerabilities.

**Remediation:** Add explicit `build: { sourcemap: false }` to `vite.config.ts`. Optionally add a post-build check that verifies no `.map` files exist in the output directory. The security property restored is explicit prevention of source code exposure.

---

### SEC-006: JWT in localStorage — Accepted Risk with CSP Defense-in-Depth Gap

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Data Exposure — Token Storage |
| **Effort** | Large (weeks) |
| **Priority** | 11 |

**Description:** The auth store persists the JWT in `localStorage`. While this is a common SPA pattern, `localStorage` is accessible to any JavaScript running in the same origin. The application does NOT set a Content Security Policy (CSP) header, which would provide defense-in-depth against XSS-based token theft. This is a well-understood tradeoff for static SPA applications that cannot use httpOnly cookies without a server-side runtime.

**Evidence:**
`frontend/src/lib/stores/auth.svelte.ts:15-23`
```typescript
const STORAGE_KEY = 'pulse_jwt';

function readTokenFromStorage(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}
```

**Impact:** In the event of an XSS vulnerability, an attacker could read the JWT from localStorage and impersonate the user against the API.

**Remediation:** Implement a strict Content Security Policy (CSP) to prevent inline scripts and limit script-src to known origins. Long-term: if a BFF proxy is introduced, migrate to httpOnly cookie-based sessions. The security property restored is defense-in-depth limiting XSS blast radius.

---

### SEC-007: WebSocket Upgrader Allows All Origins (No Production Restriction)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Cross-Origin WebSocket Hijacking |
| **Effort** | Small (hours) |
| **Priority** | 10 |
| **Status** | ✅ Fixed |

**Description:** The WebSocket upgrader has `CheckOrigin` configured to always return `true` with no environment-based conditional. This permissive configuration applies to both development and production deployments. Per requirement 4.7, the WebSocket upgrade endpoint should validate the Origin header against a configured allowlist in non-development mode.

**Evidence:**
`backend/internal/api/handlers/ws.go:22-28`
```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}
```

**Impact:** Any domain can establish a WebSocket connection to Pulse if they possess a valid token. Combined with token leakage vectors (SEC-002 log exposure), this creates a viable attack path for monitoring data exfiltration.

**Remediation:** Implement origin validation using `PULSE_BASE_URL`. In production, `CheckOrigin` should validate the `Origin` header matches the configured base URL. In dev mode, allow all origins. The security property restored is defense-in-depth against cross-origin WebSocket abuse.

**Resolution:** `NewWSHandler` now accepts `baseURL` and `devMode` parameters. In production with `PULSE_BASE_URL` set, `CheckOrigin` validates the Origin header against the configured base URL. Dev mode and non-browser clients (no Origin header) remain unrestricted.

---

### SEC-008: WebSocket Auth Failure Reveals Missing vs Invalid Token (Information Disclosure)

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Information Disclosure |
| **Effort** | Small (hours) |
| **Priority** | 10 |
| **Status** | ✅ Fixed |

**Description:** The WebSocket handler returns two distinct error messages: `"missing token query parameter"` for empty tokens and `"invalid or expired token"` for invalid ones. This violates requirement 4.4 which mandates identical error responses. Additionally, the empty-token path does NOT perform a dummy bcrypt comparison, creating a measurable timing difference (~200ms).

**Evidence:**
`backend/internal/api/handlers/ws.go:52-73`
```go
func (wh *WSHandler) Handle(c *gin.Context) {
    rawToken := c.Query("token")
    if rawToken == "" {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": gin.H{
                "code":    "UNAUTHORIZED",
                "message": "missing token query parameter",
            },
        })
        return  // No dummy bcrypt compare
    }

    if !wh.validateToken(c, rawToken) {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": gin.H{
                "code":    "UNAUTHORIZED",
                "message": "invalid or expired token",
            },
        })
        return
    }
```

**Impact:** Allows attackers to enumerate whether a WebSocket endpoint requires authentication and distinguish between "no token" and "bad token" states via timing.

**Remediation:** Unify error responses to always return `"invalid or expired token"` regardless of failure reason. Add a dummy bcrypt comparison on the empty-token path. The security property restored is uniform authentication failure responses.

**Resolution:** Both code paths now return identical `"invalid or expired token"` messages. The empty-token path performs a dummy bcrypt comparison before responding, eliminating the timing side-channel.

---

### SEC-009: Combined Auth Missing Dummy Bcrypt on Empty Authorization Header

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Timing Side-Channel |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `combinedAuth` middleware returns a 401 immediately when the `Authorization` header is missing without performing a dummy bcrypt comparison. This creates a timing difference between requests with no auth header (~0ms) and requests with an invalid token (~200ms). The standalone `BearerAuth` middleware correctly calls `failWithDummyCompare(c)`.

**Evidence:**
`backend/internal/api/router.go:157-161`
```go
func combinedAuth(queries *db.Queries, jwtSecret []byte) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
            unauthorized(c)  // No dummyCompare()
            return
        }
```

`backend/internal/api/middleware/auth.go:33-36`
```go
authHeader := c.GetHeader("Authorization")
if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
    failWithDummyCompare(c)  // Correct: timing-safe
    return
}
```

**Impact:** Low — an attacker can determine a request lacks authentication by measuring response time. Mostly informational since the 401 status already reveals this.

**Remediation:** Add `dummyCompare()` before `unauthorized(c)` on the empty/missing header path in `combinedAuth`. The security property restored is uniform response timing.

---

### SEC-010: {@html} Used for Chart Tooltip Content

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | XSS — {@html} usage |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `HistoryChartExplorer` component uses `{@html tooltipContent}` to render a tooltip built from chart data. The content originates from numeric data arrays (timestamps, latency numbers, state codes) — server-generated values, not user text input. The risk is minimal but the pattern is suboptimal — if future changes introduce user-controlled strings into tooltips, this would become a direct XSS vector.

**Evidence:**
`frontend/src/components/HistoryChartExplorer.svelte:450`
```svelte
{@html tooltipContent}
```

Tooltip built at lines 173-194:
```typescript
let content = `<div class="text-xs font-mono">${timeStr}</div>`;
content += `<div class="text-xs">Avg: ${avg != null ? formatLatency(avg) : 'N/A'}</div>`;
content += `<div class="text-xs">State: ${stateLabel(stateVal)}</div>`;
```

**Impact:** Currently safe since data sources are numeric/fixed-string. If future changes introduce user-controlled strings, this becomes a stored XSS vector.

**Remediation:** Replace `{@html tooltipContent}` with a Svelte component that renders fields via text interpolation. This eliminates the XSS vector regardless of future data source changes.

---

### SEC-011: Key Rotation Does Not Cover Notification Channel Encrypted Data

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Key Management — Incomplete Rotation |
| **Effort** | Medium (days) |
| **Priority** | 5 |

**Description:** The `cmd/rotate` command re-encrypts secrets and monitor credentials but does not rotate encrypted webhook header values (stored in `notification_channels.config` JSON) or the SMTP settings password (`smtp_settings.password_enc`). Both are encrypted using the same `PULSE_SECRET_KEY` via AES-256-GCM. After key rotation, all webhook deliveries and SMTP email sending will fail because the stored ciphertext cannot be decrypted with the new key.

**Evidence:**
`backend/cmd/rotate/main.go:69-140`
```go
// Only processes secrets and credentials:
secrets, err := queries.ListAllSecrets(ctx)
// ...
credentials, err := queries.ListAllCredentials(ctx)
// ...
// No handling of notification_channels or smtp_settings tables
```

Compared with encryption sites:
- `backend/internal/api/handlers/notification_channels.go:710` — `crypto.Encrypt(h.secretKey, []byte(header.Value))`
- `backend/internal/api/handlers/smtp_settings.go:101` — `crypto.Encrypt(h.secretKey, []byte(*req.Password))`

**Impact:** Key rotation will break the notification subsystem entirely. Webhook deliveries will fail with decryption errors, and SMTP notifications will stop. The operator will have no indication until notifications start failing during an actual downtime event.

**Remediation:** Extend the rotation command to re-encrypt notification_channels webhook header values and smtp_settings password_enc within the same transaction. The security property restored is complete key rotation without operational breakage.

---

### SEC-012: Key Rotation Uses log.Fatalf — Deferred Transaction Rollback Never Executes

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Key Management — Transaction Safety |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** The key rotation command uses `log.Fatalf` for all error paths during re-encryption. `log.Fatalf` calls `os.Exit(1)`, which terminates the process without running deferred functions. The `defer tx.Rollback(ctx)` at line 64 will NOT execute on failure. PostgreSQL will eventually roll back when the connection drops, but this creates a window where the transaction holds locks.

**Evidence:**
`backend/cmd/rotate/main.go:64-67,85`
```go
defer func() {
    _ = tx.Rollback(ctx)
}()
// ...
log.Fatalf("secret %s: failed to decrypt with old key: %v", s.ID, err)
// ^ os.Exit(1) — deferred rollback never runs
```

**Impact:** On failure, table rows remain locked until PostgreSQL detects the dead connection. If the operator immediately retries, it could block waiting for the lock.

**Remediation:** Replace `log.Fatalf` calls with explicit error handling that returns an error, letting the deferred rollback execute naturally. The security property restored is deterministic transaction cleanup on failure.

---

### SEC-013: Log Sanitization Does Not Redact WebSocket Token from Request Path

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Log Sanitization — Token Exposure |
| **Effort** | Small (hours) |
| **Priority** | 9 |
| **Status** | ✅ Fixed |

**Description:** The `SanitizedLogger` middleware uses gin's `param.Path` which includes query parameters. The WebSocket endpoint (`/ws?token=<jwt>`) passes the token as a query parameter, meaning every connection attempt logs the token in plaintext. The existing `sanitizeHeaders()` only redacts the `Authorization` header — not URL query strings.

**Evidence:**
`backend/internal/api/middleware/logging.go:37-47`
```go
return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s\n",
    param.TimeStamp.Format(time.RFC3339),
    param.StatusCode,
    param.Latency,
    param.Method,
    param.Path, // includes ?token=<raw_value>
)
```

**Impact:** Any system with log aggregation stores JWT/API tokens in plaintext. An attacker with log access gains valid authentication tokens with a 24-hour exploitation window.

**Remediation:** Add query parameter sanitization: parse `param.Path`, replace `token` parameter value with `[REDACTED]`. The security property restored is that authentication credentials are never persisted in log storage.

**Resolution:** Added `sanitizePath()` to the logging middleware. Any `token=` parameter in the logged path is replaced with `token=[REDACTED]` before output. WebSocket connection logs now show `/ws?token=[REDACTED]`.

---

### SEC-014: WebSocket Auth Not Using Shared combinedAuth Middleware (Code Duplication Risk)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Authentication Architecture |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** The WebSocket handler implements its own `validateToken()` that duplicates the JWT and API token validation logic from `combinedAuth`. The implementations have diverged: the WS handler does NOT call `queries.TouchAPIToken()` on successful API token authentication, making it impossible to audit when tokens were last used via WebSocket. Future security fixes to `combinedAuth` may not propagate to the WS path.

**Evidence:**
`backend/internal/api/router.go:193` — TouchAPIToken called on success:
```go
_ = queries.TouchAPIToken(c.Request.Context(), matched.ID)
c.Set("user_id", matched.UserID.String())
c.Next()
```

`backend/internal/api/handlers/ws.go:130-136` — TouchAPIToken NOT called:
```go
c.Set("user_id", candidates[i].UserID.String())
return true  // No TouchAPIToken call
```

**Impact:** API tokens used for WebSocket connections don't get `last_used_at` updated. Future security improvements may not propagate to WS auth.

**Remediation:** Refactor token validation into a shared function consumed by both `combinedAuth` and the WS handler. The security property restored is a single source of truth for authentication logic.

---

### SEC-015: Proto Source Handler Bypasses sqlc Layer with Direct Pool Query

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Code Hygiene — sqlc Bypass |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The proto source delete handler uses `h.pool.Exec()` directly with a raw SQL string instead of a sqlc-generated query. While properly parameterized (no injection risk), it bypasses the sqlc type safety layer and the error is silently discarded.

**Evidence:**
`backend/internal/api/handlers/proto_source.go:410-412`
```go
_, _ = h.pool.Exec(c.Request.Context(),
    "UPDATE monitors SET settings = $1, updated_at = now() WHERE id = $2",
    updatedSettings, monitorID,
)
```

**Impact:** No security vulnerability. The query uses positional parameters. However, schema changes could break this query without compile-time detection, and errors are silently discarded.

**Remediation:** Add a sqlc query for this operation and handle the error explicitly. The property restored is consistent query management and error visibility.

---

## 4. Performance Findings

### PERF-001: VirtualList DOM Node Cap Correctly Enforced

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Rendering — Virtualization |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The VirtualList implementation correctly caps rendered DOM nodes at 60 via the `endIndex` derived computation. The buffer is clamped to [5, 20], and scroll handling is RAF-throttled. Implementation uses spacer divs for scroll position and proper ARIA semantics.

**Evidence:**
`frontend/src/components/VirtualList.svelte:57-64`
```svelte
let endIndex = $derived.by(() => {
  const maxRendered = 60;
  const rawEnd = Math.min(rawEndIndex, items.length);
  const count = rawEnd - startIndex;
  if (count > maxRendered) {
    return startIndex + maxRendered;
  }
  return rawEnd;
});
```

**Impact:** No issue. The DOM cap is correctly implemented and maintains smooth scrolling with 500+ monitors.

**Remediation:** None required.

---

### PERF-002: VirtualList Uses Index-Based Keying Instead of Stable Item Identity

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Rendering — Virtualization |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The `{#each}` block uses `(startIndex + i)` as the key — a positional index rather than a stable item identifier. When the list scrolls, items at the same position get different keys, causing Svelte to destroy and recreate DOM nodes instead of recycling them.

**Evidence:**
`frontend/src/components/VirtualList.svelte:97-104`
```svelte
{#each visibleItems as item, i (startIndex + i)}
  <div
    class="virtual-list-row"
    style="height: {itemHeight}px;"
    role="listitem"
  >
    {@render row(item, startIndex + i)}
  </div>
{/each}
```

**Impact:** During continuous scrolling of 500+ monitors, nodes are recreated instead of recycled, increasing GC pressure and potentially causing frame drops on lower-end devices.

**Remediation:** Use a stable item identifier (e.g., `item.id`) as the each-block key. This restores the DOM recycling performance property.

---

### PERF-003: Initial JS Bundle Well Under 200 KB Gzipped Target

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Bundle Size |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The initial page load preloads 22 JS modules totaling ~138 KB raw / ~50 KB gzipped — approximately 25% of the 200 KB budget.

**Evidence:**
`frontend/build/index.html` — 22 `rel="modulepreload"` links:
- Raw JS: 137,736 bytes
- Gzipped: ~50,451 bytes
- CSS: 37,868 bytes raw / 7,156 bytes gzipped

**Impact:** No issue. Bundle size is healthy with substantial headroom.

**Remediation:** None required.

---

### PERF-004: CodeMirror 127 KB Gzipped but Properly Code-Split

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Code Splitting |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The CodeMirror editor suite produces a 409 KB raw / 127 KB gzipped chunk. This is NOT in the initial bundle — it's loaded only by notification/webhook template editing pages. The code-splitting is effective.

**Evidence:**
`frontend/build/_app/immutable/chunks/DbluOvrW.js` — 409,485 bytes raw, 126,841 bytes gzipped.
Loaded only by `nodes/6.DA41_QK8.js` and `nodes/7.BLgoDtZb.js` (not in `index.html` preloads).

**Impact:** Users navigating to webhook template editor experience a noticeable load delay. Only affects admin users configuring webhooks.

**Remediation:** Consider dynamic `import()` of CodeMirror only when the user clicks into the template editor field. Alternatively, evaluate lighter alternatives for JSON template editing.

---

### PERF-005: Route Node 5 at 39 KB Gzipped Exceeds 20 KB Route Module Threshold

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Code Splitting |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Route node 5 is 108 KB raw / 39 KB gzipped, nearly double the 20 KB threshold. This node contains the monitor detail page with chart rendering, history display, and notification binding management inlined into a single chunk.

**Evidence:**
`frontend/build/_app/immutable/nodes/5.BUITZNHn.js` — 107,984 bytes raw, 38,894 bytes gzipped.

**Impact:** First navigation to monitor detail page downloads 39 KB of JS. On slow mobile networks, this adds ~1.5 seconds to perceived page load.

**Remediation:** Split the monitor detail page into sub-components loaded on demand: lazy-load the HistoryChart and notification bindings section independently.

---

### PERF-006: HistoryChart Uses Legacy API, Does Not React to Data Changes

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reactivity — Svelte 5 Patterns |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The HistoryChart component uses Svelte 4's `export let data` pattern instead of Svelte 5's `$props()` rune. It does NOT react to prop changes — the chart is only created in `onMount`. If the parent updates the data prop, the chart shows stale data until remount.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:8-11`
```typescript
export let data: HistoryPoint[] = [];

let chartContainer: HTMLElement;
let chart: uPlot | null = null;
```

**Impact:** The chart does not update when new history points arrive via WebSocket. Users see stale data until they navigate away and back.

**Remediation:** Migrate to `$props()` and add a `$effect` that calls `chart.setData()` when data changes. This restores the real-time update property.

---

### PERF-007: uPlot Instance Properly Destroyed on Unmount

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Memory — Chart Lifecycle |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The HistoryChart correctly destroys the uPlot instance in `onDestroy`, nulling the reference to allow GC.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:71-80`
```typescript
function destroyChart() {
  if (chart) {
    chart.destroy();
    chart = null;
  }
}

onDestroy(() => {
  destroyChart();
});
```

**Impact:** No retained references or detached canvas elements after unmount.

**Remediation:** None required.

---

### PERF-008: HistoryChart Reads Computed Styles on Every createChart Invocation

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Rendering — Layout Thrashing |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** Each `createChart()` call invokes `getComputedStyle(document.documentElement)` to read theme colors. This triggers a synchronous style recalculation. Minor impact since it's called once per chart mount, not in a loop.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:35-37`
```typescript
const styles = getComputedStyle(document.documentElement);
const axisStroke = styles.getPropertyValue('--color-text-muted').trim() || '#64748b';
const gridStroke = styles.getPropertyValue('--color-border').trim() || '#e2e8f0';
```

**Impact:** Negligible for a single chart but could compound if used in a list without virtualization.

**Remediation:** Cache theme colors at the module level or read them once per theme change. This eliminates forced style recalculations during component initialization.

---

### PERF-009: Svelte 5 Reactivity Patterns Clean — No Deep Derived Chains

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Reactivity — Derived State Depth |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** All stores use Svelte 5 runes correctly with shallow derived chains. Maximum derived depth is 2 levels. No cascading re-computation issues.

**Evidence:**
`frontend/src/lib/stores/monitors.svelte.ts:37-43`
```typescript
const list = $derived<Monitor[]>(Array.from(monitors.values()));
const totalCount = $derived<number>(monitors.size);
const healthyCount = $derived<number>(
  Array.from(monitors.values()).filter((m) => m.state === 'up').length
);
```

**Impact:** No issue. Derived chains are shallow and predictable.

**Remediation:** None required.

---

### PERF-010: No Request Deduplication for Concurrent Identical Fetches

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Network Efficiency |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The API client does not implement request deduplication. On WebSocket reconnect, the layout fires `getMonitors(1, 500)` while the dashboard page simultaneously calls `dashboardStore.load()`. Without deduplication, rapid navigations or reconnect oscillations generate duplicate in-flight requests.

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53`
```svelte
onStatusChange(status) {
  if (status === 'connected') {
    getMonitors(1, 500)
      .then((response) => {
        monitorStore.setMonitors(response.data);
      })
```

`frontend/src/routes/+page.svelte:50-56`
```svelte
$effect(() => {
  const currentStatus = connectionStore.status;
  if (initialLoadDone && previousConnectionStatus !== 'connected' && currentStatus === 'connected') {
    dashboardStore.load();
```

**Impact:** On reconnection, at minimum 2 parallel requests fire. At 500+ monitors with multiple tabs, this multiplies bandwidth waste and server load.

**Remediation:** Implement a request deduplication layer in `apiRequest()` that tracks in-flight GET requests by URL and returns the same Promise for duplicates.

---

### PERF-011: No AbortController Cancellation on Component Unmount or Navigation

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Network Efficiency |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The `apiRequest()` function creates an AbortController solely for the 15-second timeout. No page component passes an AbortController for unmount cancellation. When users navigate away, in-flight requests continue to completion, potentially updating state for destroyed components.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:78-89`
```svelte
async function fetchData() {
  loading = true;
  error = null;
  try {
    const [monitorData, historyData, incidentsData, statsData] = await Promise.all([
      getMonitor(id),
      getMonitorHistory(id, from, to),
      getMonitorIncidents(id, 1, 5),
      getMonitorStats(id),
    ]);
```

No `onDestroy` or AbortController cleanup in any route page.

**Impact:** Wasted bandwidth on unused responses. Potential state corruption if responses arrive after navigation.

**Remediation:** Extend `apiRequest()` to accept an external `AbortSignal`. In each page, create a per-fetch AbortController and abort in effect cleanup.

---

### PERF-012: Reconnect Refetch Triggers Simultaneous Redundant Monitor Fetches

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Network Efficiency |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** On WebSocket reconnection, the layout fires `getMonitors(1, 500)` on every `'connected'` status — including the initial connection. If the user is on the monitors list page, that page also fetches its own data, creating overlap.

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53` — layout fires on every `'connected'` status including initial.

**Impact:** Extra network request on each connection. Moderate impact at scale (500+ monitors JSON payload fetched twice).

**Remediation:** Gate the layout refetch behind a reconnection flag, or implement the deduplication layer from PERF-010.

---

### PERF-013: No Explicit Manual Chunking or Bundle Size CI Guard

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Build Configuration |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `vite.config.ts` contains minimal configuration with no `manualChunks` or bundle size guardrails. No automated check prevents future dependencies from landing in the initial bundle.

**Evidence:**
`frontend/vite.config.ts:1-14` — only SvelteKit plugin and dev server proxy configured.

**Impact:** No bundle size regression detection in CI. A new dependency could inflate the initial bundle past 200 KB without detection.

**Remediation:** Add `rollup-plugin-visualizer` for bundle analysis and a CI check asserting initial bundle gzipped size < 200 KB.

---

### PERF-014: Static Font Assets Served with no-cache Instead of Immutable Cache-Control

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Static Asset Caching |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The `isHashedAsset()` function only recognizes files under `_app/` as cacheable with immutable headers. Self-hosted font files and brand assets in `static/` receive `Cache-Control: no-cache`, forcing revalidation on every page navigation.

**Evidence:**
`backend/internal/api/router.go:338-342`
```go
func isHashedAsset(filePath string) bool {
    if strings.HasPrefix(filePath, "_app/") {
        return true
    }
    return false
}
```

`frontend/src/app.css:1-8`
```css
@font-face {
  font-family: 'Inter';
  font-style: normal;
  font-weight: 600;
  font-display: swap;
  src: url('/fonts/inter-semibold.woff2') format('woff2');
}
```

**Impact:** The Inter WOFF2 font (~15-25 KB) is re-downloaded or revalidated on every page load. Unnecessary network overhead.

**Remediation:** Extend `isHashedAsset()` to match known-immutable static paths: `fonts/`, `brand/`. The cache-once-serve-forever property is restored for immutable assets.

---

### PERF-015: Patch-Merge Update Pattern Efficient and Correctly Implemented

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Real-Time Update Efficiency |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The monitor store's patch-merge uses `Map<string, Monitor>` for O(1) lookup. Only `state` and `last_checked_at` are merged. Unknown IDs are silently discarded. Implementation is correct and efficient for 500+ monitors.

**Evidence:**
`frontend/src/lib/stores/monitors.svelte.ts:63-71`
```typescript
function applyPatch(patch: MonitorPatch): void {
  const existing = monitors.get(patch.monitor_id);
  if (!existing) return;
  const updated = applyMonitorPatch(existing, patch);
  const next = new Map(monitors);
  next.set(patch.monitor_id, updated);
  monitors = next;
}
```

**Impact:** No negative impact. Pattern is sound.

**Remediation:** None required.

---

### PERF-016: Full Monitor List Refetch on WebSocket Reconnection Correctly Implemented

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Real-Time State Reconciliation |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** On reconnection, the client fetches `getMonitors(1, 500)` and replaces the entire store via `setMonitors()`. This correctly reconciles state after disconnection per requirement 7.6.

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53`
```svelte
onStatusChange(status) {
  if (status === 'connected') {
    getMonitors(1, 500)
      .then((response) => {
        monitorStore.setMonitors(response.data);
      })
```

**Impact:** Correctly reconciles state after disconnection.

**Remediation:** None required.

---

### PERF-020: Database Connection Pool Uses pgx Defaults — Undersized for Worker Count

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Database Pool Sizing |
| **Effort** | Small (hours) |
| **Priority** | 5 |

**Description:** The database connection pool is created via `pgxpool.New(ctx, databaseURL)` without explicit pool configuration. The pgx/v5 default `MaxConns` is `max(4, runtime.NumCPUs())` — typically 4-16. In production, the scheduler runs 200 workers and notification dispatcher runs 50 workers, all competing for this shared pool. Connection exhaustion is almost certain under sustained load.

**Evidence:**
`backend/internal/store/postgres/pool.go:14-22`
```go
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
    pool, err := pgxpool.New(ctx, databaseURL)
    if err != nil {
        return nil, fmt.Errorf("postgres pool init: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("postgres ping: %w", err)
    }
    return pool, nil
}
```

**Impact:** Under load, workers block on `pool.Acquire()`, causing check-cycle durations to spike. At 500 monitors with 1-second tick intervals, this becomes a bottleneck that degrades monitoring reliability.

**Remediation:** Set `MaxConns` to at least `PULSE_SCHEDULER_WORKERS + PULSE_NOTIFICATION_WORKERS + 20` (270 in default prod config). Also set `MinConns` for warm baseline and `MaxConnLifetime` for connection health.

---

### PERF-021: Scheduler N+1 Query Pattern in Notification Fan-Out Path

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | N+1 Query Pattern |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** For every monitor check with notification bindings, the scheduler executes three sequential queries per monitor: `ListBindingsByMonitor`, `ListCheckResultsByMonitor(LIMIT 100)`, and fan-out enqueue. Each delivery worker executes 3 more queries per job. With 500 monitors and multiple bindings, this produces thousands of small queries per tick.

**Evidence:**
`backend/internal/monitor/scheduler.go:237-262`
```go
func (s *Scheduler) dispatchNotifications(ctx context.Context, m db.Monitor, result Result) {
    dbBindings, err := s.queries.ListBindingsByMonitor(ctx, m.ID)
    // ...
    consecFailures := s.countConsecutiveFailures(ctx, m.ID)
    // ...
}
```

**Impact:** At 500 monitors with bindings, each tick generates 1000+ DB round-trips for notification evaluation. Combined with undersized pool (PERF-020), this amplifies connection contention.

**Remediation:** Batch-load bindings for all due monitors in a single query. Replace `ListCheckResultsByMonitor(LIMIT 100)` with a dedicated `CountConsecutiveFailures` query. Cache channel configs in the dispatcher.

---

### PERF-022: No TimescaleDB Native Retention Policy — Custom DELETE with Subquery

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | TimescaleDB Configuration |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** The TimescaleDB hypertable has no native `add_retention_policy`. A custom Go `RetentionService` iterates monitors and runs per-monitor row-level DELETEs. This is significantly slower than TimescaleDB's native `drop_chunks()`.

**Evidence:**
`backend/internal/retention/service.go:155-167`
```go
func (s *RetentionService) deleteExpiredRows(ctx context.Context, mon db.Monitor) (int64, error) {
    cutoff := time.Now().Add(-time.Duration(mon.HistoryRetentionDays) * 24 * time.Hour)
    tag, err := s.pool.Exec(ctx,
        `DELETE FROM check_results
         WHERE id IN (
           SELECT id FROM check_results
           WHERE monitor_id = $1
             AND checked_at < $2
           LIMIT $3
         )`,
        mon.ID, cutoff, s.deleteLimit,
    )
```

**Impact:** With 500 monitors and 30-day retention, row-level DELETE is orders of magnitude slower than chunk-based operations. Cleanup cycles consume significant DB resources.

**Remediation:** Add a TimescaleDB `add_retention_policy` as a safety net. Enable chunk-based compression for older data. The property restored is efficient time-range operations.

---

### PERF-023: Hub ClientCount() Uses Mutex Not Updated by Run Loop (Data Race)

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Concurrency Correctness |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** `Hub.ClientCount()` reads `len(h.clients)` under `h.mu.RLock()`, but `Run()` modifies `h.clients` without holding this mutex — using single-goroutine ownership via channels. Reading a map while another goroutine writes is undefined behavior under Go's memory model.

**Evidence:**
`backend/internal/hub/hub.go:126-131`
```go
func (h *Hub) ClientCount() int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return len(h.clients)
}
```

`Run()` loop modifies `h.clients` without locking `h.mu`.

**Impact:** Potential data race. The core broadcast functionality is unaffected (single-goroutine ownership), but calling `ClientCount()` concurrently would crash under race detector.

**Remediation:** Use an `atomic.Int64` counter or expose `ClientCount()` as a channel request to the Run loop.

---

### PERF-024: Scheduler Poll Rebuilds Prometheus Labels on Every Tick

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Scheduler Efficiency |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `poll()` method calls `ListAllTagKeys(ctx)` and `CountMonitors(ctx)` on every tick (1-second interval) regardless of whether any monitors are due. These 2 unnecessary queries per second compete for DB connections.

**Evidence:**
`backend/internal/monitor/scheduler.go:141-157`
```go
func (s *Scheduler) poll(ctx context.Context, jobs chan<- monitorJob) {
    if s.dynMetrics != nil {
        allKeys, err := s.queries.ListAllTagKeys(ctx)
        if err != nil {
            log.Printf("scheduler: list tag keys for rebuild: %v", err)
        } else {
            s.dynMetrics.RebuildLabels(allKeys)
        }
        count, err := s.queries.CountMonitors(ctx)
        if err == nil {
            s.dynMetrics.MonitorsTotal.Set(float64(count))
        }
    }
```

**Impact:** Two extra queries per second add negligible cost individually but consume connection pool slots under contention.

**Remediation:** Debounce to run at most every 30-60 seconds, or move to a separate low-frequency goroutine.

---

### PERF-025: Notification Dispatcher Worker Uses context.Background() for Delivery

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Context Propagation |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The dispatcher worker calls `processJob(context.Background(), ...)` and `dispatch()` creates its own `context.WithTimeout(context.Background(), 30*time.Second)`. In-flight deliveries have contexts independent of shutdown signal. Workers continue processing up to 30 seconds after shutdown.

**Evidence:**
`backend/internal/notification/dispatcher.go:169-179`
```go
func (d *Dispatcher) worker(id int) {
    defer d.wg.Done()
    for {
        select {
        case job, ok := <-d.jobs:
            if !ok { return }
            d.processJob(context.Background(), id, job)
        case <-d.done:
            return
        }
    }
}
```

**Impact:** During shutdown, deliveries may hang up to 30 seconds. Mitigated by the drain timeout, making practical risk low.

**Remediation:** Pass a shutdown-aware context derived from the dispatcher lifecycle. This enables faster cooperative drain.

---

### PERF-026: Scheduler Job Channel Blocks Senders When Workers Are Busy

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Backpressure Design |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The scheduler sends jobs to a buffered channel with `ctx.Done()` fallback. When all workers are busy and buffer is full, `poll()` blocks. This is correct backpressure behavior — no checks are dropped or panicked.

**Evidence:**
`backend/internal/monitor/scheduler.go:185-190`
```go
for _, job := range monitorJobs {
    select {
    case <-ctx.Done():
        return
    case jobs <- job:
    }
}
```

**Impact:** None — correct bounded-concurrency design.

**Remediation:** None required.

---

### PERF-027: WebSocket Hub Broadcast — Correct Slow-Consumer Eviction

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | WebSocket Throughput |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The hub broadcasts using `select/default` per client. Full send buffers trigger immediate disconnect — no blocking of other clients. The broadcast channel is buffered (256) with non-blocking drop on overflow.

**Evidence:**
`backend/internal/hub/hub.go:85-93`
```go
case message := <-h.broadcast:
    for client := range h.clients {
        select {
        case client.send <- message:
        default:
            delete(h.clients, client)
            close(client.send)
        }
    }
```

**Impact:** None — correct design meeting the 200+ client fan-out requirement.

**Remediation:** None required.

---

### PERF-028: Notification Dispatcher — Correct Independence from Scheduler

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Concurrency Architecture |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The dispatcher operates a fully independent worker pool with non-blocking enqueue via `select/default`. Full buffer drops jobs with metric increment rather than blocking the scheduler.

**Evidence:**
`backend/internal/notification/dispatcher.go:141-154`
```go
func (d *Dispatcher) Enqueue(job DeliveryJob) {
    if d.stopping.Load() != 0 {
        d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
        return
    }
    select {
    case d.jobs <- job:
    default:
        d.metrics.DroppedTotal.WithLabelValues(channelType).Inc()
    }
}
```

**Impact:** None — correct decoupled design.

**Remediation:** None required.

---

## 5. Code Quality Findings

### QUAL-001: Missing SvelteKit Error Boundary Pages

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Error Handling |
| **Effort** | Small (hours) |
| **Priority** | 9 |

**Description:** No `+error.svelte` files exist anywhere in `frontend/src/routes/`. Without error boundaries, any uncaught runtime error during component rendering bubbles to the default SvelteKit error page with no custom UX or recovery action.

**Evidence:**
`frontend/src/routes/` (entire tree)
```
No +error.svelte files found at root, /monitors, /settings, /notifications, or any nested route.
```

**Impact:** An unhandled exception results in a generic SvelteKit 500 page with no branding, retry button, or navigation path back to the app.

**Remediation:** Add a root-level `+error.svelte` with a "Go Home" link styled with project theme tokens. Consider per-section error pages with contextual retry actions. This restores the graceful degradation property.

---

### QUAL-002: Unchecked Type Assertions in WebSocket Message Handler

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | TypeScript Strictness |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The WebSocket message handler uses `as` casts to narrow parsed JSON payload to specific types without runtime validation. A malformed server message could assign incorrect field types to the cast objects.

**Evidence:**
`frontend/src/lib/ws.ts:222-243`
```typescript
switch (envelope.type) {
  case 'monitor_status': {
    const patch = envelope.payload as MonitorPatch;
    monitorStore.applyPatch(patch);
    patchBus.publish(patch);
    break;
  }
  case 'monitor_tags_changed': {
    const tagsPayload = envelope.payload as MonitorTagsChangedPayload;
    monitorStore.applyTagsChange(tagsPayload.monitor_id, tagsPayload.tags);
    break;
  }
}
```

**Impact:** Malformed messages could corrupt the monitor store with `undefined` values. Defense-in-depth gap — the server is trusted but malformed messages propagate silently.

**Remediation:** Add lightweight runtime guards (e.g., `typeof patch.monitor_id === 'string'`) before applying patches.

---

### QUAL-003: Explicit `any` Types in Test Files

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | TypeScript Strictness |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** Several test files use explicit `any` type annotations, primarily in mock setups for CodeMirror and Proto source components. Production source code is free of `any` types.

**Evidence:**
`frontend/src/components/PayloadEditor.test.ts:19-30`
```typescript
const EditorView = vi.fn().mockImplementation(function (this: any, config: any) {
  this.state = { doc: { toString: () => config.state?.doc ?? '' } };
  this.dispatch = vi.fn();
  return this;
});
(EditorView as any).updateListener = { of: vi.fn(() => ({})) };
```

**Impact:** No production impact. Reduces type safety within tests.

**Remediation:** Replace `any` with typed mock utilities. Low priority — informational only.

---

### QUAL-004: Tailwind `dark:` Prefix Violates Theme Convention

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Convention Adherence |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** The `ProtoSourceUpload.svelte` component uses Tailwind's `dark:` prefix, violating the project's `[data-theme="dark"]` CSS selector convention. Makes this component inconsistent with the rest of the codebase.

**Evidence:**
`frontend/src/components/ProtoSourceUpload.svelte:205`
```html
<div class="rounded-md border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700
  dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300" role="alert">
```

**Impact:** The `dark:` classes may not activate correctly and break convention consistency.

**Remediation:** Replace `dark:` prefixed classes with CSS custom properties from `app.css` or the `[data-theme="dark"]` selector pattern.

---

### QUAL-005: Hardcoded User-Visible Strings Not Using `t()` Function

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Convention Adherence |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** Multiple components contain hardcoded English strings that bypass the i18n `t()` function. Per AGENTS.md: "All user-visible strings MUST use the `t()` function."

**Evidence:**
`frontend/src/components/HistoryExplorer.svelte:85-104`
```svelte
Data has been truncated to the monitor's retention period ({retentionDays} days).
...
Retry
...
No data available for the selected period.
```

`frontend/src/routes/monitors/[id]/+page.svelte:132`
```typescript
error = err instanceof Error ? err.message : `Failed to ${newStatus === 'paused' ? 'pause' : 'resume'} monitor.`;
```

Additional occurrences in NotificationChannelForm, MonitorDeliveryLogs, MonitorNotificationBindings, PendingNotificationBindings, and notifications store.

**Impact:** These strings cannot be translated. With 13 supported locales, untranslated text degrades UX for non-English users.

**Remediation:** Add keys to `en.json` and all locale files, replace hardcoded strings with `t()` calls.

---

### QUAL-006: Hardcoded Semantic Color Classes Instead of CSS Custom Properties

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Convention Adherence |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Many components use hardcoded Tailwind color classes for semantic states (error, success, interactive) instead of CSS custom properties from the theme system.

**Evidence:**
`frontend/src/routes/monitors/+page.svelte:138-145`
```html
<div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center">
  <p class="text-sm text-rose-700">{error}</p>
```

`frontend/src/routes/settings/ApiTokenSection.svelte:135`
```html
class="mt-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
```

**Impact:** Error/success colors are not governed by the theme system. Dark mode relies on Tailwind palette rather than centralized tokens.

**Remediation:** Extend `app.css` token system with semantic background/border variants. Replace hardcoded color classes.

---

### QUAL-007: No Test Coverage for Notification Store and Routes

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Test Coverage |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The notification subsystem (store, route pages, channel form) has zero test coverage — representing a full milestone (J) of functionality with no regression protection.

**Evidence:**
```
frontend/src/lib/stores/notifications.svelte.ts — no .test.ts file
frontend/src/routes/notifications/ — no __tests__/ directory
frontend/src/components/NotificationChannelForm.svelte — no test file
frontend/src/components/MonitorNotificationBindings.svelte — no test file
```

**Impact:** Regressions in notification CRUD, binding management, or delivery log display go undetected.

**Remediation:** Add unit tests for the notifications store and integration tests for channel form validation.

---

### QUAL-008: Component Data Fetching Mixed with Presentation

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Component Architecture |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Several reusable components (`HistoryExplorer`, `MonitorNotificationBindings`, `MonitorDeliveryLogs`) perform their own API calls, manage loading/error state, and render UI — mixing data-fetching with presentation.

**Evidence:**
`frontend/src/components/HistoryExplorer.svelte:30-48`
```typescript
async function fetchHistory() {
  loading = true;
  error = null;
  try {
    const response = await getMonitorHistoryExtended(
      monitorId, selectedRange.from, selectedRange.to, step
    );
    points = response.points ?? [];
  } catch (err: unknown) {
    error = err instanceof Error ? err.message : 'Failed to load history data.';
  } finally {
    loading = false;
  }
}
```

**Impact:** Components are harder to test in isolation and harder to reuse with pre-fetched data. Pragmatic SvelteKit pattern, not a correctness issue.

**Remediation:** Consider extracting data-fetching logic into composable functions. Low priority architectural improvement.

---

### QUAL-009: Silent `catch` Blocks in Auth Store

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Error Handling |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** The auth store's localStorage operations catch errors silently without logging or user feedback. A user in private browsing mode has no indication their session cannot persist.

**Evidence:**
`frontend/src/lib/stores/auth.svelte.ts:29-33`
```typescript
function writeTokenToStorage(token: string): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Silently fail — storage quota or access issue
  }
}
```

**Impact:** If localStorage is unavailable, the user can log in but loses their session on refresh without warning. Rare edge case.

**Remediation:** Log a console warning when localStorage operations fail. Consider showing a one-time informational toast.

---

### QUAL-010: Monitor Detail Page State Indicators Use Color Only

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Accessibility |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** The monitor detail page uses hardcoded color classes for state indicators. The small status dot uses only color to differentiate "up" from "down". However, adjacent text labels always provide a text alternative, making this compliant in the current UI.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:36-40`
```typescript
const stateColors: Record<string, string> = {
  up: 'bg-emerald-500',
  down: 'bg-rose-500',
  unknown: 'bg-slate-400'
};
```

**Impact:** The state dot alone does not convey state to color-blind users, but labels are always present.

**Remediation:** Confirm all instances of state dots have text labels. Consider adding `aria-label` to dot spans.

---

### QUAL-011: Tab Navigation Lacks `tablist` Role and Arrow Key Support

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Accessibility |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The monitor detail page tab UI uses `role="tab"` and `aria-selected` but the container lacks `role="tablist"`. Arrow-key navigation is not implemented. Users can only navigate between tabs via the Tab key.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:187-206`
```html
<div class="border-b border-[var(--color-border)]">
  <nav class="-mb-px flex gap-6" aria-label="Monitor tabs">
    <button
      type="button"
      onclick={() => activeTab = 'overview'}
      aria-selected={activeTab === 'overview'}
      role="tab"
    >
```

Missing: `role="tablist"`, `tabindex="-1"` on inactive tabs, `aria-controls`, arrow key handler.

**Impact:** Screen readers may not announce the widget as a tab list. Keyboard-only users cannot use arrow keys.

**Remediation:** Add `role="tablist"` to the container. Implement arrow-key navigation. Set proper tabindex management.

---

### QUAL-012: Uncaught Promise in Monitor Create Page Submit Flow

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Error Handling |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** The `handleSubmit` function chains multiple awaits (createMonitor, createCredential, createNotificationBinding). If the monitor creates successfully but credentials fail, the user sees an error without knowing the monitor exists.

**Evidence:**
`frontend/src/routes/monitors/create/+page.svelte:8-35`
```typescript
async function handleSubmit(values, pendingCredential?, pendingBindings?) {
  const created = await createMonitor(values);
  if (pendingCredential) {
    await createCredential(created.id, pendingCredential);
  }
  if (pendingBindings && pendingBindings.length > 0) {
    await Promise.all(pendingBindings.map((binding) => { ... }));
  }
  await goto(`/monitors/${created.id}`);
}
```

**Impact:** Partial creation state — monitor exists but without credentials/bindings. Confusing UX.

**Remediation:** Handle secondary failures independently — show a warning toast and still navigate to the new monitor.

---

### QUAL-020: HTTP Server Lacks Graceful Shutdown — In-Flight Requests Aborted on SIGTERM

| Field | Value |
|-------|-------|
| **Severity** | High |
| **Category** | Shutdown |
| **Effort** | Small (hours) |
| **Priority** | 5 |

**Description:** The application uses `gin.Engine.Run()` which calls `http.ListenAndServe` with no shutdown hook. The shutdown goroutine stops background services but never calls `http.Server.Shutdown(ctx)`. In-flight requests are terminated immediately on SIGTERM.

**Evidence:**
`backend/cmd/pulse/main.go:225-226`
```go
if err := r.Run(addr); err != nil {
    log.Fatalf("server exited with error: %v", err)
}
```

The shutdown goroutine (line 207-222) has no reference to an `http.Server`:
```go
go func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    sig := <-sigCh
    reminderScheduler.Stop()
    dispatcher.Shutdown(drainCtx)
    wsHub.Stop()
    appCancel()
}()
```

**Impact:** Any HTTP request in progress at shutdown is killed mid-flight. Clients receive connection-reset errors. Database transactions may be left uncommitted.

**Remediation:** Replace `r.Run(addr)` with an explicit `http.Server{Handler: r}` and call `srv.Shutdown(ctx)` with a drain timeout. The graceful-drain property is restored.

---

### QUAL-021: Notification Dispatcher Workers Use context.Background()

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Context Propagation |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** The dispatcher worker calls `processJob(context.Background(), ...)`. Delivery operations run with contexts not tied to application shutdown. The parent context should derive from a cancellable source.

**Evidence:**
`backend/internal/notification/dispatcher.go:246-247`
```go
case job, ok := <-d.jobs:
    if !ok { return }
    d.processJob(context.Background(), id, job)
```

**Impact:** During shutdown, workers may hang for up to 30 seconds per delivery. Partially mitigated by drain timeout.

**Remediation:** Pass a shutdown-aware context to `processJob`. This restores the context-propagation property.

---

### QUAL-022: ICMP Resolver Uses context.Background() — Ignores Per-Check Timeout

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Context Propagation |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The ICMP checker's `resolveAddr()` calls `net.DefaultResolver.LookupIP(context.Background(), ...)`. DNS lookup is not bounded by the per-check timeout context. A stalled DNS resolution can block a worker indefinitely.

**Evidence:**
`backend/internal/monitor/icmp.go:235`
```go
ips, err := net.DefaultResolver.LookupIP(context.Background(), network, addr)
```

**Impact:** A stalled DNS resolution can exhaust scheduler workers, halting the entire scheduler.

**Remediation:** Accept the check context and pass it to `LookupIP`. This ensures DNS resolution respects the monitor timeout.

---

### QUAL-023: Hub.ClientCount() Has a Data Race with Run() Loop

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Concurrency — Resource Leak |
| **Effort** | Small (hours) |
| **Priority** | 10 |

**Description:** `Hub.ClientCount()` reads `h.clients` under `h.mu.RLock()`, but `Run()` modifies `h.clients` without ever acquiring `h.mu`. Concurrent unsynchronized read and write to a map is undefined behavior under Go's memory model.

**Evidence:**
`backend/internal/hub/hub.go:127-130`
```go
func (h *Hub) ClientCount() int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return len(h.clients)
}
```

`Run()` modifies map without locking:
```go
case client := <-h.register:
    h.clients[client] = struct{}{}
```

**Impact:** If `ClientCount()` is called concurrently with `Run()`, the program will crash under race detector. Currently unused externally but exported and inviting future use.

**Remediation:** Use an `atomic.Int64` counter or expose as a channel request to the Run loop.

---

### QUAL-024: Notification processJob Half-Handling Pattern

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Error Handling |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** In `processJob()`, errors from `dispatch()` are logged, recorded in delivery_logs, and counted in metrics — but never propagated. The retry mechanism is embedded inside `dispatch()` rather than managed by the caller. Acceptable but not idiomatic.

**Evidence:**
`backend/internal/notification/dispatcher.go:287-294`
```go
if err != nil {
    d.metrics.DeliveriesTotal.WithLabelValues(channelType, "failure").Inc()
    log.Printf("notification: delivery failed ...")
    d.LogDelivery(ctx, job.ChannelID, job.MonitorID, job.BindingID,
        job.TriggerType, job.Attempt, "failure", err.Error())
}
```

**Impact:** Low. Pattern works but makes retry logic less visible to maintainers.

**Remediation:** Extract the retry decision into `processJob()` for more transparent control flow.

---

### QUAL-025: Timescale and Retention Stores Bypass sqlc Layer

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | sqlc Consistency |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** The `internal/store/timescale/` and `internal/retention/` packages execute SQL directly via `pool.Exec()`/`pool.Query()` bypassing sqlc. Queries are parameterized (no injection risk) but lose compile-time type checking.

**Evidence:**
`backend/internal/store/timescale/store.go:60-71`
```go
_, err := s.pool.Exec(ctx,
    `INSERT INTO check_results (monitor_id, checked_at, state, latency_ms, status_code, error, ssl_days_remaining)
     VALUES ($1, $2, $3, $4, $5, $6, $7)`,
    pt.MonitorID, pt.CheckedAt, pt.State, pt.LatencyMs, pt.StatusCode, pt.Error, pt.SslDaysRemaining,
)
```

**Impact:** Schema changes could silently break these queries without build-time detection.

**Remediation:** Migrate these queries into sqlc `.sql` files. This restores the single-query-management-layer property.

---

### QUAL-026: Proto Registry Compilation Uses context.Background()

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Context Propagation |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** Proto registry's `ParseProtoFiles()` calls `compiler.Compile(context.Background(), ...)`. This compilation step could take arbitrary time for large file sets. No timeout or cancellation from the HTTP handler propagates.

**Evidence:**
`backend/internal/proto/registry.go:78`
```go
compiled, err := compiler.Compile(context.Background(), filenames...)
```

**Impact:** A large proto file set could hang the handler. Limited blast radius since it's admin-only.

**Remediation:** Accept a context from the caller and pass it to compilation. This restores the timeout-propagation property.

---

### QUAL-027: Missing %w Verb in reflect.go Breaks Error Chain

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Error Handling |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** Several error returns in `internal/proto/reflect.go` use `fmt.Errorf("...: %s", err)` without `%w`, breaking `errors.Is()` and `errors.As()` chains.

**Evidence:**
`backend/internal/proto/reflect.go:80-81`
```go
if errResp := resp.GetErrorResponse(); errResp != nil {
    return nil, fmt.Errorf("server does not support reflection: %s", errResp.GetErrorMessage())
}
```

**Impact:** Callers cannot programmatically distinguish between different gRPC reflection failures.

**Remediation:** Define sentinel errors and use `%w` verb for wrapping. This restores the error-chain property.

---

### QUAL-028: Hub.Run() Goroutine Not Context-Aware

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Resource Leak |
| **Effort** | Small (hours) |
| **Priority** | 15 |

**Description:** `Hub.Run()` uses `h.done` channel close for termination but doesn't accept or select on a `context.Context`. Cannot participate in the application's context cancellation hierarchy.

**Evidence:**
`backend/internal/hub/hub.go:68-69`
```go
func (h *Hub) Run() {
    for {
        select {
        ...
        case <-h.done:
```

**Impact:** Low — current shutdown works via explicit `Stop()`. Defense-in-depth concern if shutdown ordering changes.

**Remediation:** Modify `Run()` to accept and select on a `context.Context` as fallback termination.

---

### QUAL-029: Shutdown Sequence Does Not Drain HTTP Server Before Stopping Services

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Shutdown |
| **Effort** | Small (hours) |
| **Priority** | 9 |

**Description:** The shutdown implementation stops the notification dispatcher and WebSocket hub first, then cancels the app context. The HTTP server never stops accepting. Requests arriving during shutdown use already-stopped services and fail with nil-pointer or closed-channel errors.

**Evidence:**
`backend/cmd/pulse/main.go:207-222`
```go
go func() {
    sig := <-sigCh
    reminderScheduler.Stop()
    dispatcher.Shutdown(drainCtx)
    wsHub.Stop()
    appCancel()
}()
```

**Impact:** Requests during shutdown encounter partially-torn-down services. WebSocket clients get connection resets rather than graceful close frames.

**Remediation:** Correct the sequence: (1) `srv.Shutdown()` — drain HTTP, (2) `appCancel()` — stop scheduler, (3) drain dispatcher, (4) `wsHub.Stop()`, (5) close pool. This restores the graceful-drain-ordering property.

---

## 6. Improvements Catalog

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
| 18 | QUAL-007 | Add test coverage for notification store and routes | Notification module has 0% test coverage. Critical delivery logic lacks regression protection. | Backend + Frontend / notifications | Medium | Quality | 10 |
| 19 | QUAL-021 | Propagate parent context to dispatcher workers | Workers use `context.Background()`, ignoring shutdown signals. In-flight deliveries may run indefinitely after SIGTERM. | Backend / notification | Small | Reliability | 10 |
| 20 | QUAL-023 | Fix Hub.ClientCount() data race | `ClientCount()` reads the clients map without synchronization while `Run()` modifies it. Race detector flags this under concurrent access. | Backend / hub | Small | Reliability | 10 |

#### Priority 11 — Medium Risk / Possible (Lower Likelihood)

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
| 33 | SEC-010 | Sanitize server-generated chart tooltip content | `{@html}` renders tooltip content from server data. While currently safe, a future API change could introduce stored XSS. | Frontend / HistoryChart | Small | Security | 14 |
| 34 | SEC-014 | Unify WS auth with combinedAuth middleware | WS auth duplicates the combined auth logic independently. Divergence risk: fixes to REST auth may not propagate to WS path. | Backend / WS handler | Medium | Security | 14 |
| 35 | SEC-015 | Route proto source handler through sqlc layer | Proto source handler uses direct `pool.Query()` bypassing the sqlc abstraction. Reduces consistency guarantees and auditability. | Backend / monitor | Small | Quality | 14 |
| 36 | PERF-004 | Defer CodeMirror load to field focus | CodeMirror 127 KB gzipped loads at route level. Deferring to editor focus reduces initial page weight. | Frontend / routes | Small | Performance | 14 |
| 37 | PERF-005 | Reduce route node 5 bundle size | Route node at 39 KB gzipped exceeds the 20 KB threshold. Consider code-splitting heavy components or lazy-loading sub-routes. | Frontend / routes | Medium | Performance | 14 |
| 38 | PERF-012 | Deduplicate reconnect refetch requests | WebSocket reconnection triggers multiple simultaneous monitor fetches. Consolidate into a single fetch with shared promise. | Frontend / WS + stores | Small | Performance | 14 |
| 39 | PERF-013 | Add bundle size CI guard | No automated check prevents bundle size regressions. A CI step comparing against size budgets catches growth early. | Frontend / CI | Small | Quality | 14 |
| 40 | PERF-023 | Fix Hub ClientCount() mutex consistency | ClientCount() reads a counter not atomically updated by the Run() loop, creating stale-read potential under concurrency. | Backend / hub | Small | Reliability | 14 |
| 41 | PERF-024 | Pre-allocate Prometheus labels in scheduler | Scheduler rebuilds label strings on every tick. Pre-allocate label sets at registration time for zero-alloc metric updates. | Backend / scheduler | Small | Performance | 14 |
| 42 | PERF-025 | Pass shutdown context to notification workers | Notification dispatcher worker uses context.Background() for delivery. Should use shutdown-aware context. | Backend / notification | Small | Performance | 14 |
| 43 | QUAL-002 | Add type guards for WebSocket message assertions | Unchecked type assertions in WS message handler may panic on malformed messages in unexpected states. | Backend / hub messages | Small | Quality | 14 |
| 44 | QUAL-006 | Use CSS custom properties for semantic colors | Hardcoded Tailwind color classes bypass the theme system. Use `var(--color-*)` tokens for consistent theme switching. | Frontend / components | Medium | Quality | 14 |
| 45 | QUAL-008 | Separate data fetching from presentation components | Several route components mix `fetch` calls with rendering logic, making them harder to test and reuse. | Frontend / routes | Medium | Quality | 14 |
| 46 | QUAL-011 | Add keyboard navigation to tab components | Tab widgets lack `role="tablist"` and arrow key support, failing WCAG 2.1 AA keyboard accessibility requirements. | Frontend / components | Small | Quality | 14 |
| 47 | QUAL-024 | Fix notification processJob half-handling | Error from processJob is logged but the job is also marked as failed — double handling pattern. Choose one error path. | Backend / notification | Small | Quality | 14 |
| 48 | QUAL-025 | Route timescale/retention queries through sqlc | Direct SQL queries in timescale and retention stores bypass the sqlc layer, reducing type safety and query auditability. | Backend / store | Medium | Quality | 14 |
| 49 | QUAL-026 | Propagate context to proto registry compilation | Proto registry uses `context.Background()` for compilation. Should accept the server startup context for cancellation support. | Backend / monitor | Small | Quality | 14 |
| 50 | QUAL-027 | Use %w verb in reflect.go error wrapping | Missing `%w` verb breaks `errors.Is`/`errors.As` chains, making error inspection unreliable for callers. | Backend / monitor | Small | Quality | 14 |

#### Priority 15 — Low Risk / Possible

| # | ID(s) | Title | Description | Component | Effort | Impact | Priority |
|---|--------|-------|-------------|-----------|--------|--------|----------|
| 51 | SEC-WS-LEN | Enforce maximum token length on WS query parameter | No length limit on the token query parameter. Extremely long values could cause memory allocation issues in URL parsing. | Backend / WS handler | Small | Security | 15 |
| 52 | PERF-WS-TAB | Add cross-tab WebSocket coordination | Multiple browser tabs each maintain independent WS connections and reconnection timers. A SharedWorker or BroadcastChannel could consolidate. | Frontend / WS client | Medium | Performance | 15 |
| 53 | QUAL-028 | Make Hub.Run() goroutine context-aware | Hub.Run() uses an infinite loop without selecting on context cancellation. Shutdown relies on channel close rather than context propagation. | Backend / hub | Small | Reliability | 15 |

---

### Dependency Upgrade Recommendations

#### Frontend Dependencies

| Package | Current | Latest Stable | Status | Recommendation |
|---------|---------|---------------|--------|----------------|
| `vite` | ^5.4.10 | 6.x | 1 major behind | Upgrade to Vite 6. Vite 5 is in maintenance mode; active security patches target Vite 6 only. |
| `vitest` | ^2.1.8 | 3.x | 1 major behind | Upgrade to Vitest 3. Aligns with Vite 6, improved snapshot isolation. |
| `tailwindcss` | ^3.4.14 | 4.x | 1 major behind | Evaluate Tailwind CSS 4 migration when ecosystem stabilizes. Non-urgent — v3 still maintained. |

#### Backend Dependencies

| Package | Current | Latest Stable | Status | Recommendation |
|---------|---------|---------------|--------|----------------|
| `gorilla/websocket` | v1.5.3 | v1.5.x | Archived | Maintenance-only since 2024. No new CVE patches will be issued. Consider `nhooyr.io/websocket` long-term. |
| `golang.org/x/crypto` | v0.48.0 | Latest | Current | Keep updated — frequently receives CVE patches. Configure automated dependency updates. |

No critical CVEs were identified in the current dependency set. Configure Dependabot or Renovate for both `frontend/package.json` and `backend/go.mod` with weekly security-only update checks.

---

### Summary

| Category | Count |
|----------|-------|
| Quick Wins (Security/Reliability + Small effort) | 11 |
| Strategic Improvements (Security/Reliability + Large effort) | 2 |
| Total actionable improvements | 53 |
| Dependency upgrades recommended | 3 (Vite 6, Vitest 3, Tailwind 4 evaluation) |

**Recommended execution order:**
1. **Quick Wins** (1–2 sprints) — address all 11 items for immediate risk reduction
2. **Priority 10 Medium-effort items** (2–3 sprints) — SEC-001, PERF-021, QUAL-007
3. **Strategic: SEC-004 token rotation** (dedicated sprint) — largest auth risk
4. **Strategic: SEC-006 CSP + httpOnly** (dedicated sprint) — defense-in-depth completion
5. **Remaining Priority 13–15 items** — backlog, address opportunistically

---

## 7. Future Roadmap

This roadmap recommends strategic investments that go beyond individual finding remediation. Items are organized by timeframe and category, building on the patterns identified across the security, performance, and code quality audit domains.

---

### Short-Term (1–3 Months)

These items address foundational gaps that block further maturity. Most relate directly to high-priority findings but require architectural effort beyond a single-file fix.

#### Architecture

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-01 | **Content Security Policy (CSP)** | Implement a strict CSP with `script-src 'self'`, `style-src 'self' 'unsafe-inline'` (for Tailwind), and `connect-src` restricted to the API origin. Primary defense-in-depth for JWT-in-localStorage (SEC-001, SEC-006). | None |
| R-02 | **Graceful shutdown sequence** | Refactor main shutdown: (1) stop HTTP listener, (2) drain in-flight HTTP, (3) stop scheduler, (4) drain notification workers, (5) close WebSocket hub, (6) close DB pool. Addresses QUAL-020 and QUAL-029. | None |
| R-03 | **CI/CD pipeline** | Implement GitHub Actions with: `go test ./...`, `go vet`, `golangci-lint`, `pnpm test`, OpenAPI validation, Docker build, image push. Prerequisite for all automated quality gates. | None |
| R-04 | **Token rotation mechanism** | Implement short-lived access tokens (15 min) with refresh token in HttpOnly cookie. Addresses SEC-004 (24h static session). | R-01 (CSP protects the refresh cookie) |

#### Security

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-05 | **Rate limiting middleware** | Per-IP rate limiting on `/auth/login` (5/min), `/auth/setup` (3/min), global 100 req/s per IP. Token bucket with in-memory store. | None |
| R-06 | **Key rotation scope expansion** | Extend rotation to re-encrypt notification channel credentials within same transaction. Addresses SEC-011. | None |
| R-07 | **Log redaction hardening** | Extend `SanitizeLog` to cover WS upgrade paths. Add structured logging middleware for Authorization header redaction at gin level. Addresses SEC-013. | None |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-08 | **Error boundary pages** | Add SvelteKit `+error.svelte` at root and per-route-group. Addresses QUAL-001. | None |
| R-09 | **Bundle size CI guard** | Add `size-limit` CI step failing if initial JS > 200 KB gzipped or any route chunk > 30 KB gzipped. Addresses PERF-003. | R-03 (CI pipeline) |

---

### Medium-Term (3–6 Months)

These items improve operational maturity and prepare for growth beyond single-instance deployment.

#### Architecture

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-10 | **E2E test infrastructure** | Introduce Playwright for critical user paths: login, monitor CRUD, real-time WS updates, notification config. Target 10–15 scenarios. | R-03 (CI pipeline) |
| R-11 | **API versioning strategy** | Define deprecation policy for `/api/v1`. Introduce `Sunset`/`Deprecation` headers. Prepare router for `/api/v2`. | None |
| R-12 | **WebSocket auth refactor** | Consolidate WS auth with HTTP middleware (shared function). Move token from query parameter to first message after upgrade. Addresses SEC-002, SEC-014. | R-04 (token rotation) |

#### Scalability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-13 | **Connection pool auto-tuning** | Pool sizing derived from `PULSE_SCHEDULER_WORKERS + PULSE_NOTIFICATION_WORKERS + 20`. Add pool metrics to Prometheus. Addresses PERF-020. | None |
| R-14 | **Notification fan-out batch queries** | Replace N+1 pattern with batch query for all triggered bindings in single round-trip. Addresses PERF-021. | None |
| R-15 | **TimescaleDB native retention** | Replace custom DELETE with `add_retention_policy()`. Addresses PERF-022. | None |

#### Observability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-16 | **Structured logging migration** | Replace `log.Printf`/`log.Fatalf` with `slog` (Go 1.21+). JSON in production, text in dev. Resolves SEC-012. | None |
| R-17 | **Platform self-health endpoint** | Extend `/healthz` with component-level health: DB, pool utilization, scheduler, dispatcher, hub. Expose as Prometheus gauges. | None |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-18 | **API documentation site** | Generate docs from `openapi.yaml` using Redoc or Stoplight. Serve at `/docs` in production. | None |
| R-19 | **Developer onboarding guide** | Write `docs/CONTRIBUTING.md` covering local setup, running tests, adding checkers/channels/routes. | None |
| R-20 | **Frontend test coverage expansion** | Integration tests for notification management, monitor CRUD, WS reconnection. Target zero-coverage modules from QUAL-007. | R-10 (E2E infra) |

---

### Long-Term (6–12 Months)

These items address scalability beyond 500 monitors and prepare for multi-instance deployment.

#### Scalability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-21 | **Horizontal scheduler scaling** | Distributed lock (PostgreSQL advisory locks or Redis) for multi-instance workload sharing via consistent hashing. | R-13, R-15 |
| R-22 | **Read replica support** | Secondary read-only pool for list/history queries. Route GET endpoints to replica. | R-13 |
| R-23 | **Queue-based notification dispatch** | External queue (PostgreSQL LISTEN/NOTIFY or Redis Streams) for cross-process notification workers. | R-14, R-16 |
| R-24 | **Check result streaming pipeline** | Event bus (PostgreSQL logical replication or NATS) between scheduler and consumers. Decouples producers from consumers. | R-21 |

#### Observability

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-25 | **Distributed tracing** | OpenTelemetry spans across HTTP → DB → notification. Export to Jaeger or OTLP backend. | R-16 |
| R-26 | **Self-alerting** | Pulse monitors itself: `/healthz`, DB health, notification delivery success rate. Proves pipeline works end-to-end. | R-17 |
| R-27 | **Operational dashboard** | Grafana dashboard: scheduler throughput, check latency, WS clients, notification success, pool utilization. | R-17, R-13 |

#### Security

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-28 | **Audit logging** | Record admin actions in `audit_log` table with actor, action, resource, timestamp, IP. Prepares for compliance. | R-16 |
| R-29 | **RBAC preparation** | Role-based access model (admin, operator, viewer). Middleware reads roles from JWT claims. | R-04 |
| R-30 | **Secret scanning in CI** | Add `gitleaks` or `trufflehog` to prevent accidental secret commits. | R-03 |

#### Developer Experience

| # | Item | Description | Dependencies |
|---|------|-------------|--------------|
| R-31 | **Plugin architecture for checkers** | Refactor checker registry for external protocol plugins (Go plugin or subprocess). | R-11 |
| R-32 | **Development workflow automation** | Add `make lint`, `make fmt`, `make check-all`. Pre-commit hooks via `lefthook`. | R-03 |
| R-33 | **Storybook for UI components** | Set up Histoire (Svelte 5) for isolated component development and visual regression testing. | R-10 |

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

---

## 8. Appendices

### 8.1 Requirements Coverage Matrix

This matrix maps each requirement from the audit specification to the findings that address it, or explicitly notes "no issues found" where the system meets the requirement.

| Requirement | Description | Findings | Status |
|-------------|-------------|----------|--------|
| **1.1** | JWT storage mechanism across browser persistence layers | SEC-001 | Documented — localStorage only |
| **1.2** | Token transmission paths for exposure vectors | SEC-002 | Documented — URL query param logged |
| **1.3** | Token clearing on 401/4401 removes from state and storage | — | No issues found — clearToken() correctly removes from state and localStorage |
| **1.4** | JWT client-side validation (expiry, signature) | SEC-003 | Documented — no validation present |
| **1.5** | Token refresh/rotation, session lifetime classification | SEC-004 | Documented — Medium (24h > 8h threshold) |
| **1.6** | Finding format with file, line, severity, remediation | All findings | All use consistent template |
| **1.7** | WS token max length + URL logging | SEC-002, SEC-WS-LEN | Documented — no max length, logs full URL |
| **2.1** | User-input components verify sanitization before DOM/API | — | No issues found — all inputs use framework-default escaping |
| **2.2** | {@html} directives classified as safe/unsafe | SEC-010 | Documented — 1 instance, Low risk (server-generated data) |
| **2.3** | WS message handler rejects invalid JSON, typed consumption | — | No issues found — invalid JSON discarded, typed fields used as text |
| **2.4** | URL construction checks for path traversal and encoding | — | No issues found — API IDs are server-generated UUIDs, WS token properly encoded |
| **2.5** | API error messages rendered as text only (no {@html}) | — | No issues found — all error messages use text interpolation |
| **2.6** | Findings report for each XSS risk | SEC-010 | Only 1 finding — strong XSS posture overall |
| **3.1** | Secret values not persisted beyond creation | — | No issues found — show-once pattern, component state cleared on dismiss |
| **3.2** | DevTools/storage do not expose secret values | — | No issues found — API returns redacted values after creation |
| **3.3** | No console statements referencing secrets in production | — | No issues found — zero console.log in production code |
| **3.4** | Source maps absent or access-controlled | SEC-005 | Documented — implicit suppression, no explicit config |
| **3.5** | No server-side env vars in frontend bundle | — | No issues found — zero matches for PULSE_SECRET_KEY et al. |
| **3.6** | Critical data exposure classification | SEC-005, SEC-006 | No cleartext secrets exposed; defense-in-depth gaps noted |
| **4.1** | All /api/v1 endpoints require auth except public routes | — | No issues found — only login, setup, healthz are public |
| **4.2** | JWT restricts to HMAC-only, rejects alg:none | — | No issues found — correct type assertion in all 3 validation points |
| **4.3** | Bcrypt cost ≥10, dummy hash on failure paths | SEC-009 | Partial gap — cost 10 correct, but combinedAuth missing dummy on empty header |
| **4.4** | WS auth returns identical error, no info disclosure | SEC-008 | Documented — different messages, timing gap on empty token |
| **4.5** | Combined auth identical 401 for JWT and API token | SEC-009 | Timing gap on empty header path |
| **4.6** | Single-admin resource isolation | — | No issues found — single-admin by design, documented |
| **4.7** | Origin header validation on WS upgrade | SEC-007 | Documented — CheckOrigin always returns true |
| **5.1** | AES-256-GCM nonce via crypto/rand, key validation, tamper detection | — | No issues found — correct implementation |
| **5.2** | Write-only semantics across all secret-bearing endpoints | — | No issues found — all endpoints correctly redact |
| **5.3** | Log sanitization for auth headers, passwords, tokens | SEC-013 | Documented — WS token not redacted from path |
| **5.4** | Parameterized queries, no SQL injection | SEC-015 | No injection risk — one handler bypasses sqlc but uses params |
| **5.5** | Key rotation transactional integrity and timeout | SEC-011, SEC-012 | Documented — incomplete scope, log.Fatalf prevents rollback |
| **5.6** | API token bcrypt storage and constant-time comparison | — | No issues found — correct implementation |
| **6.1** | VirtualList max 60 DOM nodes, 55fps, <10MB heap growth | PERF-001, PERF-002 | Cap correct; index-based keying suboptimal for recycling |
| **6.2** | Bundle size <200KB gzipped, code-splitting gaps | PERF-003, PERF-004, PERF-005 | Bundle healthy; 2 route-level items noted |
| **6.3** | Svelte 5 reactivity (runes, derived depth, subscription cleanup) | PERF-006, PERF-009, PERF-010 | HistoryChart uses legacy API; rest is clean |
| **6.4** | uPlot lifecycle — no retained references after unmount | PERF-007 | No issues found — correctly destroyed |
| **6.5** | CSS strategy — unused rules, render-blocking, theme switch | PERF-011 (informational) | No issues found — efficient strategy |
| **6.6** | i18n lazy-loading — only active locale in initial bundle | PERF-012 (informational) | No issues found — correctly implemented |
| **7.1** | WS reconnection thundering herd prevention | PERF-013 (WS-TAB) | Correctly implemented with backoff+jitter; no cross-tab coordination |
| **7.2** | Patch-merge O(1) efficiency | PERF-015 | No issues found — all criteria met |
| **7.3** | API request patterns — refetching, deduplication | PERF-010 | Documented — no deduplication for concurrent requests |
| **7.4** | AbortController wiring on fetch calls | PERF-011 | Documented — timeout works, unmount cancellation missing |
| **7.5** | Static asset caching — Cache-Control, content hashes | PERF-014 | Documented — hashed assets correct, fonts miss cache |
| **7.6** | Full refetch on WS reconnection | PERF-016 | No issues found — correctly implemented |
| **8.1** | Bounded worker pool, fair distribution, no goroutine growth | PERF-026 | No issues found — correct backpressure design |
| **8.2** | WS hub broadcast fan-out to 200 clients, slow-consumer eviction | PERF-027 | No issues found — correct design |
| **8.3** | DB connection pool sizing, N+1 patterns, indexes | PERF-020, PERF-021 | Documented — pool undersized, N+1 in notification fan-out |
| **8.4** | Notification dispatcher independence (non-blocking, separate pool) | PERF-028 | No issues found — correct independence |
| **8.5** | TimescaleDB hypertable, retention policy, query performance | PERF-022 | Documented — no native retention policy |
| **8.6** | Report metric violations with observed values and remediation | All PERF findings | Template followed for all findings |
| **9.1** | TypeScript strictness — any types, missing returns, nullable access | QUAL-002, QUAL-003 | Documented — test files only; production is clean |
| **9.2** | Error handling — uncaught promises, missing boundaries, silent failures | QUAL-001, QUAL-009, QUAL-012 | Documented — missing error boundaries and silent catches |
| **9.3** | Component architecture — mixed concerns, prop drilling, state patterns | QUAL-008 | Documented — data/presentation mixing in some components |
| **9.4** | Test coverage — auth flows, WS reconnection, error states | QUAL-007 | Documented — notification module at 0% coverage |
| **9.5** | Accessibility — keyboard, ARIA, contrast, accessible names | QUAL-010, QUAL-011 | Documented — tab navigation gap, color-only state indicators |
| **9.6** | Convention adherence — CSS vars, i18n t(), theme selectors | QUAL-004, QUAL-005, QUAL-006 | Documented — dark: prefix violation, hardcoded strings, hardcoded colors |
| **9.7** | Finding format with category, file, line, description, expected pattern | All QUAL findings | Template followed for all findings |
| **10.1** | Error handling — missing %w, sentinel errors, double-handling | QUAL-024, QUAL-027 | Documented — missing %w, half-handling pattern |
| **10.2** | Package structure — unexported symbols, oversized interfaces | — | No issues found — clean package boundaries |
| **10.3** | Context propagation — Background/TODO in handlers, missing timeouts | QUAL-021, QUAL-022, QUAL-025, QUAL-026 | Documented — 4 instances of context.Background() misuse |
| **10.4** | Resource leaks — unclosed bodies, DB rows, goroutines | QUAL-023, QUAL-028 | Documented — Hub data race, Run() not context-aware |
| **10.5** | Graceful shutdown — drain sequence | QUAL-020, QUAL-029 | Documented — no HTTP drain, incorrect ordering |
| **10.6** | All DB queries use sqlc layer | SEC-015, QUAL-025 | Documented — proto handler and timescale/retention bypass sqlc |
| **10.7** | Finding format with file, severity, category, description | All QUAL findings | Template followed for all findings |
| **11.1** | Improvements section with title, description, component, effort, impact, priority | Section 6 | ✅ Complete |
| **11.2** | Effort categories (Small/Medium/Large) and impact categories | Section 6 | ✅ Complete |
| **11.3** | Priority via risk matrix (severity × likelihood, 1-16) | Section 6, findings-scored.md | ✅ Applied to all findings |
| **11.4** | Quick wins and strategic improvements subsections | Section 6 | ✅ Included |
| **11.5** | Dependency upgrade recommendations with CVEs | Section 6 (Dependency Upgrades) | ✅ 3 frontend upgrades recommended |
| **12.1** | Roadmap with short/medium/long-term timeframes | Section 7 | ✅ Complete |
| **12.2** | Architectural improvements beyond point fixes | R-01, R-02, R-03, R-04, R-10, R-11, R-12 | ✅ Covered |
| **12.3** | Scalability beyond 500-monitor target | R-13, R-14, R-15, R-21, R-22, R-23, R-24 | ✅ Covered |
| **12.4** | Observability improvements | R-16, R-17, R-25, R-26, R-27 | ✅ Covered |
| **12.5** | Security hardening for self-hosted deployment | R-05, R-06, R-07, R-28, R-29, R-30 | ✅ Covered |
| **12.6** | Developer experience improvements | R-08, R-09, R-18, R-19, R-20, R-31, R-32, R-33 | ✅ Covered |
| **13.1** | Section ordering: Exec Summary → Methodology → Findings → Improvements → Roadmap → Appendices | Document structure | ✅ Correct ordering |
| **13.2** | Executive summary with counts, risk posture, top-3 | Section 1 | ✅ Complete |
| **13.3** | Consistent finding template (ID, Severity, Category, etc.) | All findings | ✅ All use template |
| **13.4** | Methodology section with scope, techniques, limitations | Section 2 | ✅ Complete |
| **13.5** | Single markdown file at docs/AUDIT.md | This document | ✅ Delivered |
| **13.6** | Code snippets (max 10 lines) as evidence | All findings | ✅ All include evidence |
| **13.7** | Findings ordered by severity then ID within each domain | Sections 3, 4, 5 | ✅ Ordered |
| **13.8** | Table of contents matching section headings | Top of document | ✅ Included |

---

### 8.2 Finding ID Index

Alphabetical index of all findings in this report.

| ID | Section | Severity | Title |
|----|---------|----------|-------|
| PERF-001 | 4 | Informational | VirtualList DOM node cap correctly enforced |
| PERF-002 | 4 | Medium | VirtualList uses index-based keying |
| PERF-003 | 4 | Informational | Initial JS bundle well under 200 KB target |
| PERF-004 | 4 | Low | CodeMirror 127 KB gzipped but properly code-split |
| PERF-005 | 4 | Low | Route node 5 at 39 KB exceeds threshold |
| PERF-006 | 4 | Medium | HistoryChart uses legacy API, no reactivity |
| PERF-007 | 4 | Informational | uPlot properly destroyed on unmount |
| PERF-008 | 4 | Low | HistoryChart reads computed styles per mount |
| PERF-009 | 4 | Informational | Svelte 5 reactivity patterns clean |
| PERF-010 | 4 | Medium | No request deduplication |
| PERF-011 | 4 | Medium | No AbortController on component unmount |
| PERF-012 | 4 | Low | Reconnect refetch redundancy |
| PERF-013 | 4 | Low | No bundle size CI guard |
| PERF-014 | 4 | Medium | Static fonts miss immutable cache |
| PERF-015 | 4 | Informational | Patch-merge pattern efficient |
| PERF-016 | 4 | Informational | Full refetch on reconnect correct |
| PERF-020 | 4 | High | DB connection pool undersized |
| PERF-021 | 4 | Medium | N+1 query in notification fan-out |
| PERF-022 | 4 | Medium | No TimescaleDB native retention |
| PERF-023 | 4 | Low | Hub ClientCount() data race |
| PERF-024 | 4 | Low | Scheduler rebuilds Prometheus labels per tick |
| PERF-025 | 4 | Low | Notification worker uses context.Background() |
| PERF-026 | 4 | Informational | Scheduler backpressure correct |
| PERF-027 | 4 | Informational | Hub slow-consumer eviction correct |
| PERF-028 | 4 | Informational | Notification dispatcher independent |
| QUAL-001 | 5 | Medium | Missing SvelteKit error boundary pages |
| QUAL-002 | 5 | Low | Unchecked type assertions in WS handler |
| QUAL-003 | 5 | Informational | any types in test files only |
| QUAL-004 | 5 | Low | Tailwind dark: prefix convention violation |
| QUAL-005 | 5 | Medium | Hardcoded strings not using t() |
| QUAL-006 | 5 | Low | Hardcoded semantic colors |
| QUAL-007 | 5 | Medium | No test coverage for notifications |
| QUAL-008 | 5 | Low | Data fetching mixed with presentation |
| QUAL-009 | 5 | Low | Silent catch blocks in auth store |
| QUAL-010 | 5 | Informational | Color-only state indicators (with labels) |
| QUAL-011 | 5 | Low | Tab navigation lacks tablist/arrow keys |
| QUAL-012 | 5 | Informational | Uncaught promise in monitor create |
| QUAL-020 | 5 | High | HTTP server lacks graceful shutdown |
| QUAL-021 | 5 | Medium | Dispatcher workers use context.Background() |
| QUAL-022 | 5 | Medium | ICMP resolver ignores check timeout |
| QUAL-023 | 5 | Medium | Hub.ClientCount() data race |
| QUAL-024 | 5 | Low | processJob half-handling pattern |
| QUAL-025 | 5 | Low | Timescale/retention bypass sqlc |
| QUAL-026 | 5 | Low | Proto registry uses context.Background() |
| QUAL-027 | 5 | Low | Missing %w in reflect.go |
| QUAL-028 | 5 | Low | Hub.Run() not context-aware |
| QUAL-029 | 5 | Medium | Shutdown sequence incorrect ordering |
| SEC-001 | 3 | Medium | JWT in localStorage — XSS exfiltration |
| SEC-002 | 3 | Medium | WS token in URL — log exposure |
| SEC-003 | 3 | Low | No client-side JWT expiry check |
| SEC-004 | 3 | Medium | No token refresh — 24h static session |
| SEC-005 | 3 | Low | No explicit source map suppression |
| SEC-006 | 3 | Medium | localStorage JWT with CSP gap |
| SEC-007 | 3 | Medium | WS upgrader allows all origins |
| SEC-008 | 3 | Medium | WS auth information disclosure |
| SEC-009 | 3 | Low | Missing dummy bcrypt on empty header |
| SEC-010 | 3 | Low | {@html} for chart tooltips |
| SEC-011 | 3 | High | Key rotation missing notification data |
| SEC-012 | 3 | Medium | Key rotation log.Fatalf prevents rollback |
| SEC-013 | 3 | Medium | Log sanitization misses WS token |
| SEC-014 | 3 | Low | WS auth duplicates combinedAuth |
| SEC-015 | 3 | Low | Proto handler bypasses sqlc |

---

*End of Pulse Security and Performance Audit Report*
