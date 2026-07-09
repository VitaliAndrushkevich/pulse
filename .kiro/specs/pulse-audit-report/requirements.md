# Requirements Document

## Introduction

A comprehensive audit document for the Pulse uptime monitoring platform. The deliverable is a structured read-only audit report covering security, performance, code quality, improvements, and future roadmap. No code changes are implemented — the output is a document with categorized findings by severity. The frontend is the primary audit focus, with backend reviewed as secondary priority.

## Glossary

- **Audit_Report**: The structured markdown document produced as the deliverable, containing all findings organized by domain and severity
- **Auditor**: The agent or person executing the audit process according to these requirements
- **Finding**: A single observation documented with severity, description, evidence location, and remediation recommendation
- **Severity_Level**: Classification of finding impact — Critical, High, Medium, Low, or Informational
- **Frontend_Application**: The SvelteKit 5 application in `frontend/` including API client, WebSocket client, stores, components, and routes
- **Backend_Application**: The Go binary in `backend/` including API handlers, scheduler, WebSocket hub, notification dispatcher, and data layer
- **Security_Domain**: Audit area covering authentication, authorization, data exposure, injection, XSS, CSRF, token handling, and secrets management
- **Performance_Domain**: Audit area covering rendering efficiency, bundle size, network requests, memory usage, WebSocket throughput, and database query patterns
- **Code_Quality_Domain**: Audit area covering TypeScript strictness, error handling patterns, test coverage, accessibility compliance, and adherence to project conventions

## Requirements

### Requirement 1: Security Audit — Frontend Authentication and Token Handling

**User Story:** As a platform operator, I want the audit to evaluate frontend authentication security, so that I can identify token exposure risks and session management weaknesses.

#### Acceptance Criteria

1. THE Auditor SHALL examine JWT storage mechanism across all browser persistence layers (localStorage, sessionStorage, cookies, IndexedDB) and document risks using a four-level Severity_Level classification: Critical (token exposed to unauthorized parties without user action), High (token exposed under attacker-controlled conditions), Medium (suboptimal practice increasing attack surface), Low (defense-in-depth improvement with no direct exploit path)
2. THE Auditor SHALL evaluate token transmission paths (Bearer header on API requests, query parameter on WebSocket) for exposure vectors including browser history, server logs, and referrer leakage
3. THE Auditor SHALL verify that token clearing on 401 responses and auth expiry (WS close code 4401) removes the token from both reactive state and all browser storage (localStorage key, sessionStorage, cookies), and SHALL document any code path where a stale token reference could persist in a JavaScript closure or module-scoped variable after clearToken is invoked
4. THE Auditor SHALL assess whether the frontend performs any JWT validation (expiry check, signature verification) or relies entirely on server-side rejection
5. THE Auditor SHALL document whether automatic token refresh or rotation mechanisms exist, record the effective session lifetime (from token issuance to forced re-authentication), and classify the risk as High if session lifetime exceeds 24 hours without re-validation, Medium if it exceeds 8 hours, or Low if 8 hours or less
6. WHEN a finding is documented, THE Audit_Report SHALL include the file path, line number range, Severity_Level (Critical/High/Medium/Low), description of the vulnerability or weakness, and a recommended remediation that specifies the target state (what should change) and the security property it restores
7. WHEN the WebSocket token is transmitted via query parameter, THE Auditor SHALL verify whether the server enforces a maximum token length of 2048 characters and SHALL document whether connection upgrade requests log the full URL (exposing the token in server access logs)

### Requirement 2: Security Audit — Frontend Input Handling and XSS Prevention

**User Story:** As a platform operator, I want the audit to assess XSS and injection risks in the frontend, so that I can understand whether user inputs are safely handled.

#### Acceptance Criteria

