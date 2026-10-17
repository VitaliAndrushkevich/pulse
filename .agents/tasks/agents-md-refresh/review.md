# AGENTS.md refresh: changelog collapsed to feature inventory, facts corrected, dev workflow added

The change rewrites `AGENTS.md` so it matches the repo as it stands at `460cf50`. The per-milestone changelog (A–J, CI) is replaced by a feature inventory table that points to `.kiro/specs/<name>/`. The two partial env var tables become one 26-row table with defaults taken from the code. A new "Development Workflow" section adds a correct run-locally matrix, pre-finish checks that mirror PR CI, and recipes for migrations, monitor types and endpoints. Every factual correction traces back to a cited line in `report.md`, and spot checks against the repo agree. Normative rules from the API, security, performance, WS, theming, i18n and MCP sections are all still present.

Watch for: two of the `docs/` files the new text links to (`PRODUCT-ROADMAP.md`, `TECHNICAL-DEBT.md`) are untracked, so they have to be committed together with AGENTS.md or the links will be broken on other clones (confirmed). The heartbeat status line says "0 tasks done". That is true today, but an implementation worktree already exists, so the line must change in the same PR that starts implementing (confirmed). The file grew from about 25.8 KB to about 28.8 KB, and there is some minor overlap between Key Files and Repository Map (confirmed, non-blocking).

**Verdict**: APPROVED

## High-level view

Milestone history, volatile counts ("218 frontend tests"), the incorrect "7-day window" history claim, "Settings (secrets)" and the dead `docs/MILESTONES.md` link are all gone. Each inventory row says what exists and names the spec folder that holds the history. Heartbeat is marked "Planned — NOT implemented" in the inventory and listed in future scope. "Alert channels" is no longer in future scope.

The factual corrections hold up when checked against the repo. `make dev` and `make dev-local` are now described the right way round. `make openapi` is described as printing the spec version, not validating it. The validate-locales command is fixed. `PULSE_NOTIFICATION_WORKERS` now defaults to 10 and `DATABASE_URL` has its default. A third WS message type is added. Reminders are marked as not wired (TD-004), and the rotate-key gap is noted (TD-008). All six workflows are listed with their triggers. The MCP co-change list keeps the 12 endpoints that the report checked against `http_client.go`.

The new rules about known spec drift and the validate-locales caveat are accurate (spot-checked). The doc grew by about 3 KB. Most of that is high-value workflow and command content, but the Key Files table partly repeats the Repository Map and some per-section paths.

The diff itself only touches `AGENTS.md`. The working tree also has a pre-existing `README.md` edit and untracked docs and worktree folders from the user's own in-progress work. None of those came from this change.

<details>
<summary>Issues (4)</summary>

1. **Untracked linked docs** — `docs/PRODUCT-ROADMAP.md` and `docs/TECHNICAL-DEBT.md` are referenced in Repository Map, Feature Inventory, Security (TD-008) and Notifications (TD-003/004/005), but `git ls-files docs/` shows only `API.md`, `AUDIT-Jul-2026.md` and `DEVELOPMENT.md`. Commit them in the same commit as AGENTS.md. Non-blocking.
2. **Heartbeat status will go stale** — "0 tasks done" is correct now, but branch `feat/heartbeat-impl` and `.worktrees/heartbeat` already exist. The heartbeat implementation PR must update the inventory row, the planned paragraph, the future-scope list and the env table (`PULSE_HEARTBEAT_WORKERS`). Non-blocking; flag it for that PR.
3. **Key Files overlap** — some Key Files rows repeat paths already given in Repository Map and the MCP, Notifications, i18n and Workflow sections (the MCP entrypoint, `register.go`, the i18n module, locale files). Trim the table to rows that no other section mentions if token budget matters. Non-blocking.
4. **`make test` description slightly imprecise** — "the same without `-race`" also leaves out `-count=1` and the build step. Consider "`make test` = `go test ./...` (no race, cached)". Non-blocking nit.

</details>

<details>
<summary>Details</summary>

### Normative rules survived the rewrite

