# Design Review — Heartbeat Historical Event Timeline

Reviewer: fresh-context design-review subagent (no prior authoring context).
Scope reviewed: `.agents/tasks/heartbeat-timeline/design.md` (full, §0–§11).
Method: every structural claim about the existing codebase was re-read against
the real source in the worktree at
`/Users/vandrushkevich/workdir/personal/pulse/.worktrees/heartbeat-timeline`.

**Verdict: APPROVED** — 0 HIGH, 0 MEDIUM, 6 NIT.

This is an unusually well-grounded design. It is a second revision that already
absorbed a prior review (its §11 responses are accurate against the source). I
independently verified the write-path integration points, the retention wiring,
the read-endpoint contract, the OpenAPI/MCP obligations, and the frontend
mirroring. Every load-bearing assumption checked out. The remaining findings are
all cosmetic or documentation-level and do not block implementation.

---

## Verified Assumptions

Each was confirmed by reading the cited source, not by trusting the design text.

1. **`afterTransition` is the single ping side-effect site for both success and
   fail.** `ping.go`: `Success` calls `afterTransition(c, row, "up",
   updated.PreviousState)` and `Fail` calls `afterTransition(c, row, "down",
   updated.PreviousState)`. Placing the one write on the first line of
   `afterTransition` covers both paths with a single call, and `Fail` must not
   write separately — exactly as §2.3 specifies. **Confirmed.**

2. **`MarkHeartbeatUp` / `MarkHeartbeatFail` return the prior state atomically.**
   `heartbeats.sql`: both use a `WITH prev AS (SELECT state ... FOR UPDATE)` CTE
   and `RETURNING h.*, prev.previous_state`, so the generated Row types carry
   `PreviousState`. §2.3's use of `updated.PreviousState` is valid. **Confirmed.**

3. **The evaluator's `markDown` captures `previousState := row.State` before
   `MarkHeartbeatDown`, and its side effects run only when a row was actually
   updated.** `evaluator.go`: `MarkHeartbeatDown`'s `WHERE id=$1 AND state='up'`
   guard makes the transition fire exactly once; §2.4's "exactly one `missed`
   row per episode" claim holds. The sweep's `previous_state` is always `"up"`.
   **Confirmed.**

4. **`MarkHeartbeatDown` returns a plain `db.Heartbeat` (no `PreviousState`
   field), unlike the ping queries.** §2.4's code uses `updated.ID` and the
   locally-captured `previousState` (not `updated.PreviousState`), which is the
   correct shape. **Confirmed** (the design does not confuse the two Row types).

5. **Heartbeat `state` CHECK is `('up','down','paused','new')`.** `017_heartbeats.up.sql`.
   The new `previous_state` column CHECK in §1.2 uses the identical enum, so any
   observed prior state (including `paused`/`new` from a paused heartbeat
   pinging success) satisfies the constraint. **Confirmed.**

6. **The monitor path validates retention with `validateRetentionDays`
   (nil→30, else [1,365]) and returns `400 INVALID_RETENTION_PERIOD`.**
   `monitors.go` `Create`/`Put` both call `apiError(c, 400,
   "INVALID_RETENTION_PERIOD", err.Error())`. §3.2's deliberate correction (use
   `INVALID_RETENTION_PERIOD`, not the earlier review's `VALIDATION_ERROR`) is
   right and matches the real convention. **Confirmed.**

7. **`validateHeartbeatFields` signature.** `heartbeats.go`:
   `func validateHeartbeatFields(name string, intervalPtr, gracePtr *int32,
   statusPtr *string) (interval, grace int32, status string, errMsg string)` —
   it returns no retention value, so a separate retention validator threaded
   alongside it (§3.2) is the correct shape. The handlers build
   `db.CreateHeartbeatParams` / `db.UpsertHeartbeatParams` with no retention
   field today. **Confirmed.**

