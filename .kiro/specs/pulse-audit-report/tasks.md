# Implementation Plan: Pulse Audit Report

## Overview

This plan follows the 6-phase pipeline defined in the design: Inventory → Security Analysis → Performance Analysis → Code Quality Analysis → Synthesis & Classification → Report Assembly. Each task reads source code, analyzes patterns, documents findings, and assembles sections of the final `docs/AUDIT.md` deliverable. No code changes are made — the output is a structured markdown audit document.

## Tasks

- [x] 1. Phase 1: Inventory and Scoping
  - [x] 1.1 Map frontend auditable surfaces
    - Traverse `frontend/src/` directory structure (components, routes, lib, stores, locales)
    - List all Svelte components, route files, API client, WS client, and store modules
    - Identify input-accepting components (forms, text fields, user-controlled data flows)
    - Document the full file inventory for subsequent analysis phases
    - _Requirements: 13.4_

  - [x] 1.2 Map backend auditable surfaces
    - Traverse `backend/internal/` packages (api, crypto, hub, monitor, notification, store, token)
    - Identify auth middleware, handlers, scheduler, dispatcher, and data access layers
    - Document exported symbols, interfaces, and package boundaries
    - _Requirements: 13.4_

  - [x] 1.3 Audit dependencies and configuration
    - Read `frontend/package.json` for frontend dependencies and versions
    - Read `backend/go.mod` for Go module dependencies and versions
    - Read `docker-compose*.yml`, `.env.example`, and `Dockerfile` for infrastructure config
    - Read `backend/api/openapi.yaml` for API contract surface
    - Check for known CVEs via dependency version analysis
    - _Requirements: 11.5, 13.4_

- [x] 2. Phase 2: Security Analysis — Frontend
  - [x] 2.1 Audit frontend authentication and token handling
    - Read `frontend/src/lib/api.ts` and `frontend/src/lib/ws.ts` for token usage
    - Read auth store (`frontend/src/lib/stores/`) for JWT storage mechanism
    - Trace token lifecycle: storage, transmission (Bearer header, WS query param), clearing on 401/4401
    - Check for JWT client-side validation (expiry, signature)
    - Assess session lifetime and refresh/rotation mechanisms
    - Document findings with SEC-NNN IDs following the finding template
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7_

  - [x] 2.2 Audit frontend input handling and XSS prevention
    - Search all Svelte components for `{@html}` directives and classify each as safe/unsafe
    - Examine MonitorForm, login form, settings forms, notification channel forms for input sanitization
    - Verify WebSocket message handler rejects invalid JSON without DOM propagation
    - Check URL construction in API and WS clients for path traversal and encoding
    - Verify error messages from API responses use text interpolation only (no `{@html}`)
    - Document findings with SEC-NNN IDs
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

  - [x] 2.3 Audit frontend data exposure and privacy
    - Verify secret values are not persisted in stores, localStorage, or sessionStorage beyond creation
    - Check production build output for `console.log`/`console.debug` referencing secrets
    - Verify source maps are absent from static adapter build output or access-controlled
    - Search generated bundles for environment variable names/values
    - Document findings with SEC-NNN IDs
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_

  - [x] 2.4 Audit backend authentication and authorization
    - Read `backend/internal/api/router.go` for auth middleware coverage on all endpoints
    - Verify JWT algorithm restriction (HMAC-only, reject `alg: "none"`)
    - Check bcrypt cost factor and timing-safe comparison with dummy hash on failure
    - Read `backend/internal/api/handlers/ws.go` for WS auth error responses
    - Verify combined auth middleware produces identical 401 responses for JWT and API token
    - Check for single-admin role resource isolation
    - Verify Origin header validation on WebSocket upgrade
    - Document findings with SEC-NNN IDs
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_

  - [x] 2.5 Audit backend data protection
    - Read crypto module for AES-256-GCM implementation (nonce generation, key validation)
    - Verify write-only semantics across all secret-bearing API endpoints
    - Check log sanitization in request handlers, scheduler, and notification delivery
    - Verify all queries use parameterized sqlc-generated code (no SQL injection vectors)
    - Assess key rotation transactional integrity and timeout handling
    - Verify API token bcrypt storage and constant-time comparison
    - Document findings with SEC-NNN IDs
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