1. THE Auditor SHALL examine all user-input-accepting components (MonitorForm, login form, settings forms, notification channel forms) and for each text input field verify that submitted values pass through framework-default escaping or explicit sanitization prior to rendering in the DOM or inclusion in API request bodies
2. THE Auditor SHALL identify every `{@html}` directive in Svelte components and for each occurrence determine whether the rendered content originates from user-controlled data (direct input, API response containing user-submitted values, or WebSocket payloads), documenting each as safe (static/system-generated content only) or unsafe (user-controlled content rendered without sanitization)
3. THE Auditor SHALL verify that the WebSocket message handler (`ws.ts`) rejects messages that fail JSON parsing without propagating content to the DOM, and that typed payload fields (e.g., `monitor_id`, `state`, `error`) are consumed only as text node content or attribute values via Svelte template bindings rather than injected as raw HTML
4. THE Auditor SHALL check URL construction in the API client (`api.ts`) and WebSocket client (`ws.ts`) and verify that dynamic path segments (monitor IDs, credential IDs, binding IDs) do not permit path traversal (e.g., values containing `../` or `/`) and that query parameter values are encoded via `URLSearchParams` or `encodeURIComponent` before inclusion in request URLs
5. THE Auditor SHALL verify that error messages originating from API responses (the `message` field of the error envelope) are rendered exclusively as text content via Svelte text interpolation (`{expression}`) and are never passed to `{@html}` directives or assigned to `innerHTML`
6. WHEN the audit is complete, THE Auditor SHALL produce a findings report listing each identified risk with its location (file and line), severity (critical, high, medium, low), affected data flow (input source to render point), and a recommended remediation action

### Requirement 3: Security Audit — Frontend Data Exposure and Privacy

**User Story:** As a platform operator, I want the audit to identify data exposure risks in the frontend, so that I can prevent sensitive information from leaking to unauthorized parties.

#### Acceptance Criteria

1. THE Auditor SHALL verify that secret values (API tokens, SMTP passwords, webhook headers, JWT secrets) are never persisted in frontend reactive state (Svelte stores), localStorage, or sessionStorage beyond the initial creation response, and that once the user navigates away from the creation view, no cleartext secret value remains accessible in any client-side storage or in-memory store
2. THE Auditor SHALL verify that browser DevTools (Network tab responses, localStorage entries, sessionStorage entries, Application tab cookies) do not expose secret values (API token raw values, SMTP passwords, webhook header values, encryption keys) — confirming that API responses for these resources return redacted or omitted secret fields after creation
3. THE Auditor SHALL verify that the production build output (generated JavaScript bundles) contains zero `console.log`, `console.debug`, or `console.info` statements that reference authentication tokens, passwords, API keys, or request/response bodies containing secret fields
4. THE Auditor SHALL verify that source maps (.map files) are either absent from the static adapter build output directory or are served only to authenticated requests with a valid session — confirming by requesting .map file URLs without authentication and receiving a 403 or 404 response
5. THE Auditor SHALL verify that the static adapter output (HTML, JS, and CSS files in the build directory) does not contain any values from server-side environment variables (PULSE_SECRET_KEY, PULSE_JWT_SECRET, DATABASE_URL, PULSE_METRICS_PASSWORD) by searching all generated bundle files for these variable names and their known values
6. IF the Auditor identifies any cleartext secret value exposed in frontend state, DevTools-accessible storage, production console output, publicly accessible source maps, or embedded environment variables, THEN THE Auditor SHALL classify the finding as a critical data exposure risk and document the exact location (file path, storage key, or network response field) where the exposure occurs

### Requirement 4: Security Audit — Backend Authentication and Authorization

**User Story:** As a platform operator, I want the audit to evaluate backend auth mechanisms, so that I can confirm the API and WebSocket endpoints are properly protected.

#### Acceptance Criteria

1. THE Auditor SHALL verify that all API endpoints under `/api/v1` require authentication (JWT or API token) except explicitly public endpoints (`POST /auth/login`, `GET /auth/setup`, `POST /auth/setup`, `GET /healthz`)
2. THE Auditor SHALL verify that JWT implementation restricts signing algorithms to HMAC-only (rejects tokens with `alg: "none"` or non-HMAC algorithms), validates signature against the configured secret, and rejects tokens with an `exp` claim in the past
3. THE Auditor SHALL verify that API token hashing uses bcrypt with a cost factor of at least 10 and that all authentication failure paths (missing token, invalid prefix, no matching hash, revoked token, expired token) execute a dummy bcrypt comparison to maintain uniform response timing within 50ms variance
4. WHEN WebSocket authentication fails, THE Auditor SHALL verify that the endpoint returns a static error response identical in structure and message to the REST API 401 response, executes a dummy bcrypt comparison on failure paths, and does not disclose whether the failure was due to an invalid signature, expired token, or non-existent token
5. THE Auditor SHALL verify that the combined auth middleware produces identical HTTP 401 response bodies, identical response headers, and applies the same token expiry and revocation checks regardless of whether the credential is a JWT or an API token
6. IF the system supports only a single admin role, THEN THE Auditor SHALL verify that authenticated requests cannot access or modify resources belonging to a different user ID than the one encoded in the authenticated token (JWT `user_id` claim or API token owner)
7. THE Auditor SHALL verify that the WebSocket upgrade endpoint validates the Origin header against a configured allowlist in non-development mode and rejects upgrade requests from unrecognized origins with an HTTP 403 response

