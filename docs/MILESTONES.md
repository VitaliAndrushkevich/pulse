# Pulse — Project Milestones & Current Stage

## Project Overview

Pulse is a self-hosted uptime monitoring platform (API-first, single-container deployment). The MVP targets HTTP/HTTPS, TCP, UDP, and WebSocket monitors with realtime status updates, designed to handle 500+ monitors.

---

## Current Stage: MVP Complete + Branding 🎉

All milestones (A–J) are done. The project is a fully packaged, single-container deployment with embedded frontend, production Docker Compose, cohesive brand identity, comprehensive documentation, and CI pipeline.

```
[████████████████████████████████████] 100%
  A ✓  B ✓  C ✓  D ✓  E ✓  F ✓  G ✓  H ✓  I ✓
```

---

## Milestone A: Foundations ✅ DONE

**Goal:** Repository bootstrap, local developer workflow, baseline deployment pipeline.

What's delivered:
- Go backend module with `cmd/pulse/main.go` entrypoint
- SvelteKit + TypeScript + Tailwind frontend scaffold
- `docker-compose.yml` (Pulse + TimescaleDB/PostgreSQL 16 with health checks)
- `docker-compose.dev.yml` for local development
- `Makefile` with `dev`, `build`, `test`, `migrate`, `run` targets
- Migration tooling via `golang-migrate` (`cmd/migrate`)
- Multi-stage Dockerfile (Go build → distroless runtime)

---

## Milestone B: Data Layer ✅ DONE

**Goal:** Durable config/state in PostgreSQL, time-series writes/reads through TimescaleDB.

What's delivered:
- Full PostgreSQL schema: `users`, `api_tokens`, `secrets`, `monitors`, `incidents`, `check_results`
- Proper indexes for scheduler priority queue and query patterns
- `sqlc`-generated typed query layer (CRUD + paginated lists for all resources)
- TimescaleDB write/query helpers (`WriteCheckResult`, `QueryHistory`)
- Fail-fast startup: process exits non-zero when Postgres is unreachable or TimescaleDB extension is unavailable
- `X-Request-ID` middleware and `/healthz` endpoints

---

## Milestone C: Security & Secrets ✅ DONE

**Goal:** Secure at-rest secret handling, single-user auth primitives, API token management.

Delivered:
- ✅ AES-256-GCM encryption/decryption module (`internal/crypto`)
- ✅ Secret write-only API (values never returned in responses)
- ✅ API token create/list/revoke with bcrypt hash storage
- ✅ Log sanitization middleware (strip auth headers and secret fields)
- ✅ Key rotation command (`make rotate-key`) with transactional re-encryption

---

## Milestone D: Monitor Execution Engine ✅ DONE

**Goal:** Reliable check execution for all MVP protocols, scheduler that scales to 500+ monitors.

Delivered:
- ✅ `Checker` interface and shared `Result` model (`internal/monitor/checker.go`)
- ✅ HTTP/HTTPS checker with configurable expected status codes (explicit list or range), SSL certificate chain validation, expiry threshold, custom headers, redirect control
- ✅ TCP checker — dial + latency measurement via context-aware `net.Dialer`
- ✅ UDP checker — reachability mode (default) + payload/response validation mode
- ✅ WebSocket checker — `gorilla/websocket` with optional handshake message validation
- ✅ Bounded worker pool scheduler (configurable: `PULSE_SCHEDULER_WORKERS`, default 50 dev / 200 production)
- ✅ Priority-based polling via `ListActiveMonitorsDue` (ordered by `next_check_at ASC NULLS FIRST`)
- ✅ Dual-write results to TimescaleDB (time-series) and `check_results` table (API)
- ✅ PostgreSQL `LISTEN/NOTIFY` wakeups — zero-delay scheduling on monitor create/update (migration `003_monitor_notify_trigger`)
- ✅ Graceful shutdown via context cancellation; workers drain before exit
- ✅ `gorilla/websocket` dependency added for WebSocket checker

---

## Milestone E: API Surface & Contract ✅ DONE

**Goal:** Complete versioned REST API with idempotent monitor management, OpenAPI contract.

Delivered:
- ✅ Full `gin` router under `/api/v1` with standardized error envelope and X-Request-ID
- ✅ JWT auth (`POST /api/v1/auth/login`) + combined middleware (JWT + API token)
- ✅ Monitor CRUD with idempotent `PUT /monitors/{id}` (ON CONFLICT DO UPDATE)
- ✅ Monitor history endpoint (`GET /monitors/{id}/history`) backed by TimescaleDB (7-day max window)
- ✅ Incidents list endpoints (global + per-monitor, paginated, optional status=open filter)
- ✅ Prometheus `/metrics` endpoint (`pulse_monitor_up`, `pulse_monitor_response_time_seconds`, `pulse_monitors_total`)
- ✅ Committed `backend/api/openapi.yaml` (OpenAPI 3.0.3, all endpoints documented)

