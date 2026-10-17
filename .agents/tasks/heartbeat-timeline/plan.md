# Implementation Plan — Heartbeat Historical Event Timeline

Source of truth: the APPROVED design at `.agents/tasks/heartbeat-timeline/design.md`
(review: `.agents/tasks/heartbeat-timeline/design-review.md`, verdict APPROVED,
0 HIGH / 0 MEDIUM / 6 NIT). This plan sequences that design faithfully; it does
not re-decide architecture. All paths are absolute from the worktree root
`/Users/vandrushkevich/workdir/personal/pulse/.worktrees/heartbeat-timeline`.

Branch: `feat/heartbeat-timeline` (based off `feat/heartbeat-monitoring`). Work
inside the worktree only; relative paths land in the parent repo.

The design's 6 NIT findings are folded into the relevant items below:
- NIT 1: `putHeartbeatRequest` is a type alias of `createHeartbeatRequest` — add
  the field once (item 5).
- NIT 2: the sweep's `previousState` is always `"up"`, so pass `&previousState`
  directly; the ping path uses the `if prev != ""` guard (items 3, 4).
- NIT 3: ping-path event uses a fresh `time.Now().UTC()`; the tiny skew vs. the
  UPDATE instant is accepted (item 3).
- NIT 4: the DB CHECK backstop surfaces `VALIDATION_ERROR`; the primary handler
  gate returns `INVALID_RETENTION_PERIOD` (item 5).
- NIT 5: single-bucket worst-state ordering is `missed > fail > success`; the
  legend always lists all three (item 9).
- NIT 6: the frontend client omits `step` deliberately; v1 renders raw events
  (item 8).

Dependency order: 1 (migration) → 2 (store API) → 3,4 (write-path wiring) →
5 (retention field + sqlc) → 6 (retention pruning) → 7 (read endpoint) →
8 (OpenAPI + MCP check) → 9 (frontend). Each item leaves the tree buildable.

---

