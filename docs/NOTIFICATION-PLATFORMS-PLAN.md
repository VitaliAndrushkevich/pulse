# Notification secrets, draft testing, and native platforms

Investigation: 2026-10-23. Status: implemented on `feat/notification-platforms` in an isolated worktree based on `main` (the repository has no `master` branch). See `docs/NOTIFICATIONS.md` for shipped behavior. Database-backed migration and rotation verification passed after Docker was started; real-provider account smoke tests still require disposable destinations.

## Confirmed defects and gaps

1. **Editing a webhook destroys saved header values.** `NotificationChannelForm.svelte` initializes editable header values from the GET response, which contains `[REDACTED]`, and submits those strings on save. `NotificationChannelHandler.Update` loads the existing row but discards its config. `processConfigForStorage` then encrypts the submitted placeholders. Delivery successfully decrypts them and sends literal `[REDACTED]`. This affects every custom header, including Authorization. Existing webhook delivery tests start with encrypted values and do not exercise this lifecycle.
2. **Empty headers fail delivery.** Storage skips encryption for empty values, but delivery always attempts decryption.
3. **Tests only use persisted settings.** `POST /api/v1/notifications/channels/{id}/test` cannot test unsaved edits or a new channel. The form has no Test action.
4. **Redaction fails open on malformed stored JSON.** `redactChannelConfig` returns raw configuration when parsing fails. Provider secrets require a fail-closed response projection.
5. **Retries are not connected.** `RetryQueue` exists and is tested, but production never constructs it or invokes `Schedule`; failed dispatcher deliveries are logged without retrying. Metrics currently classify every channel as `unknown`.
6. **Heartbeat presentation lacks resource kind.** Heartbeat dispatch fills `MonitorData`; existing email links assume `/monitors/{id}`. Native messages need correct heartbeat wording and links.

If a saved token has already been overwritten with the placeholder, the original cannot be recovered from that row. Re-enter the token once after the fix; rotation is only necessary if the token was exposed elsewhere.

## Delivery order

### 1. Secret preservation regression fix

Ship independently so the existing bug does not wait for provider development.

- Distinguish API read metadata from write values. Show blank secret replacement inputs with a saved-value indicator; never initialize input values from redaction markers.
- Define the webhook header update contract: the submitted header list replaces the list; an existing header with omitted `value` retains its encrypted value; an explicit string replaces it, including an empty string; removing a row removes the header. Missing values on new headers are invalid.
- Match retained headers by validated, case-insensitive HTTP header name. Reject duplicate names. A rename beyond casing requires a replacement value; do not transfer a secret based on array position. Reordering must be safe.
- For compatibility with the current UI, treat `[REDACTED]` as preserve only for an existing matching header. Reject it on create/new headers rather than storing a placeholder. Document this reserved marker.
- Preserve ciphertext directly rather than decrypting and re-encrypting unchanged values. Encrypt explicit empty values too.
- Use a transaction with row locking for read/merge/write, preventing an unrelated edit from restoring stale ciphertext after a concurrent credential replacement.
- Secret retention only applies within the same channel type. Reject attempts to retain a secret across type changes.
- Project safe responses for create/list/get/update; malformed stored config must produce a controlled error or sanitized configuration, never raw data.
- Update OpenAPI in the same change, including distinct create/update/read schemas and errors. Keep the existing response marker compatible for existing clients while adding saved-value metadata for the new UI.

Acceptance: create with `Authorization: Bearer test-token`, reopen, change name/body/method, save repeatedly, and test. A local receiver must still observe the original Authorization value. Replacement and deletion must also work.

### 2. Test current configuration during setup

Add authenticated `POST /api/v1/notifications/channels/test` alongside the existing saved-channel endpoint. Implement and document the REST contract before wiring the UI.

Proposed request: `{ name, type, config, existing_channel_id? }`. The optional ID resolves retained secrets on edits; a new channel must provide its required secrets. Reject incompatible existing channel types and unknown IDs. Reject malformed requests before attempting outbound delivery.

