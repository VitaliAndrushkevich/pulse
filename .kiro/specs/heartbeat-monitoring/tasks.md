# Heartbeat and Scheduled-Job Monitoring — Implementation Tasks (PF-01, First Useful Version)

Ordered, dependency-sequenced implementation checklist derived from `design.md` and `requirements.md`.
Each task is atomic, references concrete absolute paths, and ends in a buildable state.

Scope guard: this plan covers ONLY the first useful version. The following are explicitly OUT OF SCOPE
and MUST NOT be implemented here (requirements Non-Goals 1–7): run lifecycle events with run ids / per-run
duration; timezone-aware cron schedules; concurrent-run / duplicate / late-arrival / clock-boundary semantics
beyond the single-statement idempotency in Task 4; a dedicated MCP heartbeat tool; restore/validity
verification; an independently hosted external receiver; per-ping TimescaleDB history/charts.

Build/test commands (verified against `Makefile`, `backend/sqlc.yaml`, `frontend/package.json`):
- Backend build: `make build` (from repo root) — `go build ./cmd/pulse`.
- Backend tests: `make test` — `go test ./...` (CI runs with the race detector + TimescaleDB service).
- Migrations: `make migrate` / `make migrate-down`.
- OpenAPI validation: `make openapi`.
- Frontend (run from `frontend/`): `pnpm test`, `pnpm check`, `pnpm validate-locales`, `pnpm build`.

Development Skills (per `AGENTS.md`): load the Golang skills (database, concurrency, error handling, testing,
security, observability) for all backend tasks; load `svelte-code-writer` and `svelte-core-bestpractices`
for every `.svelte` / `.svelte.ts` task.

---

## Phase 1 — Data layer (migration + sqlc queries + models)

- [ ] 1. Create the heartbeat migration up/down pair.
      Create `heartbeats`, `heartbeat_tags`, `heartbeat_bindings` tables with all CHECK constraints and indexes
      from design §1.1–§1.3 (`idx_heartbeats_next_deadline` partial index `WHERE status='active' AND state='up'`,
      `idx_heartbeats_token_prefix`, `idx_heartbeats_status_created_at`, `idx_heartbeat_tags_*`,
      `idx_heartbeat_bindings_heartbeat_id`, unique `uq_channel_heartbeat`). `.down.sql` drops in reverse
      dependency order (`heartbeat_bindings`, `heartbeat_tags`, `heartbeats`). Do NOT alter `delivery_logs` and
      do NOT add any pg_notify trigger.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/migrations/017_heartbeats.up.sql`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/migrations/017_heartbeats.down.sql`
      Verify: `make migrate` applies cleanly, then `make migrate-down` rolls back cleanly with no error.

- [ ] 2. Author the heartbeat sqlc query files.
      Write `heartbeats.sql` (GetHeartbeat, GetHeartbeatForUpdate, ListHeartbeats, CountHeartbeats,
      CreateHeartbeat, UpsertHeartbeat, UpdateHeartbeatDeadline, MarkHeartbeatDown [`WHERE id=$1 AND state='up'`],
      MarkHeartbeatUp [FOR UPDATE CTE returning previous_state], MarkHeartbeatFail [same CTE pattern],
      RotateHeartbeatToken, GetHeartbeatByTokenPrefix, ListDueHeartbeats, CountHeartbeatsByState,
      DeleteHeartbeat), `heartbeat_tags.sql` (DeleteHeartbeatTags, InsertHeartbeatTag, ListTagsByHeartbeat), and
      `heartbeat_bindings.sql` (CreateHeartbeatBinding, GetHeartbeatBinding, ListBindingsByHeartbeat,
      ListBindingsByHeartbeatPaginated, CountBindingsByHeartbeat, UpdateHeartbeatBinding, DeleteHeartbeatBinding)
      per design §2.2, mirroring the existing `monitors.sql` / `monitor_tags.sql` / `notification_channels.sql`
      query styles.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/queries/heartbeats.sql`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/queries/heartbeat_tags.sql`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/queries/heartbeat_bindings.sql`
      Verify: `make build` succeeds after Task 3 regenerates code (this task alone adds `.sql` files only).

- [ ] 3. Regenerate sqlc code from the new queries.
      Run sqlc with the existing config `backend/sqlc.yaml` so the generated `heartbeats.sql.go`,
      `heartbeat_tags.sql.go`, `heartbeat_bindings.sql.go`, and updated `models.go` are produced and committed
      (generated files are checked in).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/heartbeats.sql.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/heartbeat_tags.sql.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/heartbeat_bindings.sql.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/models.go`
      Verify: `make build` compiles; `make test` passes the existing store package tests.