- [ ] 1. Create migration 018 for the `heartbeat_events` hypertable plus the
      per-heartbeat `history_retention_days` column on `heartbeats`.
      Up migration: `CREATE TABLE heartbeat_events` with columns `id UUID NOT NULL
      DEFAULT gen_random_uuid()`, `heartbeat_id UUID NOT NULL REFERENCES
      heartbeats(id) ON DELETE CASCADE`, `occurred_at TIMESTAMPTZ NOT NULL`,
      `event_type TEXT NOT NULL CHECK (event_type IN ('success','fail','missed'))`,
      `previous_state TEXT CHECK (previous_state IN ('up','down','paused','new'))`,
      `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `PRIMARY KEY (id,
      occurred_at)`; index `idx_heartbeat_events_hb_id_occurred_at ON
      heartbeat_events (heartbeat_id, occurred_at DESC)`; `SELECT
      create_hypertable('heartbeat_events','occurred_at', if_not_exists => TRUE)`
      with a comment noting the timescaledb extension is created in migration 004
      (do NOT re-declare it). Also `ALTER TABLE heartbeats ADD COLUMN
      history_retention_days INTEGER NOT NULL DEFAULT 30 CONSTRAINT
      chk_heartbeat_retention_range CHECK (history_retention_days >= 1 AND
      history_retention_days <= 365)` (mirrors migration 011). Down migration:
      `ALTER TABLE heartbeats DROP COLUMN IF EXISTS history_retention_days;` then
      `DROP TABLE IF EXISTS heartbeat_events;`.
      Files: `backend/migrations/018_heartbeat_events.up.sql`,
      `backend/migrations/018_heartbeat_events.down.sql`
      Verify: `cd backend && go build -o /dev/null ./cmd/pulse` compiles (migrations
      are embedded via `migrations.go`). If Postgres is reachable
      (`pg_isready -h localhost -p 5432 -t 3`), startup auto-applies migrations
      cleanly; otherwise note DB apply was skipped.

- [ ] 2. Add the heartbeat-event type, the `HeartbeatEventWriter` interface, and
      the three store methods to the Timescale store, mirroring
      `CheckPoint`/`WriteCheckResult`/`QueryHistory`. Declare `type HeartbeatEvent
      struct { HeartbeatID uuid.UUID; EventType string; PreviousState *string;
      OccurredAt time.Time }` and `type HeartbeatEventWriter interface {
      WriteHeartbeatEvent(ctx context.Context, ev HeartbeatEvent) error }` in the
      `timescale` package (neutral location both `handlers` and `heartbeat`
      import, avoiding an import cycle). Add methods on `*Store`:
      `WriteHeartbeatEvent` (single parameterized INSERT into
      `heartbeat_events (heartbeat_id, occurred_at, event_type, previous_state)`),
      `QueryHeartbeatEvents(ctx, heartbeatID, start, end)` (SELECT `heartbeat_id,
      event_type, previous_state, occurred_at WHERE heartbeat_id=$1 AND occurred_at
      >= $2 AND occurred_at < $3 ORDER BY occurred_at ASC`, slice init
      `make([]HeartbeatEvent, 0)`), and `CountHeartbeatEventsByType(ctx,
      heartbeatID, start, end)` (`SELECT event_type, COUNT(*) ... GROUP BY
      event_type`, returns `map[string]int64`). No `time_bucket` method (design
      §2.1).
      Files: `backend/internal/store/timescale/store.go`
      Verify: `cd backend && go build -o /dev/null ./cmd/pulse` compiles. Add unit
      coverage behind the DB-gated suite in item 7's verification; `*timescale.Store`
      must satisfy `timescale.HeartbeatEventWriter` by method set.

- [ ] 3. Wire the ping-path event write (success + explicit fail) through the
      single `afterTransition` site. Add field `events timescale.HeartbeatEventWriter`
      to `PingHandler`; change `NewPingHandler` to take it as a new TRAILING
      positional parameter (`events timescale.HeartbeatEventWriter`); a nil writer
      is valid and skips the write. Add `eventTypeForState(state string) string`
      (the single ping state→event mapping: `up`→`success`, `down`→`fail`,
      `default` logs and returns `fail`; it never emits `missed`). Add a private
      `writeEvent(ctx, id, eventType, prev string)` helper that no-ops on nil
      writer, builds `prevPtr` only when `prev != ""`, uses `OccurredAt:
      time.Now().UTC()`, and logs (never fails the ping) on error. Call
      `h.writeEvent(c.Request.Context(), row.ID, eventTypeForState(newState),
      previousState)` on the FIRST line of `afterTransition`. Do NOT add a second
      write in `Fail` (both `Success` and `Fail` funnel through `afterTransition`).
      Update the sole `NewPingHandler` call in `ping_test.go` (line ~40) to pass a
      trailing arg (`nil`, plus a recording fake for the new write assertions).
      Files: `backend/internal/api/handlers/ping.go`,
      `backend/internal/api/handlers/ping_test.go`
      Verify: `cd backend && go test -race -count=1 ./internal/api/handlers/...`
      passes, including new tests asserting success→`success`, fail→`fail`, nil
      writer no-op, and a swallowed writer error still returns 200.

- [ ] 4. Wire the missed-run sweep event write in the evaluator. Add field `events
      timescale.HeartbeatEventWriter` to `Evaluator`; change `NewEvaluator` to take
      it as a new TRAILING positional parameter; nil is valid. In `markDown`, after
      a successful `MarkHeartbeatDown` (the `WHERE state='up'` guard fires once per
      episode) and alongside the existing metric/broadcast/dispatch block, write
      exactly one `missed` event: `e.events.WriteHeartbeatEvent(ctx,
      timescale.HeartbeatEvent{HeartbeatID: updated.ID, EventType: "missed",
      PreviousState: &previousState, OccurredAt: time.Now().UTC()})` guarded by
      `if e.events != nil`, logging on error. `previousState` is the locally
      captured `row.State` (always `"up"`). Update the three `NewEvaluator` call
      sites in `evaluator_test.go` (~lines 150, 202, 243) to pass a trailing arg
      (`nil`, plus a recording fake for the new "exactly one missed on transition,
      zero when guard matches no row" test).
      Files: `backend/internal/heartbeat/evaluator.go`,
      `backend/internal/heartbeat/evaluator_test.go`
      Verify: `cd backend && go test -race -count=1 ./internal/heartbeat/...`
      passes, including the new missed-write assertions.

- [ ] 5. Thread per-heartbeat retention end-to-end through the API resource and
      sqlc. In `heartbeats.go`: add `HistoryRetentionDays *int32
      \`json:"history_retention_days,omitempty"\`` to `createHeartbeatRequest`
      (ONCE — `putHeartbeatRequest` is a type alias and inherits it); add
      `validateHeartbeatRetentionDays(days *int32) (int32, error)` (nil→30, else
      [1,365]); invoke it right after `validateHeartbeatFields` in both `Create`
      and `Put`, returning `apiError(c, 400, "INVALID_RETENTION_PERIOD",
      rerr.Error())` on failure; pass the validated value into
      `db.CreateHeartbeatParams{...HistoryRetentionDays: retentionDays}` and
      `db.UpsertHeartbeatParams{...HistoryRetentionDays: retentionDays}`; add
      `HistoryRetentionDays int32 \`json:"history_retention_days"\`` to
      `heartbeatResponse` and set it in `toHeartbeatResponse`. In
      `backend/internal/store/postgres/queries/heartbeats.sql`: add
      `history_retention_days` to `CreateHeartbeat` (INSERT columns + value) and
      `UpsertHeartbeat` (INSERT columns + value AND the `ON CONFLICT DO UPDATE SET
      history_retention_days = EXCLUDED.history_retention_days`). Then
      `cd backend && sqlc generate`; never hand-edit `heartbeats.sql.go`. Update
      the hand-written projections `markUpRowToHeartbeat` / `markFailRowToHeartbeat`
      in `ping.go` to copy the new `HistoryRetentionDays` field.
      Files: `backend/internal/api/handlers/heartbeats.go`,
      `backend/internal/store/postgres/queries/heartbeats.sql`,
      `backend/internal/store/postgres/` (regenerated),
      `backend/internal/api/handlers/ping.go`
      Verify: `cd backend && sqlc generate && sqlc diff` is clean (no drift); then
      `go test -race -count=1 ./internal/api/handlers/... && go build -o /dev/null
      ./cmd/pulse`.