- [x] 3. Checkpoint — Security analysis complete
  - Ensure all security findings are documented with SEC-NNN IDs, ask the user if questions arise.

- [x] 4. Phase 3: Performance Analysis — Frontend
  - [x] 4.1 Audit frontend rendering and bundle performance
    - Read VirtualList implementation and verify DOM node cap (max 60)
    - Assess bundle size from build output or configuration (initial JS < 200 KB gzipped target)
    - Identify code-splitting gaps (dependencies > 50 KB, non-lazy-loaded route modules > 20 KB)
    - Evaluate Svelte 5 reactivity patterns (runes, derived state depth, subscription cleanup)
    - Check uPlot lifecycle for retained references and detached canvas elements after unmount
    - Assess CSS strategy (unused rules, render-blocking styles, theme switch repaint)
    - Verify i18n lazy-loading (only active locale in initial bundle, dynamic import for others)
    - Document findings with PERF-NNN IDs
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6_

  - [x] 4.2 Audit frontend network and real-time performance
    - Evaluate WebSocket reconnection strategy for thundering herd prevention
    - Assess patch-merge update pattern efficiency (O(1) lookup, field-level merge, unknown ID handling)
    - Check API request patterns for unnecessary refetches and missing deduplication
    - Verify AbortController wiring on all fetch calls and component unmount cleanup
    - Evaluate static asset caching strategy (Cache-Control, content-hashed filenames)
    - Verify full monitor list refetch on WebSocket reconnection
    - Document findings with PERF-NNN IDs
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6_

  - [x] 4.3 Audit backend scheduler and concurrency performance
    - Read scheduler implementation for bounded worker pool enforcement
    - Assess WebSocket hub broadcast fan-out and slow-consumer eviction
    - Evaluate database connection pool sizing relative to worker count
    - Check for N+1 query patterns in scheduler poll and notification fan-out
    - Verify notification dispatcher independence (non-blocking enqueue, separate worker pool)
    - Assess TimescaleDB hypertable configuration and retention policy
    - Document findings with PERF-NNN IDs
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

- [x] 5. Checkpoint — Performance analysis complete
  - Ensure all performance findings are documented with PERF-NNN IDs, ask the user if questions arise.

- [x] 6. Phase 4: Code Quality Analysis
  - [x] 6.1 Audit TypeScript and frontend patterns
    - Search for explicit `any` types, missing return-type annotations on exports, unchecked nullable access
    - Assess error handling (uncaught promise rejections, missing error boundaries, silent failures)
    - Evaluate component architecture (mixed concerns, prop drilling, local vs store state)
    - Check test coverage distribution (auth flows, WS reconnection, error states)
    - Assess WCAG 2.1 AA compliance (keyboard navigation, ARIA, color contrast, accessible names)
    - Check adherence to AGENTS.md conventions (CSS custom properties, i18n `t()`, theme selectors)
    - Document findings with QUAL-NNN IDs
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7_

  - [x] 6.2 Audit Go backend patterns
    - Evaluate error handling (missing `%w`, sentinel vs custom types, double-handling)
    - Assess package structure (unexported symbols, oversized interfaces, import cycles)
    - Check context propagation (Background/TODO in handlers, missing ctx.Done, missing timeouts)
    - Identify resource leaks (unclosed HTTP bodies, DB rows, file handles, goroutines without termination)
    - Verify graceful shutdown sequence (stop accepting → drain → close connections → exit)
    - Confirm all DB queries use sqlc layer (no raw SQL concatenation)
    - Document findings with QUAL-NNN IDs
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6, 10.7_

- [x] 7. Checkpoint — Code quality analysis complete
  - Ensure all code quality findings are documented with QUAL-NNN IDs, ask the user if questions arise.