### Requirement 5: Security Audit — Backend Data Protection

**User Story:** As a platform operator, I want the audit to assess backend data protection, so that I can confirm secrets are encrypted and sensitive data is not leaked through APIs or logs.

#### Acceptance Criteria

1. THE Auditor SHALL verify AES-256-GCM encryption implementation confirming that each encryption operation generates a unique nonce via crypto/rand (no two stored ciphertexts share the same 12-byte nonce prefix), that the key is loaded as a base64-decoded 32-byte value validated at startup, and that decryption rejects tampered ciphertext by returning an authentication error
2. THE Auditor SHALL assess that API responses from all secret-bearing endpoints (secrets CRUD, API tokens, SMTP settings, notification channel credentials, monitor credentials) never return raw secret values, and that write-only semantics are enforced by confirming GET and LIST responses omit or mask encrypted field content across every endpoint that stores encrypted data
3. THE Auditor SHALL evaluate application log output across request handling, scheduler execution, and notification delivery for exposure of auth headers (Authorization, Bearer tokens), passwords, API tokens, and encryption keys, and verify that the SanitizeLog function replaces detected secret patterns with a fixed redaction placeholder and suppresses entries where redaction cannot be confirmed complete
4. THE Auditor SHALL check database queries for SQL injection vectors by confirming all query construction uses parameterized sqlc-generated code or pgx parameterized queries, and that no string concatenation or fmt.Sprintf constructs SQL statements with user-supplied input
5. THE Auditor SHALL assess the key rotation mechanism confirming it executes all re-encryption (secrets and credentials) within a single database transaction that rolls back entirely on any per-record failure, completes within the 2-minute context timeout, exits with a non-zero status code on failure, and leaves all stored values unchanged if any step fails
6. THE Auditor SHALL verify that API tokens are stored as bcrypt hashes with the raw token disclosed only in the creation response, and that token validation uses constant-time comparison with a dummy bcrypt hash on authentication failure to prevent timing side-channel attacks

### Requirement 6: Performance Audit — Frontend Rendering and Bundle

**User Story:** As a platform operator, I want the audit to evaluate frontend rendering performance, so that I can confirm the UI remains responsive with 500+ monitors.

#### Acceptance Criteria

1. THE Auditor SHALL evaluate the VirtualList implementation verifying that rendered DOM nodes never exceed the configured maximum of 60 at any scroll position, that scrolling 500+ items maintains a frame rate of at least 55 fps, and that heap memory growth does not exceed 10 MB over 5 minutes of continuous scrolling
2. THE Auditor SHALL assess bundle size verifying the initial JS bundle does not exceed 200 KB gzipped, and SHALL identify code-splitting gaps where any single dependency contributes more than 50 KB gzipped to the initial bundle or any route-specific module larger than 20 KB gzipped is loaded in the initial bundle instead of being lazy-loaded
3. THE Auditor SHALL evaluate Svelte 5 reactivity patterns (runes usage) verifying no component re-renders when none of its reactive inputs have changed, that derived state chains do not exceed 3 levels of depth, and that all store subscriptions are cleaned up when the owning component is destroyed
4. THE Auditor SHALL check that chart rendering (uPlot) leaves no retained references to destroyed instances and no detached canvas elements after the parent component unmounts, confirmed by heap snapshot comparison before mount and after unmount showing zero uPlot-related growth
5. THE Auditor SHALL assess CSS strategy (Tailwind + custom properties) verifying that unused CSS rules do not exceed 10% of total stylesheet size, that only critical-path styles are render-blocking, and that theme switching via the data-theme attribute completes repaint within 100 ms
6. THE Auditor SHALL evaluate i18n lazy-loading verifying that only the active locale is included in the initial bundle, that the remaining 12 locales are loaded on demand via dynamic import, and that total i18n contribution to the initial bundle does not exceed 15 KB gzipped

