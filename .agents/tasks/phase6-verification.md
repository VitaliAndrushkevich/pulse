# Phase 6 — Frontend + i18n — Verification

Scope: tasks 22–29 of `.kiro/specs/heartbeat-monitoring/tasks.md`.
All commands run from `frontend/` in the worktree `.worktrees/heartbeat` (branch `feat/heartbeat-impl`).

## Commands run and results

| Gate | Command | Result |
|------|---------|--------|
| Type check | `pnpm check` | PASS — `svelte-check found 0 errors and 57 warnings` (warnings are the pre-existing `state_referenced_locally` pattern shared with `MonitorForm.svelte`; none are errors) |
| Locale coverage | `pnpm validate-locales` | PASS — all 678 keys present across the 11 script-checked locales; `ar` and `be` (omitted from the script's locale list, per AGENTS.md) verified manually to have full key parity (0 missing, 49 heartbeat keys each) |
| Unit/property tests | `pnpm test` | PASS — 50 files, 551 tests passed, including the 13 new heartbeat tests |
| Production build | `pnpm build` | PASS — static adapter wrote `build/`; heartbeat routes embedded |

## What was implemented

- **Task 22** — `heartbeats.*` section + `nav.heartbeats` added to `src/locales/en.json` (source of truth), exactly per design §9.5.
- **Task 23** — identical keys added to all 12 other locales (`ar be de es fr it ja ko pt ru tr zh`) using the English string as a placeholder.
- **Task 24** — `Heartbeat` / `HeartbeatStatusPayload` types + `getHeartbeats`, `getHeartbeat`, `createHeartbeat`, `updateHeartbeat`, `deleteHeartbeat`, `rotateHeartbeatToken`, `listHeartbeatBindings`, `createHeartbeatBinding`, `updateHeartbeatBinding`, `deleteHeartbeatBinding` via the existing `apiRequest<T>` helper (`src/lib/api.ts`, `src/lib/types.ts`).
- **Task 25** — dedicated patch-merge store `src/lib/stores/heartbeats.svelte.ts` (Svelte 5 runes, `Map<string, Heartbeat>`), pure `applyHeartbeatPatch` merging only `state`/`last_success_at`/`next_expected_deadline`, unknown-id discard — mirrors `monitors.svelte.ts`.
- **Task 26** — `heartbeat_status` case added to `handleMessage` in `src/lib/ws.ts`, dispatching to `heartbeatStore.applyPatch`; `WsMessage['type']` union + payload typing extended with `'heartbeat_status'` / `HeartbeatStatusPayload`.
- **Task 27** — components `HeartbeatRow.svelte`, `HeartbeatForm.svelte`, `HeartbeatBindings.svelte`, `PingExamples.svelte`; reuse `ShowOnceModal`, `TagEditor`, `Pagination`, `VirtualList`; all strings via `t('heartbeats.*')`; theming via CSS custom properties, no Tailwind `dark:`; the independence note is rendered in `PingExamples`.
- **Task 28** — routes `heartbeats/+page.svelte` (VirtualList + Pagination), `heartbeats/[id]/+page.svelte` (ping URL section + rotate via ShowOnceModal, state/interval/grace/last-success/next-deadline/tags, bindings, usage examples), `heartbeats/[id]/edit/+page.svelte`, `heartbeats/create/+page.svelte`; `nav.heartbeats` entry added to `+layout.svelte` next to Monitors.
- **Task 29** — `src/lib/stores/heartbeats.property.test.ts` (fast-check: field-subset merge, idempotency, unknown-id discard) + `src/lib/stores/heartbeats.test.ts` (store actions + `HeartbeatForm` client-side validation via `@testing-library/svelte`).

## Notes / scoped decisions

- The raw ping token is write-only (returned only on create and rotate). The detail page therefore shows the `token_prefix` and surfaces the full working ping URL once via `ShowOnceModal` on rotate; the `PingExamples` snippets on the detail page use a `<TOKEN>` placeholder the operator substitutes. The create flow surfaces the real `ping_url` once via `ShowOnceModal`.
- `HeartbeatForm` initializes `$state` from props, producing the same `state_referenced_locally` lint warnings already present in `MonitorForm.svelte`; this matches the established form convention in the codebase.