- [x] 8. Phase 5: Synthesis and Classification
  - [x] 8.1 Score and prioritize all findings
    - Apply the 4×4 risk matrix (severity × likelihood) to assign priority scores 1-16 to each finding
    - Assign effort estimates (Small/Medium/Large) to each finding
    - Order findings within each domain section by severity (Critical → Informational), then by ID
    - _Requirements: 11.3, 13.7_

  - [x] 8.2 Build the improvements catalog
    - Consolidate findings into remediation items with title, description, component, effort, impact, priority
    - Identify quick wins (Security/Reliability impact + Small effort) for dedicated subsection
    - Identify strategic improvements (Security/Reliability impact + Large effort) for dedicated subsection
    - Add dependency upgrade recommendations for CVEs or major-version-behind packages
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5_

  - [x] 8.3 Build the future roadmap
    - Define short-term (1-3 months), medium-term (3-6 months), long-term (6-12 months) recommendations
    - Address architectural improvements beyond point fixes (CSP, E2E tests, CI/CD)
    - Identify scalability investments for growth beyond 500-monitor target
    - Recommend observability improvements (logging, tracing, self-alerting)
    - Define security hardening measures (rate limiting, brute-force protection, audit logging)
    - Recommend developer experience improvements (documentation, onboarding, workflow)
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6_

- [x] 9. Phase 6: Report Assembly
  - [x] 9.1 Write the executive summary and methodology
    - Compile total finding counts by severity
    - Determine overall risk posture assessment (Critical/High/Medium/Low)
    - Identify top-3 priority items requiring immediate attention
    - Write methodology section (what was examined, techniques used, scope limitations, exclusions)
    - _Requirements: 13.2, 13.4_

  - [x] 9.2 Assemble the full `docs/AUDIT.md` document
    - Create `docs/AUDIT.md` with all sections in order: Executive Summary, Methodology, Security Findings, Performance Findings, Code Quality Findings, Improvements Catalog, Future Roadmap, Appendices
    - Generate table of contents after the document title
    - Ensure all findings use the consistent template (ID, Severity, Category, Title, Description, Evidence, Impact, Remediation, Effort)
    - Verify sequential Finding IDs within each category (no gaps)
    - Verify all requirement acceptance criteria are covered or explicitly noted as "no issues found"
    - Include requirements coverage matrix in appendices
    - _Requirements: 13.1, 13.2, 13.3, 13.5, 13.6, 13.7, 13.8_

- [x] 10. Final checkpoint — Report validation
  - Verify structural completeness: all 8 required sections present in correct order
  - Verify every finding has all template fields populated (no placeholders)
  - Verify cross-reference consistency (Improvements reference existing findings)
  - Verify priority ordering within domain sections
  - Verify all 13 requirements have coverage in the report
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- This is a read-only audit — no code changes are produced. The sole deliverable is `docs/AUDIT.md`.
- Finding IDs use format: SEC-NNN, PERF-NNN, QUAL-NNN (zero-padded sequential numbers)
- Priority scoring uses the 4×4 risk matrix from the design (severity × likelihood, scores 1-16)
- Frontend (SvelteKit 5) is the primary audit focus; backend (Go) is secondary
- Tasks are ordered following the 6-phase pipeline: Inventory → Security → Performance → Code Quality → Synthesis → Assembly
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation between major analysis phases

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3"] },
    { "id": 1, "tasks": ["2.1", "2.2", "2.3"] },
    { "id": 2, "tasks": ["2.4", "2.5"] },
    { "id": 3, "tasks": ["4.1", "4.2", "4.3"] },
    { "id": 4, "tasks": ["6.1", "6.2"] },
    { "id": 5, "tasks": ["8.1"] },
    { "id": 6, "tasks": ["8.2", "8.3"] },
    { "id": 7, "tasks": ["9.1"] },
    { "id": 8, "tasks": ["9.2"] }
  ]
}
```