### Requirement 7: Performance Audit — Frontend Network and Real-Time

**User Story:** As a platform operator, I want the audit to evaluate network efficiency and WebSocket performance, so that I can confirm real-time updates remain efficient at scale.

#### Acceptance Criteria

1. THE Auditor SHALL assess the WebSocket reconnection strategy (exponential backoff with jitter, 1s base to 30s cap, ±25% jitter) for thundering herd prevention, verifying that 10 or more concurrent browser tabs reconnecting simultaneously do not produce overlapping reconnection attempts within the same jitter window
2. THE Auditor SHALL evaluate the patch-merge update pattern for efficiency when receiving high-frequency monitor status updates (500+ monitors checking every 60 seconds), verifying that each patch applies in O(1) map lookup time, only merges `state` and `last_checked_at` fields, and discards patches for unknown monitor IDs without error
3. THE Auditor SHALL check API request patterns for refetching of identical resources within 5 seconds without an intervening mutation, missing Cache-Control response headers on list endpoints, and duplicate in-flight requests to the same endpoint that could be deduplicated via shared promises
4. THE Auditor SHALL assess the API client timeout (15 seconds) by verifying that the AbortController signal is wired to every fetch call, that the timeout fires and aborts the request if no response arrives within 15 seconds, and that pending requests are cancelled via AbortController when the owning component unmounts or navigates away
5. THE Auditor SHALL evaluate static asset caching strategy by verifying that production responses include Cache-Control headers with max-age of at least 1 year for content-hashed assets, that built filenames contain content hashes for cache-busting on deploy, and that assets are serveable from a separate origin without requiring cookie or session state
6. WHEN the WebSocket connection is lost and the client holds stale monitor state, THE Auditor SHALL verify that the client performs a full monitor list refetch upon successful reconnection before resuming patch-merge processing

### Requirement 8: Performance Audit — Backend Scheduler and Concurrency

**User Story:** As a platform operator, I want the audit to evaluate backend performance at scale, so that I can confirm the system handles 500+ monitors without degradation.

#### Acceptance Criteria

1. THE Auditor SHALL assess the bounded worker pool scheduler for concurrency limiting to the configured PULSE_SCHEDULER_WORKERS value (default 200), verify that no unbounded goroutine growth occurs under load of 500 or more active monitors, verify fair distribution such that no single monitor starves (all due monitors are checked within 2 tick intervals of their next_check_at), and verify that when all workers are busy the job channel blocks senders without dropping checks or panicking
2. THE Auditor SHALL evaluate the WebSocket hub broadcast for fan-out to at least 200 simultaneous connected clients, verify that broadcast latency from hub.Broadcast call to message delivery remains below 100 milliseconds at the 95th percentile, and verify that clients whose send buffer (256 messages) is full are disconnected within one broadcast cycle without blocking delivery to other clients
3. THE Auditor SHALL assess database connection pool configuration for pool size appropriate to the worker count (at least PULSE_SCHEDULER_WORKERS + 20 connections), identify any N+1 query patterns in scheduler poll or notification fan-out paths, verify that indexes exist on all columns used in WHERE and ORDER BY clauses of hot-path queries (ListActiveMonitorsDueWithTags, ListCheckResultsByMonitor, ListBindingsByMonitor), and verify that no connection exhaustion occurs under sustained load of 500 monitors with 1-second tick intervals
4. THE Auditor SHALL evaluate the notification dispatcher for independent throughput by verifying that notification delivery latency does not increase scheduler check-cycle duration by more than 1 millisecond (non-blocking enqueue via select/default), verify that when the notification job buffer (256 capacity) is full new jobs are dropped with a metric increment rather than blocking, and verify that notification workers (PULSE_NOTIFICATION_WORKERS, default 50) do not share a goroutine pool with scheduler workers
5. THE Auditor SHALL check TimescaleDB hypertable configuration on the check_results table for correct time-partitioning on the checked_at column, verify that per-monitor history_retention_days (configurable 1–365 days, default 30) is enforced by a retention policy or scheduled cleanup, and verify that time-range queries over the maximum configured retention window complete within 200 milliseconds at the 95th percentile for a single monitor with 1-second check intervals
6. IF the audit identifies any criterion in criteria 1–5 that does not meet its stated threshold, THEN THE Auditor SHALL report the specific metric value observed, the threshold that was violated, and a remediation recommendation with estimated complexity (low, medium, high)