---

## Milestone F: Realtime Channel (WebSocket) ✅ DONE

**Goal:** Diff-based state updates over WebSocket for live dashboard.

Delivered:
- ✅ WebSocket hub (`internal/hub`) with fan-out, ping/pong keepalive, slow-consumer eviction (256-msg buffer)
- ✅ Typed message envelope with `monitor_status` (diff/patch) and `connected` message types
- ✅ Scheduler → Hub broadcast integration (check results sent to hub after each execution)
- ✅ Authenticated `/ws` endpoint with query-param token validation (JWT + API token)
- ✅ Close code 4401 for auth expiration signaling to clients
- ✅ Dummy bcrypt comparison on all auth failure paths for timing safety

---

## Milestone G: Frontend Product ✅ DONE

**Goal:** Responsive dashboard and monitor workflows at 500+ monitor scale.

Delivered:
- ✅ Core type system (`types.ts`), validation (`validation.ts`), formatting (`format.ts`)
- ✅ API client with Bearer auth, 15s timeout, error envelope parsing, X-Request-ID in toasts
- ✅ Reactive stores (Svelte 5 runes): AuthStore, MonitorStore (patch-merge), ToastStore, ConnectionStore
- ✅ WebSocket client with exponential backoff (1s–30s, ±25% jitter), 4401 auth-expired handling
- ✅ VirtualList component with DOM recycling (max 60 nodes, RAF-throttled scroll, configurable buffer 5–20)
- ✅ MonitorRow, MonitorForm (create/edit modes with type-specific settings), Pagination, HistoryChart (uPlot), Toast, ConnectionBadge components
- ✅ Login page with email/password validation, inline error display (401 → "Invalid email or password")
- ✅ Auth guard in layout with route protection and redirect to `/login`
- ✅ Dashboard with stats bar (total/healthy/unhealthy) and VirtualList rendering
- ✅ Monitor list page with pagination and error/empty states
- ✅ Monitor detail page with history chart, incident timeline, edit/delete actions
- ✅ Monitor create/edit pages with MonitorForm integration and secret reference dropdown
- ✅ Settings page with secrets management (create form, metadata-only display, value cleared immediately)
- ✅ WebSocket lifecycle wired to layout (connect on auth, disconnect on logout, re-fetch on reconnect)
- ✅ Real-time updates: WS patches update dashboard rows and detail view in-place via reactive store
- ✅ 141 unit tests passing (Vitest + fast-check + @testing-library/svelte) (expanded to 218 with branding tests in Milestone I)

---

## Milestone H: Packaging & Release ✅ DONE

**Goal:** Single-binary, single-container production artifact.

Delivered:
- ✅ Static frontend embedded via `//go:embed` with SPA catch-all routing and immutable cache headers
- ✅ Multi-stage Dockerfile (node:lts-alpine → golang:1.26-alpine → distroless)
- ✅ `.dockerignore` for minimal build context
- ✅ Production docker-compose with health checks, restart policies, `env_file`
- ✅ `.env.example` with all variables documented and generation commands
- ✅ Makefile targets: `build-frontend`, `build-all` (production build with embedded assets)
- ✅ Complete README with quick start, architecture, API examples, development docs
- ✅ CI workflow (GitHub Actions) — PR checks with backend tests (race detector, TimescaleDB service) and frontend checks (typecheck, locale validation, tests, build), concurrency groups

---

## Post-MVP Enhancements ✅

**Goal:** Developer-experience improvements applied after MVP completion.

Delivered:
- ✅ Replaced npm with pnpm as the frontend package manager — faster installs, stricter dependency resolution, `pnpm-lock.yaml` committed, `package-lock.json` removed
- ✅ Added frontend dev container with HMR to `docker-compose.dev.yml` — `node:lts-alpine` service running `pnpm dev --host` (Vite) on port 5173 with bind-mounted sources and named volume for `node_modules`
- ✅ Updated Dockerfile frontend stage to use corepack + pnpm (`pnpm install --frozen-lockfile`, `pnpm run build`)
- ✅ Updated Makefile `build-frontend` target with pnpm guard and pnpm commands
- ✅ All project documentation updated (README, AGENTS.md, copilot-instructions, MILESTONES)

---

## Milestone I: Branding & Theming ✅ DONE

**Goal:** Cohesive visual identity with ECG-inspired logo, theme-aware color system, and light/dark switching.

