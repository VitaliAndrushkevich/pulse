# Design Review — Heartbeat and Scheduled-Job Monitoring (PF-01, First Useful Version)

**Scope:** Fresh review of `design.md` against `requirements.md` and `AGENTS.md`. This is a planning-only review — no build/test was run. The real source files cited by the design were read to confirm the claims are grounded.

**Verdict:** APPROVED (0 HIGH, 0 MEDIUM; NITs recorded below, none blocking).

---

## Summary

The design is unusually well-grounded. It correctly models heartbeats as a parallel resource rather than a `monitors.type`, reuses the proven scheduler/dispatcher/hub/token primitives, and documents every deviation from `AGENTS.md` with a concrete, verified justification. Each of the 13 requirements maps to a concrete design section, and all non-goals are explicitly deferred (not planned). The two prior HIGH findings and two MEDIUM findings (recorded in the design's own "Review Responses (iteration 2)") have been addressed with mechanisms I independently verified against source:

- The `EvaluateAndDispatchWithPayload` injection point is real and necessary — `EvaluateAndDispatch` does build the payload internally via `buildPayload`, which never sets `Monitor.Name`, and `enrichPayload` does call `GetMonitor` when `Monitor.Name == ""`.
- The trigger-translation-at-call-site fix is correct and necessary — `StateTracker.Evaluate` emits `monitor_down`/`monitor_up` via a hardcoded switch, and the same `bindings` slice feeds both `extractTriggerConditions` and `bindingMatchesTrigger`, so untranslated heartbeat vocabulary would silently enqueue zero jobs.
- `delivery_logs.monitor_id` is `NOT NULL` with no FK; the design's requirement to set `DeliveryJob.MonitorID = heartbeatID` is correct and the audit-insert constraint is satisfied.
- The ping-write concurrency model (single atomic `UPDATE … RETURNING` prior state via a `FOR UPDATE` CTE) is sound for the stated idempotency guarantee.

---

## Requirement coverage

| Req | Addressed in | Notes |
|-----|--------------|-------|
| R1 CRUD | §3.1, §3.3, §3.4 | Mirrors `MonitorHandler`; `parsePagination`, error envelope, `INVALID_ID`, tags. |
| R2 Idempotent PUT | §2.2 (`UpsertHeartbeat`, `GetHeartbeatForUpdate`), §3.4 | `409 CONFLICT` on `40001/40P01/55P03`; PUT does not rotate token; deadline recompute on interval/grace change (R2.4). |
| R3 Ping ingestion | §4.1–§4.3 | Root-level, GET+POST, 404 dummy-compare, paused-ping accepted, non-blocking dispatch. |
| R4 Deadline evaluation | §5.1, §5.2 | Periodic sweep, partial index, once-per-episode guard, shutdown drain. |
| R5 Notifications | §5.3 | Reuses dispatcher/state-tracker/retry/reminders; dedicated bindings table; trigger translation. |
| R6 Scoped credential | §6 | Write-once raw token; SHA-256 at rest (justified deviation); rotate endpoint; admin separation. |
| R7 OpenAPI + MCP | §12 | OpenAPI co-change mandated; MCP evaluated — new resource, no existing tool touched. |
| R8 WebSocket | §7 | New `heartbeat_status` type, diff/patch, drop-on-full, dedicated store merge. |
| R9 Frontend + i18n | §9 | Routes/store/components mirror monitors; concrete `en.json` key set; validate-locales; RTL logical CSS. |
| R10 Performance | §10 | Bounded pool via `PULSE_HEARTBEAT_WORKERS`, batch 500, indexed lookup, non-blocking paths. |
| R11 Examples | §11 | Shell/cron/CI examples; independence/validity note surfaced in UI and docs. |
| R12 Security/logging | §4.2, §6, §12.x | Constant-time compare, dummy compare, sanitized logger, no secrets in labels/payloads. |
| R13 Metrics | §8 | `pulse_heartbeat_up`, `pulse_heartbeats_total`, `pulse_*` naming, no token labels. |

All 13 requirements are addressed. All 7 non-goals are listed as deferred and are not planned into the design.

---

## AGENTS.md conformance

- **API conventions:** `/api/v1` prefix, pagination (`page`/`limit`, clamp 100), error envelope `{error:{code,message}}`, `X-Request-ID` inherited from `requestIDMiddleware`, idempotent PUT — all present. ✔
- **Ping placement:** Root-level `/ping/:token[/fail]` registered on the engine like `/ws` (verified `/ws` is engine-level and `apiPrefixes = {"/api/","/ws","/metrics","/healthz","/swagger"}`); design extends `apiPrefixes` with `/ping`. Ingestion/admin separation is enforced structurally (never under `protected`, never sets `user_id`). ✔
- **Security — token storage:** The deviation from bcrypt to SHA-256 for ping tokens is explicit, scoped to ping tokens only, and justified on both performance (bcrypt ~60–100ms on an unauthenticated high-frequency path = CPU-exhaustion vector) and threat-model grounds (256-bit random secret has no dictionary to slow-guess). API tokens/passwords keep bcrypt. Write-once raw token exposure matches `TokenHandler.Create`. ✔ (deviation justified)
- **Performance 500+/bounded workers:** Periodic sweep modeled on `monitor.Scheduler` (`DefaultBatchSize = 500` verified), bounded pool, partial index. ✔
- **WebSocket protocol:** Dedicated `heartbeat_status` type, diff/patch merge, buffered drop-on-full hub, non-blocking broadcast. ✔
- **i18n mandate:** New `heartbeats` section added to `en.json` first then all 13 locales; `validate-locales` gate; no hardcoded strings; `[data-theme="dark"]` and CSS custom properties, no `dark:`. ✔
- **OpenAPI source-of-truth co-change:** Mandated in the same commit, all new paths/schemas enumerated, `make openapi` gate. ✔
- **MCP co-change rule:** Correctly evaluated — heartbeat endpoints are new resources absent from the co-change endpoint list and change no listed endpoint's shape, so no existing MCP tool/`PulseClient` method is affected; new MCP tool explicitly deferred. ✔

---

## Design-decision justifications (all present and sound)

- **Deadline-evaluation mechanism (periodic sweep vs LISTEN/NOTIFY vs per-heartbeat timers):** Justified in §5.1 and Alternatives. A missed run is the *absence* of a row change, so NOTIFY has no event to fire on; per-heartbeat timers violate bounded-pool. Sweep is correct. ✔
- **Notification binding (dedicated table vs generalizing `channel_bindings`):** Justified — `channel_bindings.monitor_id` has a `REFERENCES monitors(id)` FK (verified) that a heartbeat id would violate; the dispatcher key is already `uuid.UUID` and resource-agnostic. ✔
- **Token storage deviation:** Justified (above). ✔

---

## Findings

### NIT-1 — `ListActiveMonitorsDue` vs `ListActiveMonitorsDueWithTags` as the cited mirror
**Where:** §5.1 / §2.2 cite `ListActiveMonitorsDue` as the model for `ListDueHeartbeats`.
**Observation:** Both queries exist in `monitors.sql.go`, but the scheduler actually polls via `ListActiveMonitorsDueWithTags` (verified in `scheduler.go` `poll()`). The cited `ListActiveMonitorsDue` is a real, simpler analogue and is the better model for heartbeats (which do not need the tag join in the due-scan), so the citation is accurate and the choice is reasonable. No change required.
**Fix (optional):** Add a one-line note that heartbeats intentionally use the tag-free due query because the sweep does not need tags, unlike the monitor scheduler.

### NIT-2 — `MarkHeartbeatUp` RETURNING form shown two ways
**Where:** §2.2 `MarkHeartbeatUp`/`MarkHeartbeatFail`.
**Observation:** The first inline form `UPDATE … RETURNING (SELECT state FROM heartbeats WHERE id=$1) AS previous_state, *` would read the *post-update* state in the subquery and is not reliable. The design immediately corrects this to the CTE form (`WITH prev AS (SELECT state … FOR UPDATE) UPDATE … RETURNING (SELECT state FROM prev) …`), which is sound. The two forms side by side could confuse the implementer.
**Fix:** Delete the first (incorrect) inline form and keep only the CTE form as the specified query, so there is a single unambiguous SQL to implement.

### NIT-3 — Template vocabulary says "Monitor" for heartbeat alerts
**Where:** §5.3 "Template data" carried note.
**Observation:** Reusing `TemplateData.Monitor` and the existing email/webhook templates means heartbeat alerts render the label "Monitor". The design explicitly accepts this for v1 and defers a vocabulary pass. This is a conscious, documented tradeoff, not a defect. R5.8 (payload carries id/name/state/prev/timestamp/BaseURL) is still met.
**Fix (optional):** None required for v1; the deferral is acceptable and recorded.

### NIT-4 — `pulse_heartbeat_up` gauge update on explicit-fail ping path
**Where:** §8 (metrics) and §4.3 (ping processing).
**Observation:** §8 says gauge updates happen "at the same points as state transitions: success/recovery → 1, miss/explicit-fail → 0", and the ping handler is listed as holding `heartbeat.Metrics`. The evaluator path (§5.2) explicitly updates the gauge on `markDown`. The explicit-fail *ping* path (§4.3) describes the DB write, dispatch, and broadcast but does not explicitly restate the gauge-to-0 update. R13.3 requires it. It is implied by §8 and the handler holding `Metrics`, so this is a documentation gap, not a design gap.
**Fix (optional):** Add a half-sentence to §4.3's "Explicit failure" and "Success" bullets stating the handler sets `pulse_heartbeat_up` to 0/1 respectively, matching §8.

---

## Verified Assumptions

1. `token.Generate()` returns `(raw, prefix, hash, err)` and computes a bcrypt hash internally (`backend/internal/token/token.go`). The heartbeat path discarding the bcrypt return and the optional `GenerateRaw()` helper suggestion are both valid. `RawBytes = 32` (256-bit), `PrefixLen = 8`, base64url raw ≈ 43 chars — matches the design.
2. `EvaluateAndDispatch` builds the payload internally via `buildPayload(monitorID, result)`, which sets `Status/PreviousStatus/ResponseTime/Timestamp` but **never** `Monitor.Name` (`backend/internal/notification/fanout.go`). The injection-point rationale is correct.
3. `enrichPayload` runs `GetMonitor(job.MonitorID)` only when `job.Payload.Monitor.Name == ""` (`backend/internal/notification/dispatcher.go`). A pre-filled name skips the lookup, as the design states.
4. The same `bindings` slice feeds both `extractTriggerConditions → StateTracker.Evaluate` and `bindingMatchesTrigger` (`tc.Type == event.Type`) in `fanout.go`. Translating the whole `BindingWithChannel.Triggers` is required, as designed.
5. `StateTracker.Evaluate` emits events typed `monitor_down`/`monitor_up`/`degraded`/`ssl_expiring`/`n_failures_in_row` via a hardcoded switch (`backend/internal/notification/state_tracker.go`). Leaving it untouched and translating at the call site is correct.
6. `delivery_logs.monitor_id` is `UUID NOT NULL` with no FK (comment confirms "audit trail persists after monitor deletion"); `binding_id` is nullable (`backend/migrations/014_notification_channels.up.sql`). Setting `MonitorID = heartbeatID` satisfies the constraint.
7. `channel_bindings.monitor_id` is `NOT NULL REFERENCES monitors(id) ON DELETE CASCADE` with `uq_channel_monitor` unique constraint — a heartbeat id would violate the FK, so a dedicated `heartbeat_bindings` table is justified.
8. `/ws` is registered directly on the gin engine (`r.GET("/ws", …)`), and `apiPrefixes = {"/api/","/ws","/metrics","/healthz","/swagger"}` with `NoRoute(spaHandler)` as the SPA catch-all (`backend/internal/api/router.go`). Root-level ping placement and the `apiPrefixes` extension are correctly modeled.
9. The dummy-compare timing-safe pattern exists in `combinedAuth` (`router.go`), `middleware/auth.go` (`failWithDummyCompare`), and `ws.go` (`wsDummyHash`). The ping path's SHA-256 dummy-compare mirrors this intent.
10. `ListAPITokensByPrefix` uses `WHERE prefix = $1` (`api_tokens.sql.go`); the column is `prefix`, which the design acknowledges (NIT-1 of its own iteration) and names the heartbeat column `token_prefix` for clarity.
11. `monitor.Scheduler` uses `DefaultBatchSize = 500`, `DefaultTickInterval = 1s`, bounded pool, and `ListActiveMonitorsDueWithTags` in `poll()` (`scheduler.go`). The sweep model is grounded.
12. `GetMonitorForUpdate` is a `SELECT … FOR UPDATE` row lock used by `MonitorHandler.Put` with `409 CONFLICT` mapping (`monitors.sql.go`, `monitors.go`). The idempotent-PUT mirror is accurate.
13. Monitors use `status IN ('active','paused')` and `state IN ('up','down','unknown')` plus `idx_monitors_next_check_at` (partial `WHERE status='active'`) and `idx_monitors_status_created_at` (`001_initial.up.sql`). The heartbeat schema's two-field split and index design correctly mirror this, with `new` substituted for `unknown` and documented.

## Unverified / Wrong Assumptions

None found. Every source-grounded claim checked was accurate. The only items of note are the NITs above (one incorrect-then-corrected SQL form, two documentation gaps, and one citation refinement), none of which rise to a correctness defect that blocks implementation.