8. **The retention service structure §3.1 copies.** `retention/service.go`:
   `runCycle` pages `ListMonitors{Limit, Offset}`, terminates on `len(page) <
   batchSize`, uses `maxConsecutiveBatchErrors = 3`, and on a per-item delete
   error logs and `continue`s. `deleteExpiredRows` uses raw `pool.Exec` with a
   `DELETE ... WHERE id IN (SELECT ... LIMIT $3)` capped at `deleteLimit` (10k).
   §3.1's heartbeat loop mirrors all of this exactly. **Confirmed.**

9. **`ListHeartbeats` signature.** `heartbeats.sql`: `ListHeartbeats :many ...
   LIMIT $1 OFFSET $2` → `db.ListHeartbeatsParams{Limit, Offset}`, matching
   §3.1's paging claim. **Confirmed.**

10. **Monitor history handler patterns §4 mirrors.** `history.go`: `INVALID_ID`,
    `NOT_FOUND` ("monitor not found"), `VALIDATION_ERROR` for bad `from`/`to`
    and `from>=to`, `INVALID_STEP` for `step` outside [60, 86400], 24h default
    window, `retentionClamp`, and `make([]..., 0, n)` slice init (never null).
    §4.2–§4.4 reproduce these faithfully. **Confirmed.**

11. **Router `Deps`, gating, and ping-handler call site.** `router.go`: `Deps`
    has `TimescaleStore *timescale.Store`; the history handler is registered
    under `if deps.TimescaleStore != nil`; and line 166 is literally
    `handlers.NewPingHandler(deps.Queries, deps.Hub, deps.HeartbeatMetrics,
    deps.PingDispatcher)`. §2.2's call-site table and §4.1/§7 gating match.
    **Confirmed.**

12. **`main.go` evaluator construction and `timescaleStore` availability.**
    `main.go`: `heartbeat.NewEvaluator(heartbeat.Config{Workers:...}, queries,
    heartbeatMetrics, wsHub, heartbeatDispatch)` and `timescaleStore :=
    timescale.New(pool)` exists (and is already passed as `Deps.TimescaleStore`).
    Appending `timescaleStore` as the trailing writer arg (§2.2/§7) is feasible.
    **Confirmed.**

13. **Test call-site enumeration.** `evaluator_test.go` has `NewEvaluator` at
    exactly lines 150, 202, 243 (§2.2 said "~150, 202, 243" — exact). `ping_test.go`
    has exactly one `NewPingHandler` call, at line 40:
    `handlers.NewPingHandler(db.New(fdb), nil, metrics, disp)`. §2.2's table is
    accurate. **Confirmed.**

14. **WS `heartbeat_status` carries no `event_type`.** `hub/messages.go`:
    `HeartbeatStatusPayload` is `{heartbeat_id, state, last_success_at?,
    next_expected_deadline?, timestamp}`. §5.6's claim that the WS patch is a
    state patch (not a timeline event) and cannot drive live-append is correct.
    **Confirmed.**

15. **Reference `StatusTimeline.svelte` really is legacy-styled.**
    `StatusTimeline.svelte`: hardcoded `bg-emerald-500` / `bg-rose-500` /
    `bg-amber-500` and literal English ("Uptime", "Healthy", "Down", "Mixed",
    "No check data available...", popover "+ N more" capped at 5). §5.3–§5.5's
    "deliberate divergence to comply with AGENTS.md" is justified, and the
    uptime thresholds (≥99 / ≥95) and popover cap it mirrors are real.
    **Confirmed.**

16. **Frontend detail page shape §5.6 edits.** `routes/heartbeats/[id]/+page.svelte`
    exists; order is Header → "Overview fields" card → `<PingExamples>` (Usage
    examples) → `<HeartbeatBindings>`; it has `fetchData()` and an `$effect`
    keyed on `heartbeatId`. Inserting the timeline card between the overview
    card and `PingExamples` (§5.6) is valid. **Confirmed.**

17. **`Heartbeat` TS interface lacks `history_retention_days`.** `types.ts`: the
    `Heartbeat` interface (line 175+) has no retention field (the line-65 one
    belongs to `Monitor`). §5.1 correctly adds it. **Confirmed.**

