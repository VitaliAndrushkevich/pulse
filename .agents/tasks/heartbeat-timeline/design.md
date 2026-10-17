# Heartbeat Historical Event Timeline — Technical Design

## Overview

Heartbeats today are a push / dead-man's-switch resource that stores only
**current** state: the `heartbeats` row is overwritten on every ping and on
every missed-run sweep, so there is no historical trail. This change adds a
durable, append-only event log for heartbeats and surfaces it as a clear,
color-coded timeline on the heartbeat detail page (`/heartbeats/[id]`), plus a
small summary (uptime % and missed-run count over the selected window).

The design mirrors the existing monitor-history architecture end to end: a
TimescaleDB hypertable written through `backend/internal/store/timescale`, a
read endpoint shaped like `GET /monitors/{id}/history`, per-resource retention
wired into the existing retention sweep, and a detail-page timeline component
modeled on `StatusTimeline.svelte`. The key adaptation is that heartbeat events
are **discrete episodes** (one row per ping success, explicit fail, or missed-run
transition) rather than scheduled check samples, so the timeline renders event
markers/segments on a time axis rather than fixed-count beat buckets.

Because nothing is recorded today, the timeline only shows data from the moment
the write-path ships forward — **there is no backfill**. A new or empty
heartbeat MUST render an explicit empty state so an operator does not mistake an
empty timeline for a bug.

Scope is the heartbeat **detail page only** — not the list page, not the
dashboard. No MCP changes: the AGENTS.md MCP co-change list contains no
heartbeat endpoint, no existing MCP tool consumes heartbeat routes, and a
dedicated heartbeat MCP tool is a deferred non-goal.

### Technology stack (locked once approved)

- Backend: Go 1.26, `gin` v1.12, `pgx/v5` (`pgxpool`), TimescaleDB (via the
  shared pool), `sqlc` for relational queries. Time-series access uses raw
  `pgx` through `backend/internal/store/timescale`, NOT `sqlc`.
- Migrations: `golang-migrate/v4`, embedded in `backend/migrations/` and
  auto-applied at startup. Next sequential number is **018**.
- Frontend: Svelte 5 + SvelteKit 2, TypeScript strict, Tailwind 3.4, i18n via
  `t()` from `$lib/i18n`, CSS custom properties from `app.css`, dark mode via
  `[data-theme="dark"]`.

---

## 1. Data model: the `heartbeat_events` hypertable

### 1.1 Decision — new hypertable mirroring `check_results`

Monitor history uses a TimescaleDB hypertable `check_results` partitioned on
`checked_at`, created in migrations 001 (`CREATE TABLE`) and 004
(`create_hypertable`). We mirror that shape for heartbeats. This was the
approved storage approach ("option A"); the sections below refine **how**, not
whether.

Alternatives considered and rejected:

- **Reuse `check_results` with a synthetic `monitor_id`.** Rejected: it would
  overload a monitor-scoped table with a foreign-key to `monitors`, break the
  `state IN ('up','down')` CHECK (heartbeats have a third `missed` event), and
  pollute monitor retention/aggregation queries. Separate table keeps the two
  resources cleanly isolated, exactly as the heartbeat relational tables
  (migration 017) are isolated from monitor tables.
- **A plain (non-hypertable) Postgres table.** Rejected: it would diverge from
  the established time-series pattern and lose Timescale's partition pruning and
  retention ergonomics that the monitor path already relies on. The task
  explicitly calls for mirroring `backend/internal/store/timescale`.

### 1.2 Schema

Migration `backend/migrations/018_heartbeat_events.up.sql`:

```sql
-- 018_heartbeat_events.up.sql
-- Append-only historical event log for heartbeats (dead-man's-switch resource).
-- One row is written on every ping success, every explicit fail ping, and every
-- missed-run sweep transition. Mirrors check_results (migrations 001/004) but
-- models DISCRETE heartbeat episodes, not scheduled check samples.

CREATE TABLE heartbeat_events (
    id            UUID        NOT NULL DEFAULT gen_random_uuid(),
    heartbeat_id  UUID        NOT NULL REFERENCES heartbeats (id) ON DELETE CASCADE,
    occurred_at   TIMESTAMPTZ NOT NULL,
    event_type    TEXT        NOT NULL CHECK (event_type IN ('success', 'fail', 'missed')),
    -- Optional transition context: the observed state the heartbeat held before
    -- this event (one of up/down/paused/new). Lets the UI label recoveries vs.
    -- repeated downs without a second query. Nullable because the first ever
    -- event for a heartbeat has no prior observed state captured.
    previous_state TEXT       CHECK (previous_state IN ('up', 'down', 'paused', 'new')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, occurred_at)
);

CREATE INDEX idx_heartbeat_events_hb_id_occurred_at
    ON heartbeat_events (heartbeat_id, occurred_at DESC);

-- The timescaledb extension is created in migration 004
-- (004_timescaledb_check_results.up.sql), which runs earlier; it is intentionally
-- NOT repeated here, so no CREATE EXTENSION is needed before create_hypertable.
SELECT create_hypertable('heartbeat_events', 'occurred_at', if_not_exists => TRUE);
```

Migration `backend/migrations/018_heartbeat_events.down.sql`:

```sql
DROP TABLE IF EXISTS heartbeat_events;
```

Design rationale:

- **Composite PK `(id, occurred_at)`** matches `check_results` — Timescale
  requires the partition column in every unique constraint.
- **`occurred_at` is the partition/time column.** It carries the real event
  instant (ping arrival `now`, explicit fail `now`, or the sweep's transition
  `now`), not `created_at` (which is a write-audit default). The read path
  windows and buckets on `occurred_at`.
- **`event_type` CHECK is the source of truth** for the three event kinds. The
  frontend maps these to colors; `missed` is the heartbeat-specific episode that
  has no `check_results` analog.
- **`previous_state` is kept minimal and nullable.** It is populated from the
  `previous_state` the write-path already reads atomically (`MarkHeartbeatUp` /
  `MarkHeartbeatFail` return it; the evaluator passes `row.State`). It is
  advisory: the timeline reads correctly from `event_type` alone, but
  `previous_state` lets the UI distinguish a recovery (`success` after `down`)
  from a steady-state success without extra lookups. This is the only "optional
  transition context" allowed by the task, and it is cheap.
- **`ON DELETE CASCADE`** removes a heartbeat's event history when the heartbeat
  is deleted, consistent with `check_results` → `monitors`.

### 1.3 Retention column

Add per-heartbeat retention to the `heartbeats` table in the **same migration**
(018), mirroring `monitors.history_retention_days` (migration 011):

```sql
ALTER TABLE heartbeats
    ADD COLUMN history_retention_days INTEGER NOT NULL DEFAULT 30
    CONSTRAINT chk_heartbeat_retention_range
        CHECK (history_retention_days >= 1 AND history_retention_days <= 365);
```