Every removed `-` line in the diff was compared against the new file. The removed rule lines (the `/api/v1` prefix, the embed rule, AES-256-GCM, combined auth, the Go/Svelte stack, the pnpm rule, the i18n `t()` rule, the WS message types) all come back as `+` lines with extra detail added. The old wording is still in place each time. Theming, dark mode, performance, the token/bcrypt/log-sanitization security rules, constant-time WS auth and the MCP breaking-change rule were not touched.

The notification deep-dive keeps the substance of each removed subsection in bullet form: channel features, trigger ranges, the pipeline steps, the retry schedule and the non-retryable classes, template variables, and metric names. The deleted 16-row endpoint table is replaced by route groups plus a pointer to `openapi.yaml`, and the bullets add `/monitors/{id}/delivery-logs`, which the old table missed. The only descriptive details dropped are changelog trivia with no rule attached, such as the `pulse-theme-mode` localStorage key, the 15s API timeout and the WS backoff constants.

### Spot-checked claims

These checks were run against the working tree:

- Paths that exist: `docs/AUDIT-Jul-2026.md`, `backend/migrations/016_quic_monitor_type.up.sql`, `frontend/src/components/dashboard/`, `backend/internal/monitor/notify.go`, `backend/internal/monitor/dynamic_metrics.go`, `.atl/skill-registry.md`, `skills-lock.json`, `.github/copilot-instructions.md`.
- `.github/workflows/` contains exactly the six workflows that are listed.
- `.kiro/hooks/` contains the four hooks named in Agent Tooling.
- In `openapi.yaml`, `/monitors/{id}/credentials/{credentialId}` has only `put` and `delete`, and `/monitors/{id}/delivery-logs` is missing. This matches the "known spec drift" line.
- `validate-locales.ts` lists 11 locales without `ar` and `be`, which matches the caveat.
- `frontend/package.json` has `check` and `validate-locales`.
- The hub broadcast buffer is 256.
- `tasks.md` for heartbeat has 0 checked tasks.
- `MILESTONES`, `alert channels` and `218` no longer appear in AGENTS.md.

The coder's own verification is recorded in report §12: 110 path tokens checked with `test -e`, all make targets and pnpm scripts, versions, and an env-var cross-check that excludes MCP error codes. That evidence is enough for the rest of the claims.

### Size and remaining duplication

At HEAD, AGENTS.md was 25,763 bytes, not the "~22 KB" the report says. It is now 28,772 bytes. About 6 KB of changelog was removed, and about 9 KB of operational content was added: workflow tables, recipes, the full env table and a larger Key Files map. That suits the user's goal of efficient development.

The remaining redundancy is small but real. Some Key Files rows repeat paths that Repository Map and the per-section text already give. TD-004 for reminders is mentioned in both the Feature Inventory row and the Notifications section. Heartbeat appears in the Feature Inventory paragraph and in Delivery Constraints. That second mention is deliberate and useful as a guardrail.

### Doc-link and lifecycle coupling

Two kinds of reference depend on work that is not committed yet. `docs/PRODUCT-ROADMAP.md` and `docs/TECHNICAL-DEBT.md` exist only as untracked files, so AGENTS.md (and the user's pre-existing README edit) will have broken links on any other clone until they are committed. The heartbeat "NOT implemented / 0 tasks" wording is tied to a branch that is about to change it; `feat/heartbeat-impl` already has a worktree at `.worktrees/heartbeat`, currently at the same commit with no changes. The heartbeat `design.md` already plans to update AGENTS.md, so this is a reminder for that PR, not a defect here.

</details>

<details>
<summary>File map</summary>

- `AGENTS.md` — the changelog was replaced by a feature inventory. The two env tables were merged into one, and notification detail was condensed. New sections cover the repository map, development workflow, commands reference and agent tooling. Facts were corrected (make targets, validate-locales, defaults, WS types, secrets→credentials, reminders, rotate-key, workflows, future scope).
- Out of scope (pre-existing user changes, not from this step): `README.md` (roadmap/tech-debt link), untracked `docs/PRODUCT-ROADMAP*.md`, `docs/TECHNICAL-DEBT*.md`, `.worktrees/`.

Full diff: `git -C /Users/vandrushkevich/workdir/personal/pulse diff -- AGENTS.md`

</details>