18. **No `heartbeats.timeline` i18n key collision.** `en.json`: the existing
    `timeline` block (line ~214) is nested under the **monitors** detail section
    (siblings `responseTime`, `configuration`, `incidents`); the `heartbeats`
    object begins at line ~814. §5.7's new `heartbeats.timeline` block does not
    collide. **Confirmed.**

19. **`apiRequest<T>` helper exists** (`api.ts` line 130) and is the house
    pattern used by `getMonitor`/history calls, so §5.2's
    `getHeartbeatHistory` is idiomatic. **Confirmed.**

20. **MCP has no heartbeat consumer.** The new route is not in the AGENTS.md MCP
    co-change list (monitor/incident/tag/dashboard only), so §4.5's "no MCP
    changes" is correct. (The design also tells the implementer to re-grep
    `mcp/internal/` to re-confirm — good hygiene.) **Confirmed.**

### Approved decisions honored

- Storage = new hypertable mirroring `check_results` ("option A"). ✔ (§1.1)
- Retention needed, per-heartbeat, reusing the existing sweep service. ✔ (§1.3, §3)
- Clear, visible status identification in the reference UI. ✔ (§5.3 color-coded
  three-way grammar + legend + uptime threshold).
- Scope = visual timeline on the heartbeat detail page only. ✔ (§5.6, §8)

### AGENTS.md conventions honored

- sqlc never hand-edited; `heartbeat_events` is timescale-only raw `pgx`, not
  sqlc. ✔ (§2.1, §6.5)
- Migration 018 up **and** down, extension not re-declared (created in 004). ✔
  (§1.2, §1.3)
- `/api/v1` prefix, standard error envelope, `X-Request-ID` via middleware. ✔
  (§4.2)
- OpenAPI updated in the same change; schemas + enums enumerated. ✔ (§4.5)
- i18n across all 13 locales with English placeholders; manual `ar`/`be` check
  flagged. ✔ (§5.7)
- CSS custom properties + `[data-theme="dark"]`, never `dark:`. ✔ (§5.3)

---

## Unverified / Wrong Assumptions

None of the design's load-bearing assumptions were found wrong. Two of its own
§10 "unverified" items are inherently implementation-time (`sqlc diff` must be
clean after regeneration; `*timescale.Store` satisfying the interface compiles
only after the method is added) — those are correctly flagged as
"confirm during implementation" and are not design defects.

One residual item the design cannot fully settle on paper and the implementer
must verify at build time:

- **sqlc regeneration is purely additive.** §6 assumes adding
  `heartbeats.history_retention_days` only appends a field to `db.Heartbeat`
  and the `MarkHeartbeat*Row` projections. This is very likely (additive column,
  `SELECT *`/`RETURNING *`), but it is a generated-code assumption — the gate is
  `sqlc generate` + a clean `sqlc diff`, exactly as §6/§9 require. Not a blocker.

---

## Findings

### NIT 1 — `putHeartbeatRequest` is a type alias, so §3.2 step 1 overstates the edit
Location: §3.2 step 1 ("Add to `createHeartbeatRequest` and `putHeartbeatRequest`").
`heartbeats.go` declares `type putHeartbeatRequest = createHeartbeatRequest` (a
type **alias**, not a distinct struct). Adding the field to
`createHeartbeatRequest` automatically adds it to `putHeartbeatRequest`.
Fix: reword to "Add the field once to `createHeartbeatRequest`;
`putHeartbeatRequest` is an alias and inherits it." No code impact.

### NIT 2 — `strPtr` helper referenced in §2.4 but never defined
Location: §2.4 (`PreviousState: strPtr(previousState)`).
The evaluator package has no `strPtr` helper, and the ping-path `writeEvent`
(§2.3) uses an inline `var prevPtr *string; if prev != "" {...}` instead. The
two sites use different idioms for the same pointer conversion.
Fix: either define `func strPtr(s string) *string { return &s }` in the
`heartbeat` package, or inline the same `if previousState != ""` guard the ping
path uses. Since the sweep's `previousState` is always `"up"` (never empty), a
plain `&previousState` is also acceptable — pick one and state it.