Range 1–365, default 30 — identical to monitors (approved: "reuse the existing
retention service pattern"). The down migration drops both the table and the
column:

```sql
-- 018_heartbeat_events.down.sql
ALTER TABLE heartbeats DROP COLUMN IF EXISTS history_retention_days;
DROP TABLE IF EXISTS heartbeat_events;
```

> Adding a column to `heartbeats` forces regeneration of the sqlc models for
> that table (every `heartbeats.sql` query uses `RETURNING *` / `SELECT *`), so
> `history_retention_days` appears on `db.Heartbeat` and all projection rows.
> This is covered in §2.3 and §6.

---

## 2. Backend write path

### 2.1 Timescale store API

Extend `backend/internal/store/timescale/store.go` with a heartbeat-event type
and three methods, following the existing `CheckPoint` / `WriteCheckResult` /
`QueryHistory` shapes exactly.

```go
// HeartbeatEvent is a single discrete heartbeat episode stored in TimescaleDB.
type HeartbeatEvent struct {
    HeartbeatID   uuid.UUID
    EventType     string  // "success" | "fail" | "missed"
    PreviousState *string // observed state before the event, nil if unknown
    OccurredAt    time.Time
}

// WriteHeartbeatEvent appends one event row into the heartbeat_events hypertable.
func (s *Store) WriteHeartbeatEvent(ctx context.Context, ev HeartbeatEvent) error

// QueryHeartbeatEvents returns events for one heartbeat in [start, end),
// ordered by occurred_at ascending. Mirrors QueryHistory.
func (s *Store) QueryHeartbeatEvents(
    ctx context.Context, heartbeatID uuid.UUID, start, end time.Time,
) ([]HeartbeatEvent, error)

// CountHeartbeatEventsByType returns per-type counts for one heartbeat in
// [start, end). Backs the summary (missed-run count) in a single round-trip.
func (s *Store) CountHeartbeatEventsByType(
    ctx context.Context, heartbeatID uuid.UUID, start, end time.Time,
) (map[string]int64, error)
```

`WriteHeartbeatEvent` issues a single parameterized `INSERT`:

```sql
INSERT INTO heartbeat_events (heartbeat_id, occurred_at, event_type, previous_state)
VALUES ($1, $2, $3, $4)
```

`QueryHeartbeatEvents` selects `heartbeat_id, event_type, previous_state,
occurred_at` where `heartbeat_id = $1 AND occurred_at >= $2 AND occurred_at <
$3 ORDER BY occurred_at ASC`.

`CountHeartbeatEventsByType` runs `SELECT event_type, COUNT(*) ... GROUP BY
event_type` over the same window and returns a map so the handler can read
`counts["missed"]` and compute uptime without materializing every row twice.

**Why no aggregation/`time_bucket` method (unlike monitors).** Monitor history
needs latency downsampling because checks fire on a fixed cadence and a 30-day
window is huge. Heartbeat events are sparse and discrete (one per ping, not one
per second); the number of events over a typical window is small, so the
timeline renders raw events directly. If a heartbeat ever produces a very large
event count the frontend buckets client-side (§5.3) and the list is virtualized
(§5.4) — we do not need server-side `time_bucket` for the first version. This is
a deliberate simplification called out so a reviewer can challenge it; the raw
query is bounded by retention (≤365 days) and the per-heartbeat index.

### 2.2 Decision — where the write is issued

The write-path must record an event on **every ping success, every explicit
fail ping, and every missed-run sweep transition** (approved decision 1). All
three sites already compute the exact information `heartbeat_events` needs
(the row, the new state, and the atomically-read `previous_state`).

Two placement options were considered:

- **Option A — write inside the existing handlers/evaluator** at the same point
  they already fire metrics/broadcast/dispatch (`PingHandler.afterTransition`,
  `PingHandler.Fail`, `Evaluator.markDown`).
- **Option B — write via a new DB trigger** on `heartbeats` state changes.

**Chosen: Option A.** It keeps the event write beside the other non-blocking
side effects, reuses the `previous_state` already returned by the atomic SQL,
and records `success` events even when the state does **not** transition (a
healthy heartbeat pinging every minute must still produce a continuous success
trail for the timeline — a trigger keyed on state *change* would miss those).
Option B cannot see "a success ping that kept state = up," which is most of the
timeline's content, so it is rejected.

**Introduce an event-writer seam** so the handler/evaluator do not depend on the
concrete Timescale store directly and stay unit-testable with a fake (mirrors
the existing `PingDispatcher` / `Dispatcher` seams):

```go
// HeartbeatEventWriter records heartbeat timeline events. A nil writer is valid
// and simply skips the write (the DB state update, metric, broadcast, and
// dispatch still happen) — matching how a nil hub/dispatcher is tolerated today.
type HeartbeatEventWriter interface {
    WriteHeartbeatEvent(ctx context.Context, ev timescale.HeartbeatEvent) error
}
```

`*timescale.Store` already satisfies this interface by method set. We inject it
into `PingHandler` and `heartbeat.Evaluator` as the interface type, keeping the
seam.

#### Injection mechanism and the exact call sites to update

Both constructors have fixed positional signatures today, so adding the writer
changes their shape and **every existing caller must be updated in the same
change or `go test ./...` fails to compile**. We add the writer as a **trailing
positional parameter**, consistent with how the existing `dispatcher`/`Dispatcher`
seam was added last (not a functional option, not a post-construction field
mutation — those would diverge from the current constructor style for no benefit).
A nil writer is valid everywhere, so callers that have no writer pass `nil`.

New signatures:

```go
// handlers package (ping.go)
func NewPingHandler(queries *db.Queries, wsHub *hub.Hub, metrics *heartbeat.Metrics,
    dispatcher PingDispatcher, events HeartbeatEventWriter) *PingHandler

// heartbeat package (evaluator.go)
func NewEvaluator(cfg Config, queries Queries, metrics *Metrics, wsHub *hub.Hub,
    dispatcher Dispatcher, events HeartbeatEventWriter) *Evaluator
```

> Note: the `HeartbeatEventWriter` interface is declared once and referenced by
> both packages. To avoid an import cycle (the `heartbeat` package must not
> import `handlers`), declare the interface in a neutral location both can import
> — either in the `heartbeat` package (which `handlers` already imports for
> `heartbeat.Metrics`) or in the `timescale` package alongside `HeartbeatEvent`.
> **Chosen: declare it in the `timescale` package** (`type HeartbeatEventWriter
> interface { WriteHeartbeatEvent(ctx, HeartbeatEvent) error }`), because both
> `handlers` and `heartbeat` already import `timescale` for the event type and
> `*timescale.Store` is the sole production implementer. Each constructor then
> takes `timescale.HeartbeatEventWriter`.

Every call site to update (pass `nil` where no writer is wired yet; production
wiring in §7 passes the real store):

| File | Call | Change |
|------|------|--------|
| `backend/internal/api/router.go` | `handlers.NewPingHandler(deps.Queries, deps.Hub, deps.HeartbeatMetrics, deps.PingDispatcher)` | append `deps.HeartbeatEventWriter` (new `Deps` field, §7) |
| `backend/cmd/pulse/main.go` | `heartbeat.NewEvaluator(heartbeat.Config{...}, queries, heartbeatMetrics, wsHub, heartbeatDispatch)` | append `timescaleStore` |
| `backend/internal/api/handlers/ping_test.go` | `handlers.NewPingHandler(db.New(fdb), nil, metrics, disp)` (1 call) | append `nil` (or a test fake when asserting the write path, §9) |
| `backend/internal/heartbeat/evaluator_test.go` | `heartbeat.NewEvaluator(...)` at 3 sites (~lines 150, 202, 243) | append `nil` (or a recording fake in the test that asserts the `missed` write, §9) |

This is a mechanical, enumerated update; after it, no caller is left with the
old arity.

### 2.3 Integration point 1 & 2 — ping success and explicit fail

In `backend/internal/api/handlers/ping.go`:

- `PingHandler` gains an `events timescale.HeartbeatEventWriter` field (may be
  nil); `NewPingHandler` takes it as the new trailing parameter (§2.2).
- `Success` writes a `success` event; `Fail` writes a `fail` event. Both already
  hold the updated row and the atomically-read `updated.PreviousState`.

**Single-source rule (do not add a second write).** The ping-path event write
happens in **exactly one place**: the first line of `afterTransition`.
`afterTransition` is the single side-effect site for *both* ping paths — `Success`
calls `afterTransition(c, row, "up", updated.PreviousState)` and `Fail` calls
`afterTransition(c, row, "down", updated.PreviousState)` (verified in `ping.go`).
Because both routes funnel through `afterTransition`, placing the write there
covers success **and** explicit fail with one call. `Fail` MUST NOT write an
event itself — doing so would double-record every explicit-fail event. The write
runs **after** the DB `UPDATE` has already committed, alongside the existing
non-blocking metric/broadcast/dispatch calls.

```go
func (h *PingHandler) afterTransition(c *gin.Context, row db.Heartbeat, newState, previousState string) {
    h.writeEvent(c.Request.Context(), row.ID, eventTypeForState(newState), previousState)
    // ... existing metric / hub broadcast / dispatcher calls unchanged ...
}
```

`eventTypeForState` is the **single source** of the ping state→event mapping and
maps only the two states the ping path can produce:

```go
// eventTypeForState maps the ping-path observed state to a heartbeat_events
// event_type. It is only ever called from afterTransition with "up" or "down".
// "missed" is NOT a ping-path event — it is produced ONLY by the evaluator
// (§2.4). The default branch guards against a future caller mis-mapping.
func eventTypeForState(state string) string {
    switch state {
    case "up":
        return "success"
    case "down":
        return "fail"
    default:
        // Unreachable on the ping path. Log and fall back to "fail" rather than
        // writing an invalid event_type that would violate the CHECK constraint.
        log.Printf("ping: eventTypeForState called with unexpected state=%q", state)
        return "fail"
    }
}
```

So: `up`→`success`, `down`→`fail`, and `missed` is never emitted from the ping
path. There is exactly one write per ping, from one place, driven by one mapping.

`writeEvent` is a small private helper:

```go
func (h *PingHandler) writeEvent(ctx context.Context, id uuid.UUID, eventType, prev string) {
    if h.events == nil {
        return
    }
    var prevPtr *string
    if prev != "" {
        prevPtr = &prev
    }
    if err := h.events.WriteHeartbeatEvent(ctx, timescale.HeartbeatEvent{
        HeartbeatID:   id,
        EventType:     eventType,
        PreviousState: prevPtr,
        OccurredAt:    time.Now().UTC(),
    }); err != nil {
        log.Printf("ping: write heartbeat event failed id=%s type=%s: %v", id, eventType, err)
    }
}
```

### 2.4 Integration point 3 — missed-run sweep

In `backend/internal/heartbeat/evaluator.go`, the evaluator's `markDown` already
runs its side effects **only when `MarkHeartbeatDown` actually updated a row**
(the `WHERE state = 'up'` guard makes a missed-run episode fire exactly once).
We write the `missed` event at that same guarded point, so exactly one `missed`
row is produced per missed-run episode — no duplication across repeated sweeps.

- Add a nilable `events timescale.HeartbeatEventWriter` field to `Evaluator`,
  set from the new trailing `NewEvaluator` parameter (§2.2). This is the **same**
  interface used by the ping handler, declared once in the `timescale` package.
- `markDown` already captures `previousState := row.State` **before** calling
  `MarkHeartbeatDown` (verified in `evaluator.go`), and its side effects run only
  when the `WHERE state = 'up'` guard matched a row — so writing the event at
  that same guarded point yields **exactly one `missed` row per missed-run
  episode**, with no duplication across repeated sweep ticks.
- In `markDown`, after the successful `MarkHeartbeatDown` and before/alongside
  the metric+broadcast+dispatch block:

```go
if e.events != nil {
    if err := e.events.WriteHeartbeatEvent(ctx, timescale.HeartbeatEvent{
        HeartbeatID:   updated.ID,
        EventType:     "missed",
        PreviousState: strPtr(previousState), // always "up" for a sweep miss
        OccurredAt:    time.Now().UTC(),
    }); err != nil {
        log.Printf("heartbeat evaluator: write missed event failed id=%s: %v", updated.ID, err)
    }
}
```

### 2.5 Error handling for the write path

For each write the failure handling is concrete:

| Operation | Failure condition | Recoverable? | Caller sees | Logged |
|-----------|-------------------|--------------|-------------|--------|
| `WriteHeartbeatEvent` (ping success/fail) | DB/Timescale error, context cancel | Yes — the heartbeat state UPDATE already committed; only the history row is lost | Nothing — ping still returns `200 OK`. The event write is a non-blocking side effect, exactly like the metric/broadcast/dispatch that follow it | Yes, at the handler, with heartbeat id + event type (never the token) |
| `WriteHeartbeatEvent` (sweep missed) | DB/Timescale error, context cancel | Yes — the down transition already committed; only the history row is lost | N/A (background sweep) | Yes, at the evaluator, with heartbeat id |

Rationale: the authoritative state lives in the `heartbeats` row and is updated
in the same atomic statement that returns `previous_state`. The event log is an
observability artifact. Failing the ping (returning 500) because a history row
could not be appended would make the dead-man's-switch *less* reliable — the
opposite of the product intent. A dropped event leaves a one-event gap in the
timeline, which is acceptable and strictly better than a dropped ping. Writes
are best-effort and logged; they never block or fail the primary operation. This
matches how `WriteCheckResult` failures are handled on the monitor path.

Ordering note: the event write is issued **after** the state UPDATE commits, so
a failed write never implies a wrong current state. It runs synchronously within
the already-running handler/worker goroutine (not spawned) to avoid unbounded
goroutine growth — the insert is a single bounded statement.

---

## 3. Retention wiring

### 3.1 Decision — extend the existing retention sweep

`retention.RetentionService` (`backend/internal/retention/service.go`) iterates
monitors in pages and deletes expired `check_results` rows per monitor. We
extend the **same service and the same cycle** to also prune `heartbeat_events`,
rather than adding a second service — the approved decision is to "wire
`heartbeat_events` pruning into the existing retention sweep service."

Within `runCycle`, after the monitor-pruning loop, add a parallel heartbeat loop
that pages `ListHeartbeats` and deletes expired `heartbeat_events` per heartbeat
using each heartbeat's `history_retention_days`:

```go
func (s *RetentionService) deleteExpiredHeartbeatEvents(ctx context.Context, hb db.Heartbeat) (int64, error) {
    cutoff := time.Now().Add(-time.Duration(hb.HistoryRetentionDays) * 24 * time.Hour)
    tag, err := s.pool.Exec(ctx,
        `DELETE FROM heartbeat_events
         WHERE id IN (
           SELECT id FROM heartbeat_events
           WHERE heartbeat_id = $1 AND occurred_at < $2
           LIMIT $3
         )`,
        hb.ID, cutoff, s.deleteLimit,
    )
    if err != nil {
        return 0, err
    }
    return tag.RowsAffected(), nil
}
```

The heartbeat loop **mirrors the monitor loop in `runCycle` exactly** — it is
not a loosely "parallel" loop but a structural copy that differs only in the
list query, the delete query, and the per-item retention field:

- **Paging:** page `ListHeartbeats` via `db.ListHeartbeatsParams{Limit:
  int32(s.batchSize), Offset: offset}` (verified signature: `LIMIT $1 OFFSET
  $2`), incrementing `offset += int32(s.batchSize)` each page — identical to the
  monitor loop.
- **Termination:** stop when `len(page) < s.batchSize` (the existing last-page
  test), identical to the monitor loop.
- **Error tolerance:** reuse the same `maxConsecutiveBatchErrors` (3) counter —
  a batch-level error increments it, a success resets it, exceeding it aborts
  the cycle.
- **Per-item isolation:** on a single heartbeat's delete error, `log` and
  `continue` to the next heartbeat (one heartbeat's failure never blocks the
  others), identical to the monitor per-item handling.
- **Delete:** uses `deleteExpiredHeartbeatEvents` with that heartbeat's own
  `hb.HistoryRetentionDays`, reusing the `deleteLimit` cap (10,000 rows per
  DELETE statement).

Keeping the two loops structurally identical avoids a subtly different page
boundary (off-by-one), a missing error-isolation `continue`, or a different
termination test. The existing `pulse_retention_rows_deleted_total` counter is
incremented for heartbeat rows too; a reviewer may prefer a separate counter
label — noted as a minor open choice in §9, defaulting to reusing the single
counter to avoid metric churn.

`ListHeartbeats` and the `heartbeats` row already exist; the only new field they
carry is `HistoryRetentionDays` (from §1.3). No new sqlc query is required for
retention because the DELETE is raw `pool.Exec`, consistent with
`deleteExpiredRows`.

### 3.2 Retention field on the API resource

Expose `history_retention_days` on the heartbeat resource. The existing
create/PUT path funnels field validation through
`validateHeartbeatFields(name, interval, grace, status) → (interval, grace,
status, errMsg)` (verified in `heartbeats.go`), which returns **no** retention
value, and the handlers build `db.CreateHeartbeatParams` / `db.UpsertHeartbeatParams`
with **no** retention field today. Retention is therefore threaded as a
**separate, parallel validation** — mirroring exactly how `monitors.go` does it
with `validateRetentionDays` alongside its own field validation. The full thread,
end to end:

**1. Request DTOs** (`backend/internal/api/handlers/heartbeats.go`). Add to
`createHeartbeatRequest` and `putHeartbeatRequest`:

```go
HistoryRetentionDays *int32 `json:"history_retention_days,omitempty"`
```

(Pointer so "omitted" is distinguishable from an explicit value; nil → default
30. This matches the monitor request DTOs.)

**2. Validation helper.** Add a heartbeat-local `validateHeartbeatRetentionDays`
that is a copy of `monitors.go`'s `validateRetentionDays` (nil → `30`; else must
be in `[1, 365]`, otherwise an error):

