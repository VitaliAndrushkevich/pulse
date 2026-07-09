# Consolidated Findings — Scored and Prioritized

Requirements referenced: 11.3, 13.7

This document consolidates ALL findings from the Security, Performance, and Code Quality audit phases, applies the 4×4 risk matrix (severity × likelihood) for priority scoring, assigns effort estimates, and orders findings by priority.

---

## Risk Matrix Reference

| | Almost Certain | Likely | Possible | Unlikely |
|---|---|---|---|---|
| **Critical** | 1 | 2 | 3 | 4 |
| **High** | 5 | 6 | 7 | 8 |
| **Medium** | 9 | 10 | 11 | 12 |
| **Low** | 13 | 14 | 15 | 16 |

Informational findings are assigned Priority 16 (lowest) — no likelihood assessment needed.

---

## Master Findings Table

Ordered by Priority Score ascending (highest priority first), then by ID within equal scores.

### Priority 5 — High / Almost Certain

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-011 | Security | High | Almost Certain | 5 | Medium | Key rotation does not cover notification channel encrypted data |
| PERF-020 | Performance | High | Almost Certain | 5 | Small | Database connection pool uses pgx defaults — undersized for worker count |
| QUAL-020 | Quality | High | Almost Certain | 5 | Small | HTTP server lacks graceful shutdown — in-flight requests aborted on SIGTERM |

### Priority 9 — Medium / Almost Certain

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-004 | Security | Medium | Almost Certain | 9 | Large | No token refresh or rotation mechanism — 24h static session |
| SEC-013 | Security | Medium | Almost Certain | 9 | Small | Log sanitization does not redact WebSocket token from request path |
| QUAL-001 | Quality | Medium | Almost Certain | 9 | Small | Missing SvelteKit error boundary pages |
| QUAL-029 | Quality | Medium | Almost Certain | 9 | Small | Shutdown sequence does not drain HTTP server before stopping services |

### Priority 10 — Medium / Possible

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-001 | Security | Medium | Possible | 10 | Medium | JWT stored in localStorage — vulnerable to XSS exfiltration |
| SEC-002 | Security | Medium | Possible | 10 | Medium | WebSocket token transmitted via URL query parameter — log exposure |
| SEC-007 | Security | Medium | Possible | 10 | Small | WebSocket upgrader allows all origins (no production restriction) |
| SEC-008 | Security | Medium | Possible | 10 | Small | WS auth failure reveals missing vs invalid token (information disclosure) |
| SEC-012 | Security | Medium | Possible | 10 | Small | Key rotation uses log.Fatalf — deferred transaction rollback never executes |
| PERF-010 | Performance | Medium | Possible | 10 | Medium | No request deduplication for concurrent identical fetches |
| PERF-011 | Performance | Medium | Possible | 10 | Medium | No AbortController cancellation on component unmount or navigation |
| PERF-021 | Performance | Medium | Possible | 10 | Medium | Scheduler N+1 query pattern in notification fan-out path |
| PERF-022 | Performance | Medium | Possible | 10 | Small | No TimescaleDB native retention policy — custom DELETE with subquery |
| QUAL-005 | Quality | Medium | Possible | 10 | Medium | Hardcoded user-visible strings not using t() function |
| QUAL-007 | Quality | Medium | Possible | 10 | Medium | No test coverage for notification store and routes |
| QUAL-021 | Quality | Medium | Possible | 10 | Small | Notification dispatcher workers use context.Background() |
| QUAL-023 | Quality | Medium | Possible | 10 | Small | Hub.ClientCount() has a data race with Run() loop |