### Requirement 9: Code Quality Audit — TypeScript and Frontend Patterns

**User Story:** As a developer, I want the audit to assess frontend code quality, so that I can identify areas where best practices are not followed.

#### Acceptance Criteria

1. THE Auditor SHALL evaluate TypeScript strictness compliance by reporting each instance of explicit `any` type usage, missing return-type annotations on exported functions, unchecked nullable access (property access or method call without prior null/undefined guard), and type assertions (`as` casts) that widen types or suppress compiler errors, referencing the file path and line number for each finding
2. THE Auditor SHALL assess error handling patterns across the frontend by reporting: uncaught promise rejections (async calls without `.catch()` or surrounding try/catch), components lacking an error boundary ancestor for async operations, and silent failures defined as catch blocks that neither display user-facing feedback via toast/notification nor re-throw the error
3. THE Auditor SHALL evaluate component architecture by reporting: components that mix data-fetching logic with rendering logic in the same file, prop chains passing through 3 or more intermediate components without consumption, and state managed via local component variables where the project's store pattern (`frontend/src/lib/stores/`) is the established convention for shared state
4. THE Auditor SHALL check test coverage distribution by reporting modules with 0% test coverage among auth flows, WebSocket reconnection logic, and error state handling, and flagging any category where fewer than 50% of its modules have at least one associated test
5. THE Auditor SHALL assess accessibility compliance against WCAG 2.1 AA by reporting: interactive elements not reachable via keyboard Tab navigation, missing or invalid ARIA attributes on custom widgets, text color contrast ratios below 4.5:1 for normal text or below 3:1 for large text, and images or icon buttons lacking accessible names
6. THE Auditor SHALL evaluate adherence to project conventions documented in AGENTS.md by reporting: hardcoded color values or Tailwind color classes used instead of CSS custom properties (e.g., `var(--color-brand-primary)`), user-visible strings not wrapped in the `t()` i18n function, and elements using Tailwind `dark:` prefix instead of the `[data-theme="dark"]` selector strategy
7. THE Auditor SHALL produce an audit report where each finding includes the violation category, file path, line number or range, a description of the violation, and the expected pattern or convention, grouped by acceptance criterion

### Requirement 10: Code Quality Audit — Go Backend Patterns

**User Story:** As a developer, I want the audit to assess backend code quality, so that I can identify structural issues and deviation from Go best practices.

#### Acceptance Criteria

1. THE Auditor SHALL evaluate error handling patterns, flagging: errors wrapped without `%w` verb (breaking `errors.Is`/`errors.As` chains), use of sentinel errors where custom types with context fields would improve debuggability, and instances where an error is both logged and returned to the caller (double-handling)
2. THE Auditor SHALL assess package structure, flagging: exported symbols not consumed by any external package, interfaces defined with more than 5 methods where consumers use fewer, and import cycles detected by `go vet` or equivalent static analysis
3. THE Auditor SHALL evaluate context propagation, flagging: use of `context.Background()` or `context.TODO()` inside HTTP request handlers instead of `r.Context()`, blocking operations that do not select on `ctx.Done()` or pass the context to downstream calls, and missing timeout/deadline on outbound calls (HTTP, DB, gRPC)
4. THE Auditor SHALL check for resource leaks, flagging: HTTP response bodies not closed via `defer resp.Body.Close()` after a non-nil response, database connections or rows not closed after use, file handles without deferred close, and goroutines lacking a termination path tied to context cancellation or channel close
5. THE Auditor SHALL assess graceful shutdown implementation, verifying the drain sequence follows: (1) stop accepting new requests, (2) drain in-flight requests within the configured drain timeout, (3) close persistent connections (database pools, WebSocket hub), (4) exit with status 0 on success or non-zero if drain timeout is exceeded
6. THE Auditor SHALL evaluate whether all database queries in `backend/internal/store/` use sqlc-generated code, flagging any raw SQL string concatenation, `fmt.Sprintf`-constructed queries, or direct `db.Query`/`db.Exec` calls with inline SQL that bypass the sqlc layer
7. WHEN the audit is complete, THE Auditor SHALL produce a findings report where each finding includes: the file path and line range, a severity level (critical, warning, or info), the violated pattern category (error-handling, package-structure, context-propagation, resource-leak, shutdown, or sqlc-consistency), and a one-line description of the issue

