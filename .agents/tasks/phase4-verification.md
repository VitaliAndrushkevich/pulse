# Phase 4 Verification — Notifications integration

Scope: tasks 16–19 of `.kiro/specs/heartbeat-monitoring/tasks.md`. Implemented from scratch
(no `phase4-review.json` present → not a revision).

## Approved decisions honored
- Reuse the existing dispatcher, StateTracker, retry queue, delivery_logs, and reminders unchanged.
- Heartbeat triggers are translated to `monitor_down` / `monitor_up` **before** dispatch, on the
  whole `BindingWithChannel.Triggers` slice (HIGH-2), so both `extractTriggerConditions`/`Evaluate`
  and `bindingMatchesTrigger` line up.
- The payload builder pre-fills `Monitor.Name`, `BaseURL`, and `Timestamp`, so `enrichPayload`
  skips its `GetMonitor` lookup for heartbeat ids (HIGH-1 regression guard).
- `DeliveryJob.MonitorID = heartbeatID` and `BindingID = heartbeatBindingID`, satisfying
  `delivery_logs.monitor_id NOT NULL` with no FK (MEDIUM-1).

## Files added
- `backend/internal/heartbeat/dispatch.go` — thin heartbeat dispatch entry point. Implements both
  the evaluator `Dispatcher` seam (`DispatchMissedRun`) and the ping handler `PingDispatcher` seam
  (`DispatchHeartbeat`), delegating to a shared context-based `dispatch`. Loads bindings via
  `ListBindingsByHeartbeat`, decodes the stored triggers JSONB, translates them, builds the
  pre-filled `heartbeatPayloadBuilder`, and calls `EvaluateAndDispatchWithPayload`.
- `backend/internal/notification/heartbeat_triggers_test.go` — table-driven `ValidateHeartbeatTriggers`
  and `TranslateHeartbeatTriggers` mapping tests.
- `backend/internal/heartbeat/dispatch_test.go` — once-per-episode `monitor_down`, once-per-transition
  `monitor_up` via a reused real `StateTracker` keyed by the heartbeat UUID; per-heartbeat dedup
  isolation; jobs carry a non-empty `Monitor.Name`; no-bindings and nil-dispatcher no-ops.

## Files modified
- `backend/internal/notification/fanout.go` — added `EvaluateAndDispatchWithPayload`
  (payload-injecting sibling); refactored `EvaluateAndDispatch` to delegate to it with the existing
  `buildPayload`. Monitor behavior is byte-for-byte identical (same `buildPayload`, same resource key
  as `MonitorID`, same fan-out). `StateTracker.Evaluate` left untouched.
- `backend/internal/notification/heartbeat_triggers.go` — exported the existing (Phase 3, unused)
  `translateHeartbeatTriggers` as `TranslateHeartbeatTriggers` so the cross-package dispatch helper
  can call it. Validation/mapping logic unchanged.
- `backend/cmd/pulse/main.go` — constructed `heartbeat.NewDispatch(queries, dispatcher, cfg.BaseURL)`
  and wired it into both the evaluator (`DispatchMissedRun` seam) and the router
  (`PingDispatcher`), replacing the Phase 3 `nil` placeholders.

## Monitor-behavior-unaffected proof
- `EvaluateAndDispatch` now has one line: `d.EvaluateAndDispatchWithPayload(ctx, monitorID, result,
  bindings, buildPayload)`. The job fields, trigger evaluation, dedup key, and payload are produced
  by the identical code paths as before.
- The entire existing `internal/notification` test suite (`fanout_test.go`, `dispatcher_test.go`,
  `state_tracker_test.go`, `retry_test.go`, `delivery_log_test.go`, `shutdown_test.go`,
  `validation_test.go`, reminders, smtp, webhook) passes **unchanged** under `-race -count=1`.

## Commands run
Preflight: `nc -z -w3 localhost 5432` → succeeded (Postgres reachable).

- `make -C .worktrees/heartbeat build` → exit 0 (builds `./cmd/pulse`).
- `make -C .worktrees/heartbeat test` → `go test ./...` all packages `ok`, exit 0.
- `go test -race -count=1 ./internal/notification/... ./internal/heartbeat/...` → PASS, no data races.
- `go build ./...` → exit 0.

## Database availability
Postgres was reachable on `localhost:5432`. The Phase 4 tests themselves are pure unit tests (fakes
for queries and the notification dispatcher) and require no database. DB-gated integration tests
(`PULSE_RUN_DB_TESTS`) were NOT force-run in Phase 4; `make test` runs them in skip mode, so no
integration-style heartbeat DB assertions were executed in this phase — the Phase 1/3 store and
evaluator integration tests already cover the DB paths.

## Scope / git hygiene
- Only feature files staged. The pre-existing working-tree edits to `AGENTS.md`, `README.md`, and
  `docs/API.md` (staged roadmap/doc changes carried on this branch) were intentionally left
  untracked/unstaged per the user's instruction and belong to the documentation phase (Phase 8).