- [ ] 6. Prune `heartbeat_events` inside the existing retention sweep. In
      `retention/service.go` add `deleteExpiredHeartbeatEvents(ctx, hb db.Heartbeat)
      (int64, error)` — raw `pool.Exec` DELETE `WHERE id IN (SELECT id FROM
      heartbeat_events WHERE heartbeat_id=$1 AND occurred_at<$2 LIMIT $3)` with
      cutoff `now - hb.HistoryRetentionDays*24h` and `s.deleteLimit`. In `runCycle`,
      after the monitor loop, add a STRUCTURAL COPY of that loop over
      `ListHeartbeats` (`db.ListHeartbeatsParams{Limit: int32(s.batchSize), Offset:
      offset}`): same `len(page) < s.batchSize` termination, same
      `maxConsecutiveBatchErrors=3` tolerance, same per-item log-and-`continue`
      isolation, incrementing the shared `s.rowsDeleted` counter for pruned rows.
      Files: `backend/internal/retention/service.go`
      Verify: `cd backend && go test -race -count=1 ./internal/retention/... &&
      go build -o /dev/null ./cmd/pulse`. DB-gated retention assertion lives in
      item 7's gated suite.

- [ ] 7. Add the `GET /api/v1/heartbeats/{id}/history` handler and register it,
      plus DB-gated timescale tests. New file
      `backend/internal/api/handlers/heartbeat_history.go` with
      `HeartbeatHistoryHandler{queries, tsStore}`, `NewHeartbeatHistoryHandler`, and
      `Register(rg)` → `rg.GET("/heartbeats/:id/history", h.GetHistory)`. Mirror
      `history.go`: parse `step` (validate [60,86400] → `400 INVALID_STEP`, echoed
      but inert), parse+validate `id` (`400 INVALID_ID`), `GetHeartbeat` (`404
      NOT_FOUND "heartbeat not found"`), default window last 24h, parse `from`/`to`
      RFC3339 (`400 VALIDATION_ERROR`), require `from.Before(to)` (`400
      VALIDATION_ERROR "from must be before to"`), `retentionClamp(from, now,
      hb.HistoryRetentionDays)` for `truncated`. Call `QueryHeartbeatEvents` and
      `CountHeartbeatEventsByType`; build the response per design §4.3:
      `heartbeat_id, from, to, truncated`, `summary{uptime_percent (null when
      total==0 else success_count/total_events*100, 1 decimal), missed_count,
      fail_count, success_count, total_events}`, `events[]` with `event_type`,
      `previous_state` (omitempty), `occurred_at` RFC3339, initialized
      `make([]..., 0, n)` (never null). Register in `router.go` inside the
      `protected` group gated by `if deps.TimescaleStore != nil`, immediately after
      the heartbeat CRUD registration, via `NewHeartbeatHistoryHandler(deps.Queries,
      deps.TimescaleStore)`. Add a `HeartbeatEventWriter`/`Deps` field
      `HeartbeatEventWriter timescale.HeartbeatEventWriter` to `api.Deps`, append
      `deps.HeartbeatEventWriter` to the `NewPingHandler(...)` call in `router.go`,
      and in `backend/cmd/pulse/main.go` set `HeartbeatEventWriter: timescaleStore`
      in the `api.Deps{...}` literal and append `timescaleStore` to the
      `heartbeat.NewEvaluator(...)` call. Add DB-gated tests in
      `backend/internal/store/timescale/store_test.go` for
      write→query round-trip (window, ascending, `[start,end)`),
      `CountHeartbeatEventsByType` counts, and retention pruning; plus handler
      param-parsing / summary-math unit tests (`success_count + fail_count +
      missed_count == total_events`, null uptime at zero events).
      Files: `backend/internal/api/handlers/heartbeat_history.go`,
      `backend/internal/api/router.go`, `backend/cmd/pulse/main.go`,
      `backend/internal/store/timescale/store_test.go`,
      `backend/internal/api/handlers/heartbeat_history_test.go`
      Verify: `cd backend && go test -race -count=1 ./... && go build -o /dev/null
      ./cmd/pulse`. If `pg_isready -h localhost -p 5432 -t 3` succeeds, also run
      `PULSE_RUN_DB_TESTS=1 go test ./internal/store/timescale/...`; otherwise run
      the suite WITHOUT the flag (tests skip) and state integration tests were
      skipped — never claim they passed.