### Requirement 11: Improvements Catalog

**User Story:** As a platform operator, I want the audit to produce a prioritized list of improvements, so that I can plan remediation work effectively.

#### Acceptance Criteria

1. THE Audit_Report SHALL include an Improvements section that consolidates all findings into remediation items, where each item includes: a title, a description of the finding, the affected component or area, the assigned effort category, the assigned impact category, and the assigned priority score
2. WHEN listing improvements, THE Audit_Report SHALL categorize each item by effort (Small: hours, Medium: days, Large: weeks) and impact (Security, Performance, Quality, Reliability)
3. THE Audit_Report SHALL prioritize improvements using a risk matrix combining Severity_Level (Critical, High, Medium, Low) and likelihood (Almost Certain, Likely, Possible, Unlikely), producing a numeric priority score from 1 (highest) to 16 (lowest), and SHALL list items in ascending priority score order
4. THE Audit_Report SHALL identify quick wins (impact category of Security or Reliability AND effort category of Small) in a dedicated subsection, separately from strategic improvements (impact category of Security or Reliability AND effort category of Large) in another dedicated subsection
5. IF a project dependency has a published CVE in a public vulnerability database or is more than one major version behind the latest stable release, THEN THE Audit_Report SHALL include a dependency upgrade recommendation specifying the package name, current version, recommended version, and the associated CVE identifier or performance regression description

### Requirement 12: Future Roadmap

**User Story:** As a platform operator, I want the audit to recommend a future roadmap, so that I can plan long-term platform evolution beyond immediate fixes.

#### Acceptance Criteria

1. THE Audit_Report SHALL include a Roadmap section with short-term (1-3 months), medium-term (3-6 months), and long-term (6-12 months) recommendations
2. THE Roadmap SHALL address architectural improvements that cannot be achieved through point fixes (e.g., CSP implementation, E2E test infrastructure, CI/CD pipeline)
3. THE Roadmap SHALL identify scalability investments needed to grow beyond the 500-monitor design target
4. THE Roadmap SHALL recommend observability improvements (logging, tracing, alerting on the platform itself)
5. THE Roadmap SHALL include security hardening measures appropriate for a self-hosted deployment (rate limiting, brute-force protection, audit logging)
6. THE Roadmap SHALL recommend developer experience improvements (documentation, onboarding, development workflow)

### Requirement 13: Audit Report Structure and Format

**User Story:** As a reader of the audit report, I want the document to follow a consistent structure, so that findings are easy to navigate and act upon.

#### Acceptance Criteria

1. THE Audit_Report SHALL use the following top-level sections in order: Executive Summary, Methodology, Security Findings, Performance Findings, Code Quality Findings, Improvements Catalog, Future Roadmap, Appendices
2. THE Executive Summary SHALL include total finding counts by severity, a risk posture assessment (Critical/High/Medium/Low overall rating), and top-3 priority items requiring immediate attention
3. WHEN documenting a finding, THE Audit_Report SHALL use a consistent template containing: Finding ID (format: `{CATEGORY}-{NNN}` where CATEGORY is SEC, PERF, or QUAL and NNN is a zero-padded sequential number), Severity (one of Critical, High, Medium, Low, Informational), Category, Title, Description, Evidence (file path + line range), Impact, Remediation, and Effort estimate (one of Small: hours, Medium: days, Large: weeks)
4. THE Audit_Report SHALL include a Methodology section describing what was examined, tools or techniques used, scope limitations, and areas explicitly excluded from the audit
5. THE Audit_Report SHALL be written in English and delivered as a single markdown file at `docs/AUDIT.md` in the project root
6. IF a finding references code, THEN THE Audit_Report SHALL include the relevant code snippet (maximum 10 lines) or a direct file reference with line numbers
7. THE Audit_Report SHALL present findings within each domain section ordered by Severity_Level from Critical to Informational, with findings of equal severity ordered by their Finding ID
8. THE Audit_Report SHALL include a table of contents after the document title with hyperlinks to each top-level section and to each individual finding by its Finding ID