- Validate and resolve exactly the config that Save would use, without writing a channel, updating settings, creating bindings, recording delivery logs, or scheduling retries.
- Reuse a shared delivery service for draft tests, saved tests, and production. Avoid separate provider switches that can drift.
- Keep the existing 10-second test deadline and clearly mark sample messages as tests. Tests send a real message. Use a fresh delivery ID per test.
- Return `{ success, channel_type, channel_id?, error? }`; delivery failure remains a normal test result, while invalid requests use the API error envelope. Secrets must never appear in returned errors.
- Add Test beside Cancel/Save, with `type="button"`, independent pending state, duplicate-send prevention, and accessible inline success/failure feedback. Share payload construction between Save and Test.
- Test uses the current unsaved destination, body, headers, and provider fields. Preserve edits and stay on the page. Clear stale success feedback when configuration changes.
- Bound concurrent outbound test requests; use an existing limiter if suitable, otherwise a small shared semaphore with a documented busy response. Cap request and provider response sizes.

Acceptance: on create and edit, Test delivers current unsaved settings while the database remains unchanged; retained secrets work without re-entry.

### 3. Shared delivery foundation and native channels

Keep email and generic webhook. Add first-class `slack`, `matrix`, and `telegram` channel types using the existing JSONB config and binding model.

Create a shared channel service with explicit operations for validation, secret resolution/storage, response projection, and delivery. Provider implementations must accept a context and stable delivery ID. Place the service/provider boundary so `notification` does not import its child provider packages; wire concrete providers from startup. Preserve existing email/webhook behavior with compatibility tests.

Expand the channel-type CHECK constraint in a new migration; do not edit migration 014. The next migration number in this checkout is 021; verify again at implementation time. No new tables or queries appear necessary beyond locking support. Define a safe rollback policy: never silently delete native channel rows when reverting the constraint.

| Platform | Configuration | Initial implementation |
| --- | --- | --- |
| Slack | Incoming webhook URL, encrypted in full | Block Kit message plus accessible plain-text fallback. Provide Pulse Slack app setup/manifest guidance and existing brand assets. Sender identity/icon/channel are controlled by the Slack app configuration. |
| Telegram | Encrypted bot token; chat ID as a string; optional topic/thread ID | Bot API `sendMessage`; escaped HTML or plain text, no link preview. Validate HTTP status and JSON `ok`; support group topics. Explain bot membership/permissions in setup guidance. |
| Matrix | Homeserver URL; room ID; encrypted access token | Dedicated bot already joined to an unencrypted room; authenticated `m.notice` using a stable transaction ID. Check room encryption state and reject encrypted rooms explicitly. Full E2EE requires persistent crypto/device state and is deferred. |

Native secret fields use omission-to-preserve semantics on update, explicit replacement, and read-only configured flags. Never return Slack webhook URLs or bot/access tokens. Require credentials on creation and type changes. Existing generic webhook URL/template exposure is separate; do not automatically migrate existing webhooks into native channels.

Use a common Pulse message model: resource kind/name, event, status transition, target where applicable, UTC timestamp, response time for monitor checks, incident duration when known, and optional resource link. Carry trigger type so degraded/SSL/failure alerts have accurate titles. Distinguish monitors from heartbeats and link to `/heartbeats/{id}` appropriately. Use JSON encoders and platform escaping; truncate safely for each provider's limits. Test missing/zero data and long Unicode names.

### 4. Delivery reliability required for native providers

- Wire retry scheduling into failed production deliveries and shutdown lifecycle. Keep four total attempts and bounded scheduling; do not retry tests.
- Classify permanent configuration/auth/permission failures separately from network, 5xx, and rate-limit failures. Parse provider success/error bodies where required.
- Support Slack `Retry-After`, Telegram `parameters.retry_after`, and Matrix rate-limit guidance. Schedule cooldowns outside delivery workers; do not block workers with sleeps. Coordinate cooldowns per destination and avoid re-enqueue storms under pressure.
- Keep the same Matrix transaction ID across attempts. Slack/Telegram can duplicate messages after ambiguous network failures; document at-least-once behavior rather than promising deduplication.
- Redact credential-bearing URLs, tokens, headers, and provider error bodies before they reach HTTP responses, server logs, or delivery logs. Do not wrap raw transport errors containing secret URLs.
- Disable redirects for token-bearing native requests and validate destinations: Slack supported incoming webhook hosts, fixed Telegram API host, and configurable Matrix homeserver without URL credentials. Keep self-hosted Matrix reachable. Do not forward credentials to redirected hosts.
- Resolve actual provider type for metrics, including drops, without introducing per-drop database lookups.
- Include all new encrypted fields in key rotation. The existing command omits notification headers and SMTP password; define and test coverage for all notification secrets so providers do not enlarge this known gap.