- [ ] 8. Update the OpenAPI spec in the same change and confirm no MCP change is
      needed. In `backend/api/openapi.yaml` add path `GET /heartbeats/{id}/history`
      under the heartbeats tag with `id` path param, `from`/`to`/`step` query
      params, a `200` referencing new schema `HeartbeatHistoryResponse` (with
      `HeartbeatEvent` and `HeartbeatHistorySummary`), and `400`/`404` referencing
      the existing error envelope. `HeartbeatEvent.event_type` enum
      `[success, fail, missed]`; `previous_state` enum `[up, down, paused, new]`.
      Add `history_retention_days` (integer, min 1, max 365, default 30) to the
      existing heartbeat request + response schemas, mirroring the monitor schema.
      MCP check: grep `mcp/internal/` to confirm no tool consumes `/heartbeats/*`
      and the route is absent from the AGENTS.md MCP co-change list → record "no
      MCP changes required".
      Files: `backend/api/openapi.yaml`
      Verify: `cd backend && go build -o /dev/null ./cmd/pulse` (handlers/spec in
      sync); `grep -r "heartbeat" mcp/internal/ || true` returns no tool consumer
      (document the result). YAML must parse (e.g. `python3 -c "import
      yaml,sys; yaml.safe_load(open('backend/api/openapi.yaml'))"` if PyYAML is
      available; otherwise rely on the build).

