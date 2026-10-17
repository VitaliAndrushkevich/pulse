# Pulse — Project Milestones

Full milestone breakdown for Pulse. The at-a-glance status table also lives in the
"Current Progress" section of [AGENTS.md](../AGENTS.md); this file carries the per-milestone detail.

| Milestone | Status |
|-----------|--------|
| A: Foundations | ✅ Done |
| B: Data Layer | ✅ Done |
| C: Security & Secrets | ✅ Done |
| D: Monitor Engine | ✅ Done |
| E: API Surface | ✅ Done |
| F: WebSocket Realtime | ✅ Done |
| G: Frontend Product | ✅ Done |
| H: Packaging & Release | ✅ Done |
| I: Branding & Theming | ✅ Done |
| J: Notifications | ✅ Done |
| K: Heartbeat (push) Monitoring (PF-01 first useful version) | ✅ Done |

---

## Milestone A: Foundations ✅ Done
Repository bootstrap, local developer workflow, baseline deployment pipeline: Go backend module,
SvelteKit + TypeScript + Tailwind frontend scaffold, docker-compose files, `Makefile`, migration
tooling, and the multi-stage Dockerfile (Go build → distroless runtime).

## Milestone B: Data Layer ✅ Done
Durable config/state in PostgreSQL and time-series writes/reads through TimescaleDB: full schema with
indexes, the `sqlc`-generated typed query layer, TimescaleDB write/query helpers, fail-fast startup
on unreachable dependencies, `X-Request-ID` middleware, and `/healthz`.

## Milestone C: Security & Secrets ✅ Done
AES-256-GCM encryption module, write-only secret handling, API token create/list/revoke with bcrypt
hash storage, log-sanitization middleware, and the key-rotation command with transactional re-encryption.

## Milestone D: Monitor Engine ✅ Done
The `Checker` interface and all protocol checkers (HTTP/HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC,
DNS, ICMP, SMTP, QUIC), the bounded worker-pool scheduler, priority-based polling, dual-write results,
PostgreSQL `LISTEN/NOTIFY` wakeups, and graceful shutdown.

## Milestone E: API Surface ✅ Done
Full `gin` router under `/api/v1` with the standard error envelope, JWT + combined API-token auth,
monitor CRUD with idempotent `PUT`, history and incidents endpoints, Prometheus `/metrics`, and the
committed OpenAPI 3.0.3 spec as source of truth.

## Milestone F: WebSocket Realtime ✅ Done
The WebSocket hub with fan-out, keepalive, and slow-consumer eviction; the typed message envelope with
`monitor_status` (diff/patch) and `connected`; scheduler → hub broadcast; and the authenticated `/ws`
endpoint with query-param token validation.

## Milestone G: Frontend Product ✅ Done
The full SvelteKit app: type system, API client, Svelte 5 runes stores (patch-merge), WebSocket client
with backoff, `VirtualList` DOM recycling, monitor list/detail/create/edit pages, settings, and
real-time in-place updates.

## Milestone H: Packaging & Release ✅ Done
Static frontend embedded via `//go:embed` with SPA catch-all, the multi-stage distroless Dockerfile,
production docker-compose, documented `.env.example`, and the CI pipeline (backend + frontend checks).

## Milestone I: Branding & Theming ✅ Done
The CSS custom-property theme system, Tailwind integration, FOUC-prevention script, self-hosted Inter
font, `BrandLockup` and `ThemeSwitcher` components (tri-state light/dark/system cycling), static brand
assets and generators, and property-based theming tests.

## Milestone J: Notifications ✅ Done
The asynchronous, non-blocking notification pipeline: email (SMTP) and webhook channels, per-monitor
bindings and triggers, bounded delivery worker pool, exponential-backoff retry, delivery logs, and
the reminder scheduler.

## Milestone K: Heartbeat (push) Monitoring (PF-01 first useful version) ✅ Done
Heartbeat (push) monitoring as a resource parallel to monitors — the "dead man's switch" for scheduled
jobs. Delivered in this first useful version:

- Heartbeat resource with `expected_interval_seconds`, `grace_seconds`, `last_success_at`, persisted
  `next_expected_deadline`, lifecycle `status` (`active`/`paused`), and observed `state`
  (`up`/`down`/`paused`/`new`).
- Admin CRUD under `/api/v1/heartbeats` (idempotent `PUT`, paginated list, tags) plus `rotate-token`
  and per-heartbeat notification bindings, all behind combined JWT/API-token auth.
- Root-level, token-scoped ping ingestion (`/ping/{token}` success, `/ping/{token}/fail` explicit
  failure) with ingestion strictly separated from administration.
- Scoped ingestion credential: a 256-bit random ping token stored as SHA-256 + lookup prefix, returned
  raw exactly once on create/rotate, verified with a constant-time compare.
- A bounded missed-run sweep evaluator (default 50 workers via `PULSE_HEARTBEAT_WORKERS`, batch 500,
  partial index) that marks a heartbeat down once per missed-run episode.
- Missed-run and recovery notifications routed through the existing dispatcher, StateTracker, retry
  queue, and delivery logs via a dedicated `heartbeat_bindings` table and trigger translation.
- Realtime `heartbeat_status` WebSocket diff/patch messages, Prometheus metrics
  (`pulse_heartbeat_up`, `pulse_heartbeats_total`), and a full localized frontend UI
  (list/detail/create/edit + nav entry) with ready-to-copy shell/cron/CI ping examples.

Explicitly deferred as non-goals (not shipped in this version): run-lifecycle events (run ids /
per-run duration), timezone-aware cron schedules, concurrent-run / duplicate / late-arrival /
clock-boundary semantics, a dedicated MCP heartbeat tool, restore-validity verification, an
independently hosted external receiver, and per-ping history/charts.