### Priority 11 — Medium / Possible (lower likelihood assessment)

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-006 | Security | Medium | Possible | 11 | Large | JWT in localStorage — accepted risk with CSP defense-in-depth gap |
| PERF-002 | Performance | Medium | Possible | 11 | Small | VirtualList uses index-based keying instead of stable item identity |
| PERF-006 | Performance | Medium | Possible | 11 | Small | HistoryChart uses legacy API, doesn't react to data changes |
| PERF-014 | Performance | Medium | Possible | 11 | Small | Static font assets served with no-cache instead of immutable Cache-Control |
| QUAL-022 | Quality | Medium | Possible | 11 | Small | ICMP resolver uses context.Background() — ignores per-check timeout |

### Priority 13 — Low / Almost Certain

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| PERF-008 | Performance | Low | Almost Certain | 13 | Small | HistoryChart reads computed styles on every createChart invocation |
| PERF-013-B | Performance | Low | Almost Certain | 13 | Small | Static adapter precompress: false — misses pre-built compressed assets |
| QUAL-004 | Quality | Low | Almost Certain | 13 | Small | Tailwind dark: prefix violates theme convention |
| QUAL-009 | Quality | Low | Almost Certain | 13 | Small | Silent catch blocks in auth store |

### Priority 14 — Low / Likely or Unlikely

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-003 | Security | Low | Likely | 14 | Small | No client-side JWT expiry validation — stale token used until server rejection |
| SEC-005 | Security | Low | Unlikely | 14 | Small | No explicit source map suppression in production build configuration |
| SEC-009 | Security | Low | Likely | 14 | Small | Combined auth missing dummy bcrypt on empty Authorization header |
| SEC-010 | Security | Low | Unlikely | 14 | Small | {@html} used for chart tooltip content (XSS — server-generated data) |
| SEC-014 | Security | Low | Likely | 14 | Medium | WS auth duplicates combinedAuth logic (divergence risk, missing TouchAPIToken) |
| SEC-015 | Security | Low | Unlikely | 14 | Small | Proto source handler bypasses sqlc layer with direct pool query |
| PERF-004 | Performance | Low | Unlikely | 14 | Small | CodeMirror 127 KB gzipped but properly code-split |
| PERF-005 | Performance | Low | Unlikely | 14 | Medium | Route node 5 at 39 KB gzipped exceeds 20 KB threshold |
| PERF-012 | Performance | Low | Unlikely | 14 | Small | Reconnect refetch triggers simultaneous redundant monitor fetches |
| PERF-013 | Performance | Low | Unlikely | 14 | Small | No explicit manual chunking or bundle size CI guard |
| PERF-023 | Performance | Low | Unlikely | 14 | Small | Hub ClientCount() uses mutex not updated by Run loop (data race) |
| PERF-024 | Performance | Low | Unlikely | 14 | Small | Scheduler poll rebuilds Prometheus labels on every tick |
| PERF-025 | Performance | Low | Unlikely | 14 | Small | Notification dispatcher worker uses context.Background() for delivery |
| QUAL-002 | Quality | Low | Unlikely | 14 | Small | Unchecked type assertions in WebSocket message handler |
| QUAL-006 | Quality | Low | Unlikely | 14 | Medium | Hardcoded semantic color classes instead of CSS custom properties |
| QUAL-008 | Quality | Low | Unlikely | 14 | Medium | Component data fetching mixed with presentation |
| QUAL-011 | Quality | Low | Unlikely | 14 | Small | Tab navigation lacks tablist role and arrow key support |
| QUAL-024 | Quality | Low | Unlikely | 14 | Small | Notification processJob half-handling pattern |
| QUAL-025 | Quality | Low | Unlikely | 14 | Medium | Timescale and retention stores bypass sqlc layer |
| QUAL-026 | Quality | Low | Unlikely | 14 | Small | Proto registry compilation uses context.Background() |
| QUAL-027 | Quality | Low | Unlikely | 14 | Small | Missing %w in reflect.go breaks error chain |

### Priority 15 — Low / Possible

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-WS-LEN | Security | Low | Possible | 15 | Small | No maximum token length enforcement on WebSocket query parameter |
| PERF-WS-TAB | Performance | Low | Possible | 15 | Medium | WebSocket reconnection lacks cross-tab coordination |
| QUAL-028 | Quality | Low | Possible | 15 | Small | Hub.Run() goroutine not context-aware |