- [ ] 9. Build the frontend timeline: types, API client, component, detail-page
      integration, empty state, and i18n across all 13 locales. In
      `frontend/src/lib/types.ts` add `history_retention_days: number` to the
      `Heartbeat` interface and the `HeartbeatEventType`, `HeartbeatEvent`,
      `HeartbeatHistorySummary`, `HeartbeatHistoryResponse` types (design §5.1). In
      `frontend/src/lib/api.ts` add `getHeartbeatHistory(id, from, to)` using
      `apiRequest<HeartbeatHistoryResponse>('GET',
      \`/heartbeats/${id}/history?${params}\`)` — intentionally omit `step`. Create
      `frontend/src/components/HeartbeatTimeline.svelte` modeled on
      `StatusTimeline.svelte`'s layout/interaction but with heartbeat semantics
      (`success`/`fail`/`missed`); bucket when events exceed a `maxMarkers`
      threshold where a bucket's color is the worst event (`missed > fail >
      success`); a `fail`/`missed` marker/bucket opens a popover (capped list),
      `success` is non-interactive; summary row shows `uptime_percent` (em dash
      when null, color thresholds ≥99/≥95) and `missed_count` plus a legend listing
      all three states. MUST use CSS custom properties / brand utilities from
      `app.css` (never hardcoded palette classes), `[data-theme="dark"]` (never
      `dark:`), and route EVERY visible string through `t('...')`. Render the
      mandatory empty state when `summary.total_events === 0` and not
      loading/error. In `frontend/src/routes/heartbeats/[id]/+page.svelte` add
      `history`/`historyLoading`/`historyError` state, fetch
      `getHeartbeatHistory(heartbeatId, now-24h, now)` in `fetchData()`, and render
      `<HeartbeatTimeline .../>` in a card titled `t('heartbeats.timeline.title')`
      between the Overview card and `<PingExamples>`. Add the `timeline` block
      (keys per design §5.7) under `heartbeats` in
      `frontend/src/locales/en.json` FIRST, then replicate the same keys
      (English placeholders) into every other locale:
      `ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh`. Add a component test
      (`frontend/src/components/HeartbeatTimeline.test.ts`) covering empty state,
      success/missed/fail rendering, worst-state bucketing, popover failure list,
      and that no literal strings leak (all via `t()`).
      Files: `frontend/src/lib/types.ts`, `frontend/src/lib/api.ts`,
      `frontend/src/components/HeartbeatTimeline.svelte`,
      `frontend/src/routes/heartbeats/[id]/+page.svelte`,
      `frontend/src/locales/en.json` and the 12 other locale JSON files,
      `frontend/src/components/HeartbeatTimeline.test.ts`
      Verify: `cd frontend && pnpm install && pnpm check && pnpm validate-locales
      && pnpm test && pnpm build` all pass. Because `validate-locales` omits `ar`
      and `be` and only warns on missing keys, also confirm the `timeline` keys
      exist in `ar.json` and `be.json` manually.

---

## Final verification (whole change, mirrors `pull_request_opened.yml`)

- Backend: `cd backend && go test -race -count=1 ./... && go build -o /dev/null
  ./cmd/pulse`.
- sqlc: `cd backend && sqlc generate && sqlc diff` clean.
- Frontend: `cd frontend && pnpm install && pnpm check && pnpm validate-locales
  && pnpm test && pnpm build` (+ manual `ar`/`be` key check).
- DB-gated timescale tests ONLY after `pg_isready -h localhost -p 5432 -t 3`
  succeeds: `cd backend && PULSE_RUN_DB_TESTS=1 go test
  ./internal/store/timescale/...`. If no DB, run without the flag (tests skip)
  and state that integration tests were skipped — never claim they passed.
- OpenAPI and handlers in sync in the same change; MCP confirmed unaffected.
