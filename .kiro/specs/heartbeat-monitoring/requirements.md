# Heartbeat and Scheduled-Job Monitoring — Requirements (PF-01, First Useful Version)

## Introduction

Pulse currently monitors reachability by actively probing targets (HTTP, TCP, DNS, etc.). This leaves a blind spot: a host can stay online while scheduled jobs — backups, invoice generation, certificate renewal, data synchronization, nightly imports — silently stop running. Active probing cannot see a cron job that never fired.

This feature adds **heartbeat monitoring** (also called "dead man's switch" or passive/push monitoring, in the style of [Healthchecks cron monitoring](https://healthchecks.io/docs/monitoring_cron_jobs/)). Instead of Pulse calling out to a target, the scheduled job calls *in* to Pulse at the end of a successful run. Pulse records the arrival, computes when the next ping is expected, and raises an alert when the deadline passes with no ping. The job can also report an explicit failure.

This document specifies **only the "first useful version"** of PF-01. It conforms to the project conventions in `AGENTS.md`: API-first `/api/v1` contract with the OpenAPI spec as source of truth, stable UUIDs, idempotent `PUT`, paginated lists, the standard error envelope, `X-Request-ID`, combined JWT + API-token admin auth, encryption/write-only secret handling, the existing bounded-worker-pool and notification/fan-out patterns, native WebSocket diff/patch broadcasts, and mandatory i18n on the SvelteKit frontend.

### Terminology

- **Heartbeat** — a monitored scheduled job resource. The administrative/configuration object.
- **Ping** — an inbound HTTP request from the job to Pulse reporting a success or an explicit failure.
- **Ping token** — an unguessable opaque credential embedded in the ping URL. It authorizes reporting for exactly one heartbeat and nothing else.
- **Expected interval** — how often the job is expected to ping (success).
- **Grace period** — extra time allowed after the interval elapses before the heartbeat is declared missed.
- **Next-expected-deadline** — the persisted absolute timestamp `last_success + interval + grace`. The evaluator declares the heartbeat down when `now` passes this deadline.

### Resource and status model (decided)

A heartbeat has: stable UUID `id`, `name`, `expected_interval_seconds`, `grace_seconds`, `last_success_at` (nullable), `next_expected_deadline` (nullable, persisted), `status` (lifecycle: `active` | `paused`), `state` (observed: `up` | `down` | `paused` | `new`), `tags`, `created_at`, `updated_at`, plus write-only ping-token material (`token_prefix` stored for display, `token_hash` stored for verification; raw token returned only at creation/rotation). See the glossary note below on the two separate status fields.

> **Lifecycle vs. observed state.** The existing `monitors` table carries both a lifecycle `status` (`active`/`paused`) and an observed `state` (`up`/`down`/`unknown`); see `backend/migrations/001_initial.up.sql` and `backend/internal/api/handlers/monitors.go`. Heartbeats follow the same two-field split. The scope brief lists `up/down/paused/new` as the status values the UI shows; this spec models them as the **observed `state`** (`new` = created, never pinged, deadline not yet meaningful; `paused` = surfaced when lifecycle `status = paused`), while lifecycle `status` stays `active`/`paused` to match the monitors convention. Requirement acceptance criteria below use `state` for observed values and `status` for lifecycle.

---

## Requirement 1: Heartbeat administrative CRUD API

**User story:** As an operator, I want to create, list, read, update, and delete heartbeats through the authenticated `/api/v1` API, so that I can manage scheduled-job monitoring the same way I manage probe monitors.

Grounding: mirror `MonitorHandler` (`backend/internal/api/handlers/monitors.go`) — route group `/heartbeats`, `apiError` envelope and `parsePagination` from `helpers.go`, `max limit = 100`, transactional create/upsert with tags, UUID path parsing with `INVALID_ID`.

### Acceptance Criteria

1.1. GIVEN a valid JWT or Bearer API token WHEN the client sends `POST /api/v1/heartbeats` with a valid body THEN the system creates a heartbeat with a server-generated UUID, persists it, and responds `201 Created` with the heartbeat representation **including the raw ping token exactly once** (see Requirement 6).

1.2. GIVEN heartbeats exist WHEN the client sends `GET /api/v1/heartbeats?page={p}&limit={l}` THEN the system responds `200 OK` with a paginated envelope `{ data, total, page, limit, total_pages }`, defaulting `page=1`, `limit=20`, and clamping `limit` to a maximum of 100.

1.3. GIVEN the list endpoint WHEN `page` or `limit` is non-numeric or out of range THEN the system applies the same tolerant defaulting behavior used by `parsePagination` (invalid values fall back to defaults rather than erroring), consistent with the monitors list endpoint.

1.4. GIVEN a heartbeat with id `{id}` WHEN the client sends `GET /api/v1/heartbeats/{id}` THEN the system responds `200 OK` with the heartbeat representation and **never** includes the raw ping token or token hash (only `token_prefix` and a reconstructable ping URL hint are allowed).

1.5. GIVEN no heartbeat with id `{id}` WHEN the client sends `GET /api/v1/heartbeats/{id}` THEN the system responds `404 Not Found` with error code `NOT_FOUND`.

1.6. GIVEN any heartbeat endpoint WHEN `{id}` is not a valid UUID THEN the system responds `400 Bad Request` with error code `INVALID_ID`.

1.7. GIVEN a `POST`/`PUT` body WHEN `name` is missing/empty, OR `expected_interval_seconds` is not a positive integer, OR `grace_seconds` is negative THEN the system responds `400 Bad Request` with error code `VALIDATION_ERROR` and a readable message; interval and grace bounds are enforced (interval minimum 1 second; sensible documented maximums).

1.8. GIVEN a heartbeat WHEN the client sends `DELETE /api/v1/heartbeats/{id}` THEN the system deletes the heartbeat and its ping-token material, cascades its notification bindings, clears any in-memory evaluator/notification state for that id, and responds `204 No Content`.

1.9. GIVEN tags provided on create/update WHEN they are validated THEN the system reuses the existing `tags.ValidateTags` rules and persists them in the same transaction as the heartbeat, returning `400 INVALID_TAGS` on failure (mirrors monitors).

1.10. GIVEN all heartbeat admin endpoints WHEN a request arrives without a valid JWT or Bearer API token THEN the system responds `401 Unauthorized` with error code `UNAUTHORIZED` (combined-auth middleware, same protected group as monitors).

---

## Requirement 2: Idempotent create-or-update (PUT)

**User story:** As an operator automating configuration, I want `PUT /api/v1/heartbeats/{id}` to be idempotent create-or-update, so that re-applying the same configuration does not create duplicates — matching the `PUT /monitors/{id}` contract.

Grounding: `MonitorHandler.Put` uses `UpsertMonitor` with row-level locking (`GetMonitorForUpdate` → `SELECT ... FOR UPDATE`) and maps serialization failures (`40001`/`40P01`/`55P03`) to `409 CONFLICT`.

### Acceptance Criteria

2.1. GIVEN no heartbeat with id `{id}` WHEN the client sends `PUT /api/v1/heartbeats/{id}` with a valid body THEN the system creates the heartbeat with that id and responds `200 OK`.

2.2. GIVEN an existing heartbeat with id `{id}` WHEN the client sends `PUT /api/v1/heartbeats/{id}` with a valid body THEN the system updates the mutable fields (name, interval, grace, tags, lifecycle status) in place without creating a new resource and responds `200 OK`.

2.3. GIVEN the same valid `PUT` request is sent twice with identical bodies THEN the resulting persisted heartbeat state is identical after both calls (idempotent), and no duplicate rows or duplicate tags are created.

2.4. GIVEN a `PUT` that changes `expected_interval_seconds` or `grace_seconds` WHEN the heartbeat already has a `last_success_at` THEN the system recomputes and persists `next_expected_deadline = last_success_at + new_interval + new_grace` so the new schedule takes effect immediately; if there is no `last_success_at`, `next_expected_deadline` remains null and `state` remains `new`.

2.5. GIVEN concurrent `PUT`s on the same id WHEN a database serialization/lock conflict occurs THEN the system responds `409 Conflict` with error code `CONFLICT` and a retry-advising message (mirrors monitors).

2.6. GIVEN a `PUT` body WHEN it is invalid (same validation as 1.7) THEN the system responds `400 VALIDATION_ERROR` and does not create or modify any heartbeat.

> **Decision — ping token on PUT.** `PUT` does **not** rotate the ping token (a rotation would silently break every already-deployed cron entry). The raw token is returned only on initial `POST` creation and on the explicit rotate endpoint (Requirement 6.5). `PUT` responses never contain the raw token.

---

## Requirement 3: Ping ingestion endpoint (success and explicit failure)

**User story:** As a scheduled job, I want to report success or explicit failure with a single unauthenticated HTTP call carrying only my unguessable token, so that cron/CI jobs can signal completion without holding admin credentials.

Grounding: root-level placement like `/ws` (registered directly on the engine in `router.go`, not under `/api/v1`); the `apiPrefixes` SPA guard in `router.go` must be extended so `/ping/...` returns the ingestion response, not the SPA fallback. Token verification reuses the prefix-lookup + bcrypt + constant-time dummy-compare pattern from `combinedAuth` / `WSHandler.validateToken` and `internal/token`.

### Acceptance Criteria

3.1. GIVEN a heartbeat with a valid ping token WHEN a client sends `GET /ping/{token}` (success ping) THEN the system records the success, updates `last_success_at = now`, recomputes and persists `next_expected_deadline = now + expected_interval_seconds + grace_seconds`, transitions `state` to `up`, and responds `200 OK` with a minimal body (e.g. `OK`).

3.2. GIVEN a heartbeat with a valid ping token WHEN a client sends `GET /ping/{token}/fail` (explicit failure ping) THEN the system records the explicit failure, transitions `state` to `down`, triggers the missed/failure notification path (Requirement 5), and responds `200 OK` with a minimal body; the system does **not** advance `last_success_at` on a failure ping.

3.3. GIVEN the ping endpoints WHEN a `POST` is sent instead of `GET` THEN the system accepts it identically (both `GET` and `POST` are allowed, because cron wrappers and CI runners vary; a body, if any, is ignored in the first version).

3.4. GIVEN an unknown, malformed, or revoked token WHEN a ping is received THEN the system performs a constant-time dummy bcrypt comparison (timing-safe, mirroring `dummyCompare` / `wsDummyHash`) and responds `404 Not Found` with a minimal body, revealing nothing about whether the token prefix matched.

3.5. GIVEN the ping endpoints WHEN they are called THEN they require **no** JWT, **no** Bearer API token, and **no** session — authorization is the possession of the unguessable token only, and a valid token authorizes reporting for exactly that one heartbeat and no administrative capability whatsoever (hard separation of ingestion from administration).

3.6. GIVEN a successful or failure ping WHEN it is processed THEN the system broadcasts the resulting state change over the WebSocket hub (Requirement 8) and the response latency is minimal (ingestion must not block on notification delivery or WS fan-out; both are dispatched non-blocking as the scheduler does today).

3.7. GIVEN a ping arrives WHEN logs are written THEN the raw token is never logged in full (log the `token_prefix` or heartbeat id only), consistent with the log-sanitization requirement.

3.8. GIVEN a heartbeat whose lifecycle `status = paused` WHEN a success ping arrives THEN the system still records `last_success_at` but does **not** evaluate missed-run alerts while paused (see Requirement 4.6); the ping is accepted (`200 OK`) rather than rejected.

---

## Requirement 4: Deadline evaluation and missed-run detection

**User story:** As an operator, I want Pulse to detect when a heartbeat's deadline passes with no ping and mark it down, so that silent job failures surface as alerts.

Grounding: reuse the bounded-worker-pool evaluator pattern from `backend/internal/monitor/scheduler.go` — a ticking loop that queries due rows (indexed by a deadline column, like `idx_monitors_next_check_at`), dispatches to a fixed worker pool, never spawns unbounded goroutines, and is designed for 500+ resources (`DefaultBatchSize = 500`). Reuse graceful shutdown via `context` cancellation.

### Acceptance Criteria

4.1. GIVEN a heartbeat in `state = up` with a persisted `next_expected_deadline` WHEN `now > next_expected_deadline` and no ping has arrived THEN the evaluator transitions the heartbeat to `state = down` (missed run), persists the transition, and triggers the missed-run notification path (Requirement 5).

4.2. GIVEN a heartbeat marked `down` for a missed run WHEN a subsequent success ping arrives THEN the heartbeat transitions `down → up` (recovery), `last_success_at` and `next_expected_deadline` are updated, and the recovery notification path fires (Requirement 5.2).

4.3. GIVEN a newly created heartbeat that has never been pinged (`state = new`, `next_expected_deadline` null) WHEN the evaluator runs THEN it does **not** mark the heartbeat down (no deadline exists yet); the heartbeat leaves `new` only upon its first ping.

4.4. GIVEN the evaluator loop WHEN it polls for due heartbeats THEN it fetches a bounded batch (default 500) of heartbeats whose `next_expected_deadline` has passed and whose `state` is not already `down`/`paused`, using an indexed query, and processes them through a bounded worker pool whose size is configurable via an environment variable (see Requirement 10) with no unbounded goroutine growth.

4.5. GIVEN the evaluator is running WHEN the process receives a shutdown signal (context cancelled) THEN the loop stops cleanly, drains in-flight work, and exits without leaking goroutines (mirrors scheduler shutdown).

4.6. GIVEN a heartbeat whose lifecycle `status = paused` WHEN the evaluator runs THEN it is skipped entirely (no missed-run detection, no alert) and its observed `state` is reported as `paused`.

4.7. GIVEN a missed-run transition WHEN it is detected THEN the system marks the heartbeat down **once** per missed-run episode (it must not re-fire a fresh "down" alert on every evaluator tick while the heartbeat stays down), consistent with the once-per-transition dedup behavior of the existing `StateTracker`.

4.8. GIVEN the evaluator tick interval WHEN it is chosen THEN deadline detection latency (time between deadline passing and the heartbeat being marked down) is bounded and documented (e.g. a short fixed tick, like the scheduler's 1s tick), so missed runs are detected promptly even at 500+ heartbeats.

---

## Requirement 5: Missed-run and recovery notifications

**User story:** As an operator, I want to be alerted when a heartbeat misses its run or explicitly fails, and when it recovers, using Pulse's existing notification channels, so that I don't have to build a separate alerting path.

Grounding: integrate into the existing dispatcher/fan-out/state-dedup subsystem (`backend/internal/notification/` — `dispatcher.go`, `fanout.go`, `state_tracker.go`, `validation.go`). Reuse the binding model (`channel_bindings`-style), the `EvaluateAndDispatch` fan-out, the bounded delivery worker pool, non-blocking enqueue (`select/default`), delivery logging, and reminder scheduler. Reuse channel CRUD (`/api/v1/notifications/channels`) and the per-resource binding handler pattern (`NotificationBindingHandler`).

### Acceptance Criteria

5.1. GIVEN a heartbeat with a notification binding whose triggers include a "down"/missed trigger WHEN the heartbeat transitions to `down` (missed run per 4.1, or explicit failure per 3.2) THEN the system enqueues a notification delivery job per matching binding, non-blocking, through the existing dispatcher, and records a delivery-log entry for each attempt.

5.2. GIVEN a heartbeat binding whose triggers include a "recovery"/up trigger WHEN the heartbeat transitions `down → up` THEN the system fires a recovery notification once per recovery, mirroring the `monitor_up` recovery semantics in `StateTracker.Evaluate`.

5.3. GIVEN a heartbeat that remains `down` across multiple evaluator ticks WHEN state is unchanged THEN the system does **not** send duplicate "down" notifications for the same ongoing missed-run episode (state deduplication), reusing the StateTracker once-per-transition semantics; the dedup flags are cleared on recovery.

5.4. GIVEN heartbeat notification bindings WHEN the client manages them THEN the API exposes `POST`/`GET`/`PUT`/`DELETE /api/v1/heartbeats/{id}/notification-bindings[/{bindingId}]`, paginated list, validating `channel_id` presence and trigger validity, reusing the binding handler conventions (`NotificationBindingHandler`) and the standard error envelope with field-level `details`.

5.5. GIVEN heartbeat trigger types WHEN they are validated THEN the allowed heartbeat trigger set is defined (minimum: `heartbeat_missed` and `heartbeat_recovered`; an explicit-failure trigger may be represented by `heartbeat_missed` firing on explicit-fail pings) and validation returns `400 VALIDATION_ERROR` with `details` for unknown types, following the `ValidateTriggers` pattern. (The existing monitor trigger types such as `degraded`/`ssl_expiring`/`n_failures_in_row` do **not** apply to heartbeats and are rejected for heartbeat bindings.)

5.6. GIVEN a heartbeat delivery is attempted and fails with a retryable error WHEN it is processed THEN it uses the existing exponential-backoff retry queue (30s → 60s → 120s, max 4 attempts) unchanged; non-retryable errors fail permanently and are logged.

5.7. GIVEN the notification state keyed per resource WHEN a heartbeat is deleted THEN its notification/dedup state is cleared (mirroring `StateTracker.ClearState` on monitor deletion) and its bindings are removed via cascade.

5.8. GIVEN notification payload/template data WHEN a heartbeat alert is rendered THEN it carries at least the heartbeat id, name, state, previous state, the missed deadline / last success timestamp, and `BaseURL` for links, reusing the existing `TemplateData` shape (extended minimally for heartbeats where needed) so email/webhook channels render consistently.

> **Decision — binding/state keying.** The existing dispatcher keys `StateTracker` and bindings by `monitor_id` (a `uuid.UUID`). Heartbeat bindings and heartbeat notification state are keyed by the heartbeat's UUID through the same code paths. The design phase must choose between (a) a separate heartbeat bindings/evaluation path reusing the notification primitives, or (b) generalizing the dispatcher's resource key; this spec requires reuse of the dedup, fan-out, retry, and delivery-log behavior either way, and does not permit a parallel ad-hoc alerting implementation.

---

## Requirement 6: Scoped ingestion credential (unguessable, write-only, admin-separated)

**User story:** As a security-conscious operator, I want each heartbeat's ping credential to be unguessable, shown only once, scoped to a single heartbeat, and incapable of administration, so that distributing a ping URL to a cron host never risks admin access or secret disclosure.

Grounding: reuse `internal/token` (`crypto/rand` 256-bit → base64url, `token_prefix`, bcrypt hash) and the write-only "shown once" response pattern from `TokenHandler.Create` (`createTokenResponse` returns raw token once; list/get never return the hash). Verification mirrors `combinedAuth` prefix-lookup + bcrypt.

### Acceptance Criteria

6.1. GIVEN a heartbeat is created WHEN the ping token is generated THEN it is produced from a cryptographically secure RNG with at least 256 bits of entropy, encoded unguessably (base64url, no padding), via the existing `token.Generate()` primitive or an equivalent in the same package.

6.2. GIVEN a heartbeat is created or its token rotated WHEN the response is returned THEN the **raw** ping token (and/or the full ping URL containing it) is included exactly once in that single response and never again.

6.3. GIVEN any subsequent `GET`/`LIST` of a heartbeat WHEN the response is built THEN it exposes only non-secret token metadata (`token_prefix`, created/rotated timestamp) and never the raw token or the bcrypt hash.

6.4. GIVEN the ping token at rest WHEN it is persisted THEN only a bcrypt hash and a lookup prefix are stored (never the raw token, never a reversibly encrypted form), consistent with the API-token storage model.

6.5. GIVEN an operator needs to invalidate a leaked token WHEN they call `POST /api/v1/heartbeats/{id}/rotate-token` (authenticated) THEN the system generates a new token, invalidates the old one (old ping URLs stop working), and returns the new raw token exactly once.

6.6. GIVEN the ping token WHEN it is used at the `/ping/...` endpoint THEN it grants only the capability to report success/failure for its own heartbeat and confers no read/list/update/delete or any other administrative capability (ingestion is strictly separated from administration — hard requirement).

6.7. GIVEN token verification WHEN an invalid token is presented THEN a constant-time dummy comparison runs so response timing does not leak whether a prefix matched (reuse the dummy-hash pattern).

---

## Requirement 7: OpenAPI contract co-change and MCP evaluation

**User story:** As an API consumer and as a maintainer, I want the OpenAPI contract to describe every heartbeat endpoint in the same change that adds the handlers, so that the spec remains the authoritative API surface.

Grounding: `AGENTS.md` API Conventions — "OpenAPI contract is the source of truth ... MUST be reflected in `backend/api/openapi.yaml` in the same commit." Spec is OpenAPI 3.0.3 at `backend/api/openapi.yaml` (3078 lines, existing `/monitors`, `/notifications/*` paths). `make openapi` validates it.

### Acceptance Criteria

7.1. GIVEN heartbeat handlers are added or changed WHEN the change is prepared THEN `backend/api/openapi.yaml` is updated in the **same change** to document every new path: `/heartbeats`, `/heartbeats/{id}`, `/heartbeats/{id}/rotate-token`, `/heartbeats/{id}/notification-bindings`, `/heartbeats/{id}/notification-bindings/{bindingId}`, and the root-level `/ping/{token}` and `/ping/{token}/fail` ingestion paths.

7.2. GIVEN the OpenAPI additions WHEN they are written THEN each path documents request/response schemas, the standard error envelope (`error.code`/`error.message`), pagination params (`page`,`limit`) on list endpoints, the `X-Request-ID` response header, and the security scheme (combined JWT/API-token for admin paths; **no** auth scheme on the `/ping` paths, documented as token-in-path).

7.3. GIVEN the write-only token WHEN the schemas are defined THEN the create/rotate response schema shows the raw token field while the get/list schemas omit it, making the write-only contract explicit.

7.4. GIVEN the finished spec WHEN validated THEN `make openapi` passes with no errors.

7.5. GIVEN the MCP co-change rule in `AGENTS.md` WHEN heartbeat endpoints are assessed THEN the requirement records that the heartbeat endpoints are **new** resources not in the MCP co-change endpoint list (which covers `GET/POST/PUT/DELETE /monitors*`, `/incidents*`, `/tags*`, `/dashboard/summary`), so **no existing MCP tool or `PulseClient` method requires modification** for this first version; adding a new MCP heartbeat tool is explicitly a non-goal (deferred). This evaluation itself satisfies the co-change rule.

---

## Requirement 8: Realtime status updates over WebSocket

**User story:** As a user watching the UI, I want heartbeat status changes to appear in real time without a refresh, so that missed runs and recoveries are visible immediately.

Grounding: reuse the hub broadcast pattern (`backend/internal/hub/hub.go`, `messages.go` — typed envelope `{ type, payload }`, diff/patch payloads, buffered broadcast that drops slow consumers). Frontend consumes via `frontend/src/lib/ws.ts` → store `applyPatch` (patch-merge, discard unknown ids).

### Acceptance Criteria

8.1. GIVEN a heartbeat changes observed `state` (via ping success, explicit failure, missed-run detection, or recovery) WHEN the change is persisted THEN the backend broadcasts a diff/patch message over the existing hub carrying the heartbeat id, new `state`, `last_success_at`, `next_expected_deadline`, and a server timestamp.

8.2. GIVEN the WebSocket message envelope WHEN a heartbeat update is sent THEN it uses a dedicated message type (e.g. `heartbeat_status`) added alongside the existing `monitor_status`/`connected`/`monitor_tags_changed` types, following the diff/patch convention (clients merge, not replace).

8.3. GIVEN the hub broadcast channel is full (slow consumer) WHEN a heartbeat message cannot be delivered THEN the message is dropped for that consumer rather than blocking the broadcast, consistent with existing hub behavior.

8.4. GIVEN the frontend receives a `heartbeat_status` patch WHEN it is dispatched THEN a heartbeat store applies it with patch-merge semantics into local state (reusing the `monitorStore.applyPatch` approach), discarding patches for unknown heartbeat ids, so open list/detail views update in place.

8.5. GIVEN a heartbeat ingestion ping WHEN it triggers a broadcast THEN the broadcast is non-blocking relative to the ping response (the ping returns promptly regardless of hub state), matching the scheduler's non-blocking broadcast.

---

## Requirement 9: Frontend UI with mandatory i18n

**User story:** As a user, I want pages to create, view, edit, and list heartbeats, copy the ping URL, and see ready-to-paste cron/shell/CI examples, with all text localized, so that I can set up job monitoring quickly in my language.

Grounding: SvelteKit routes under `frontend/src/routes/` (mirror `monitors/` → list `+page.svelte`, `[id]/`, `[id]/edit/`, `create/`), API client additions in `frontend/src/lib/api.ts`, WS handling in `ws.ts`, a store in `frontend/src/lib/stores/`, components in `frontend/src/components/`. Reuse `VirtualList.svelte`, `Pagination.svelte`, `TagEditor.svelte`, and the write-once `ShowOnceModal.svelte`. Nav entry added like `nav.monitors`. Mandatory i18n via `t()` with keys added to `en.json` first then all 13 locale files.

### Acceptance Criteria

9.1. GIVEN the app WHEN the user navigates THEN a "Heartbeats" entry appears in the main nav and routes to a heartbeat list page that lists heartbeats with pagination and, for large collections, DOM-recycling virtualization (reusing `VirtualList`/`Pagination`), showing per-heartbeat observed state (`up`/`down`/`paused`/`new`), name, interval, and last success.

9.2. GIVEN create and edit pages WHEN the user submits the form THEN it calls the heartbeat API client functions (added to `api.ts`, Bearer-authenticated, 15s timeout, error-envelope parsing, `X-Request-ID`) to `POST`/`PUT` the heartbeat, with client-side validation for name, interval, and grace.

9.3. GIVEN a heartbeat was just created or its token rotated WHEN the raw ping token/URL is returned THEN the UI shows it exactly once in a `ShowOnceModal`-style dialog with copy-to-clipboard, and the raw value is cleared from component state on dismiss and never re-fetched.

9.4. GIVEN a heartbeat detail page WHEN it renders THEN it shows the ping URL (success and `/fail` variants), a copy-to-clipboard control, observed state, interval, grace, last success, next expected deadline, tags, and a notification-bindings section reusing the per-resource binding UI pattern.

9.5. GIVEN the detail/create page WHEN it displays usage help THEN it shows ready-to-copy shell (`curl`), cron, and CI examples that embed the heartbeat's ping URL (see Requirement 11), each with copy-to-clipboard.

9.6. GIVEN any new user-visible string WHEN it is added THEN it is referenced via `t('heartbeats.*')` (or `common.*` for shared strings) and the key is added to `frontend/src/locales/en.json` first, then to **every** other locale file (`ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh`) using the English value as a placeholder where no translation exists; no display string is hardcoded in templates or scripts.

9.7. GIVEN theming and dark mode WHEN components are styled THEN they use CSS custom properties / brand utilities from `app.css` and the `[data-theme="dark"]` strategy — never Tailwind's `dark:` prefix and never hardcoded brand color classes.

9.8. GIVEN localization validation WHEN the frontend check runs THEN `pnpm --filter frontend run validate-locales` passes (no missing keys across locales), and for RTL (Arabic) the heartbeat pages use direction-aware CSS (e.g. logical margins) so layout renders correctly.

9.9. GIVEN real-time updates WHEN a `heartbeat_status` patch arrives THEN the open list and detail views update in place via the heartbeat store (Requirement 8.4) without a full reload.

---

## Requirement 10: Performance and bounded concurrency

**User story:** As an operator running many jobs, I want heartbeat evaluation and ingestion to stay reliable at 500+ heartbeats, so that scale does not degrade detection or the UI.

Grounding: `AGENTS.md` performance rules + scheduler constants (`DefaultBatchSize = 500`, bounded worker pool sized by env var, buffered channels, diff/patch WS, frontend virtualization).

### Acceptance Criteria

10.1. GIVEN the heartbeat evaluator WHEN it runs THEN it uses a bounded worker pool (no unbounded goroutines) sized via a dedicated environment variable (e.g. `PULSE_HEARTBEAT_WORKERS`) with a documented default, and processes due heartbeats in bounded batches (default 500), designed to handle 500+ heartbeats.

10.2. GIVEN due-heartbeat lookup WHEN the evaluator queries the database THEN the query is backed by an index on `next_expected_deadline` (partial index excluding paused heartbeats, analogous to `idx_monitors_next_check_at`), so polling stays efficient at scale.

10.3. GIVEN WebSocket updates WHEN heartbeat state changes THEN only diff/patch payloads are broadcast (never full-state snapshots), reusing the hub's buffered, drop-on-full behavior.

10.4. GIVEN the frontend heartbeat list WHEN it renders a large collection THEN it uses virtualization (DOM recycling) so render stays non-blocking.

10.5. GIVEN ping ingestion WHEN a burst of pings arrives THEN each ping does a single indexed token-prefix lookup plus bcrypt verify and returns promptly, with notification fan-out and WS broadcast dispatched non-blocking (the ping request path never blocks on them).

---

## Requirement 11: Shell, cron, and CI ping examples (documentation)

**User story:** As a new user, I want copy-pasteable examples for shell, cron, and CI, so that I can instrument a real job in minutes and measure setup time (per the PF-01 pilot).

Grounding: PF-01 explicitly requires "shell, cron, and CI examples." Pulse ships docs (README, `docs/`) and the UI shows usage help (Requirement 9.5).

### Acceptance Criteria

11.1. GIVEN the documentation WHEN heartbeat usage is described THEN it includes a **shell** example using `curl` to send a success ping at the end of a job and (optionally) a `/fail` ping on error, including a pattern that reports failure when the preceding command exits non-zero.

11.2. GIVEN the documentation WHEN cron usage is described THEN it includes a **cron** example wrapping a scheduled command so a success ping fires only after the command succeeds (and a `/fail` ping fires on failure), with a note that pinging before the work completes defeats the purpose.

11.3. GIVEN the documentation WHEN CI usage is described THEN it includes a **CI** example (e.g. a pipeline step) that pings on job success, suitable for a nightly/scheduled pipeline.

11.4. GIVEN the examples WHEN rendered in the UI THEN they are populated with the specific heartbeat's ping URL and offer copy-to-clipboard (Requirement 9.5); the same examples (with a placeholder URL) appear in the docs.

11.5. GIVEN the examples WHEN written THEN they note the independence principle from PF-01: an independently hosted receiver can detect loss of Pulse itself, and a success ping proves the command ran, not that its output (e.g. a backup) is valid/restorable — so examples do not overstate what a green heartbeat guarantees.

---

## Requirement 12: Security and log sanitization

**User story:** As a security reviewer, I want heartbeat handling to meet Pulse's security requirements, so that the feature introduces no secret-disclosure or injection risk.

Grounding: `AGENTS.md` Security Requirements; `middleware.SanitizedLogger`; constant-time comparisons already used for tokens.

### Acceptance Criteria

12.1. GIVEN any API response WHEN it is produced THEN no raw ping token is returned except the single create/rotate response (Requirement 6.2), and no bcrypt hash is ever returned.

12.2. GIVEN logs WHEN heartbeat/ping activity is logged THEN auth material is stripped and raw tokens are never written in full (prefix or id only), reusing the sanitized-logger approach.

12.3. GIVEN ping token verification WHEN any invalid token is presented THEN a constant-time dummy bcrypt comparison runs regardless of whether the prefix matched (timing-safe), responding `404` without distinguishing failure modes.

12.4. GIVEN the ping endpoints WHEN they are exposed at the root level THEN they are reachable without admin auth by design, but confer no capability beyond reporting for their one heartbeat, and the SPA catch-all / `apiPrefixes` guard is updated so `/ping/...` is handled by the ingestion handler (not the SPA fallback and not a JSON 404 meant for `/api/`).

12.5. GIVEN heartbeat metric labels and WS payloads WHEN they are built THEN they contain no token material and no secret values.

---

## Requirement 13: Prometheus metrics

**User story:** As an operator, I want heartbeat metrics in the existing `/metrics` endpoint using the established naming, so that I can observe heartbeat health and alert in Prometheus.

Grounding: existing metrics use `pulse_*` names (`pulse_monitor_up`, `pulse_monitors_total`, `pulse_notification_*`) via `prometheus/client_golang`, exposed at `/metrics` (optionally Basic-Auth protected).

### Acceptance Criteria

13.1. GIVEN the `/metrics` endpoint WHEN heartbeat metrics are added THEN they follow the `pulse_*` naming convention and are registered on the existing Prometheus registry.

13.2. GIVEN heartbeat metrics WHEN defined THEN they include at minimum a per-heartbeat up/down gauge (e.g. `pulse_heartbeat_up`) and a total-heartbeats gauge (e.g. `pulse_heartbeats_total`), with labels consistent with existing monitor metrics and containing no secret material.

13.3. GIVEN heartbeat state changes WHEN they occur THEN the corresponding metric values are updated (up on success/recovery, down on miss/explicit-fail), analogous to how the scheduler updates monitor metrics.

---

## Requirement 14: Project documentation update

**User story:** As a maintainer, I want the project documentation updated in the same change that adds heartbeat monitoring, so that `AGENTS.md`, the README, and the milestone record stay in sync with the shipped code and a future contributor can understand the subsystem without reading the source.

Grounding: `AGENTS.md` is the authoritative project guideline file and already documents each subsystem in depth (e.g. the "Notifications" and "Localization (i18n)" sections, the "Current Progress" milestone table, the "Key Files Reference" table, and the environment-variable table). `docs/MILESTONES.md` holds the full milestone breakdown referenced from `AGENTS.md`. The README ships the user-facing quick start and feature docs. Documentation is a distinct deliverable, not a side effect of code tasks.

### Acceptance Criteria

14.1. GIVEN heartbeat monitoring is implemented WHEN `AGENTS.md` is updated THEN it gains a dedicated "Heartbeats" subsystem section (modeled on the existing "Notifications" section) describing the resource model (lifecycle `status` vs. observed `state`), the ingestion vs. administration separation, the ping endpoints, the periodic-sweep evaluator, and the notification integration.

14.2. GIVEN the `AGENTS.md` "Key Files Reference" table WHEN it is updated THEN it lists the new heartbeat files (migration, sqlc queries, `internal/heartbeat` evaluator/dispatch/metrics, the admin + ping handlers, the WS message type, the frontend store/components/routes).

14.3. GIVEN the `AGENTS.md` API endpoint documentation WHEN it is updated THEN it includes a heartbeat endpoints table (admin CRUD, rotate-token, notification-bindings) and the root-level `/ping/{token}` and `/ping/{token}/fail` ingestion endpoints, consistent with how the notification endpoints are tabulated.

14.4. GIVEN the `AGENTS.md` environment-variable table WHEN it is updated THEN it documents `PULSE_HEARTBEAT_WORKERS` (bounded evaluator worker pool, with its default), alongside the existing `PULSE_*` variables.

14.5. GIVEN the `AGENTS.md` "Current Progress" section and `docs/MILESTONES.md` WHEN they are updated THEN they record heartbeat monitoring (PF-01 first useful version) as a delivered capability, consistent with the existing milestone/status format.

14.6. GIVEN `.env.example` WHEN it is updated THEN it documents `PULSE_HEARTBEAT_WORKERS` with its default and a short description, matching the style of the other documented variables.

14.7. GIVEN the README WHEN it is updated THEN it describes heartbeat monitoring as a feature and includes the shell, cron, and CI ping examples and the independence principle (satisfying Requirement 11 in user-facing docs).

14.8. GIVEN the documentation update WHEN it is written THEN it stays within the first-useful-version scope and does not document any Non-Goal item as a shipped capability.

---

## Non-Goals (explicitly deferred — do NOT implement in this change)

These are PF-01 **follow-up** items from the roadmap and the scope brief, deferred to a later change:

1. **Run lifecycle events** — start/success/failure events carrying run identifiers and per-run **duration**. The first version records only success/explicit-failure arrival and the computed next deadline, not individual run instances or timing.
2. **Timezone-aware cron schedules** — expressing expected timing as cron expressions with timezone/DST semantics. The first version uses a simple fixed `expected_interval_seconds` + `grace_seconds` only.
3. **Concurrent-run, duplicate, late-arrival, and clock-boundary handling** — explicit semantics for overlapping runs, duplicate pings for the same run, pings arriving after the deadline, and behavior across clock/DST boundaries. Deferred with the run-lifecycle work.
4. **A dedicated MCP heartbeat tool** — exposing heartbeats through the MCP server (`mcp/internal/tools/`, `mcp/internal/pulseapi/`). No existing MCP tool needs co-change (Requirement 7.5); adding a new one is out of scope here.
5. **Restore/validity verification** — proving that a successful backup command actually produced a restorable backup. A green heartbeat is evidence the command ran, not that its result is valid (noted in docs per 11.5), but verifying result validity is out of scope.
6. **Independently hosted external receiver** — PF-01 notes an independently hosted receiver can detect loss of Pulse itself. Shipping/operating such an external receiver is out of scope; the first version is the in-Pulse heartbeat resource and ingestion endpoint.
7. **Heartbeat-specific history time-series / charts** — storing and charting every ping in TimescaleDB like `check_results`. The first version persists current state and `last_success_at`/`next_expected_deadline` on the heartbeat row; detailed per-ping history is deferred (overlaps with run-lifecycle events).