```go
func validateHeartbeatRetentionDays(days *int32) (int32, error) {
    if days == nil {
        return 30, nil
    }
    if *days < 1 || *days > 365 {
        return 0, fmt.Errorf("history_retention_days must be between 1 and 365, got %d", *days)
    }
    return *days, nil
}
```

**3. Invocation in `Create` and `Put`.** Immediately **after** the existing
`validateHeartbeatFields` call (and its `verr` check), call the retention
validator and, on error, return the **same error code the monitor path uses**:

```go
interval, grace, status, verr := validateHeartbeatFields(...)
if verr != "" {
    apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", verr)
    return
}
retentionDays, rerr := validateHeartbeatRetentionDays(req.HistoryRetentionDays)
if rerr != nil {
    apiError(c, http.StatusBadRequest, "INVALID_RETENTION_PERIOD", rerr.Error())
    return
}
```

> Error-code consistency note: the monitor path returns
> `400 INVALID_RETENTION_PERIOD` (not `VALIDATION_ERROR`) for an out-of-range
> retention value — verified in `monitors.go` (`Create`/`Put` both call
> `apiError(c, http.StatusBadRequest, "INVALID_RETENTION_PERIOD", err.Error())`).
> The heartbeat path uses the **same** `INVALID_RETENTION_PERIOD` code and the
> **same** message `"history_retention_days must be between 1 and 365, got N"`
> so the two resources behave identically. (The design review's suggested
> `VALIDATION_ERROR` is superseded here to match the real codebase convention;
> the finding's intent — thread and validate the value — is fully satisfied.)

**4. Carry into the sqlc params.** Pass the validated `retentionDays` into both
params structs built in the handlers:

```go
db.CreateHeartbeatParams{ ..., HistoryRetentionDays: retentionDays }
db.UpsertHeartbeatParams{ ..., HistoryRetentionDays: retentionDays }
```

**5. sqlc queries** (`backend/internal/store/postgres/queries/heartbeats.sql`):
- `CreateHeartbeat`: add `history_retention_days` to the INSERT column list and
  a corresponding positional value.
- `UpsertHeartbeat`: add `history_retention_days` to the INSERT column list and
  value **and** to the `ON CONFLICT DO UPDATE SET` clause
  (`history_retention_days = EXCLUDED.history_retention_days`) so a PUT can
  change retention on an existing heartbeat. Without the `DO UPDATE SET` line, a
  PUT would silently ignore a retention change.

Then `cd backend && sqlc generate` and confirm `sqlc diff` is clean. Never
hand-edit `heartbeats.sql.go`.

**6. Response DTO.** Add `HistoryRetentionDays int32 \`json:"history_retention_days"\``
to `heartbeatResponse` and set it in `toHeartbeatResponse`:
`HistoryRetentionDays: hb.HistoryRetentionDays`.

**7. DB CHECK as backstop.** The migration-018 CHECK (`>= 1 AND <= 365`) also
enforces the range. A value that somehow bypasses the handler surfaces as Postgres
error `23514` and is mapped to a `400` by the existing check-constraint error
path, so the DB is the final guard. The handler validation is the primary,
user-friendly gate.

---

## 4. Read endpoint: `GET /api/v1/heartbeats/{id}/history`

### 4.1 Decision — a dedicated handler mirroring monitor history

Add the endpoint in a new handler file
`backend/internal/api/handlers/heartbeat_history.go` (keeping it beside
`heartbeats.go` but separate from the monitor `history.go`, since it depends on
the Timescale store and has a distinct response shape). It mirrors
`HistoryHandler` for parameter parsing, defaults, and retention clamping, but
returns discrete events and a summary instead of check samples/aggregates.

```go
type HeartbeatHistoryHandler struct {
    queries *db.Queries
    tsStore *timescale.Store
}

func (h *HeartbeatHistoryHandler) Register(rg *gin.RouterGroup) {
    rg.GET("/heartbeats/:id/history", h.GetHistory)
}
```

Registered in `backend/internal/api/router.go` inside the `protected` group,
gated by `deps.TimescaleStore != nil` (exactly like the monitor history handler
registration), immediately after the heartbeat CRUD handler registration.

### 4.2 Request contract

Query parameters, consistent with `GET /monitors/{id}/history`:

| Param | Type | Required | Default | Validation / behavior |
|-------|------|----------|---------|------------------------|
| `from` | RFC3339 timestamp | No | `now - 24h` | Invalid → `400 VALIDATION_ERROR` ("from must be a valid RFC 3339 timestamp") |
| `to` | RFC3339 timestamp | No | `now` | Invalid → `400 VALIDATION_ERROR`. `from` must be strictly before `to`, else `400 VALIDATION_ERROR` ("from must be before to") |
| `step` | integer seconds | No | absent | Accepted for API symmetry with monitor history and validated to `[60, 86400]` (`400 INVALID_STEP` otherwise), but the first version does **not** server-side bucket — see §4.4. Reserved; if present it is echoed back and ignored for payload shape |

Path param `id` must be a valid UUID (`400 INVALID_ID` otherwise). The heartbeat
must exist (`GET` via `queries.GetHeartbeat`), else `404 NOT_FOUND`
("heartbeat not found"). All responses carry `X-Request-ID` via the existing
middleware; errors use the standard envelope `{ "error": { "code", "message" } }`.

Retention clamping reuses the monitor pattern: if `from < now -
history_retention_days`, clamp `from` to the boundary and set `truncated: true`.

> **Decision on `step`:** monitor history uses `step` to switch between raw and
> aggregated latency responses. Heartbeat events have no latency to aggregate
> and the timeline consumes discrete events, so the first version returns raw
> events only. We still **accept and validate** `step` so the endpoint contract
> stays shape-compatible with the monitor endpoint (and so a future
> server-side bucketing option is non-breaking), but it does not change the
> payload. This is an explicit, reviewable simplification; the alternative
> (omit `step` entirely) was rejected to keep the two history endpoints
> parallel for API consumers and docs.

### 4.3 Response contract

```jsonc
{
  "heartbeat_id": "9f1c...-uuid",
  "from": "2026-07-01T00:00:00Z",
  "to":   "2026-07-02T00:00:00Z",
  "truncated": false,
  "summary": {
    "uptime_percent": 98.6,   // float, 0–100, 1 decimal; null when no events
    "missed_count": 2,        // count of 'missed' events in window
    "fail_count": 1,          // count of explicit 'fail' events in window
    "success_count": 142,     // count of 'success' events in window
    "total_events": 145
  },
  "events": [
    { "event_type": "success", "previous_state": "up",   "occurred_at": "2026-07-01T00:01:03Z" },
    { "event_type": "missed",  "previous_state": "up",   "occurred_at": "2026-07-01T06:12:00Z" },
    { "event_type": "success", "previous_state": "down", "occurred_at": "2026-07-01T06:20:11Z" }
  ]
}
```

- `events` is ordered by `occurred_at` ascending (chronological, left→right on
  the timeline). It is `[]` (never `null`) when empty — the handler initializes
  the slice with `make([]..., 0, n)`, matching `history.go`.
- `previous_state` is omitted (`omitempty`) when null.
- **Uptime definition.** For the first version, `uptime_percent =
  success_count / total_events * 100`, i.e. the fraction of recorded events that
  were successful. This is an event-share metric, intentionally simple and
  computed from `CountHeartbeatEventsByType` in one query. It is **not**
  time-weighted (duration up vs. down). Time-weighting is harder to define
  before full history exists and is explicitly out of scope for v1 (§8); the
  summary label in the UI reads "Uptime" with a tooltip clarifying it is the
  share of successful check-ins over the window. Flagged for reviewer: if a
  time-weighted figure is required, it belongs in a follow-up and needs a clear
  product definition of "duration down."
- When `total_events == 0`, `uptime_percent` is `null` and all counts are `0`;
  the frontend renders the empty state (§5.5).
- **Invariant:** because `event_type` is CHECK-constrained to exactly
  `{success, fail, missed}`, `success_count + fail_count + missed_count ==
  total_events` always holds. The handler computes `total_events` as that sum (or
  equivalently a single `COUNT(*)` over the window — both must agree). A
  summary-math unit test asserts this equality (§9).
- **Uptime semantics:** `uptime_percent` counts **only `success`** as "up"; both
  `fail` and `missed` reduce it equally (they are both "not up"). I.e.
  `uptime_percent = success_count / total_events * 100`. This equivalence is
  intentional for v1 and is surfaced to the operator via the "Uptime" tooltip
  ("Share of successful check-ins over the selected window"). It is an
  event-share figure, not time-weighted (see §8).

Handler flow: parse+validate params → load heartbeat (404 if missing) → clamp
`from` to retention → `QueryHeartbeatEvents(id, from, to)` for the list →
`CountHeartbeatEventsByType(id, from, to)` for the summary → assemble response.
Two bounded queries over the same window; no N+1.

### 4.4 Edge cases

- **Empty window / brand-new heartbeat:** zero events → `events: []`,
  `summary.uptime_percent: null`, counts `0`, `200 OK`. Not an error.
- **`from == to` or `from > to`:** `400 VALIDATION_ERROR`.
- **Window entirely before retention boundary:** `from` clamps forward to the
  boundary, `truncated: true`; if no events remain, same empty payload.
- **Very large event count:** bounded by retention; raw rows returned; frontend
  buckets/virtualizes for render (§5.3–5.4). No server pagination in v1 (the
  endpoint is windowed, not paged) — noted as a reviewable limit in §9.
- **Deleted heartbeat with lingering events:** cannot occur for reads —
  `GetHeartbeat` 404s first; and `ON DELETE CASCADE` removes events on delete.

### 4.5 OpenAPI

Update `backend/api/openapi.yaml` **in the same change** (OpenAPI is the source
of truth). Add:

- Path `GET /heartbeats/{id}/history` under the heartbeats tag, with the `id`
  path param, `from`/`to`/`step` query params, `200` response referencing a new
  `HeartbeatHistoryResponse` schema, and `400`/`404` referencing the existing
  error envelope.
- Schemas `HeartbeatHistoryResponse`, `HeartbeatEvent`, and
  `HeartbeatHistorySummary` matching §4.3 exactly (field names, nullability,
  `event_type` enum `[success, fail, missed]`, `previous_state` enum
  `[up, down, paused, new]`).
- The `history_retention_days` property on the existing heartbeat schema(s)
  (request + response), range 1–365, default 30, mirroring the monitor schema.

MCP check: this endpoint is not in the AGENTS.md MCP co-change list and no
existing MCP tool consumes it → **no MCP changes**. (Confirmed against the
co-change list, which covers only monitor/incident/tag/dashboard endpoints.)

---

## 5. Frontend

### 5.1 Types (`frontend/src/lib/types.ts`)

Add `history_retention_days: number` to the `Heartbeat` interface (and the
create/update request shape via `CreateHeartbeatRequest` in `api.ts`). Add:

```ts
export type HeartbeatEventType = 'success' | 'fail' | 'missed';

export interface HeartbeatEvent {
  event_type: HeartbeatEventType;
  previous_state?: 'up' | 'down' | 'paused' | 'new';
  occurred_at: string; // RFC3339
}

export interface HeartbeatHistorySummary {
  uptime_percent: number | null;
  missed_count: number;
  fail_count: number;
  success_count: number;
  total_events: number;
}

export interface HeartbeatHistoryResponse {
  heartbeat_id: string;
  from: string;
  to: string;
  truncated: boolean;
  summary: HeartbeatHistorySummary;
  events: HeartbeatEvent[];
}
```

### 5.2 API client (`frontend/src/lib/api.ts`)

```ts
/** GET /api/v1/heartbeats/:id/history?from=&to= */
export async function getHeartbeatHistory(
  id: string, from: string, to: string
): Promise<HeartbeatHistoryResponse> {
  const params = new URLSearchParams({ from, to });
  return apiRequest<HeartbeatHistoryResponse>(
    'GET', `/heartbeats/${id}/history?${params.toString()}`
  );
}
```

Default window: last 24h, computed in the detail page (`to = now`, `from = now -
24h`), matching the monitor history default.

### 5.3 Component — `HeartbeatTimeline.svelte`

A new reusable component `frontend/src/components/HeartbeatTimeline.svelte`,
modeled on `StatusTimeline.svelte`'s visual language (color-coded bar + legend +
summary + click-to-expand failure detail) but adapted for **discrete events**:

- **Props:** `{ events: HeartbeatEvent[]; summary: HeartbeatHistorySummary;
  loading: boolean; error: string | null; from: string; to: string }`.
- **Rendering model:** events are laid out on a horizontal time axis spanning
  `[from, to]`. Each event is a marker/segment positioned by `occurred_at`;
  color is driven by `event_type`:
  - `success` → healthy color
  - `fail` → down color
  - `missed` → a distinct "missed-run" color (visually separable from `fail`)
  Use the same three-way visual grammar as the monitor timeline so the page
  reads consistently, but with heartbeat semantics (success / missed / fail).
- **Bucketing for density:** when event count exceeds a `maxMarkers` threshold
  (e.g. 120), bucket by time slice like `StatusTimeline` does, where a bucket's
  color is the "worst" event in it (missed > fail > success) so problems are
  never hidden. Below the threshold, render one marker per event.
- **Interaction:** clicking a `fail`/`missed` marker (or bucket) opens a popover
  listing the events with their `occurred_at` and type, mirroring
  `StatusTimeline`'s failure popover. `success` markers are non-interactive.
- **Summary row:** show `summary.uptime_percent` (or an em dash when null) and
  `summary.missed_count`, plus the legend. Color-threshold the uptime figure
  like the monitor timeline (≥99 healthy, ≥95 warn, else down).

