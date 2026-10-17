# Phase 8 (Documentation, Tasks 31–35) — Verification

Fresh implementation (no `phase8-review.json` present). All work in the worktree
`/Users/vandrushkevich/workdir/personal/pulse/.worktrees/heartbeat` on branch
`feat/heartbeat-impl`.

User constraint honored: **no** `docs/HEARTBEATS.md` created. Documentation lives only in
`AGENTS.md`, `docs/MILESTONES.md`, `.env.example`, and `README.md`.

Scope guard (R14.8): no non-goal is documented as shipped. Run-lifecycle events, timezone-aware
cron, concurrency/duplicate/late-arrival/clock-boundary semantics, a dedicated MCP heartbeat tool,
restore-validity verification, an external receiver, and per-ping history/charts are described only
as explicitly deferred.

## Files changed and sections added

### `AGENTS.md` (Tasks 31 + 32)
- **New "Heartbeats" subsystem section** (Task 31, R14.1), modeled on the "Notifications" section:
  - Intro + explicit first-useful-version scope with the non-goals called out as not shipped.
  - "Resource and Status Model" — lifecycle `status` (`active`/`paused`) vs. observed `state`
    (`up`/`down`/`paused`/`new`).
  - "Ingestion vs. Administration Separation" — why `/ping` is root-level and token-only.
  - "Ping Semantics and Deadline Recompute" — success/explicit-fail, deadline recompute, dummy-compare 404.
  - "Periodic-Sweep Evaluator" — bounded worker pool, partial index, once-per-episode `MarkHeartbeatDown`.
  - "Ping-Token Storage (justified bcrypt deviation)" — SHA-256 + prefix and why it deviates from bcrypt.
  - "Notification Integration" — `heartbeat_bindings`, trigger translation, reused
    dispatcher/StateTracker/retry/delivery-logs.
- **Heartbeat API endpoints table** (Task 32a, R14.3) inside the Heartbeats section: admin CRUD,
  rotate-token, notification-bindings, and root-level `/ping/{token}` + `/ping/{token}/fail`.
- **Key Files Reference additions** (Task 32b, R14.2): migration 017, sqlc query files,
  `internal/heartbeat` evaluator/dispatch/metrics/deadline, `handlers/heartbeats.go`,
  `handlers/ping.go`, `handlers/notification_bindings_heartbeat.go`, frontend store/components/routes.
  (`heartbeat_status` WS message is covered by the existing "WS messages" → `hub/messages.go` entry.)
- **Environment-variable table** (Task 32c, R14.4): `PULSE_HEARTBEAT_WORKERS` (default 50) added to the
  Infrastructure env table; also to the per-subsystem env table inside the Heartbeats section.
- **Current Progress** (Task 32d, R14.5): K row updated to
  "K: Heartbeat (push) Monitoring (PF-01 first useful version) ✅ Done"; the "Completed" bullet and the
  "Delivery Constraints" caveat updated to reflect the shipped PF-01 state (removed the stale
  "delivery stubbed / no UI / Phase 4" wording) and to list the deferred non-goals.

### `docs/MILESTONES.md` (Task 33, R14.5)
- File was deleted earlier on this branch (commit `da598da`) while `AGENTS.md` still linked to it.
  Recreated as a concise milestone breakdown matching the AGENTS.md Current Progress table (A–K), with
  a detailed "Milestone K: Heartbeat (push) Monitoring (PF-01 first useful version)" entry consistent
  with the Current Progress wording and listing the deferred non-goals. This also repairs the dangling
  AGENTS.md link.

### `.env.example` (Task 34, R14.6)
- Added a "Heartbeat Monitoring" block with `PULSE_HEARTBEAT_WORKERS=50` and a short description in the
  style of the other variables.

### `README.md` (Task 35, R11 + R14.7)
- Added a "Heartbeat Monitoring" section (after Notifications): what it is, the token model, and
  shell (`curl` success + `/fail`-on-error), cron (ping only after success), and CI (ping on success)
  examples from design §11 using the placeholder `https://pulse.example.com/ping/<TOKEN>` /
  `/ping/<TOKEN>/fail` URLs, plus the independence/validity principle (a success ping proves the
  command ran, not that output is valid/restorable; an independently hosted receiver can detect loss
  of Pulse/host). (Key Features bullet and the env-var table row were already present from earlier phases.)

## Build verification
- `make -C .worktrees/heartbeat build` → exit 0 (doc-only changes; backend compiles unchanged).

## Commit boundaries (conventional, no AI attribution / Co-Authored-By)
- `docs(heartbeat): add Heartbeats subsystem section and tables to AGENTS.md` (Tasks 31–32)
- `docs(heartbeat): record milestone in docs/MILESTONES.md` (Task 33)
- `docs(heartbeat): document PULSE_HEARTBEAT_WORKERS in .env.example` (Task 34)
- `docs(heartbeat): add README section with shell/cron/CI examples` (Task 35)

Only files this feature creates/modifies were committed. The user's roadmap/technical-debt docs were
NOT touched or committed; the pre-existing unstaged `docs/API.md` modification (from an earlier phase)
was left untouched.