### 5. Update documentation after implementation

- Update README and user-facing notification documentation with Slack, Matrix, and Telegram setup, required credentials/permissions, supported options, and Pulse branding configuration.
- Document secret preservation/replacement/removal, the one-time need to re-enter already-overwritten webhook tokens, and testing current unsaved settings during create/edit.
- Update `docs/API.md` with native channel configuration and draft-test examples using dummy credentials. Keep OpenAPI synchronized with each implementation change; do not defer contract updates to this final step.
- Explain provider limitations, including Matrix encrypted-room support, Slack sender identity, retry/rate-limit behavior, and possible duplicate messages after ambiguous delivery failures.
- Update `AGENTS.md`, relevant feature specs, `docs/PRODUCT-ROADMAP.md`, and `docs/TECHNICAL-DEBT.md` to reflect shipped behavior and resolved or remaining gaps. Update `.env.example` and environment documentation if configuration changes.
- Verify documented examples, commands, links, and setup instructions against the completed implementation before release.

## Parallel implementation boundaries

1. Agree OpenAPI/config/secret semantics and shared provider interface first.
2. In parallel: backend secret fix and regression tests; frontend secret state/draft-test UI against the agreed contract; provider adapters and local-server contract tests.
3. One integration owner handles shared handler/dispatcher wiring, migration, retry lifecycle, and startup composition to avoid overlapping edits.
4. Integrate provider forms and all locale strings after backend contracts are implemented. Run focused tests during development, then full checks once integrated.
5. Complete the documentation update against the final implemented and verified behavior before release.

Relevant files: `backend/internal/api/handlers/notification_channels.go`, `backend/api/openapi.yaml`, `backend/internal/notification/{dispatcher,retry,types}.go`, `backend/internal/heartbeat/dispatch.go`, `backend/cmd/pulse/main.go`, `backend/cmd/rotate/main.go`, `backend/migrations/`, `frontend/src/components/NotificationChannelForm.svelte`, `frontend/src/lib/{api,types}.ts`, notification routes/binding selectors, and all 13 locale files.

Existing MCP tools do not consume notification endpoints; no MCP co-change is required unless implementation changes a consumed endpoint.

## Verification and release gates

### Regression and API tests

- Add `notification_channels_test.go` using the existing handler fake-DB pattern plus an HTTP receiver. Exercise create -> read -> unrelated edit -> save -> test, asserting the actual outbound Authorization value.
- Cover repeated edits, case-only names, reorder/remove/rename, duplicates, replacement, explicit empty string, missing original, invalid ciphertext, type changes, failed validation preserving state, and concurrent updates.
- Assert plaintext/ciphertext never appears in any channel response, test error, captured application log, or persisted delivery error. Include malformed stored configs and token-bearing transport failures.
- Draft tests must not mutate channels or create logs/retries; test deadlines, concurrency limits, unauthorized access, and both JWT/API-token authentication.
- Validate OpenAPI structurally with a real validator and assert request/response examples against schemas; `make openapi` alone is insufficient.

### Provider and pipeline tests

- Local HTTP servers: exact method/path/auth/body, escaping, length limits, monitor and heartbeat wording/links, trigger variants, missing base URL, optional fields, and Matrix encryption rejection.
- Responses: success, malformed bodies, 400/401/403/404, 429 with cooldown, 5xx, redirects, timeout/cancellation, and oversized response bodies.
- Production integration: fan-out isolation, actual retry scheduling, attempt limits, stable Matrix transaction ID, queue pressure, provider labels, shutdown cancellation, and rotation round trips.
- Run migration up/down tests against disposable PostgreSQL, including behavior when native channel rows exist. Use `sqlc generate` and `sqlc diff` if query definitions change.

### Frontend and complete checks

