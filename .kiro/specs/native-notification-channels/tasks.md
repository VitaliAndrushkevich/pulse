# Native notifications implementation

- [x] Preserve unchanged webhook secrets on edit and encrypt explicit empty values.
- [x] Add fail-closed response projection and regression coverage for the edit/test lifecycle.
- [x] Add draft test API and form action using unsaved values without persistence.
- [x] Add native Slack, Telegram, and Matrix configuration, encryption, safe responses, and delivery.
- [x] Expand channel-type constraint with migration 021; downgrade refuses native rows.
- [x] Wire bounded retries and provider cooldowns; preserve Matrix transaction IDs.
- [x] Include notification secrets and SMTP passwords in atomic key rotation.
- [x] Add resource kind/trigger presentation and heartbeat links.
- [x] Add form tests and all 13 locale keys; inspect themes, RTL, narrow layouts, and keyboard controls.
- [x] Update OpenAPI, README, API guide, provider setup/runbook, AGENTS, roadmap, and technical debt.
- [x] Run migration up/down and the actual rotation command against disposable PostgreSQL; verify all seven secret types, new-key delivery, and rollback on malformed ciphertext.
- [ ] Send opt-in live smoke tests to disposable Slack, Telegram, and Matrix destinations (credentials not supplied).

Requirements/design and regression matrix: [implementation plan](../../../docs/NOTIFICATION-PLATFORMS-PLAN.md).
User setup and operational limits: [notifications guide](../../../docs/NOTIFICATIONS.md).
