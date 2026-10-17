# Heartbeat and Scheduled-Job Monitoring — Technical Design (PF-01, First Useful Version)

## Overview

This design adds passive "heartbeat" (dead-man's-switch) monitoring to Pulse. Instead of Pulse actively probing a target, a scheduled job pings Pulse at the end of a successful run. Pulse records the arrival, computes the next expected deadline (`last_success + interval + grace`), and raises an alert when that deadline passes with no ping. Jobs can also report an explicit failure.

The design is deliberately a *parallel resource* to monitors rather than a new monitor `type`. Heartbeats have no active checker, no target normalization, no `interval/timeout` probe semantics, and a fundamentally different ingestion path (unauthenticated token-in-URL ping vs. outbound probe). Forcing them into the `monitors` table would overload the `type` CHECK constraint, the scheduler's `ListActiveMonitorsDueWithTags` probe loop, and the `target` NOT NULL column. Instead we add a `heartbeats` table and a dedicated evaluator that mirrors the proven scheduler pattern, and we *reuse* the existing notification dispatcher, state tracker, WebSocket hub, tag validation, pagination helpers, error envelope, and `internal/token` primitives.

The implementation touches two AGENTS.md co-change rules: the OpenAPI contract (every new endpoint must be documented in `backend/api/openapi.yaml` in the same commit) and the MCP co-change rule (evaluated below — no existing MCP tool changes; a new heartbeat tool is deferred).

Technology stack is locked to the existing project stack: Go 1.26, `gin` v1.12, `pgx/v5`, `sqlc` (config at `backend/sqlc.yaml`), `golang-migrate/v4`, `gorilla/websocket`, `golang.org/x/crypto/bcrypt`, `crypto/sha256`, `prometheus/client_golang`, Svelte 5 + SvelteKit + TypeScript strict + Tailwind 3.4, `pnpm`. No new frameworks or libraries are introduced.

---

## 1. Data Model

### 1.1 `heartbeats` table (migration `017_heartbeats`)

Mirrors the `monitors` two-field lifecycle/observed split (`backend/migrations/001_initial.up.sql`) and the prefix-lookup + hash shape of `api_tokens` (its lookup column is `prefix`, queried via `WHERE prefix = $1` in `ListAPITokensByPrefix`; the heartbeat column is named `token_prefix` for clarity). It persists the next-expected-deadline so the evaluator can run a single indexed query, exactly like `idx_monitors_next_check_at`.

```sql
-- 017_heartbeats.up.sql
CREATE TABLE heartbeats (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                      TEXT        NOT NULL,
    expected_interval_seconds INTEGER     NOT NULL CHECK (expected_interval_seconds >= 1
                                                          AND expected_interval_seconds <= 31536000), -- max 365d
    grace_seconds             INTEGER     NOT NULL DEFAULT 0 CHECK (grace_seconds >= 0
                                                          AND grace_seconds <= 31536000),
    -- lifecycle status (operator-controlled)
    status                    TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    -- observed state (derived from pings + evaluator)
    state                     TEXT        NOT NULL DEFAULT 'new'  CHECK (state IN ('up', 'down', 'paused', 'new')),
    last_success_at           TIMESTAMPTZ,
    next_expected_deadline    TIMESTAMPTZ,  -- persisted: last_success_at + interval + grace
    -- ping ingestion credential (write-only; raw token never stored)
    token_prefix              TEXT        NOT NULL,
    token_hash                TEXT        NOT NULL,
    token_created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Evaluator priority scan: only rows that can go down.
-- Partial index excludes paused/down/new (new has NULL deadline anyway).
CREATE INDEX idx_heartbeats_next_deadline ON heartbeats (next_expected_deadline)
    WHERE status = 'active' AND state = 'up';

-- Token verification lookup (prefix narrows candidates, then hash compare).
CREATE INDEX idx_heartbeats_token_prefix ON heartbeats (token_prefix);

-- List/filter by lifecycle + recency (mirrors idx_monitors_status_created_at).
CREATE INDEX idx_heartbeats_status_created_at ON heartbeats (status, created_at);
```

`state` column rationale (Requirement glossary): `new` = created, never pinged (deadline is NULL, evaluator skips it, R4.3); `up` = last ping succeeded and deadline not yet passed; `down` = deadline missed (R4.1) or explicit-fail ping (R3.2); `paused` is surfaced in API responses when `status = 'paused'` but is **not** stored as a `state` value transition from the evaluator — the evaluator skips paused rows entirely (R4.6). To keep the CHECK simple and avoid a state the evaluator must write, the response layer maps `status = 'paused'` to an observed `state` of `paused` in the DTO (see §3.4), while the stored `state` column holds `up`/`down`/`new`. This is a presentation mapping, documented explicitly so it is not mistaken for a stored transition.

### 1.2 `heartbeat_tags` table

Reuses the exact `monitor_tags` shape (`backend/migrations/009_monitor_tags.up.sql`) so `tags.ValidateTags` and the tag autocomplete contract apply unchanged (R1.9).

```sql
CREATE TABLE heartbeat_tags (
    heartbeat_id UUID NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (heartbeat_id, key, value)
);
CREATE INDEX idx_heartbeat_tags_heartbeat_id ON heartbeat_tags (heartbeat_id);
CREATE INDEX idx_heartbeat_tags_key_value     ON heartbeat_tags (key, value);
```

### 1.3 `heartbeat_bindings` table

Decision in §6 is to use a dedicated bindings table (not generalize `channel_bindings`). It mirrors `channel_bindings` (migration 014) but references `heartbeats` and carries the heartbeat trigger set.

```sql
CREATE TABLE heartbeat_bindings (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id                UUID        NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    heartbeat_id              UUID        NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    triggers                  JSONB       NOT NULL,
    reminder_interval_minutes INT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_channel_heartbeat UNIQUE (channel_id, heartbeat_id)
);
CREATE INDEX idx_heartbeat_bindings_heartbeat_id ON heartbeat_bindings (heartbeat_id);
```

The existing `delivery_logs` table (migration 014) is **reused** for heartbeat delivery audit. Its `monitor_id` column is `UUID NOT NULL` with **no FK** (verified in `014_notification_channels.up.sql` — intentional, so the audit trail persists after resource deletion); its `binding_id` column is nullable. We rename nothing; heartbeat delivery logs store the heartbeat UUID in `monitor_id` and the heartbeat binding UUID in `binding_id`. Because `delivery_logs.monitor_id` is `NOT NULL` and `LogDelivery(ctx, channelID, monitorID, bindingID, …)` (verified in `delivery_log.go`) is always invoked by `processJob` with `job.MonitorID`, **the heartbeat dispatch path MUST set `DeliveryJob.MonitorID = heartbeatID` and `DeliveryJob.BindingID = heartbeatBindingID` at enqueue time**, or the audit insert would violate the `NOT NULL` constraint. This is guaranteed by the dispatch entry point described in §5.3 (which passes `heartbeatID` as the resource key to `EvaluateAndDispatchWithPayload`, from which `buildPayload`/the payload builder and the enqueued `DeliveryJob` both derive their id). Migration `017` does **not** rename the ambiguous `monitor_id` column (that would be a breaking change to existing monitor logs); instead `delivery_logs.monitor_id` is treated as a generic "resource id" at the heartbeat call sites, and a short code comment documents this. The alternative — a parallel `heartbeat_delivery_logs` table — was rejected because it duplicates the retry/log pipeline for no behavioral gain (R5.6 requires the existing retry queue unchanged).

### 1.4 LISTEN/NOTIFY trigger (not reused here)

The monitors table has a `trg_monitor_notify` trigger (migration 003) that wakes the scheduler. Heartbeats use a **periodic sweep** evaluator (decided in §5), so no NOTIFY trigger is added for deadline evaluation. A ping does update state in-process and broadcasts immediately, so there is no latency benefit to a DB NOTIFY for ingestion either. This keeps migration 017 free of pg_notify triggers.

---

## 2. sqlc Queries and Migration Plan

### 2.1 Migration files

Following the sequential pattern (`NNN_name.up.sql` / `.down.sql`, applied by `golang-migrate/v4` via `make migrate`):

- `017_heartbeats.up.sql` / `.down.sql` — creates `heartbeats`, `heartbeat_tags`, `heartbeat_bindings` and their indexes; the `.down.sql` drops them in reverse dependency order (`heartbeat_bindings`, `heartbeat_tags`, `heartbeats`).

No change to `delivery_logs`. No TimescaleDB hypertable (per-ping history is a non-goal, R non-goal 7).

### 2.2 sqlc query files

New files under `backend/internal/store/postgres/queries/`, regenerated with the existing sqlc config (`sql_package: pgx/v5`, `emit_pointers_for_null_types: true`, UUID/timestamptz/jsonb overrides). sqlc regeneration is run as part of the implementation (the repo's generated `*.sql.go` are committed).

`heartbeats.sql`:
- `GetHeartbeat :one` — `SELECT * FROM heartbeats WHERE id = $1`
- `GetHeartbeatForUpdate :one` — `SELECT * ... WHERE id = $1 FOR UPDATE` (row lock for idempotent PUT, mirrors `GetMonitorForUpdate`)
- `ListHeartbeats :many` / `CountHeartbeats :one` — paginated list (`LIMIT/OFFSET`, `ORDER BY created_at DESC`); optional tag filter mirrors `ListMonitorsFiltered` only if needed (first version: plain list + count, matching `ListMonitors`)
- `CreateHeartbeat :one` — insert with server-generated UUID, `state='new'`, token fields
- `UpsertHeartbeat :one` — `INSERT ... ON CONFLICT (id) DO UPDATE SET name, expected_interval_seconds, grace_seconds, status, updated_at=now()` — **does not** touch `token_prefix/token_hash/last_success_at/state` on conflict (PUT never rotates token, R2 decision)
- `UpdateHeartbeatDeadline :one` — recomputes `next_expected_deadline` and sets `state`/`last_success_at` after a PUT that changes interval/grace while a `last_success_at` exists (R2.4); returns the updated row
- `MarkHeartbeatDown :one` — `UPDATE heartbeats SET state='down' WHERE id=$1 AND state='up' RETURNING *` (the `WHERE state='up'` guard makes the evaluator sweep fire once per missed-run episode, R4.7); the returned row backs the broadcast. Used only by the evaluator (missed-run), where the single-writer sweep already serializes.
- `MarkHeartbeatUp :one` — success ping. A single atomic statement that both reads the prior state and writes the new one: `UPDATE heartbeats SET state='up', last_success_at=$2, next_expected_deadline=$3 WHERE id=$1 RETURNING (SELECT state FROM heartbeats WHERE id=$1) AS previous_state, *` — in practice implemented via a CTE (`WITH prev AS (SELECT state FROM heartbeats WHERE id=$1 FOR UPDATE) UPDATE heartbeats … RETURNING (SELECT state FROM prev) AS previous_state, …`) so the prior `state` is captured atomically with the write. Postgres serializes concurrent updates to the same row, so racing success pings each receive a distinct `previous_state`, giving idempotent recovery (§4.3, MEDIUM-2).
- `MarkHeartbeatFail :one` — explicit-fail ping. `UPDATE heartbeats SET state='down' WHERE id=$1 RETURNING (prior state) …` via the same CTE pattern; does **not** advance `last_success_at`. Returns the prior `state` as `PreviousState` for the dispatcher.
- `RotateHeartbeatToken :one` — updates `token_prefix`, `token_hash`, `token_created_at` (R6.5)
- `GetHeartbeatByTokenPrefix :many` — `SELECT id, state, status, token_hash, token_prefix, expected_interval_seconds, grace_seconds FROM heartbeats WHERE token_prefix = $1` (candidates for bcrypt compare at the ping path, mirrors `ListAPITokensByPrefix`)
- `ListDueHeartbeats :many` — `SELECT ... WHERE status='active' AND state='up' AND next_expected_deadline <= now() ORDER BY next_expected_deadline ASC LIMIT $1` (indexed, batch-bounded, mirrors `ListActiveMonitorsDue`)
- `CountHeartbeatsByState :many` — `SELECT state, COUNT(*) ... GROUP BY state` for the Prometheus `pulse_heartbeats_total` gauge
- `DeleteHeartbeat :exec` — `DELETE FROM heartbeats WHERE id=$1` (cascades tags + bindings)

`heartbeat_tags.sql`: `DeleteHeartbeatTags :exec`, `InsertHeartbeatTag :exec`, `ListTagsByHeartbeat :many` — direct analogues of `monitor_tags.sql`.

`heartbeat_bindings.sql`: `CreateHeartbeatBinding`, `GetHeartbeatBinding`, `ListBindingsByHeartbeat`, `ListBindingsByHeartbeatPaginated`, `CountBindingsByHeartbeat`, `UpdateHeartbeatBinding`, `DeleteHeartbeatBinding` — analogues of the `channel_bindings` queries used by `NotificationBindingHandler`.

---

## 3. API Endpoints

All admin endpoints live under the protected `/api/v1` group (combined JWT + Bearer API-token auth via `combinedAuth`), return the standard error envelope `{ "error": { "code", "message" } }` through `apiError`/`apiErrorWithDetails`, and inherit the `X-Request-ID` response header from `requestIDMiddleware`. List endpoints use `parsePagination` (default `page=1`, `limit=20`, clamp `limit` to 100) and return the `{ data, total, page, limit, total_pages }` envelope. A new `HeartbeatHandler` (`backend/internal/api/handlers/heartbeats.go`) mirrors `MonitorHandler`; a new `HeartbeatBindingHandler` mirrors `NotificationBindingHandler`.

### 3.1 Admin CRUD

| Method | Path | Description | Success |
|--------|------|-------------|---------|
| POST | `/api/v1/heartbeats` | Create heartbeat; returns raw ping token once | 201 |
| GET | `/api/v1/heartbeats` | Paginated list | 200 |
| GET | `/api/v1/heartbeats/{id}` | Get one (no raw token/hash) | 200 |
| PUT | `/api/v1/heartbeats/{id}` | Idempotent create-or-update | 200 |
| DELETE | `/api/v1/heartbeats/{id}` | Delete + cascade + clear state | 204 |
| POST | `/api/v1/heartbeats/{id}/rotate-token` | Rotate ping token; returns new raw token once | 200 |

### 3.2 Notification bindings (per heartbeat)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/heartbeats/{id}/notification-bindings` | Create binding |
| GET | `/api/v1/heartbeats/{id}/notification-bindings` | Paginated list |
| PUT | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Update triggers |
| DELETE | `/api/v1/heartbeats/{id}/notification-bindings/{bindingId}` | Delete binding |

### 3.3 Request / Response schemas

**Create request** (`POST`):
```json
{
  "name": "nightly-backup",
  "expected_interval_seconds": 86400,
  "grace_seconds": 3600,
  "status": "active",
  "tags": [{ "key": "team", "value": "ops" }]
}
```
Validation (R1.7, returns `400 VALIDATION_ERROR` with readable message, or `400 INVALID_TAGS` for tags): `name` required non-empty; `expected_interval_seconds` integer in `[1, 31536000]`; `grace_seconds` integer in `[0, 31536000]`; `status` ∈ {`active`,`paused`} defaulting to `active`.

**Create response (201) — the only response carrying the raw token** (R1.1, R6.2):
```json
{
  "id": "9f1c...uuid",
  "name": "nightly-backup",
  "expected_interval_seconds": 86400,
  "grace_seconds": 3600,
  "status": "active",
  "state": "new",
  "last_success_at": null,
  "next_expected_deadline": null,
  "tags": [{ "key": "team", "value": "ops" }],
  "token_prefix": "a1B2c3D4",
  "ping_token": "a1B2c3D4...43chars",
  "ping_url": "https://pulse.example.com/ping/a1B2c3D4...",
  "ping_fail_url": "https://pulse.example.com/ping/a1B2c3D4.../fail",
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```
`ping_url`/`ping_fail_url` are built from `PULSE_BASE_URL` + `/ping/<raw>`; if `PULSE_BASE_URL` is empty the raw token is still returned in `ping_token` and the URL fields are omitted. `ping_token`/`ping_url`/`ping_fail_url` appear only on create and rotate-token responses (`createHeartbeatResponse` DTO, analogous to `createTokenResponse`).

**Get / List item response** — the standard `heartbeatResponse` DTO: identical fields minus `ping_token`/`ping_url`/`ping_fail_url`; it exposes `token_prefix` and `token_created_at` only (R1.4, R6.3). `state` reflects the presentation mapping from §1.1.

**rotate-token response (200)**:
```json
{ "id": "...", "token_prefix": "e5F6g7H8", "ping_token": "e5F6...", "ping_url": "...", "ping_fail_url": "...", "token_created_at": "..." }
```

Error codes used: `VALIDATION_ERROR`, `INVALID_TAGS`, `INVALID_ID` (bad UUID, R1.6), `NOT_FOUND` (R1.5), `CONFLICT` (serialization/lock failure on PUT — same pg codes `40001`/`40P01`/`55P03` as monitors, R2.5), `UNAUTHORIZED` (R1.10), `DB_ERROR`, `INTERNAL_ERROR`.

### 3.4 Create/PUT flow details

`POST` and `PUT` run in a `pgxpool` transaction (mirror `MonitorHandler`): begin tx → (PUT only) `GetHeartbeatForUpdate` for row lock → `CreateHeartbeat`/`UpsertHeartbeat` → `DeleteHeartbeatTags` + `InsertHeartbeatTag` loop → commit. Token generation uses `token.Generate()` but stores a **SHA-256** hash, not bcrypt (see §6 decision). On `POST`, the handler calls `token.Generate()` for the raw+prefix, computes `sha256` of the raw, and persists `token_prefix` + the hex SHA-256 digest. Note that `token.Generate()` (verified in `token/token.go`) returns `(raw, prefix, bcryptHash, err)` and computes a bcrypt hash internally; the heartbeat create/rotate path **discards that bcrypt return value** and keeps only `raw`+`prefix`. The ~60–100ms bcrypt cost is paid on the rare admin create/rotate operation only (never on the ping path), so it is acceptable; to avoid the wasted work, the implementation MAY add a lightweight `token.GenerateRaw() (raw, prefix string, err error)` helper to `internal/token` that skips the bcrypt step. Either approach is fine — the SHA-256 digest is what gets persisted regardless.

PUT interval/grace change (R2.4): after upsert, if the persisted row has a non-null `last_success_at`, call `UpdateHeartbeatDeadline` to set `next_expected_deadline = last_success_at + new_interval + new_grace` and keep `state`. If `last_success_at` is null, leave `next_expected_deadline` null and `state='new'`.

DELETE (R1.8, R5.7): `DeleteHeartbeat` cascades `heartbeat_tags` and `heartbeat_bindings`; the handler then calls the evaluator's/state-tracker's `ClearState(id)` to drop in-memory dedup state, and broadcasts nothing (resource gone).

The DTO builder maps observed `state`: `if row.Status == "paused" { resp.State = "paused" } else { resp.State = row.State }`.

---

## 4. Root-Level Ping Endpoint

### 4.1 Placement and routing

Ping endpoints are registered **directly on the gin engine** (not under `/api/v1`), exactly like `/ws` in `router.go`:

```
GET  /ping/:token
POST /ping/:token
GET  /ping/:token/fail
POST /ping/:token/fail
```

Both `GET` and `POST` are accepted (R3.3) because cron wrappers and CI runners vary; request bodies are ignored in the first version. They sit outside `/api/v1` for the same reason `/ws` does: they are a distinct protocol surface with a distinct auth model (possession of an unguessable token in the URL, not a Bearer/JWT header). Browsers and `curl` one-liners must hit them without any auth header.

The `apiPrefixes` SPA guard in `router.go` is extended to include `/ping` so the SPA catch-all returns the ingestion handler's response rather than serving `index.html` or a JSON 404 meant for `/api/` (R3, R12.4). Because the ping routes are explicitly registered on the engine, gin routes them before `NoRoute`; the `apiPrefixes` addition is defensive for any `/ping/...` shape that does not match a registered route (e.g. missing token), which then returns the ingestion-style `404` rather than the SPA.

A new `PingHandler` (`backend/internal/api/handlers/ping.go`) holds `*db.Queries`, the `*hub.Hub`, the heartbeat dispatch helper (for notification dispatch), and the heartbeat `Metrics`, and is registered in `NewRouter` on the engine (gated like the hub). It is **not** added to the `protected` group. The ping write path relies on atomic single-statement `UPDATE … RETURNING` queries (§4.3), so `PingHandler` does **not** need the `*pgxpool.Pool` or an explicit transaction — the atomicity needed to read the prior state together with the write is provided by the single statement itself.

### 4.2 Token verification (ingestion vs admin separation)

```
prefix := token[:token.PrefixLen]           // 8 chars
candidates := GetHeartbeatByTokenPrefix(prefix)
if len(candidates)==0 { sha256DummyCompare(); return 404 }
for each candidate:
    if constantTimeCompare(sha256hex(token), candidate.token_hash) == 1 { matched }
if matched==nil { sha256DummyCompare(); return 404 }
```

Verification mirrors the prefix-lookup + constant-time-compare + dummy-compare pattern of `combinedAuth`/`WSHandler.validateToken` (R3.4, R6.7, R12.3), but uses `crypto/subtle.ConstantTimeCompare` over the SHA-256 hex digests rather than `bcrypt.CompareHashAndPassword` (justified in §6). A possessed ping token authorizes **only** success/failure reporting for its own heartbeat; it carries no user identity, is never set into the gin context as `user_id`, and cannot reach any admin route (hard ingestion/admin separation, R3.5, R6.6). An unknown/malformed/revoked token yields a minimal `404` body (`not found`) that reveals nothing about prefix match.

### 4.3 Ping processing

**Concurrency model (R3, adjacent to Non-Goal 3).** Ping ingestion is externally concurrent — a duplicate cron entry, a `curl --retry`, or two overlapping runs can send two near-simultaneous success pings for the same token. Unlike the monitor path (where only the single scheduler goroutine ever writes a given monitor row), the ping path has no single-writer guarantee, so a naive read-then-write (read `state`, decide "this is the recovery", write `state=up`) could let two pings each observe `state=down` and each dispatch a recovery. To prevent this, **the ping write is performed as a single atomic `UPDATE … RETURNING` statement that returns the row's prior `state`** (`MarkHeartbeatUp`, `MarkHeartbeatFail` — see §2.2). The returned prior state is the `PreviousState` fed to the dispatcher, read atomically with the write. Postgres serializes concurrent `UPDATE`s on the same row, so of two racing success pings exactly one sees `prior_state='down'` (and dispatches recovery) and the other sees `prior_state='up'` (no transition event, so `StateTracker.Evaluate` emits nothing). No row lock in application code and no `*pgxpool.Pool` on `PingHandler` is required. Within this first version a duplicate success ping is therefore **idempotent** — it refreshes `last_success_at`/`next_expected_deadline` and does not double-fire recovery; richer concurrent-run semantics (run ids, overlap, late arrivals, clock boundaries) remain deferred (Non-Goal 3).

**Success** (`/ping/:token`, R3.1): a single `MarkHeartbeatUp` statement sets `last_success_at = now`, `next_expected_deadline = now + interval + grace`, transitions `state → up`, and `RETURNING`s the prior `state`. If the prior state was `down`, this is a recovery → fire recovery notification path (§5). Respond `200` with body `OK`.

**Explicit failure** (`/ping/:token/fail`, R3.2): a single `MarkHeartbeatFail` statement transitions `state → down` (does **not** advance `last_success_at`) and `RETURNING`s the prior `state`; the handler fires the missed/failure notification path with that prior state as `PreviousState`, and responds `200` `OK`.

**Paused heartbeat** (R3.8): a success ping still records `last_success_at` and recomputes `next_expected_deadline`, but no missed-run evaluation happens while paused; the ping returns `200`. (The evaluator independently skips paused rows.)

Notification dispatch and WS broadcast are both fired **non-blocking** after the DB write, exactly like the scheduler: the dispatcher `Enqueue` uses `select/default`, and `hub.Broadcast` drops on a full channel. The ping response returns promptly regardless (R3.6, R10.5, R8.5). Logs use `token_prefix` or heartbeat id, never the raw token (R3.7, R12.2) — the existing `middleware.SanitizedLogger` is in the chain.

---

## 5. Deadline Evaluation and Missed-Run Detection

### 5.1 Mechanism decision — periodic sweep (not LISTEN/NOTIFY)

**Decision: a periodic sweep evaluator with a bounded worker pool and a short fixed tick, modeled on `monitor.Scheduler`.** The scheduler already proves this pattern at 500+ resources: a `time.Ticker`, an indexed `ListActiveMonitorsDue`-style query capped at `DefaultBatchSize = 500`, a fixed worker pool, buffered job channel for backpressure, and `context`-cancellation shutdown that drains in-flight work.

Rationale for rejecting LISTEN/NOTIFY as the evaluation trigger: NOTIFY is edge-triggered on row *changes*. A missed run is the *absence* of a change — nothing writes to the row when a deadline passes, so there is no event to notify on. You would still need a timer. The monitors `trg_monitor_notify` exists to collapse create/update latency, not to detect deadlines. A sweep is the correct tool for "time passed and nothing happened." (Alternatives note in §12.)

### 5.2 Evaluator design (`backend/internal/heartbeat/evaluator.go`)

New package `internal/heartbeat` (keeps core logic in `internal/...` and avoids coupling to the probe `monitor` package). The `Evaluator`:

```
Config: Workers (PULSE_HEARTBEAT_WORKERS, default 50 dev / 50 prod — see §10),
        TickInterval (default 1s, documented bound on detection latency, R4.8),
        BatchSize (default 500)
Run(ctx): start Workers goroutines reading from a buffered jobs chan;
          on each tick → poll(); on ctx.Done() → close(jobs), wg.Wait(), return (R4.5)
poll(ctx): rows = ListDueHeartbeats(BatchSize); for each → jobs <- row (ctx-aware send)
worker: for row := range jobs { markDown(ctx, row) }
```

`markDown` calls `MarkHeartbeatDown` (the `WHERE state='up'` guard makes it fire **once** per missed-run episode, R4.7; the returned row's prior state is `up`, used as `PreviousState`), and only if a row was actually updated does it (a) update the Prometheus `pulse_heartbeat_up` gauge to 0, (b) broadcast `heartbeat_status`, and (c) dispatch the missed-run notification path via `EvaluateAndDispatchWithPayload(ctx, heartbeatID, CheckResult{State:"down", PreviousState:"up"}, translatedBindings, heartbeatPayloadBuilder)` (§5.3). Because `ListDueHeartbeats` already filters `status='active' AND state='up' AND next_expected_deadline <= now()`, paused heartbeats (R4.6) and never-pinged `new` heartbeats with NULL deadline (R4.3) are never selected, and a heartbeat already `down` is never re-selected.

Recovery (`down → up`, R4.2) is driven by the **ping path** (§4.3), not the sweep: only an inbound success ping can clear a `down`. The ping path recomputes the deadline and fires the recovery notification.

Query backing: `idx_heartbeats_next_deadline` is a partial index on `(next_expected_deadline) WHERE status='active' AND state='up'`, so the sweep's `ORDER BY next_expected_deadline ASC LIMIT 500` is an index range scan (R4.4, R10.2).

### 5.3 Notification integration

**Decision: a dedicated `heartbeat_bindings` table and a thin heartbeat dispatch call site that reuses the existing notification primitives.** The dispatcher's `StateTracker`, fan-out matching, bounded delivery worker pool, non-blocking `Enqueue`, retry queue, delivery logging, and reminder scheduler are all reused unchanged. The **only** additive change to the notification package is a new `EvaluateAndDispatchWithPayload` method (a payload-injecting sibling of `EvaluateAndDispatch`, detailed below under "Reuse mechanics"); `EvaluateAndDispatch` is refactored to delegate to it with the existing `buildPayload`, so monitor behavior is byte-for-byte identical. We do **not** build a parallel alerting path (prohibited by R5).

Why a dedicated bindings table rather than generalizing `channel_bindings.monitor_id`: the dispatcher already keys `StateTracker` and `BindingWithChannel` by `uuid.UUID`, which is resource-agnostic — a heartbeat UUID flows through the same code with zero signature changes. The *bindings table* must stay separate because `channel_bindings.monitor_id` has a `REFERENCES monitors(id)` FK; a heartbeat id would violate that FK. Overloading the column (dropping the FK, adding a resource-type discriminator) is a more invasive and error-prone change than a parallel table that reuses the identical schema shape. The trigger set also differs (heartbeat triggers reject `degraded`/`ssl_expiring`/`n_failures_in_row`), so a separate validation path is cleaner.

A new dispatcher entry point is required because the existing `EvaluateAndDispatch` (verified in `backend/internal/notification/fanout.go`) builds the delivery payload *internally* via `buildPayload(monitorID, result)` — which sets only `Monitor.ID`, `Status`, `PreviousStatus`, `ResponseTime`, `Timestamp` and **never `Monitor.Name`** — and exposes no parameter for a caller-supplied payload. If the heartbeat path reused it as-is, every enqueued job would have `Monitor.Name == ""`, so `dispatch()` → `enrichPayload()` would run `GetMonitor(heartbeatID)`, get `pgx.ErrNoRows`, log `enrich payload failed`, and render an alert with no name/context — violating R5.8. The fix is one small, additive change to the notification package:

**Add `EvaluateAndDispatchWithPayload` to `Dispatcher`** (sibling of `EvaluateAndDispatch`, in `fanout.go`):

```go
// payloadBuilder lets a caller supply a fully-populated TemplateData so the
// dispatch path skips the enrichPayload GetMonitor lookup.
func (d *Dispatcher) EvaluateAndDispatchWithPayload(
    ctx context.Context,
    resourceID uuid.UUID,
    result CheckResult,
    bindings []BindingWithChannel,
    payloadBuilder func(uuid.UUID, CheckResult) TemplateData,
) {
    allConditions := extractTriggerConditions(bindings)
    events := d.state.Evaluate(resourceID, result, allConditions)
    if len(events) == 0 { return }
    for _, event := range events {
        for i := range bindings {
            if bindingMatchesTrigger(bindings[i], event) {
                d.Enqueue(DeliveryJob{
                    ID: uuid.New(), ChannelID: bindings[i].ChannelID,
                    MonitorID: resourceID, BindingID: bindings[i].ID,
                    TriggerType: event.Type, Attempt: 1, MaxAttempts: DefaultMaxAttempts,
                    Payload: payloadBuilder(resourceID, result), ScheduledAt: time.Now(),
                })
            }
        }
    }
}

// The existing method is refactored to delegate, keeping monitor behavior identical:
func (d *Dispatcher) EvaluateAndDispatch(ctx context.Context, monitorID uuid.UUID, result CheckResult, bindings []BindingWithChannel) {
    d.EvaluateAndDispatchWithPayload(ctx, monitorID, result, bindings, buildPayload)
}
```

This is the real injection point: `EvaluateAndDispatch` is **not** reused unchanged for payload population (it cannot carry a payload); instead the monitor path keeps its exact behavior by delegating with the existing `buildPayload`, and the heartbeat path passes a heartbeat-specific builder.

Reuse mechanics:
- The ping path and evaluator load heartbeat bindings via `ListBindingsByHeartbeat` and build `[]notification.BindingWithChannel` whose `MonitorID` field carries the heartbeat UUID (it is just the resource key — the struct is resource-agnostic). The per-binding `Triggers` are **translated to monitor semantics before the call** (see Trigger types below).
- They call `dispatcher.EvaluateAndDispatchWithPayload(ctx, heartbeatID, checkResult, translatedBindings, heartbeatPayloadBuilder)`. `StateTracker.Evaluate` keys dedup by `heartbeatID`, giving once-per-transition `down` and once-per-recovery `up` behavior for free (R5.1, R5.2, R5.3). The enqueued `DeliveryJob` therefore has `MonitorID = heartbeatID`, satisfying the `delivery_logs.monitor_id NOT NULL` audit column (MEDIUM-1, §1.3), with `BindingID = heartbeatBindingID`.
- The heartbeat `CheckResult` maps: `State` = `up`/`down`, `PreviousState` = the prior stored state returned atomically by `MarkHeartbeatUp`/`MarkHeartbeatFail` (§4.3) or read by the evaluator before `MarkHeartbeatDown`. Missed-run and explicit-fail both produce `State="down"`.

**Trigger types** (R5.5): heartbeats use a dedicated validator `ValidateHeartbeatTriggers` with the allowed set `{heartbeat_missed, heartbeat_recovered}`. `heartbeat_missed` fires on both missed-run (evaluator) and explicit-fail (ping). The monitor trigger types (`monitor_down`, `degraded`, `ssl_expiring`, `n_failures_in_row`, `monitor_up`) are rejected for heartbeat bindings with `400 VALIDATION_ERROR` + field-level `details` (reusing the `FieldError` shape).

Because `EvaluateAndDispatch`/`WithPayload` uses the **same** `bindings` slice for two things — `extractTriggerConditions(bindings)` → `StateTracker.Evaluate` (which emits events typed `monitor_down`/`monitor_up` via its hardcoded switch) **and** the fan-out matcher `bindingMatchesTrigger(bindings[i], event)` (which compares `tc.Type == event.Type`) — the translation must be applied to **the whole `BindingWithChannel.Triggers`**, not just to a separate `[]TriggerCondition` passed to `Evaluate`. If the bindings kept `heartbeat_missed`/`heartbeat_recovered` while only a side slice were translated, `bindingMatchesTrigger` would compare `"heartbeat_missed" == "monitor_down"` → false and **no `DeliveryJob` would ever be enqueued**. The chosen approach (exactly one, not interchangeable alternatives):

> At the heartbeat dispatch call site, translate every binding's `Triggers` from the heartbeat vocabulary to the monitor vocabulary **before** calling the dispatcher — `heartbeat_missed → monitor_down`, `heartbeat_recovered → monitor_up` — so both `extractTriggerConditions`/`Evaluate` and `bindingMatchesTrigger` operate on `monitor_down`/`monitor_up`. A `translateHeartbeatTriggers([]TriggerCondition) []TriggerCondition` helper performs the mapping on the `BindingWithChannel.Triggers` values used for the call.

```go
bindings := make([]notification.BindingWithChannel, 0, len(hbBindings))
for _, b := range hbBindings {
    bindings = append(bindings, notification.BindingWithChannel{
        ID: b.ID, ChannelID: b.ChannelID, MonitorID: heartbeatID,
        Triggers: translateHeartbeatTriggers(b.Triggers), // heartbeat_missed→monitor_down, heartbeat_recovered→monitor_up
    })
}
dispatcher.EvaluateAndDispatchWithPayload(ctx, heartbeatID, checkResult, bindings, heartbeatPayloadBuilder)
```

The heartbeat vocabulary (`heartbeat_missed`/`heartbeat_recovered`) is kept **only** in (i) the persisted `heartbeat_bindings.triggers` JSONB, (ii) `ValidateHeartbeatTriggers`, and (iii) the API DTO shown to clients. `StateTracker.Evaluate` is left completely untouched (no new switch cases). The resulting fired `event.Type` (`monitor_down`/`monitor_up`) now equals the translated `binding.Triggers[i].Type`, so the matcher lines up and a `DeliveryJob` is enqueued. (The reverse approach — adding `heartbeat_missed`/`heartbeat_recovered` cases to `StateTracker.Evaluate` so the fired event type equals the persisted type — is a valid alternative but is **not** used here, to avoid coupling the shared tracker to heartbeat vocabulary; it is recorded as a backlog option only.)

**Template data** (R5.8): the dispatcher's `enrichPayload` calls `GetMonitor` only when `job.Payload.Monitor.Name == ""`. The heartbeat path passes a `heartbeatPayloadBuilder` to `EvaluateAndDispatchWithPayload` that returns a fully-populated `TemplateData` **captured in a closure over the heartbeat row** (name + base URL + timestamps already loaded alongside the bindings):

```go
heartbeatPayloadBuilder := func(id uuid.UUID, r notification.CheckResult) notification.TemplateData {
    return notification.TemplateData{
        Monitor:        notification.MonitorData{ID: id, Name: hb.Name},
        Status:         r.State,
        PreviousStatus: r.PreviousState,
        Timestamp:      time.Now(),
        BaseURL:        baseURL,
    }
}
```

Because `Monitor.Name` is non-empty on every enqueued heartbeat job, `enrichPayload`'s `GetMonitor` lookup is skipped (no `ErrNoRows`, no spurious error log), satisfying R5.8 (payload carries heartbeat id, name, state, previous state, timestamp, BaseURL). The missed-deadline / last-success timestamp is surfaced via `Timestamp` plus (optionally) the `Incident` fields left zero. This reuses the existing email/webhook templates without schema changes. *(Carried note for review: email/webhook templates label the resource "Monitor"; for heartbeats this is slightly inaccurate. First version accepts this; a template-vocabulary pass is a follow-up.)*

**Reminders** (R5.6, reminder scheduler): the existing `ReminderScheduler` re-enqueues while a condition persists and clears on recovery; it operates on `StateTracker` state keyed by UUID, so heartbeat `down` reminders work through the same path once a binding has `reminder_interval_minutes` set. Validation reuses the 30–1440 minute bounds.

Retry (R5.6): unchanged — retryable delivery failures use the existing exponential-backoff queue (30s→60s→120s, max 4), non-retryable fail permanently and are logged.

---

## 6. Scoped Ingestion Credential (Token Storage)

### 6.1 Decision — SHA-256 of a 256-bit random token (deviation from bcrypt, justified)

**Decision: generate a 256-bit (`token.RawBytes = 32`) cryptographically random token via `crypto/rand`, base64url-encode it (`token.Generate()` gives raw+prefix), and store `SHA-256(raw)` as a hex digest plus the 8-char `token_prefix`. Verify with `crypto/subtle.ConstantTimeCompare` over the hex digests.**

AGENTS.md says "Store API tokens as bcrypt hashes." This is an **explicit, justified deviation** for ping tokens specifically:

1. **Performance (the governing constraint).** Ping tokens are hit on *every* cron run. At 500+ heartbeats with sub-minute jobs, the ping path is high-frequency. `bcrypt` at cost 10 is ~60–100ms *by design* (it is a deliberately slow password KDF). That is unacceptable per-request latency and a trivial CPU-exhaustion vector on an unauthenticated endpoint — an attacker spraying `/ping/<prefix>...` would force a bcrypt per prefix-matching candidate. SHA-256 is sub-microsecond. R10.5 requires "a single indexed token-prefix lookup plus [verify] and returns promptly"; bcrypt cannot meet that at scale on an unauthenticated path.
2. **Threat model difference.** bcrypt protects *low-entropy human passwords* against offline brute force after a DB leak. A ping token is a **256-bit high-entropy random secret** — it is not brute-forceable regardless of hash speed. A fast cryptographic hash (SHA-256) of a 256-bit random value provides full preimage resistance; there is no dictionary to run. The property bcrypt adds (slow KDF) buys nothing here because there is nothing to slow-guess.
3. **Still never stored raw, never reversible** (R6.4): SHA-256 is one-way; the DB holds only the digest + a lookup prefix, satisfying "never the raw token, never a reversibly encrypted form."
4. **Constant-time compare** over fixed-length hex digests preserves timing safety (R6.7, R12.3); a dummy SHA-256 + compare runs on prefix miss for uniform timing.

This mirrors how Healthchecks and similar cron-monitoring systems treat high-entropy ping URLs (unguessable slug / fast-hash lookup), not how they treat login passwords.

### 6.2 Write-only lifecycle

- Raw token returned exactly once on `POST` create and on `POST /rotate-token` (R6.2); never on `GET`/`LIST`/`PUT` (R6.3, R12.1).
- At rest: `token_prefix` + `sha256_hex` only (R6.4).
- Rotate (R6.5): `RotateHeartbeatToken` replaces prefix+hash+`token_created_at`; old URLs stop verifying immediately. PUT never rotates (R2 decision).
- The token confers no admin capability (R6.6) — enforced structurally by never setting `user_id` and never mounting ping routes under `protected`.

---

## 7. WebSocket Message Design

### 7.1 Decision — new `heartbeat_status` message type

**Decision: add a dedicated `heartbeat_status` message type (new `TypeHeartbeatStatus` constant and `HeartbeatStatusPayload` in `backend/internal/hub/messages.go`), broadcast through the existing `hub.Hub`, following the diff/patch convention.** We do not reuse `monitor_status`: its payload carries monitor-specific fields (`latency_ms`, `status_code`, `ssl_days_remaining`) that are meaningless for heartbeats, and the frontend `monitorStore.applyPatch` keys by `monitor_id` and would silently discard heartbeat ids. A separate type keeps both patch streams unambiguous and lets each store own its merge.

```go
const TypeHeartbeatStatus = "heartbeat_status"

type HeartbeatStatusPayload struct {
    HeartbeatID          string  `json:"heartbeat_id"`
    State                string  `json:"state"`                            // up | down | paused | new
    LastSuccessAt        *string `json:"last_success_at,omitempty"`        // RFC3339
    NextExpectedDeadline *string `json:"next_expected_deadline,omitempty"` // RFC3339
    Timestamp            string  `json:"timestamp"`                        // server broadcast time
}

func NewHeartbeatStatusMessage(id, state string, lastSuccess, nextDeadline *time.Time) Message { ... }
```

The payload is a diff/patch (only state + the two timestamps + server time); clients merge, not replace (R8.1, R8.2, R10.3). Broadcast reuses `hub.Broadcast`, which is buffered (256) and drops slow consumers (R8.3) and is non-blocking (R8.5). No change to `hub.go` itself — only a new message constructor in `messages.go`. Broadcast is emitted on every observed state change: success ping, explicit-fail ping, missed-run detection, recovery.

### 7.2 Frontend consumption

`ws.ts` gains a `heartbeat_status` case in the `handleMessage` switch, dispatching to a new `heartbeatStore.applyPatch` (R8.4). The patch-merge mirrors `monitorStore`: a `Map<string, Heartbeat>`, merge only `state`/`last_success_at`/`next_expected_deadline`, discard patches for unknown ids (R8.4, R9.9). The `WsMessage['type']` union and `WsEnvelope` typing are extended with `'heartbeat_status'`.

The `heartbeat_status` payload field set differs from `monitor_status` and must be typed explicitly so the store merge does **not** assume `monitor_status`'s `checked_at`/`latency_ms` fields (heartbeats have no probe check time). The exact TypeScript payload type to add:

```ts
interface HeartbeatStatusPayload {
  heartbeat_id: string;
  state: 'up' | 'down' | 'paused' | 'new';
  last_success_at?: string | null;        // RFC3339
  next_expected_deadline?: string | null; // RFC3339
  timestamp: string;                        // server broadcast time, RFC3339
}
```

`heartbeatStore.applyPatch` keys by `heartbeat_id` and merges only `state`, `last_success_at`, and `next_expected_deadline` — it never reads a `checked_at` field.

---

## 8. Prometheus Metrics

Added to the existing registry (`promRegistry` in `main.go`), `pulse_*` naming (R13.1):

- `pulse_heartbeat_up{heartbeat_id, heartbeat_name}` — gauge, 1 when `state=up`, 0 when `down` (R13.2, R13.3). Mirrors `pulse_monitor_up` label style but omits `type`/`url` (not applicable). No token material in labels (R12.5).
- `pulse_heartbeats_total` — gauge, total configured heartbeats, set from `CountHeartbeats` on each evaluator tick (mirrors how the scheduler sets `pulse_monitors_total`).

A new `heartbeat.Metrics` struct (`internal/heartbeat/metrics.go`) registers these via `reg.MustRegister`, constructed in `main.go` with the shared registry and passed to the evaluator and the ping handler. Gauge updates happen at the same points as state transitions: success/recovery → 1, miss/explicit-fail → 0. Label cardinality is bounded by heartbeat count (500+), consistent with existing monitor metric cardinality.

---

## 9. Frontend Plan

### 9.1 Routes (SvelteKit, under `frontend/src/routes/heartbeats/`)

Mirror `monitors/`:
- `heartbeats/+page.svelte` — list (uses `VirtualList` for large collections + `Pagination`, R9.1, R10.4)
- `heartbeats/[id]/+page.svelte` — detail (ping URLs, copy buttons, state, interval, grace, last success, next deadline, tags, bindings section, usage examples)
- `heartbeats/[id]/edit/+page.svelte` — edit form
- `heartbeats/create/+page.svelte` — create form

Nav entry added in `+layout.svelte` next to Monitors (R9.1), keyed `nav.heartbeats`.

### 9.2 Store (`frontend/src/lib/stores/heartbeats.svelte.ts`)

Svelte 5 runes, `Map<string, Heartbeat>`, `applyHeartbeatPatch` pure merge + `applyPatch`/`setHeartbeats`/`updateHeartbeat`/`removeHeartbeat`/`getById`/`clear`, mirroring `monitors.svelte.ts` (R8.4, R9.9).

### 9.3 API client (`frontend/src/lib/api.ts`)

Add `getHeartbeats`, `getHeartbeat`, `createHeartbeat`, `updateHeartbeat`, `deleteHeartbeat`, `rotateHeartbeatToken`, `listHeartbeatBindings`, `createHeartbeatBinding`, `updateHeartbeatBinding`, `deleteHeartbeatBinding` — all via the existing `apiRequest<T>` helper (Bearer auth, 15s timeout, error-envelope parsing, `X-Request-ID`), R9.2.

### 9.4 Components

- Reuse `ShowOnceModal.svelte` for the raw ping token/URL on create and rotate (copy-to-clipboard, cleared on dismiss, never re-fetched — R9.3).
- Reuse `TagEditor.svelte`, `Pagination.svelte`, `VirtualList.svelte`.
- New `HeartbeatRow.svelte` (list row with state badge), `HeartbeatForm.svelte` (name/interval/grace/tags with client-side validation — R9.2), `HeartbeatBindings.svelte` (per-resource binding UI, mirrors `MonitorNotificationBindings.svelte`), `PingExamples.svelte` (shell/cron/CI snippets populated with the heartbeat's ping URL, copy-to-clipboard — R9.4, R9.5, §11).
- Theming/dark mode via CSS custom properties + `[data-theme="dark"]`, never `dark:` prefix, never hardcoded brand classes (R9.7). Direction-aware CSS (logical margins) for RTL (R9.8).

### 9.5 i18n keys

A new top-level `heartbeats` section is added to `frontend/src/locales/en.json` **first**, then the identical keys are added to every other locale file (`ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh`) using the English string as a placeholder (R9.6). A `nav.heartbeats` key is added to the existing `nav` section. All user-visible strings use `t('heartbeats.*')` / `t('common.*')`; no hardcoded display strings (R9.6). `pnpm --filter frontend run validate-locales` must pass (R9.8).

Concrete key set (English values shown; placeholders in other locales):

```json
"nav": { "heartbeats": "Heartbeats" },
"heartbeats": {
  "title": "Heartbeats",
  "subtitle": "Monitor scheduled jobs by expected check-in",
  "empty": "No heartbeats yet",
  "create": "New heartbeat",
  "edit": "Edit heartbeat",
  "delete": "Delete heartbeat",
  "deleteConfirm": "Delete this heartbeat? Its ping URL will stop working.",
  "state": { "up": "Up", "down": "Missed", "paused": "Paused", "new": "Awaiting first ping" },
  "fields": {
    "name": "Name",
    "expectedInterval": "Expected interval",
    "grace": "Grace period",
    "status": "Status",
    "lastSuccess": "Last success",
    "nextDeadline": "Next expected by",
    "tags": "Tags"
  },
  "form": {
    "nameRequired": "Name is required",
    "intervalInvalid": "Expected interval must be a positive number of seconds",
    "graceInvalid": "Grace period must be zero or a positive number of seconds",
    "intervalHelp": "How often this job is expected to check in",
    "graceHelp": "Extra time allowed before a missed check-in alerts"
  },
  "token": {
    "title": "Ping URL",
    "showOnce": "Copy this now — it is shown only once",
    "copy": "Copy ping URL",
    "copyFail": "Copy failure URL",
    "copied": "Copied",
    "rotate": "Rotate token",
    "rotateConfirm": "Rotate the token? Existing ping URLs will stop working.",
    "prefix": "Token prefix"
  },
  "ping": {
    "successUrl": "Success ping URL",
    "failUrl": "Failure ping URL"
  },
  "examples": {
    "title": "Usage examples",
    "shell": "Shell",
    "cron": "Cron",
    "ci": "CI",
    "note": "A success ping proves the command ran, not that its output is valid or restorable."
  },
  "bindings": {
    "title": "Notifications",
    "add": "Add notification",
    "triggerMissed": "Missed run or explicit failure",
    "triggerRecovered": "Recovered",
    "empty": "No notifications configured"
  }
}
```

New `common.*` reuse: `common.save`, `common.cancel`, `common.delete` already exist.

---

## 10. Performance and Bounded Concurrency

- Evaluator worker pool is bounded via `PULSE_HEARTBEAT_WORKERS` (new env var, default `50`), no unbounded goroutines; batch-bounded at `DefaultBatchSize = 500` (R10.1). Default of 50 is chosen because the per-job work is a single UPDATE + non-blocking dispatch/broadcast (far cheaper than a probe), so a smaller pool than the probe scheduler's 200 is sufficient; it is documented and tunable.
- Due-lookup is index-backed by the partial `idx_heartbeats_next_deadline` (R10.2).
- WS updates are diff/patch only, through the drop-on-full buffered hub (R10.3).
- Frontend list virtualizes via `VirtualList` (R10.4).
- Ping path: single indexed prefix lookup + SHA-256 constant-time compare + one UPDATE; notification fan-out and WS broadcast are dispatched non-blocking (`select/default` enqueue, drop-on-full broadcast), so the request path never blocks on them (R10.5).

Env var is read in `main.go` via the existing `getEnvInt("PULSE_HEARTBEAT_WORKERS", 50)` helper and documented in `AGENTS.md` / `.env.example` as part of the change.

---

## 11. Shell / Cron / CI Examples (Documentation + UI)

`PingExamples.svelte` renders these with the specific heartbeat's `ping_url`/`ping_fail_url` and copy-to-clipboard (R9.5, R11.4); the same examples with a placeholder URL go into the README/docs (R11). All strings localized via `t('heartbeats.examples.*')`.

**Shell** (R11.1) — ping on success, fail on error:
```sh
# At the end of a successful job:
curl -fsS -m 10 --retry 3 https://pulse.example.com/ping/<TOKEN>
# Report failure when the preceding command fails:
run_backup || curl -fsS -m 10 https://pulse.example.com/ping/<TOKEN>/fail
```

**Cron** (R11.2) — ping only after the command succeeds:
```cron
# Pinging BEFORE the work defeats the purpose — ping only on success.
0 2 * * * /usr/local/bin/backup.sh && curl -fsS -m 10 https://pulse.example.com/ping/<TOKEN>
```

**CI** (R11.3) — nightly/scheduled pipeline step on success:
```yaml
# GitHub Actions, scheduled workflow
- name: Notify Pulse on success
  if: success()
  run: curl -fsS -m 10 https://pulse.example.com/ping/<TOKEN>
```

Independence/validity note (R11.5), surfaced in UI (`heartbeats.examples.note`) and docs: a success ping proves the command *ran*, not that its output (e.g. a backup) is valid or restorable; an independently hosted receiver can detect loss of Pulse itself, a receiver on the same failed host cannot.

---

## 12. OpenAPI and MCP Co-Change

### 12.1 OpenAPI (AGENTS.md co-change rule — MANDATORY, R7)

`backend/api/openapi.yaml` (OpenAPI 3.0.3) is updated **in the same commit** as the handlers (R7.1). New paths documented: `/heartbeats`, `/heartbeats/{id}`, `/heartbeats/{id}/rotate-token`, `/heartbeats/{id}/notification-bindings`, `/heartbeats/{id}/notification-bindings/{bindingId}`, and the root-level `/ping/{token}` and `/ping/{token}/fail`. Each documents (R7.2): request/response schemas; the standard error envelope (`error.code`/`error.message`, with `details` for binding validation); `page`/`limit` params on list endpoints; the `X-Request-ID` response header; the combined JWT/API-token security scheme on admin paths and **no** security scheme on the `/ping` paths (documented as token-in-path, no auth header). New component schemas: `Heartbeat` (get/list — no raw token), `HeartbeatCreateResponse` and `HeartbeatRotateTokenResponse` (include `ping_token`/`ping_url`/`ping_fail_url`), `HeartbeatBinding`. The write-only contract is explicit in the schema split (R7.3). `make openapi` must pass (R7.4).

### 12.2 MCP (AGENTS.md co-change rule — evaluated, R7.5)

**Decision: defer MCP heartbeat tools; no existing MCP tool or `PulseClient` method requires modification.** The AGENTS.md MCP co-change endpoint list covers only `GET/POST/PUT/DELETE /monitors*`, `/incidents*`, `/tags*`, and `/dashboard/summary`. Heartbeat endpoints are **new** resources that are not in that list and do not change any listed endpoint's request/response shape, so no handler in `mcp/internal/tools/` and no method in `mcp/internal/pulseapi/` is affected (R7.5). Adding a read-only heartbeat MCP tool is an explicit non-goal for this first version (roadmap non-goal 4). Recording this evaluation here satisfies the co-change rule's intent. If/when a `GET /heartbeats` MCP tool is later added, the co-change rule and its property-test requirement will apply to that new tool.

### 12.3 Co-change touch points summary

- **OpenAPI**: touched — all new paths/schemas added in the same commit (§12.1).
- **MCP**: evaluated — not touched; new resource, deferred tool (§12.2).

---

## Error Handling (per operation)

- **Create/PUT body invalid** (bind error, bad interval/grace, bad status): `400 VALIDATION_ERROR`, readable message; no row written. Recoverable (client fixes input). Not logged beyond the sanitized access log.
- **Invalid tags**: `400 INVALID_TAGS` via `tags.ValidateTags`.
- **Bad UUID path param**: `400 INVALID_ID`.
- **Heartbeat not found** (`pgx.ErrNoRows`): `404 NOT_FOUND`.
- **PUT serialization/lock conflict** (`40001`/`40P01`/`55P03`): `409 CONFLICT`, retry-advising message. Recoverable (client retries).
- **DB/tx failure** (begin/commit/query): `500 DB_ERROR`, generic message; underlying error logged server-side, never surfaced. Fatal for that request.
- **Token generation failure** (`crypto/rand`): `500 INTERNAL_ERROR`; raw token never returned on failure (mirrors `TokenHandler.Create`).
- **Ping token unknown/malformed/revoked**: `404` minimal body after a dummy SHA-256 compare (timing-safe); nothing logged beyond prefix/none. Not recoverable without a valid token.
- **Ping on paused heartbeat**: `200`, success recorded, no missed-run eval. Not an error.
- **Notification enqueue when buffer full**: dropped (non-blocking), `DroppedTotal` metric incremented, logged — never blocks the ping/evaluator (existing dispatcher behavior).
- **Hub broadcast when channel full**: message dropped for slow consumers, logged — never blocks (existing hub behavior).
- **Evaluator per-row failure** (`MarkHeartbeatDown` error): logged with heartbeat id, loop continues; the row remains `up` and will be retried on the next tick (idempotent via the `WHERE state='up'` guard).

## Input Validation (per external input)

- `name`: required, non-empty after trim; type string. Fail → `400 VALIDATION_ERROR`.
- `expected_interval_seconds`: required, integer, `[1, 31536000]` (DB CHECK + handler check). Fail → `400 VALIDATION_ERROR`.
- `grace_seconds`: optional, integer, `[0, 31536000]`, default 0. Fail → `400 VALIDATION_ERROR`.
- `status`: optional, enum {`active`,`paused`}, default `active`; unknown value falls back to default (mirrors monitors) — not an error.
- `tags`: optional, validated by `tags.ValidateTags`. Fail → `400 INVALID_TAGS`.
- `page`/`limit`: tolerant defaulting via `parsePagination`; `limit` clamped to 100 (R1.3). Invalid values fall back to defaults, not an error (matches monitors).
- `channel_id` (binding): required, non-nil UUID; must exist. Missing → `400 VALIDATION_ERROR` with `details`; not found → `404 NOT_FOUND`.
- `triggers` (binding): required non-empty; each `type` ∈ {`heartbeat_missed`,`heartbeat_recovered`}. Unknown → `400 VALIDATION_ERROR` with field-level `details` (R5.5).
- `reminder_interval_minutes` (binding): optional, `[30, 1440]`. Fail → `400 VALIDATION_ERROR` with `details`.
- ping `:token`: length ≥ `token.PrefixLen` else dummy-compare + `404`; constant-time hash compare.

## Invariant Ownership

- **`next_expected_deadline = last_success_at + interval + grace`**: owned by the handler/ping-path write functions (`UpdateHeartbeatDeadline`), not the DB — because it must be recomputed on PUT interval/grace change (R2.4) and on each success ping, and the sweep must never recompute it. Centralizing it in the write path keeps a single source of truth.
- **Once-per-missed-run-episode**: owned by the DB via `MarkHeartbeatDown`'s `WHERE state='up'` guard (R4.7) plus the `StateTracker` dedup (R5.3) — belt and suspenders, matching the monitors pattern.
- **Ingestion/admin separation**: owned structurally by the router (ping routes outside `protected`, never set `user_id`) — enforcement at the integration point, not per-handler checks (R3.5, R6.6).
- **Write-only token**: owned by the DTO layer (separate create/rotate response types that include the raw token vs. the standard DTO that omits it) — the type system prevents accidental leakage (R6.3).

## Testability

Unit-testable: `ValidateHeartbeatTriggers` (table-driven), the deadline recompute helper (pure function of last_success/interval/grace), the token hash+prefix+constant-time verify helpers, the `state` presentation mapping, the frontend `applyHeartbeatPatch` pure merge (Vitest + property tests mirroring `monitors.svelte.ts` tests), and the WS payload constructor. Integration-testable (Go test with the TimescaleDB/Postgres service already used by CI): migration up/down, sqlc queries, `ListDueHeartbeats` index behavior, the evaluator sweep marking a row down once, the ping path transitions and recovery, PUT idempotency and `409` on conflict, and cascade-on-delete. The evaluator is designed with injectable `Config` (tick/batch/workers) and `context` cancellation so tests can drive it deterministically with a short tick and a cancelable context, exactly like the scheduler tests.

---

## Alternatives Considered

**Token storage.** (A) **bcrypt** (AGENTS.md default) — rejected for the ping path: ~60–100ms per verify is both a per-request latency problem at 500+ heartbeats and a CPU-exhaustion vector on an unauthenticated endpoint; bcrypt's slow-KDF property adds nothing against a 256-bit random secret (no dictionary to brute-force). (B) **Unguessable random slug stored in plaintext, matched by equality** — simplest and fast, but stores the live credential in the DB; a DB read leak hands over working ping URLs. (C) **chosen: SHA-256 of a 256-bit `crypto/rand` token, constant-time hex compare** — fast (sub-µs), one-way (leak-safe, R6.4), full preimage resistance given the entropy, timing-safe. The slow-KDF benefit bcrypt provides is irrelevant for high-entropy secrets, so (C) is the correct point on the curve. The deviation from the AGENTS.md bcrypt rule is explicit and scoped to ping tokens only; API tokens and passwords keep bcrypt.

**Deadline evaluation mechanism.** (A) **LISTEN/NOTIFY-driven** (like `trg_monitor_notify`) — rejected: a missed deadline is the *absence* of an event; nothing writes the row when time passes, so there is no notification to listen for. You would still need a timer, making NOTIFY redundant for this purpose. (B) **Per-heartbeat timers/goroutines** (`time.AfterFunc` per heartbeat) — rejected: unbounded goroutine growth at 500+ heartbeats, violates the AGENTS.md bounded-pool rule and R10.1, and is hard to reconcile on restart. (C) **chosen: periodic sweep with bounded worker pool + partial index**, modeled on the proven `monitor.Scheduler` — bounded concurrency, single indexed query per tick, deterministic shutdown, documented detection-latency bound (≤ tick interval, default 1s). It is the same pattern already operating at scale in Pulse.

---

## Follow-up / Out of Scope (carried from requirements Non-Goals)

1. Run lifecycle events (start/success/failure with run ids and per-run duration).
2. Timezone-aware cron schedules (cron expressions, DST semantics).
3. Concurrent-run, duplicate, late-arrival, and clock-boundary handling.
4. A dedicated MCP heartbeat tool (`mcp/internal/tools/`, `mcp/internal/pulseapi/`) — co-change rule evaluated, no existing tool affected (§12.2).
5. Restore/validity verification of job output (green ping ≠ valid backup).
6. Independently hosted external heartbeat receiver.
7. Heartbeat-specific per-ping history time-series / charts in TimescaleDB.

Design-note backlog items surfaced above for a future pass: (a) email/webhook template vocabulary says "Monitor" — slightly inaccurate for heartbeats, accepted for v1 (a template-vocabulary generalization is deferred); (b) the heartbeat→monitor trigger translation (`heartbeat_missed → monitor_down`, `heartbeat_recovered → monitor_up`) is applied at the dispatch call site to the whole `BindingWithChannel.Triggers` (the chosen approach — §5.3); adding native `heartbeat_missed`/`heartbeat_recovered` cases to `StateTracker.Evaluate` is a recorded alternative, deferred to avoid coupling the shared tracker to heartbeat vocabulary; (c) `delivery_logs.monitor_id` is reused as a generic resource id for heartbeat audit rather than renamed (renaming would break existing monitor logs).

---

## Review Responses (iteration 2)

Responses to every finding in `design-review.json` / `design-review.md`. All HIGH and MEDIUM findings are **addressed**; all NITs are **addressed**. No finding is backlogged or ignored.

- **HIGH-1 — pre-populated `TemplateData` infeasible with `EvaluateAndDispatch`.** Addressed in §5.3. Verified in `fanout.go` that `EvaluateAndDispatch` builds the payload internally via `buildPayload` (no `Monitor.Name`) and exposes no payload parameter, and in `dispatcher.go` that `enrichPayload` runs `GetMonitor` when `Monitor.Name == ""`. The design now specifies adding a sibling `Dispatcher.EvaluateAndDispatchWithPayload(ctx, resourceID, result, bindings, payloadBuilder)` and refactoring `EvaluateAndDispatch` to delegate to it with the existing `buildPayload` (monitor behavior unchanged). The heartbeat path passes a `heartbeatPayloadBuilder` closure that pre-fills `Monitor.Name` (= heartbeat name) and `BaseURL`, so `enrichPayload`'s `GetMonitor(heartbeatID)` is skipped. The design no longer claims `EvaluateAndDispatch` is reused "unchanged" for payload population.

- **HIGH-2 — trigger translation point wrong; bindings would never match.** Addressed in §5.3. Verified in `fanout.go` that the same `bindings` slice feeds both `extractTriggerConditions → StateTracker.Evaluate` and `bindingMatchesTrigger` (`tc.Type == event.Type`), and in `state_tracker.go` that `Evaluate` emits `monitor_down`/`monitor_up` events. The design now translates the **whole `BindingWithChannel.Triggers`** (via `translateHeartbeatTriggers`) to `monitor_down`/`monitor_up` **before** calling the dispatcher, so both the evaluation and the matcher operate on monitor vocabulary and a `DeliveryJob` is enqueued. Heartbeat vocabulary is kept only in the persisted JSONB, `ValidateHeartbeatTriggers`, and the API DTO. Exactly one approach is specified (translate-at-call-site); the `StateTracker` modification is explicitly recorded as a deferred alternative, not presented as interchangeable.

- **MEDIUM-1 — `delivery_logs.monitor_id` is `NOT NULL`.** Addressed in §1.3 and §5.3. Verified in migration 014 (`monitor_id UUID NOT NULL`, no FK; `binding_id` nullable) and in `delivery_log.go` (`LogDelivery` always called with `job.MonitorID`). The design now states explicitly that the heartbeat dispatch sets `DeliveryJob.MonitorID = heartbeatID` and `BindingID = heartbeatBindingID`, satisfying the `NOT NULL` audit column.

- **MEDIUM-2 — ping write concurrency unspecified.** Addressed in §4.1, §4.3, and §2.2. The design now specifies option (a) from the review: the success/fail ping write is a single atomic `UPDATE … RETURNING` (via a `FOR UPDATE` CTE) that returns the prior `state` read atomically with the write (`MarkHeartbeatUp`, `MarkHeartbeatFail`). Postgres serializes concurrent updates to the same row, so racing success pings each get a distinct `previous_state` and recovery fires at most once. `PingHandler` keeps only `*db.Queries` (no `*pgxpool.Pool` needed). The design states that a duplicate success ping is idempotent in v1 while richer concurrent-run semantics stay deferred (Non-Goal 3).

- **NIT-1 — `api_tokens` column naming.** Addressed in §1.1: reworded to "the prefix-lookup + hash shape of `api_tokens` (its lookup column is `prefix`, queried via `WHERE prefix = $1` in `ListAPITokensByPrefix`)."

- **NIT-2 — discarded bcrypt hash from `token.Generate()`.** Addressed in §3.4: notes that `token.Generate()` returns a bcrypt hash that the heartbeat create/rotate path discards (acceptable, since create/rotate are rare admin ops), and that the implementation MAY add a `token.GenerateRaw()` helper to skip the bcrypt step.

- **NIT-3 — `heartbeat_status` payload field enumeration.** Addressed in §7.2: the exact `HeartbeatStatusPayload` TypeScript field set is enumerated (`heartbeat_id`, `state`, `last_success_at`, `next_expected_deadline`, `timestamp`), with an explicit note that the store merge keys by `heartbeat_id` and never reads `checked_at`.