**Convention compliance (important divergence from the reference component).**
`StatusTimeline.svelte` predates the current conventions: it uses hardcoded
Tailwind color classes (`bg-emerald-500`, `bg-rose-500`) and literal English
strings. The new `HeartbeatTimeline.svelte` MUST instead:

- use CSS custom properties / brand utilities from `app.css` for status colors
  (e.g. `var(--color-...)`), never hardcoded palette classes, and use
  `[data-theme="dark"]` for any dark-mode variance — never the `dark:` prefix;
- route **every** user-visible string through `t('...')` (legend labels, summary
  labels, popover titles, empty/loading/error text, time labels where textual).

We intentionally do not copy the reference component's hardcoded styles/strings;
we copy its layout and interaction model. (Flagged so a reviewer does not read
the divergence as an inconsistency — it is deliberate adherence to AGENTS.md.)

### 5.4 Virtualization

The task says virtualize only if the event list can get large. The **timeline
bar** does not need virtualization — it buckets to a bounded marker count
(§5.3). If a secondary textual event list is shown in the popover/expanded view
and could be long, cap/scroll it (the reference popover already caps to 5 with a
"+N more"); a full virtualized list is not required for v1. Decision: **no
virtualization in v1**, bounded rendering via bucketing + capped popover. Noted
as revisitable if real-world event volumes prove large.

