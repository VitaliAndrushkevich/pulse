# Phase 3 Verification — Admin API + ping ingestion + OpenAPI co-change

Scope: tasks 10–15 of `.kiro/specs/heartbeat-monitoring/tasks.md`.

## Files added
- `backend/internal/api/handlers/heartbeats.go` — HeartbeatHandler (CRUD + rotate-token).
- `backend/internal/api/handlers/notification_bindings_heartbeat.go` — HeartbeatBindingHandler.
- `backend/internal/api/handlers/ping.go` — PingHandler (root-level ingestion).
- `backend/internal/notification/heartbeat_triggers.go` — ValidateHeartbeatTriggers +
  translateHeartbeatTriggers (translate used by the Phase 4 dispatch helper).
- `backend/internal/api/handlers/heartbeats_test.go`,
  `heartbeats_pagination_property_test.go`, `ping_test.go` — handler tests.

## Files modified
- `backend/internal/api/router.go` — registered heartbeat admin + binding routes in the
  protected group; registered the four `/ping/:token[/fail]` routes on the engine; extended
  `apiPrefixes` with `/ping`; added heartbeat deps (metrics, state clearer, ping dispatcher seam).
- `backend/cmd/pulse/main.go` — read `PULSE_HEARTBEAT_WORKERS` (default 50 via `heartbeat.DefaultWorkers`),
  constructed `heartbeat.Metrics` on the shared `promRegistry`, constructed + ran the evaluator with
  `appCtx` cancellation on shutdown, wired `HeartbeatStateClear = stateTracker` and `PingDispatcher = nil`
  (dispatch wired in Phase 4).
- `backend/api/openapi.yaml` — co-change: added `/heartbeats`, `/heartbeats/{id}`,
  `/heartbeats/{id}/rotate-token`, `/heartbeats/{id}/notification-bindings[/{bindingId}]`, and
  root-level `/ping/{token}`, `/ping/{token}/fail`; added component schemas Heartbeat,
  HeartbeatCreateResponse, HeartbeatRotateTokenResponse, HeartbeatBinding, HeartbeatTag,
  HeartbeatTriggerCondition, request/list schemas, ValidationErrorResponse, FieldError, and the
  HeartbeatID / PingToken parameters. Admin paths use the combined bearerAuth scheme; `/ping` paths
  declare `security: []` (token-in-path, no auth header).

## Phase 4 seams (not implemented here, by design)
- `handlers.PingDispatcher` (ping.go) and `heartbeat.Dispatcher` (evaluator.go) are nil in Phase 3,
  so notification dispatch is skipped; WS broadcast + metric updates still fire.
- `notification.translateHeartbeatTriggers` is defined but unused until the Phase 4 dispatch helper.

## Commands run (all exit 0)
- `make -C .worktrees/heartbeat build` → builds `./cmd/pulse` with ldflags. PASS.
- `make -C .worktrees/heartbeat openapi` → "Validating spec... 1.0.0 Done." PASS.
- `make -C .worktrees/heartbeat test` → `go test ./...` all packages ok. PASS.
- `go test -count=1 ./internal/store/postgres/... ./internal/heartbeat/...` (fresh, live DB). PASS.
- `go test -race -count=1 ./internal/api/handlers/ ./internal/heartbeat/ ./internal/notification/`
  → PASS (no data races).
- `go build -o /dev/null ./cmd/pulse` → PASS.
- `go vet ./...` → clean (one pre-existing warning in `smtp_settings.go:293`, unrelated).

## Database availability
Postgres was reachable on `localhost:5432` (preflight `nc -z -w3 localhost 5432` succeeded), so the
DB-backed store integration tests and the evaluator tests ran for real and passed. New handler tests
(`heartbeats_test.go`, `ping_test.go`) use an in-memory `db.DBTX` fake (mirroring
`monitor_cascade_delete_test.go` / `tokens_integration_test.go`) and require no database.

## Test coverage summary (Task 15)
- create-returns-raw-token-once: covered via rotate-token (same DTO path); POST transactional
  success path is covered by the Phase 1 store integration test (needs a pool, not the DBTX fake).
- GET/LIST never return raw token/hash: asserted (TokenOrHash leak tests).
- paused status → observed state "paused": asserted.
- INVALID_ID / NOT_FOUND / VALIDATION_ERROR / INVALID_TAGS: asserted.
- binding trigger validation rejecting monitor triggers (with field details) + accepting heartbeat
  triggers + missing channel_id + reminder bounds: asserted.
- ping success/fail/recovery + 404 (unknown token, hash mismatch same prefix, short token) +
  paused-still-records + dummy-compare on bad token: asserted.
- pagination property test (pgregory.net/rapid): asserted.

## Notes
- Pre-existing modifications to `AGENTS.md`, `README.md`, `docs/API.md` on this branch are
  documentation-phase (Phase 8) artifacts and were intentionally NOT staged or committed in Phase 3
  (feature files only).