- Add NotificationChannelForm component tests: saved-secret preservation, replacement/removal/reorder, provider config mapping, draft test with unsaved values, no save/navigation, duplicate-click prevention, results and stale-result clearing.
- All visible text through `t()`, keys in all 13 locales. Inspect populated/empty create/edit/list states, both themes, narrow screens, keyboard focus, dropdown boundaries, Arabic RTL, and Belarusian.
- Run Svelte autofixer for touched components.
- Backend: `go test -race -count=1 ./...` and `go build -o /dev/null ./cmd/pulse`.
- Frontend: `pnpm install`, `pnpm check`, `pnpm validate-locales`, `pnpm test`, `pnpm build`.
- Final opt-in live smoke tests use disposable Slack, Telegram, and Matrix destinations. Mock-server tests cannot prove real account permissions or message appearance. Do not send to real destinations without an explicitly supplied test target.

## Suggestions and defaults

The proposed first version uses Slack incoming webhooks, Telegram bots with optional topics, and Matrix unencrypted bot rooms. Full Matrix E2EE and Slack OAuth provisioning are separate projects. Discord is a natural next rich-message adapter; ntfy/Gotify would complement Pulse's self-hosted audience. Keep those additions out of the first implementation.

## Provider references

- Slack incoming webhooks and identity restrictions: https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/
- Slack rate limits: https://docs.slack.dev/apis/web-api/rate-limits/
- Telegram sendMessage, response envelopes, and rate-limit parameters: https://core.telegram.org/bots/api#sendmessage
- Matrix message transaction IDs: https://spec.matrix.org/latest/client-server-api/#put_matrixclientv3roomsroomidsendeventtypetxnid
- Matrix end-to-end encryption: https://spec.matrix.org/latest/client-server-api/#end-to-end-encryption

## Investigation baseline results

- Passed: `go test -race -count=1 ./internal/notification/... ./internal/api/handlers/...` and backend application build.
- Passed: frontend dependency install, `pnpm check` (0 errors, 57 existing warnings), locale validation (all 13 locales), `pnpm test` (56 files, 587 tests), and production build.
- Svelte autofixer reviewed the existing notification form; its warnings do not explain the token corruption.
- These are baseline checks against existing application code. No fix, provider adapter, new regression test, live-provider send, or browser verification has been implemented/performed in this planning pass. Full backend-wide tests and database migration checks remain implementation release gates.

## Implementation verification

- Worktree: `.worktrees/notification-platforms`, branch `feat/notification-platforms`, based on local `main` at `ef2e8bc`; other worktrees were left untouched.
- Full backend race suite and server build passed. Follow-up notification race tests cover production retry wiring, longest provider cooldowns, final-attempt rate limits, rotation round trips, and legacy empty-header recovery.
- Final frontend check passed with 0 errors and 45 existing warnings outside the notification form. All 599 tests, all 13 locales, and production build passed.
- OpenAPI validated with Redocly's minimal ruleset; only 10 existing unused-component warnings remain. Generated SQL passes `sqlc diff`.
- Browser fixtures exercised 96 create/edit cases across Slack/Matrix/Telegram/Webhook, light/dark, en/ar/be, and desktop/narrow viewports: no document overflow, secret exposure, untranslated keys, or JavaScript exceptions. Native controls meet the 40px minimum. Arabic keyboard selection, dropdown boundaries, LTR operational identifiers, and unsaved test payloads were checked. No real notifications were sent.
- Migration 021 passed its PostgreSQL round-trip test, including safe downgrade refusal. Actual rotation-command integration tests passed against two disposable databases after applying all migrations: all seven encrypted credential types decrypt with the new key, the old key is rejected, persisted webhook credentials deliver successfully, and a late malformed SMTP value rolls back every earlier write. Verification databases were removed; existing application data was not used by these tests.
- The complete Docker image built successfully as `pulse:notification-platforms`.

- After Docker became available, the existing `pulse` development Compose stack was recreated from this feature worktree with named volumes preserved. PostgreSQL reports migration 21 clean, the native channel constraint includes all five types, backend `/healthz` responds successfully, frontend returns HTTP 200 and serves the native-provider form, and the draft test endpoint rejects unauthenticated requests with HTTP 401. The full image is available locally as `pulse:notification-platforms`; development services continue to use their normal Go/Vite workflow.