### 5.5 Empty state (mandatory)

When `summary.total_events === 0` (and not loading/error), the component renders
an explicit empty state — a bordered panel with
`t('heartbeats.timeline.empty')` ("No heartbeat history yet — events will appear
here after the next ping or missed run.") and
`t('heartbeats.timeline.emptyHint')`. This is required so a new/empty timeline
is not mistaken for a bug (there is no backfill; history accrues only from
ship-forward). Loading and error states mirror the detail page's existing
`loading`/`error` panels and use `t()` keys.

### 5.6 Detail-page integration

In `frontend/src/routes/heartbeats/[id]/+page.svelte`, add a timeline section
between the "Overview fields" card and the "Usage examples" block:

- Add state `let history = $state<HeartbeatHistoryResponse | null>(null)`,
  `historyLoading`, `historyError`.
- In `fetchData()` (or a sibling effect keyed on `heartbeatId`), call
  `getHeartbeatHistory(heartbeatId, from24hAgo, now)` and populate the state.
- Render `<HeartbeatTimeline ... />` inside a card titled
  `t('heartbeats.timeline.title')` ("Status Timeline (24h)").
- Realtime: the page already consumes `heartbeat_status` WS patches via
  `heartbeatStore`. For v1 the timeline is fetched on load and does **not**
  live-append WS events (the WS `heartbeat_status` message is a state patch, not
  a timeline event, and carries no `event_type`). A lightweight enhancement —
  re-fetch history when a `heartbeat_status` patch for this id arrives — is
  noted as optional in §9; v1 ships with on-load fetch + manual retry button.

### 5.7 i18n keys (mandatory, English first then all locales)

Add under the existing `heartbeats` object in
`frontend/src/locales/en.json` a `timeline` block, then replicate the **same
keys** into every other locale file (`ar, be, de, es, fr, it, ja, ko, pt, ru,
tr, zh`) using the English value as a placeholder:

```jsonc
"timeline": {
  "title": "Status Timeline (24h)",
  "uptime": "Uptime",
  "uptimeTooltip": "Share of successful check-ins over the selected window",
  "missed": "Missed runs",
  "legendSuccess": "Success",
  "legendMissed": "Missed",
  "legendFail": "Failed",
  "loading": "Loading history…",
  "loadFailed": "Failed to load heartbeat history",
  "empty": "No heartbeat history yet",
  "emptyHint": "Events will appear here after the next ping or missed run.",
  "eventSuccessAt": "Success at {time}",
  "eventMissedAt": "Missed run at {time}",
  "eventFailAt": "Explicit failure at {time}"
}
```

Run `cd frontend && pnpm validate-locales`. Per AGENTS.md, that script omits
`ar` and `be` from its list and only warns on missing keys, so verify `ar.json`
and `be.json` manually. All added JSON values are English placeholders; the keys
must be byte-identical across locales.

---

## 6. sqlc regeneration

Because migration 018 adds `heartbeats.history_retention_days`, and because the
heartbeat queries use `SELECT *` / `RETURNING *`, the generated model
`db.Heartbeat` and the projection rows (`MarkHeartbeatUpRow`,
`MarkHeartbeatFailRow`, etc.) gain the new field automatically after
regeneration. Steps:

1. Edit `backend/internal/store/postgres/queries/heartbeats.sql`:
   add `history_retention_days` to `CreateHeartbeat` and `UpsertHeartbeat`
   (INSERT column list + values; and the `UpsertHeartbeat` `DO UPDATE SET`).
2. `cd backend && sqlc generate`.
3. `sqlc diff` must be clean (no uncommitted drift).
4. Update the hand-written projection helpers in `ping.go`
   (`markUpRowToHeartbeat`, `markFailRowToHeartbeat`) to copy the new
   `HistoryRetentionDays` field, so the projected `db.Heartbeat` is complete.
5. `heartbeat_events` is **not** sqlc-managed — it is accessed only through the
   Timescale store (raw `pgx`), consistent with `check_results`.

---

## 7. Wiring (startup)

In `backend/cmd/pulse/main.go`:

- Pass the existing `timescaleStore` as the `timescale.HeartbeatEventWriter`
  (the interface declared in the `timescale` package, §2.2) into:
  - `heartbeat.NewEvaluator(...)` — the new trailing parameter (§2.2). In
    `main.go` this becomes
    `heartbeat.NewEvaluator(heartbeat.Config{...}, queries, heartbeatMetrics,
    wsHub, heartbeatDispatch, timescaleStore)`.
  - the ping handler construction path. `NewPingHandler` is constructed inside
    `api.NewRouter` (`router.go`), so add a field to `api.Deps` —
    `HeartbeatEventWriter timescale.HeartbeatEventWriter` — set it to
    `timescaleStore` in `main.go` (mirroring how `TimescaleStore` is already
    passed for monitor history), and append `deps.HeartbeatEventWriter` to the
    `handlers.NewPingHandler(...)` call in `router.go`.
  - Because the interface lives in `timescale`, no new `handlers`-typed field is
    needed and there is no import cycle.
- Register `HeartbeatHistoryHandler` in `router.go` gated on
  `deps.TimescaleStore != nil`.
- Retention already starts via `retentionSvc.Start(appCtx)`; the heartbeat
  pruning is inside the same cycle, so no new goroutine or env var is needed.

No new environment variables are introduced. (Retention interval reuses
`PULSE_RETENTION_CHECK_INTERVAL`; retention days are per-heartbeat in the DB.)
Therefore no `.env.example` / README / env-table changes are required —
confirmed against the AGENTS.md env-var checklist.

---

## 8. Out of scope (v1)

- **No backfill.** History starts accruing only from the moment the write-path
  ships. The empty state exists precisely because of this.
- **No list-page or dashboard timeline.** Detail page only.
- **No server-side `time_bucket` aggregation** for heartbeat events (frontend
  buckets for density). `step` is accepted/validated but inert.
- **No time-weighted uptime** (duration up vs. down). v1 uptime is the success
  share of recorded events.
- **No per-event WS streaming / live timeline append.** On-load fetch + manual
  retry; optional re-fetch-on-patch noted below.
- **No MCP tool / endpoint changes.**
- **No new env vars.**

---

## 9. Testability & verification

### Unit tests (no DB)
- `eventTypeForState` mapping (`up`→`success`, `down`→`fail`) in the ping
  handler; assert the sweep path only ever emits `missed`.
- `PingHandler.writeEvent` with a fake `HeartbeatEventWriter`: success/fail
  write the right type + `previous_state`; a nil writer is a no-op; a writer
  error is swallowed and logged, and the handler still returns `200`.
- Evaluator `markDown` with a fake writer: exactly one `missed` event when
  `MarkHeartbeatDown` updates a row; zero events when the guard matches no row
  (already-down). Reuse the existing evaluator fakes; the three existing
  `NewEvaluator` call sites in `evaluator_test.go` get the new trailing arg
  (`nil` for cases that don't assert the write, a recording fake for the one new
  case that does — see §2.2 call-site table).
- Ping handler `NewPingHandler` call in `ping_test.go` gets the new trailing arg
  (`nil` for existing cases; a recording fake for the new success/fail-write
  assertions).
- Handler param parsing: `from`/`to` defaults, invalid timestamps, `from>=to`,
  invalid `step`, bad UUID, retention clamping (reuse `retentionClamp`-style
  table tests from `history_property_test.go`).
- Summary math: `uptime_percent = success_count / total_events * 100`; `null`
  when `total_events == 0`; and assert the invariant `success_count +
  fail_count + missed_count == total_events` (Finding 4 / §4.3), including that
  both `fail` and `missed` reduce uptime equally.
- Frontend (Vitest + @testing-library/svelte + fast-check): `HeartbeatTimeline`
  renders empty state at zero events; renders success/missed/fail markers;
  buckets above threshold picks worst-state color; popover lists failures; all
  visible text comes from `t()` (assert no literal strings leak).

### Integration tests (TimescaleDB, gated by `PULSE_RUN_DB_TESTS=1`)
Only after a `pg_isready -h localhost -p 5432 -t 3` preflight (DB via
`make dev-local`). Add to `backend/internal/store/timescale/` tests:
- `WriteHeartbeatEvent` then `QueryHeartbeatEvents` round-trips rows in window,
  ascending, respecting `[start, end)`.
- `CountHeartbeatEventsByType` returns correct per-type counts.
- Retention: insert events older than `history_retention_days`, run a cycle,
  assert expired rows are pruned and in-window rows remain.
When no DB is reachable, run the suite **without** the flag (tests skip) and
state that integration tests were skipped — never claim they passed.

### CI checks before finishing (mirror `pull_request_opened.yml`)
- `backend/**`: `cd backend && go test -race -count=1 ./... && go build -o
  /dev/null ./cmd/pulse`.
- SQL/migration: `cd backend && sqlc generate` then confirm `sqlc diff` clean.
- `frontend/**`: `cd frontend && pnpm install && pnpm check &&
  pnpm validate-locales && pnpm test && pnpm build` (+ manual `ar`/`be` key
  check).
- `backend/api/openapi.yaml`: handler and spec in sync in the same change.

### Minor open choices (defaults chosen; reviewer may override)
- Reuse `pulse_retention_rows_deleted_total` for heartbeat rows (default) vs. a
  labeled/separate counter. Default avoids metric churn.
- Keep `step` accepted-but-inert (default) vs. drop it from the endpoint.
  Default keeps the two history endpoints parallel.
- Optional: re-fetch heartbeat history when a `heartbeat_status` WS patch for
  the open heartbeat arrives (cheap live-ish refresh). Deferred from v1.

---

## 10. Unverified assumptions (called out explicitly)

1. **`*timescale.Store` can satisfy the `HeartbeatEventWriter` interface by
   method set** once `WriteHeartbeatEvent` is added — assumed from the existing
   `WriteCheckResult` pattern; verified structurally, not yet compiled.
2. **`heartbeat.Evaluator` can take an extra nilable dependency** — confirmed by
   the design review against `evaluator.go`: the evaluator already injects narrow
   `Queries`/`Dispatcher` interfaces, and the three `NewEvaluator` call sites in
   `evaluator_test.go` plus the `main.go` site are enumerated in §2.2 for the
   trailing-arg update.
3. **`afterTransition` is the single ping side-effect site** for both success
   and fail — confirmed by the review against `ping.go` (`Success` passes `"up"`,
   `Fail` passes `"down"`); the write lives there once (§2.3), `eventTypeForState`
   is the single mapping, and `missed` is sweep-only.
4. **The reference `StatusTimeline.svelte` is the correct visual model** and its
   hardcoded colors/strings are legacy, not the standard — assumed from AGENTS.md
   conventions; the new component deliberately diverges to comply.
5. **`sqlc` regeneration will only add the new column** and not reshape existing
   generated code — assumed from the additive migration; must be confirmed with
   `sqlc diff` during implementation.
6. **No existing MCP tool consumes `/heartbeats/*`** — confirmed against the
   AGENTS.md co-change list (monitor/incident/tag/dashboard only); re-confirm by
   grepping `mcp/internal/` during implementation.

---

## 11. Responses to design review

This revision addresses the review at
`.agents/tasks/heartbeat-timeline/design-review.md` (verdict:
`CHANGES_REQUESTED`, 3 MEDIUM + 3 NIT). Every finding was re-checked against the
real source in the worktree before writing. All six are **addressed**.

- **Finding 1 (MEDIUM) — constructor signature changes break callers/tests.**
  Addressed in §2.2. The writer is injected as a **trailing positional
  parameter** on both `NewPingHandler` and `NewEvaluator`; the exact new
  signatures are given, the interface is declared once in the `timescale`
  package to avoid an import cycle, and **every** call site is enumerated in a
  table — `router.go`, `main.go`, `ping_test.go` (1), `evaluator_test.go` (3) —
  with `nil` passed where no writer is wired. Verified the call sites by grep;
  they match the review exactly.

- **Finding 2 (MEDIUM) — retention request/validation/params threading.**
  Addressed in §3.2 with a 7-step end-to-end thread: request DTO field, a
  `validateHeartbeatRetentionDays` helper (nil→30, else 1–365), its invocation
  right after `validateHeartbeatFields` in `Create`/`Put`, carrying the value
  into both `CreateHeartbeatParams` and `UpsertHeartbeatParams`, the sqlc query
  edits (INSERT list **and** `ON CONFLICT DO UPDATE SET` for PUT), and the
  response-DTO population. One deliberate correction vs. the review's suggested
  text: the error code is `INVALID_RETENTION_PERIOD` (not `VALIDATION_ERROR`),
  because that is what the real monitor path (`monitors.go`) returns — verified
  by grep. The finding's intent is fully met and the behavior now matches the
  monitor resource exactly.

- **Finding 3 (MEDIUM) — `afterTransition` dual-use ambiguity.** Addressed in
  §2.3. The design now states a single-source rule: the ping event write happens
  **only** on the first line of `afterTransition` (the one side-effect site both
  `Success` and `Fail` route through), `Fail` must **not** write directly, and
  `eventTypeForState` is the single mapping (`up→success`, `down→fail`) with an
  explicit `default` branch that logs and never emits `missed` or an invalid
  type. The tangled prose was removed.

- **Finding 4 (NIT) — summary invariant / uptime equivalence.** Addressed in
  §4.3: stated `success_count + fail_count + missed_count == total_events`
  (CHECK-enforced) and that `uptime_percent` counts only `success` as "up" while
  `fail` and `missed` reduce it equally. A summary-math unit test asserting the
  invariant is added to §9.

- **Finding 5 (NIT) — retention paging loop shown only partially.** Addressed in
  §3.1: the heartbeat loop is now specified as a structural copy of the monitor
  loop — same `ListHeartbeatsParams{Limit, Offset}` paging, same `len(page) <
  batchSize` termination, same `maxConsecutiveBatchErrors` tolerance, same
  per-heartbeat `continue`-on-error isolation — differing only in the delete
  query and the per-heartbeat `HistoryRetentionDays`.

- **Finding 6 (NIT) — `create_hypertable` extension guard comment.** Addressed in
  §1.2: added a comment in the 018 up migration noting the `timescaledb`
  extension is created in migration 004 (which runs earlier) and is intentionally
  not repeated.