- [ ] 4. Add a Go integration test for migration + core queries.
      Follow the TimescaleDB/Postgres-backed integration test pattern used by
      `backend/internal/api/handlers/tokens_integration_test.go` and `monitor_cascade_delete_test.go`. Cover:
      migration up/down; CreateHeartbeat → GetHeartbeat; MarkHeartbeatUp returning the prior state atomically;
      MarkHeartbeatDown firing once (second call affects 0 rows while already down); ListDueHeartbeats using the
      partial index (paused / `new` / already-down rows excluded); DeleteHeartbeat cascading tags + bindings.
      (Golang testing skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/store/postgres/heartbeats_integration_test.go`
      Verify: `make test` — the new integration tests pass.

---

## Phase 2 — Ingestion token helper + deadline evaluator

- [ ] 5. Add a raw-token + SHA-256 verification helper to the token package.
      Add `GenerateRaw() (raw, prefix string, err error)` (256-bit `crypto/rand` → base64url, 8-char prefix,
      skips bcrypt) and `HashRawSHA256(raw string) string` (hex SHA-256) plus a constant-time hex compare helper
      and a dummy-compare helper for timing safety, per design §6.1 and §4.2. Keep the existing bcrypt
      `Generate()` untouched (API tokens/passwords still use bcrypt).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/token/token.go`
      Verify: `make test ./internal/token/...` equivalent via `make test` — new unit tests for raw generation,
      hashing, and constant-time compare pass.

- [ ] 6. Add a unit test for the token helpers.
      Table-driven test covering entropy/prefix length of `GenerateRaw`, SHA-256 determinism, and constant-time
      compare true/false cases. Follow the existing `backend/internal/token` test conventions and the
      property-test style of `backend/internal/api/handlers/auth_bcrypt_property_test.go`. (Golang testing +
      security skills.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/token/token_test.go`
      Verify: `make test` — token helper tests pass.

- [ ] 7. Create the `internal/heartbeat` package: metrics + deadline recompute helper.
      Add `Metrics` struct registering `pulse_heartbeat_up{heartbeat_id,heartbeat_name}` and
      `pulse_heartbeats_total` on an injected `*prometheus.Registry` (design §8), and a pure
      `NextDeadline(lastSuccess time.Time, intervalSeconds, graceSeconds int) time.Time` helper (design §Invariant
      Ownership). No token material in labels. (Golang observability skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/metrics.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/deadline.go`
      Verify: `make build` compiles the new package.

- [ ] 8. Implement the periodic-sweep evaluator.
      Create `Evaluator` with injectable `Config{Workers, TickInterval(default 1s), BatchSize(default 500)}`,
      a bounded worker pool, buffered jobs channel, `time.Ticker` poll loop calling `ListDueHeartbeats`, and
      `context`-cancellation shutdown that closes jobs, waits, and drains in-flight work — modeled on
      `backend/internal/monitor/scheduler.go` (design §5.2). `markDown` calls `MarkHeartbeatDown` and only on an
      actual update sets the `pulse_heartbeat_up` gauge to 0, broadcasts `heartbeat_status`, and dispatches the
      missed-run notification path (wired in Phase 4). Set `pulse_heartbeats_total` from `CountHeartbeats` each
      tick. (Golang concurrency skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/evaluator.go`
      Verify: `make build` compiles.

- [ ] 9. Add an evaluator test with a deterministic short tick.
      Drive the evaluator with a short tick + cancelable context against the Postgres test service. Assert a past
      deadline marks the row down exactly once (no re-fire on subsequent ticks), `new`/paused rows are never
      selected, and shutdown drains without leaking goroutines. Follow the scheduler test pattern. (Golang
      testing + concurrency skills.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/evaluator_test.go`
      Verify: `make test` — evaluator tests pass.

---

## Phase 3 — Admin API + ping ingestion + OpenAPI (same phase, per AGENTS.md co-change rule)

- [ ] 10. Implement the heartbeat admin CRUD handler.
      Create `HeartbeatHandler` mirroring `backend/internal/api/handlers/monitors.go`: POST (transactional create
      with tags, returns raw ping token once via a `createHeartbeatResponse` DTO), GET list (paginated via
      `parsePagination`, max limit 100), GET one, PUT (idempotent upsert with `GetHeartbeatForUpdate` row lock;
      recompute deadline via `UpdateHeartbeatDeadline` when interval/grace change and `last_success_at` exists;
      map `40001`/`40P01`/`55P03` → `409 CONFLICT`), DELETE (cascade + `ClearState(id)` + 204), and
      POST `/rotate-token`. Use `token.GenerateRaw` + SHA-256 storage (Task 5). DTO maps `status='paused'` →
      observed `state='paused'`; `token_prefix` exposed, raw token/hash never on GET/LIST. Error codes per design
      §3.3. Build `ping_url`/`ping_fail_url` from `PULSE_BASE_URL`. (Golang error-handling + security skills.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/heartbeats.go`
      Verify: `make build` compiles.

- [ ] 11. Implement the heartbeat notification-bindings handler.
      Create `HeartbeatBindingHandler` mirroring `backend/internal/api/handlers/notification_bindings.go`:
      POST/GET(paginated)/PUT/DELETE under `/heartbeats/{id}/notification-bindings[/{bindingId}]`, validating
      `channel_id` presence/existence and triggers via `ValidateHeartbeatTriggers` (allowed set
      `{heartbeat_missed, heartbeat_recovered}`; reject monitor triggers with `400 VALIDATION_ERROR` + field
      `details`), and `reminder_interval_minutes` bounds `[30,1440]`. (Golang error-handling skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/notification_bindings_heartbeat.go`
      Verify: `make build` compiles.

- [ ] 12. Implement the root-level ping ingestion handler.
      Create `PingHandler` holding `*db.Queries`, `*hub.Hub`, the heartbeat dispatch helper, and `heartbeat.Metrics`.
      Routes `GET|POST /ping/:token` (success) and `GET|POST /ping/:token/fail` (explicit failure) per design §4.
      Verify token via prefix lookup (`GetHeartbeatByTokenPrefix`) + SHA-256 constant-time compare + dummy compare
      on miss → minimal `404`. Success: `MarkHeartbeatUp` (atomic, returns prior state) sets `last_success_at`,
      recomputes deadline, transitions `up`; fire recovery path if prior was `down`. Fail: `MarkHeartbeatFail`
      transitions `down` without advancing `last_success_at`. Paused heartbeat still records success but no
      missed-run eval. Notification dispatch + WS broadcast fired non-blocking after the write. Log `token_prefix`
      only, never the raw token. (Golang security + concurrency skills.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/ping.go`
      Verify: `make build` compiles.

- [ ] 13. Register heartbeat admin routes, ping routes, and the evaluator in the router + main.
      In `router.go`: add the `/heartbeats*` and `/heartbeats/{id}/notification-bindings*` routes to the
      `protected` group; register the four `/ping/:token[/fail]` routes directly on the engine (like `/ws` at
      line 74); extend `apiPrefixes` (line 275) to include `/ping`. In `main.go`: read
      `PULSE_HEARTBEAT_WORKERS` via `getEnvInt(..., 50)`, construct `heartbeat.Metrics` on the shared
      `promRegistry`, construct and `Run` the evaluator with context cancellation on shutdown, and wire the ping
      handler + dispatch helper. (Golang skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/router.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/cmd/pulse/main.go`
      Verify: `make build` compiles and `make run`/startup wiring is consistent; `make test` passes.

- [ ] 14. Update the OpenAPI contract in the SAME phase as the handlers (AGENTS.md source-of-truth rule).
      Add paths `/heartbeats`, `/heartbeats/{id}`, `/heartbeats/{id}/rotate-token`,
      `/heartbeats/{id}/notification-bindings`, `/heartbeats/{id}/notification-bindings/{bindingId}`, and
      root-level `/ping/{token}`, `/ping/{token}/fail`. Document request/response schemas, the error envelope
      (`error.code`/`error.message` + `details` for bindings), `page`/`limit` on list endpoints, the
      `X-Request-ID` response header, combined JWT/API-token security on admin paths and NO security scheme on
      `/ping` paths (token-in-path). Add component schemas `Heartbeat` (no raw token), `HeartbeatCreateResponse`
      and `HeartbeatRotateTokenResponse` (include `ping_token`/`ping_url`/`ping_fail_url`), `HeartbeatBinding`
      (design §12.1).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/api/openapi.yaml`
      Verify: `make openapi` passes with no errors.

- [ ] 15. Add handler tests including property-based coverage.
      Add CRUD/validation/auth tests for the heartbeat handlers and a pagination property test following
      `backend/internal/api/handlers/monitors_pagination_property_test.go` (fast-check analogue:
      Go `testing/quick`/gopter style already in the repo). Cover: create returns raw token once; GET/LIST never
      return raw token/hash; PUT idempotency + `409` on conflict; `INVALID_ID`/`NOT_FOUND`/`VALIDATION_ERROR`/
      `INVALID_TAGS`/`UNAUTHORIZED`; binding trigger validation rejecting monitor triggers; ping success/fail/
      recovery transitions and `404` + dummy-compare on bad token. (Golang testing skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/heartbeats_test.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/heartbeats_pagination_property_test.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/api/handlers/ping_test.go`
      Verify: `make test` — all new handler tests pass.

---

## Phase 4 — Notifications integration

- [ ] 16. Add `EvaluateAndDispatchWithPayload` and refactor `EvaluateAndDispatch` to delegate.
      Add the payload-injecting sibling method to `Dispatcher` (design §5.3 / HIGH-1) taking a
      `payloadBuilder func(uuid.UUID, CheckResult) TemplateData`; refactor the existing `EvaluateAndDispatch` to
      call it with the existing `buildPayload` so monitor behavior is byte-for-byte identical. Leave
      `StateTracker.Evaluate` untouched. (Golang skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/notification/fanout.go`
      Verify: `make test` — existing notification tests still pass unchanged.

- [ ] 17. Add `ValidateHeartbeatTriggers` and the heartbeat→monitor trigger translation.
      Add `ValidateHeartbeatTriggers` (allowed `{heartbeat_missed, heartbeat_recovered}`, reject others with
      field `details`) and `translateHeartbeatTriggers([]TriggerCondition) []TriggerCondition`
      (`heartbeat_missed → monitor_down`, `heartbeat_recovered → monitor_up`) applied to the whole
      `BindingWithChannel.Triggers` before dispatch (design §5.3 / HIGH-2). (Golang skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/notification/validation.go`
      (add translation helper here or in a new `heartbeat_triggers.go` in the same package)
      Verify: `make build` compiles; `make test` passes.

- [ ] 18. Implement the heartbeat dispatch helper and wire it into evaluator + ping handler.
      Create a thin dispatch entry point that loads heartbeat bindings (`ListBindingsByHeartbeat`), builds
      `[]notification.BindingWithChannel` with `MonitorID = heartbeatID` and translated triggers, constructs the
      `heartbeatPayloadBuilder` closure (pre-filled `Monitor.Name`, `BaseURL`, timestamps so `enrichPayload`
      skips `GetMonitor`), and calls `EvaluateAndDispatchWithPayload(ctx, heartbeatID, checkResult, bindings,
      heartbeatPayloadBuilder)`. Ensure `DeliveryJob.MonitorID = heartbeatID` and `BindingID = heartbeatBindingID`
      (satisfies `delivery_logs.monitor_id NOT NULL`, MEDIUM-1). Call this from the evaluator `markDown` (Task 8)
      and the ping success/fail paths (Task 12). Wire `ClearState(id)` on DELETE (Task 10). (Golang skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/dispatch.go`
      Verify: `make build` compiles; `make test` passes.

- [ ] 19. Add tests for trigger validation, translation, and dedup behavior.
      Table-driven `ValidateHeartbeatTriggers` test; `translateHeartbeatTriggers` mapping test; a dispatch test
      asserting once-per-transition `down` and once-per-recovery `up` via the reused `StateTracker` keyed by the
      heartbeat UUID, and that enqueued jobs carry a non-empty `Monitor.Name`. Follow existing notification test
      patterns. (Golang testing skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/notification/heartbeat_triggers_test.go`,
      `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/heartbeat/dispatch_test.go`
      Verify: `make test` — new tests pass.

---

## Phase 5 — Realtime (WebSocket)

- [ ] 20. Add the `heartbeat_status` WebSocket message type and constructor.
      Add `TypeHeartbeatStatus = "heartbeat_status"`, the `HeartbeatStatusPayload` struct
      (`heartbeat_id`, `state`, optional `last_success_at`, optional `next_expected_deadline`, `timestamp`), and
      `NewHeartbeatStatusMessage(id, state string, lastSuccess, nextDeadline *time.Time) Message`
      (diff/patch only), per design §7.1. No change to `hub.go` itself. (Golang skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/hub/messages.go`
      Verify: `make build` compiles. (The evaluator and ping handler already broadcast via `hub.Broadcast` from
      Tasks 8 and 12 — confirm both call `NewHeartbeatStatusMessage`.)

- [ ] 21. Add a unit test for the message constructor.
      Assert the envelope type, patch field set (nil timestamps omitted), and RFC3339 formatting. Follow the
      existing `backend/internal/hub` message tests. (Golang testing skill.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/backend/internal/hub/messages_test.go`
      (extend if present, else create)
      Verify: `make test` — message constructor test passes.

---

## Phase 6 — Frontend + i18n

- [ ] 22. Add heartbeat i18n keys to en.json FIRST (source of truth).
      Add the `heartbeats.*` section and `nav.heartbeats` key exactly as enumerated in design §9.5 to the English
      locale. All later UI tasks reference these via `t('heartbeats.*')` / `t('common.*')`.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/locales/en.json`
      Verify: `cd frontend && pnpm validate-locales` — en.json parses (other locales flagged until Task 23).
      (svelte-core-bestpractices skill.)

- [ ] 23. Add the same keys to ALL other locale files (English value as placeholder).
      Add the identical `heartbeats.*` keys and `nav.heartbeats` to every non-English locale, using the English
      string as a placeholder where no translation exists (one task, all files listed).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/locales/ar.json`,
      `.../be.json`, `.../de.json`, `.../es.json`, `.../fr.json`, `.../it.json`, `.../ja.json`, `.../ko.json`,
      `.../pt.json`, `.../ru.json`, `.../tr.json`, `.../zh.json` (all under
      `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/locales/`)
      Verify: `cd frontend && pnpm validate-locales` — passes with no missing keys across all 13 locales.

- [ ] 24. Add the heartbeat TypeScript types and API client functions.
      Add the `Heartbeat` / `HeartbeatStatusPayload` types and `getHeartbeats`, `getHeartbeat`, `createHeartbeat`,
      `updateHeartbeat`, `deleteHeartbeat`, `rotateHeartbeatToken`, `listHeartbeatBindings`,
      `createHeartbeatBinding`, `updateHeartbeatBinding`, `deleteHeartbeatBinding` via the existing
      `apiRequest<T>` helper (Bearer auth, 15s timeout, error-envelope parsing, `X-Request-ID`), per design §9.3.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/lib/api.ts`
      Verify: `cd frontend && pnpm check` — type check passes. (svelte-code-writer skill.)

- [ ] 25. Add the heartbeat store with patch-merge semantics.
      Create a Svelte 5 runes store (`Map<string, Heartbeat>`) with a pure `applyHeartbeatPatch` merge plus
      `applyPatch`/`setHeartbeats`/`updateHeartbeat`/`removeHeartbeat`/`getById`/`clear`, keyed by `heartbeat_id`,
      merging only `state`/`last_success_at`/`next_expected_deadline` and discarding unknown ids — mirroring
      `frontend/src/lib/stores/monitors.svelte.ts` (design §9.2, R8.4/R9.9).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/lib/stores/heartbeats.svelte.ts`
      Verify: `cd frontend && pnpm check` passes. (svelte-code-writer skill.)

- [ ] 26. Wire `heartbeat_status` into the WebSocket client.
      Add a `heartbeat_status` case to the `handleMessage` switch dispatching to `heartbeatStore.applyPatch`, and
      extend the `WsMessage['type']` union / envelope typing with `'heartbeat_status'` and the
      `HeartbeatStatusPayload` type (design §7.2).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/lib/ws.ts`
      Verify: `cd frontend && pnpm check` passes. (svelte-code-writer skill.)

- [ ] 27. Build the heartbeat components.
      Create `HeartbeatRow.svelte` (state badge, name, interval, last success), `HeartbeatForm.svelte`
      (name/interval/grace/tags with client-side validation), `HeartbeatBindings.svelte` (per-resource binding UI
      mirroring `MonitorNotificationBindings.svelte`), and `PingExamples.svelte` (shell/cron/CI snippets from
      design §11 populated with the heartbeat's ping URL, each copy-to-clipboard, including the independence note).
      Reuse `ShowOnceModal.svelte`, `TagEditor.svelte`, `Pagination.svelte`, `VirtualList.svelte`. All strings via
      `t('heartbeats.*')`; theming via CSS custom properties + `[data-theme="dark"]` (never `dark:`), logical
      margins for RTL (design §9.4). (svelte-code-writer + svelte-core-bestpractices skills.)
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/components/HeartbeatRow.svelte`,
      `.../HeartbeatForm.svelte`, `.../HeartbeatBindings.svelte`, `.../PingExamples.svelte` (all under
      `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/components/`)
      Verify: `cd frontend && pnpm check` passes.

- [ ] 28. Build the heartbeat routes and nav entry.
      Create `heartbeats/+page.svelte` (list with `VirtualList` + `Pagination`), `heartbeats/[id]/+page.svelte`
      (detail: ping URLs + copy, state/interval/grace/last-success/next-deadline/tags, bindings section, usage
      examples, rotate-token with ShowOnceModal), `heartbeats/[id]/edit/+page.svelte`, `heartbeats/create/+page.svelte`,
      and add a `nav.heartbeats` entry in `+layout.svelte` next to Monitors (design §9.1).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/routes/heartbeats/+page.svelte`,
      `.../heartbeats/[id]/+page.svelte`, `.../heartbeats/[id]/edit/+page.svelte`,
      `.../heartbeats/create/+page.svelte`, `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/routes/+layout.svelte`
      Verify: `cd frontend && pnpm check` passes and `pnpm build` succeeds (embeds assets).
      (svelte-code-writer + svelte-core-bestpractices skills.)

- [ ] 29. Add frontend tests including property-based coverage.
      Add a property test for `applyHeartbeatPatch` using fast-check, mirroring
      `frontend/src/lib/stores/dashboard.property.test.ts` (patch-merge idempotency, unknown-id discard,
      field-subset merge), plus unit tests for the store and `HeartbeatForm` client-side validation following
      `@testing-library/svelte` conventions.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/lib/stores/heartbeats.property.test.ts`,
      `/Users/vandrushkevich/workdir/personal/pulse/frontend/src/lib/stores/heartbeats.test.ts`
      Verify: `cd frontend && pnpm test` — new tests pass. (svelte-core-bestpractices skill.)

---

## Phase 7 — MCP evaluation

- [ ] 30. Record the MCP co-change evaluation (recommend-or-defer: DEFER).
      Per design §12.2 and requirement 7.5, heartbeat endpoints are NEW resources not in the AGENTS.md MCP
      co-change endpoint list, so NO existing MCP tool (`mcp/internal/tools/`) or `PulseClient` method
      (`mcp/internal/pulseapi/`) requires modification, and a dedicated heartbeat MCP tool is explicitly deferred
      (Non-Goal 4). This task is the documented evaluation itself — add a short note to the PR description / change
      log stating the evaluation. Do NOT add a new MCP tool.
      Files: none modified (evaluation only — no `mcp/` changes).
      Verify: `make test` for the MCP module remains green (no changes expected); confirm no `mcp/` edits in the diff.

---

## Phase 8 — Documentation (requirement 14)

- [ ] 31. Add the "Heartbeats" subsystem section to `AGENTS.md`.
      Add a dedicated "Heartbeats" section modeled on the existing "Notifications" section (R14.1): the resource
      model (lifecycle `status` `active`/`paused` vs. observed `state` `up`/`down`/`paused`/`new`), the
      ingestion-vs-administration separation and why `/ping` is root-level, the success/explicit-fail ping
      semantics and deadline recompute, the periodic-sweep evaluator (bounded worker pool, partial index,
      once-per-episode `MarkHeartbeatDown`), the SHA-256 ping-token storage and its justified scoped deviation
      from the bcrypt rule, and the notification integration (dedicated `heartbeat_bindings`, trigger translation,
      reused dispatcher/StateTracker/retry/delivery-logs). Stay within first-useful-version scope — do NOT present
      any Non-Goal as shipped (R14.8).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/AGENTS.md`
      Verify: Manual review — the Heartbeats section is present and matches the implemented behavior.

- [ ] 32. Update the `AGENTS.md` endpoint tables, Key Files Reference, env-var table, and Current Progress.
      In the same `AGENTS.md`: (a) add a heartbeat API endpoints table — admin CRUD (`POST`/`GET`/`GET {id}`/
      `PUT`/`DELETE /api/v1/heartbeats`), `/heartbeats/{id}/rotate-token`,
      `/heartbeats/{id}/notification-bindings[/{bindingId}]`, and root-level `/ping/{token}`,
      `/ping/{token}/fail` — in the style of the notifications endpoint table (R14.3); (b) add the new heartbeat
      files to the "Key Files Reference" table (migration `017`, sqlc query files, `internal/heartbeat`
      evaluator/dispatch/metrics/deadline, `handlers/heartbeats.go`, `handlers/ping.go`,
      `handlers/notification_bindings_heartbeat.go`, the `heartbeat_status` WS message, the frontend
      store/components/routes) (R14.2); (c) add `PULSE_HEARTBEAT_WORKERS` (default 50) to the environment-variable
      table (R14.4); (d) add a heartbeat row/entry to the "Current Progress" section marking PF-01 first useful
      version delivered (R14.5).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/AGENTS.md`
      Verify: Manual review — all four additions present and internally consistent with Tasks 1–30.

- [ ] 33. Update `docs/MILESTONES.md` to record heartbeat monitoring.
      Add heartbeat monitoring (PF-01 first useful version) to the milestone breakdown in the existing format
      (R14.5), consistent with the "Current Progress" update in Task 32.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/docs/MILESTONES.md`
      Verify: Manual review — the milestone entry is present and matches the Current Progress wording.

- [ ] 34. Document `PULSE_HEARTBEAT_WORKERS` in `.env.example`.
      Add `PULSE_HEARTBEAT_WORKERS` (default 50, bounded evaluator worker pool) with a short description matching
      the style of the other documented variables (R14.6).
      Files: `/Users/vandrushkevich/workdir/personal/pulse/.env.example`
      Verify: Manual review — variable documented with its default and description.

- [ ] 35. Add the heartbeat feature section + shell/cron/CI examples to the README.
      Add a heartbeat monitoring section to `README.md` describing the feature and including the shell (`curl`
      success + `/fail`-on-error pattern), cron (ping only after the command succeeds), and CI (ping on job
      success) examples from design §11 with a placeholder `/ping/<TOKEN>` URL, plus the independence principle
      (a success ping proves the command ran, not that its output is valid/restorable; an independently hosted
      receiver can detect loss of Pulse itself) — satisfying requirement 11 and R14.7.
      Files: `/Users/vandrushkevich/workdir/personal/pulse/README.md`
      Verify: Manual review — the feature section, three example blocks referencing `/ping/<TOKEN>` and
      `/ping/<TOKEN>/fail`, and the independence note are all present.

---

## Phase 9 — Final verification

- [ ] 36. Final full-stack verification.
      Run the complete verification suite to confirm the feature is buildable and green end to end.
      Files: none (verification only).
      Verify: `make build` && `make test` (backend) all pass; `make openapi` passes; from `frontend/`
      `pnpm check` && `pnpm validate-locales` && `pnpm test` && `pnpm build` all pass. Clean up any temporary
      artifacts created during verification.