Delivered:
- ✅ CSS custom properties theme system (`:root`/`[data-theme="light"]` + `[data-theme="dark"]` overrides) with full token set: brand-primary, brand-hover, bg-page, bg-surface, text-primary, text-secondary, border, success, warning, error, and brand scale 50–900
- ✅ Tailwind integration: `darkMode: ['selector', '[data-theme="dark"]']`, brand color scale, semantic aliases (success, warning, error), fontFamily.brand with Inter
- ✅ FOUC prevention inline script in `app.html` (reads localStorage, validates, falls back to `prefers-color-scheme`)
- ✅ Self-hosted Inter font (WOFF2, semi-bold 600, `font-display: swap`)
- ✅ BrandLockup component: inline SVG ECG peak with proportional scaling (size prop), full/compact variants, `currentColor` + CSS variable fallback
- ✅ ThemeSwitcher component: tri-state cycling (light → dark → system) with `data-theme` attribute update, `pulse-theme-mode` localStorage persistence, sun/moon/monitor icons, aria-labels, OS preference tracking via matchMedia, SecurityError resilience
- ✅ Static brand assets: `logo-mark.svg`, `brand-lockup.svg`, `brand-lockup-dark.svg`, PNG exports (1x/2x/4x), README guidelines
- ✅ PNG generation scripts (`generate-brand-pngs.mjs`, `generate-icons.mjs`) using `sharp`
- ✅ Favicon (32×32), Apple Touch Icon (180×180), PWA icons (192/512), `site.webmanifest`
- ✅ Layout integration: responsive BrandLockup in header (full > 640px, compact ≤ 640px), ThemeSwitcher in nav
- ✅ Login/setup pages: centered BrandLockup at 48px with viewport overflow scaling
- ✅ Theme-aware layout styles: all hardcoded slate/sky classes replaced with CSS variable references
- ✅ Property-based tests (6 properties): stroke proportionality, scaling, WCAG contrast, toggle persistence, icon correctness, token mapping
- ✅ Unit tests: BrandLockup (17 tests), ThemeSwitcher (12 tests)
- ✅ Total frontend tests: 218 (up from 141)

---

## What Exists Today (Inventory)

| Layer | Status | Notes |
|-------|--------|-------|
| PostgreSQL schema | ✅ Complete | All MVP tables, indexes, constraints |
| sqlc query layer | ✅ Complete | CRUD + paginated lists + upsert generated |
| TimescaleDB helpers | ✅ Complete | Write + range query scaffold |
| Fail-fast startup | ✅ Complete | Exits on missing dependencies |
| Docker infrastructure | ✅ Complete | Multi-stage Dockerfile, production compose, .env.example |
| Makefile | ✅ Complete | All primary targets + build-frontend, build-all |
| API router | ✅ Complete | Full CRUD for monitors, secrets, tokens, incidents, history |
| Crypto module | ✅ Complete | AES-256-GCM encrypt/decrypt + key validation |
| Protocol checkers | ✅ Complete | HTTP/HTTPS, TCP, UDP, WebSocket — all compiled and wired |
| Scheduler | ✅ Complete | Bounded worker pool, LISTEN/NOTIFY wakeups, graceful shutdown |
| Auth/JWT | ✅ Complete | Login endpoint + combined JWT/API-token middleware |
| Prometheus metrics | ✅ Complete | /metrics with monitor_up, response_time, monitors_total |
| OpenAPI spec | ✅ Complete | backend/api/openapi.yaml (3.0.3) |
| WebSocket hub | ✅ Complete | Fan-out hub with keepalive, slow-consumer eviction, auth endpoint |
| Frontend | ✅ Complete | Full SvelteKit app with stores, WS client, all pages, 218 tests |
| Branding & Theming | ✅ Complete | Logo, theme system, light/dark switch, brand assets, property tests |
| Frontend embedding | ✅ Complete | go:embed with SPA catch-all, cache headers |
| CI pipeline | ✅ Complete | PR checks: backend tests + frontend tests/build |

---

## Recommended Next Steps (Priority Order)

1. **End-to-end verification** — Run through the verification checklist below on a clean machine
2. **User seeding** — Add a `make seed` command or initial setup flow for first user creation
3. **CI enhancements** — Lint step (golangci-lint), OpenAPI drift check, Docker build verification

---

## Verification Checklist (MVP Sign-Off)

- [ ] `make dev` starts all services cleanly
- [ ] Auth login and protected route rejection
- [ ] API token raw value shown once only
- [ ] Monitor checks write visible history in TimescaleDB
- [ ] Secrets redacted in all API responses
- [ ] `/metrics` exposes required Prometheus series
- [ ] Frontend handles 500 monitor mock load without freezing
- [ ] Realtime status updates via WebSocket patches
- [ ] TCP, UDP, and WebSocket checks execute successfully
- [ ] History endpoint returns time-series data
- [ ] Fresh `docker-compose up` works end-to-end on clean machine