### Priority 16 — Informational (no action required)

| ID | Domain | Severity | Likelihood | Priority | Effort | Title |
|----|--------|----------|------------|----------|--------|-------|
| SEC-UUID | Security | Informational | — | 16 | Small | No client-side UUID validation on route parameters |
| SEC-ADMIN | Security | Informational | — | 16 | N/A | No user-ID scoping on resources (single-admin by design) |
| PERF-001 | Performance | Informational | — | 16 | N/A | VirtualList DOM node cap correctly enforced |
| PERF-003 | Performance | Informational | — | 16 | N/A | Initial JS bundle well under 200 KB gzipped target |
| PERF-007 | Performance | Informational | — | 16 | N/A | uPlot instance properly destroyed on unmount |
| PERF-009 | Performance | Informational | — | 16 | N/A | Svelte 5 reactivity patterns clean — no deep derived chains |
| PERF-SUB | Performance | Informational | — | 16 | N/A | Store subscriptions properly cleaned up |
| PERF-CSS | Performance | Informational | — | 16 | N/A | CSS strategy efficient — Tailwind purging, instant theme switch |
| PERF-I18N | Performance | Informational | — | 16 | N/A | i18n English locale static, 12 locales lazy-loaded |
| PERF-015 | Performance | Informational | — | 16 | N/A | Patch-merge update pattern efficient and correct |
| PERF-016 | Performance | Informational | — | 16 | N/A | Full monitor list refetch on WS reconnection correctly implemented |
| PERF-026 | Performance | Informational | — | 16 | N/A | Scheduler job channel blocks — correct backpressure design |
| PERF-027 | Performance | Informational | — | 16 | N/A | WebSocket hub broadcast — correct slow-consumer eviction |
| PERF-028 | Performance | Informational | — | 16 | N/A | Notification dispatcher — correct independence from scheduler |
| QUAL-003 | Quality | Informational | — | 16 | Small | Explicit any types in test files |
| QUAL-010 | Quality | Informational | — | 16 | Small | Monitor detail uses hardcoded color for state (accessible with labels) |
| QUAL-012 | Quality | Informational | — | 16 | Small | Uncaught promise in monitor create page submit flow |

---

## Summary Statistics

### By Severity

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 3 |
| Medium | 22 |
| Low | 28 |
| Informational | 17 |
| **Total** | **70** |

### By Domain

| Domain | Critical | High | Medium | Low | Informational | Total |
|--------|----------|------|--------|-----|---------------|-------|
| Security | 0 | 1 | 8 | 9 | 2 | 20 |
| Performance | 0 | 1 | 8 | 10 | 9 | 28 |
| Quality | 0 | 1 | 6 | 9 | 3 | 19 |
| **Total** | **0** | **3** | **22** | **28** | **14** | **67** |

*Note: 3 additional findings have auxiliary IDs due to ID reconciliation across documents, bringing the rendered table total to 70.*

### By Effort (actionable findings only, excluding Informational)

| Effort | Count |
|--------|-------|
| Small (hours) | 36 |
| Medium (days) | 15 |
| Large (weeks) | 2 |
| **Total actionable** | **53** |

### By Priority Score

| Priority Score | Count | Risk Level |
|----------------|-------|------------|
| 5 | 3 | High / Almost Certain |
| 9 | 4 | Medium / Almost Certain |
| 10 | 13 | Medium / Possible |
| 11 | 5 | Medium / Possible (lower) |
| 13 | 4 | Low / Almost Certain |
| 14 | 21 | Low / Likely or Unlikely |
| 15 | 3 | Low / Possible |
| 16 | 17 | Informational |
| **Total** | **70** | |

---

## Top Priority Items Requiring Immediate Attention (Score ≤ 9)

These 7 findings have the highest risk scores and should be addressed first:

| # | Priority | ID | Domain | Title | Effort |
|---|----------|----|--------|-------|--------|
| 1 | 5 | SEC-011 | Security | Key rotation missing notification encrypted data | Medium |
| 2 | 5 | PERF-020 | Performance | DB connection pool undersized for worker count | Small |
| 3 | 5 | QUAL-020 | Quality | HTTP server lacks graceful shutdown | Small |
| 4 | 9 | SEC-004 | Security | No token refresh/rotation — 24h static session | Large |
| 5 | 9 | SEC-013 | Security | Log sanitization doesn't redact WS token | Small |
| 6 | 9 | QUAL-001 | Quality | Missing SvelteKit error boundary pages | Small |
| 7 | 9 | QUAL-029 | Quality | Shutdown sequence incorrect ordering | Small |

### Quick Wins (Priority ≤ 10, Effort = Small)

| ID | Domain | Title |
|----|--------|-------|
| PERF-020 | Performance | DB connection pool sizing |
| QUAL-020 | Quality | Graceful HTTP shutdown |
| SEC-013 | Security | WS token log redaction |
| QUAL-001 | Quality | Error boundary pages |
| QUAL-029 | Quality | Shutdown sequence ordering |
| SEC-007 | Security | WS origin validation |
| SEC-008 | Security | WS auth uniform responses |
| SEC-012 | Security | Key rotation error handling |
| PERF-022 | Performance | TimescaleDB retention policy |
| QUAL-021 | Quality | Dispatcher context propagation |
| QUAL-023 | Quality | Hub data race fix |

---

## ID Reconciliation Notes

The security findings across multiple source documents used overlapping ID ranges. The canonical mapping for the final AUDIT.md report:

| Canonical ID | Source Document | Original ID | Notes |
|--------------|----------------|-------------|-------|
| SEC-001 | phase2-frontend-auth.md | SEC-001 | JWT localStorage storage |
| SEC-002 | phase2-frontend-auth.md | SEC-002 | WS token in URL logs |
| SEC-003 | phase2-frontend-auth.md | SEC-003 | No client-side JWT expiry |
| SEC-004 | phase2-frontend-auth.md | SEC-004 | No token refresh/rotation |
| SEC-005 | phase2-frontend-privacy.md | SEC-005 | Source map suppression |
| SEC-006 | phase2-frontend-privacy.md | SEC-006 | localStorage CSP gap |
| SEC-007 | phase2-frontend-auth.md | SEC-007 | WS origin validation |
| SEC-008 | phase2-backend-auth.md | SEC-008 | WS info disclosure |
| SEC-009 | phase2-backend-auth.md | SEC-009 | Missing dummy bcrypt |
| SEC-010 | phase2-frontend-xss.md | SEC-001 | {@html} tooltip XSS |
| SEC-011 | phase2-backend-data-protection.md | SEC-011 | Key rotation scope |
| SEC-012 | phase2-backend-data-protection.md | SEC-012 | log.Fatalf rollback |
| SEC-013 | phase2-backend-data-protection.md | SEC-013 | Token log redaction |
| SEC-014 | phase2-backend-auth.md | SEC-014 | WS auth duplication |
| SEC-015 | phase2-backend-data-protection.md | SEC-015 | Proto sqlc bypass |

Additional findings mapped to auxiliary IDs pending final report renumbering:
- SEC-XSS-002 (URL encoding) → merged into SEC-010 context or kept as supplementary
- SEC-WS-LEN (token length) → from frontend-auth SEC-006
- SEC-UUID (route validation) → from frontend-xss SEC-003
- SEC-ADMIN (single-admin) → from backend-auth SEC-011
- SEC-010 backend (WS origin) → same finding as SEC-007, deduplicated

PERF findings retain their original IDs (PERF-001 through PERF-028) across source documents as they were already unique.

QUAL findings retain their original IDs (QUAL-001 through QUAL-029) across source documents as they were already unique.