### NIT 3 — ping-path `OccurredAt` is a fresh `time.Now()`, not the UPDATE's instant
Location: §2.3 `writeEvent` (`OccurredAt: time.Now().UTC()`).
The event timestamp is computed after the `MarkHeartbeatUp/Fail` UPDATE already
ran with its own `now`, so the event row's `occurred_at` is a few microseconds
later than the state row's `updated_at`/`last_success_at`. Harmless for a
timeline, but the design claims `occurred_at` "carries the real event instant"
(§1.2). Fix: either accept the tiny skew explicitly (one line), or thread the
handler's existing `now` (the `Success` path already computes `now :=
time.Now().UTC()`) into `writeEvent` so the event and the state share one
instant. Prefer threading `now` on the success path for exactness.

### NIT 4 — §3.2 step 7 DB-backstop error code is imprecise
Location: §3.2 step 7 ("mapped to a `400` by the existing check-constraint error
path").
Verified in `heartbeats.go` `Create`: the existing `isCheckConstraintErr(err)`
branch returns `400 VALIDATION_ERROR` with message "invalid heartbeat values
(ensure database migrations are applied)" — not `INVALID_RETENTION_PERIOD`. The
design only claims "mapped to a 400," which is true, but a reader may assume the
backstop yields the same `INVALID_RETENTION_PERIOD` code as the primary path.
Fix: add one clause noting the DB backstop surfaces the generic
`VALIDATION_ERROR` check-constraint message, while the primary handler gate
returns `INVALID_RETENTION_PERIOD`; both are 400s. (This only matters if the
primary gate is ever bypassed, which it is not for a well-formed request.)

### NIT 5 — bucket color precedence stated once; make the three-way ordering explicit in the summary too
Location: §5.3 ("a bucket's color is the worst event in it (missed > fail >
success)").
The reference component uses `up | down | mixed`; the heartbeat adaptation uses
`success | fail | missed` with `missed > fail > success`. The precedence is
stated for bucket color but the legend/summary mapping (which of `fail`/`missed`
is "worse" when both appear in the summary counts) is only implied. Fix: state
once, in §5.3, that the single-bucket worst-state ordering is
`missed > fail > success` and that the legend lists all three regardless of
presence, so the implementer does not reinvent a precedence. Cosmetic.

### NIT 6 — `getHeartbeatHistory` ignores the validated-but-inert `step`
Location: §5.2 (`getHeartbeatHistory(id, from, to)`).
The endpoint (§4.2) accepts and validates `step` for API symmetry but the client
helper omits it entirely. That is fine for v1 (step is inert), but §4.2 and §5.2
should agree that the frontend never sends `step`. Fix: add a one-line note in
§5.2 that the client intentionally omits `step` because v1 renders raw events;
server-side `step` support is reserved. Keeps the two sections from reading as a
contradiction.

---

## Summary

The design concretely specifies all three write-path integration points (ping
success and explicit fail funnel through the single `afterTransition` write with
one `eventTypeForState` mapping; the missed-run sweep writes exactly one
`missed` row at the guarded transition), the retention wiring (a structural copy
of the monitor loop in the same `runCycle`, with the per-heartbeat
`history_retention_days` threaded end to end through DTO → validator → sqlc
params → response), the `GET /api/v1/heartbeats/{id}/history` contract (params,
defaults, retention clamp, error codes, response shape, invariants, edge cases)
with matching OpenAPI and an explicit no-MCP determination, and a frontend
timeline that mirrors `StatusTimeline`'s visual language while deliberately
upgrading to CSS custom properties, `[data-theme="dark"]`, and `t()` i18n across
all locales, with a mandatory, explicit empty state.

Every structural assumption was independently verified against the worktree
source and held. The six findings are all NIT — wording precision and two small
code-idiom choices (a missing `strPtr` helper, and the event timestamp instant).
None change the architecture or block a correct implementation.

**0 HIGH, 0 MEDIUM ⇒ APPROVED.**
