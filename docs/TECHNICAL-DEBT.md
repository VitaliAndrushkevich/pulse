# Technical debt and operational reliability

English | [Русский](TECHNICAL-DEBT.ru.md)

Reviewed: 2026-09-20. Baseline: `6fe74dd`.

This backlog records findings and recommendations from the September repository review. All items are **open**. It describes work to plan; it does not claim that fixes have shipped. New product capabilities are tracked separately in [Product roadmap](PRODUCT-ROADMAP.md).

The review covered the scheduler, notification pipeline, API, frontend state handling, deployment, CI, and documentation. Evidence is primarily static code inspection. The notification, SMTP, and webhook test suites passed during the review; that does not establish coverage of the integration failures below. Frontend checking could not complete because the local `svelte-kit` executable was unavailable. No full deployment, failure-injection exercise, or load test was performed.

Read the earlier [July audit](AUDIT-Jul-2026.md) as historical input, and revalidate its findings against current code before creating duplicate work. This document is not a complete security audit.

## Priorities and delivery order

P1 means address in the next reliability milestone. P2 means address before relying on the affected capability at scale or for company adoption. Priorities describe delivery order, not vulnerability severity. Effort is relative: S is a contained change, M crosses several components, and L changes persistence or lifecycle semantics. These are estimates, not deadlines.

| ID | Work | Priority | Effort | Evidence |
|---|---|---|---|---|
| TD-001 | Prevent overlapping monitor executions | P1 | M | Code finding |
| TD-002 | Commit check results and state transitions consistently | P1 | L | Code finding |
| TD-003 | Isolate notification conditions and deduplication per binding | P1 | M | Code finding |
| TD-004 | Connect and recover the reminder lifecycle | P1 | M | Code finding |
| TD-005 | Persist notification jobs and retries | P1 | L | Code finding and reliability design gap |
| TD-006 | Coordinate shutdown and expose meaningful readiness | P1 | M | Code finding |
| TD-007 | Correct realtime reconciliation and pagination | P1 | M | Code finding |
| TD-008 | Rotate every encrypted credential type | P1 | M | Code finding |
| TD-009 | Expose freshness and observation coverage | P2 | M | Metric semantics and product reliability gap |
| TD-010 | Measure capacity under failures and historical load | P2 | M | Validation gap |
| TD-011 | Prove installation, upgrade, backup, and restore | P2 | M | Operational validation gap |
| TD-012 | Test cross-component contracts and user journeys | P2 | M | Integration coverage gap |

Start with TD-001, TD-003, TD-006, TD-007, and TD-008. Design TD-002 and TD-005 together; implement TD-004 on the resulting persistent lifecycle. Complete TD-009 before availability reporting, and use TD-010–012 as release gates. Focused regression tests belong with each fix rather than waiting for TD-012.

## TD-001: Prevent overlapping monitor executions

**Evidence:** [scheduler.go](../backend/internal/monitor/scheduler.go) polls every second. [Due-monitor queries](../backend/internal/store/postgres/queries/monitors.sql) select eligible rows without claiming them, and `next_check_at` is advanced after execution. There is no visible in-flight guard spanning queued and running jobs.

**Impact:** A slow monitor can be enqueued repeatedly before its first execution completes. Duplicate work consumes capacity and can produce conflicting state transitions and repeated alerts. Bounded worker count alone does not prevent this.

**Work:** Establish execution ownership before enqueueing, release it on every exit path, and define behavior when a monitor is edited, paused, or deleted while queued or running. Define whether cadence is measured from scheduled time, start, or completion. Use an in-process guard if single-scheduler operation is the supported boundary; introduce database leases only if multiple schedulers are intentionally supported. Document that boundary.

**Acceptance:** A check taking several polling intervals has at most one queued/running execution per monitor. Overload does not create duplicate backlog or indefinitely starve other monitors. Pause/delete and stale configuration results have explicit behavior. Restart and cancellation do not leave monitors permanently unscheduled.

## TD-002: Commit check results and state transitions consistently

**Evidence:** [executeCheck](../backend/internal/monitor/scheduler.go) writes history, manages incidents, and updates monitor state through separate operations. Errors are logged, and later operations, WebSocket broadcasts, and notification dispatch can continue.

**Impact:** Partial database failures can leave history, current state, incidents, and delivered notifications describing different outcomes. An older overlapping execution can also overwrite newer state.

**Work:** Define a transaction boundary for a check result, incident transition, current state, and durable notification intent. Publish realtime updates after commit. Establish an execution identity and ordering policy so stale results cannot replace newer state. Coordinate with TD-001 and TD-005.

**Acceptance:** Failure injection at each write boundary leaves a consistent committed outcome. Reprocessing the same execution does not create duplicate transitions/jobs. A failed commit does not produce a successful state broadcast. Recovery from transient database failure is tested.

## TD-003: Isolate notification conditions per binding

**Evidence:** [state_tracker.go](../backend/internal/notification/state_tracker.go) shares flags such as `IsDegraded` across a monitor's conditions. Trigger events contain only a type; [fanout.go](../backend/internal/notification/fanout.go) matches bindings by that type.

**Impact:** With thresholds of 100 ms and 1,000 ms, a 200 ms result can notify both bindings. Shared flags can also make deduplication depend on condition ordering.

**Work:** Carry binding/condition identity through evaluation and delivery, and track deduplication and recovery at that same scope. Define how editing or deleting a condition affects its state.

**Acceptance:** Different latency, certificate-expiry, and failure-count thresholds behave independently. Reordering bindings does not change delivery. Each matching binding receives the intended event; unmatched bindings do not. Recovery, repeated checks, and rule edits are covered.

## TD-004: Connect and recover the reminder lifecycle

**Evidence:** [main.go](../backend/cmd/pulse/main.go) starts a reminder scheduler, but the review found no production caller of `ActivateReminder`. [Binding conversion](../backend/internal/monitor/scheduler.go) does not carry reminder interval configuration into `BindingWithChannel`.

**Impact:** Reminder implementation exists but the delivery flow does not appear to activate it. Users may configure reminders that never arrive.

**Work:** Connect activation, refresh, and deactivation to the actual binding condition lifecycle. Persist reminder due times with TD-005. Define pause, recovery, channel deletion, and binding edits without relying on a heuristic that state merely exists.

**Acceptance:** A configured binding sends an initial alert and subsequent reminders at the expected interval. Recovery stops reminders. Restart restores active reminders without an uncontrolled catch-up burst. Deleted/disabled bindings do not deliver. Verify through scheduler-to-delivery integration, not only direct calls to reminder methods.

## TD-005: Persist notification jobs and retries

**Implementation update (2026-10-23):** Retry scheduling is now connected to production failures, with native provider cooldowns and bounded shutdown. Persistence remains deferred.

**Evidence:** [dispatcher.go](../backend/internal/notification/dispatcher.go) drops jobs when the channel is full; [retry.go](../backend/internal/notification/retry.go) uses in-memory timers and can drop a retry when re-enqueueing. Pending jobs and timers do not survive process loss.

**Impact:** A widespread outage is precisely when many notifications may be generated. Delivery can be lost under that load or across a deployment. Delivery logs alone do not recover pending work.

**Work:** Store notification intent in a PostgreSQL outbox in the TD-002 transaction. Bounded workers claim due jobs; retries persist attempt count, due time, and final outcome. Define backlog limits, retention, and visible overload behavior. Keep network delivery outside monitoring workers. Persist binding condition state where necessary for restart-safe deduplication.

**Acceptance:** Committed pending jobs survive termination and restart. A full in-memory buffer does not erase persisted work. Retry exhaustion and delivery failures are inspectable. Document at-least-once delivery: a crash after a remote endpoint accepts a request can cause a duplicate. Use stable event IDs and destination idempotency where supported; do not promise exactly-once email/webhook delivery. Monitor backlog age as well as count.

## TD-006: Coordinate shutdown and meaningful readiness

**Evidence:** [main.go](../backend/cmd/pulse/main.go) handles signals by stopping background components but does not stop the HTTP server started with `r.Run()`. Notifications are drained before the scheduler is cancelled. [Health routes](../backend/internal/api/router.go) return static success responses.

**Impact:** The process can continue serving after monitoring stops, until externally killed. Checks can generate work while notification delivery is already shutting down. A successful health response does not establish that checks can be scheduled or persisted.

**Work:** Own an explicit HTTP server and coordinated application lifecycle. Mark unready, stop accepting new work, quiesce producers, persist/drain deliveries, stop the hub, and wait for workers within a defined timeout. Separate liveness from readiness, including database availability and scheduler progress. Add appropriate server timeouts.

**Acceptance:** SIGTERM exits within the documented grace period. No new checks start after quiescing. Pending alert intent remains recoverable. HTTP requests and WebSockets have tested shutdown behavior. Database failure and a stalled scheduler affect readiness without confusing resource outages with Pulse failure.

## TD-007: Correct realtime reconciliation and pagination

**Evidence:** [Layout reconnect handling](../frontend/src/routes/+layout.svelte) requests `getMonitors(1, 500)`, whereas [parsePagination](../backend/internal/api/handlers/helpers.go) only accepts limits through 100 and defaults invalid values to 20. The returned page replaces the monitor store.

**Impact:** Reconnection can leave the store with only the first 20 monitors. The system cannot rely on that request as a full reconciliation for 500+ resources.

**Work:** Choose either complete paginated reconciliation or explicit refresh of relevant pages/resources. Define how patches arriving during a refresh interact with snapshots, and reject stale updates. Verify the list and detail routes consume reconciled state consistently. Preserve bounded rendering and virtualization wherever collections are large.

**Acceptance:** With 501+ monitors, reconnect correctly refreshes all intended resources, including those outside page one. Newer patches cannot be overwritten by an older response. Creates, deletes, missed updates, and active filters behave consistently. Requests respect the API contract; error handling does not silently leave a falsely current view.

## TD-008: Rotate every encrypted credential type

**Implementation update (2026-10-23):** [The rotation command](../backend/cmd/rotate/main.go) now includes SMTP passwords, webhook headers, Slack webhook URLs, Telegram bot tokens, and Matrix access tokens alongside legacy secrets and monitor credentials. Channel ciphertext round trips and failure paths have regression coverage. [The runbook](NOTIFICATIONS.md#upgrade-and-key-rotation) requires stopping all Pulse writers. Database-backed tests now apply all migrations, execute the actual rotation binary for all seven encrypted types, reject the old key, deliver a webhook using persisted new-key credentials, and verify complete rollback on a late malformed SMTP value.

**Status:** Credential coverage and atomic failure behavior verified against disposable PostgreSQL. Concurrent rotation with running writers remains unsupported; the runbook requires downtime. A full application restart with real provider accounts remains an operational smoke test.

**Work:** Inventory encrypted fields and rotate them as a complete operation. Document whether rotation requires downtime and how concurrent writes are prevented or handled. Define rollback and key replacement order; avoid printing secrets or decrypted values.

**Acceptance:** Populate every encrypted credential type, rotate, restart using only the new key, and exercise authenticated checks plus notification delivery. An injected failure leaves all data recoverable with the documented key. Verify recovery without exposing raw secrets.

## TD-009: Expose freshness and observation coverage

**Evidence:** [Monitor statistics](../backend/internal/api/handlers/monitor_stats.go) compute successful checks divided by recorded checks. Periods with no results are absent from that denominator. Current state is not sufficient evidence of a recent successful observation.

**Impact:** A monitor can show a high success percentage despite collection gaps. Future availability reports could imply more coverage than was actually observed.

**Work:** Distinguish last observed state from freshness. Document check-success ratio separately from time-based availability. Define overdue/unknown periods, coverage, interval changes, planned maintenance, partial retention windows, and insufficient data. Propagate semantics through API, OpenAPI, UI, and MCP.

**Acceptance:** Stopping Pulse creates a visible observation gap rather than invented healthy history. A short-lived monitor is not presented as having complete 30-day coverage. Paused and maintenance periods follow explicit rules. Calculations have deterministic examples that include missing checks and retention boundaries.

## TD-010: Measure capacity under failures and historical load

**Evidence:** Bounded workers and TimescaleDB are good foundations, but the review did not establish measured 500+ monitor capacity or find a dedicated load/soak harness in the inspected files. [Pool setup](../backend/internal/store/postgres/pool.go) leaves sizing to pgx/connection configuration; size from measurements, not worker count alone.

**Work:** Add reproducible workload definitions and measure scheduling lateness, duplicate work, worker occupancy, database acquisition delay, persistence errors, alert backlog age, WebSocket drops, CPU, memory, and disk growth. Test healthy targets, widespread timeouts, slow databases, alert bursts, reconnects, and history queries during ingestion.

At 500 monitors checked every 60 seconds, expected volume is 720,000 results/day and 21.6 million results/30 days, before indexes and other records. These are arithmetic estimates, not measured throughput or disk requirements. Exercise retention cleanup against a backlog; evaluate aggregation and storage changes only after profiling.

**Acceptance:** Publish hardware, versions, configuration, workload, history size, duration, and results. Agree on latency and resource budgets before testing. Run a 24-hour soak with stable workloads and repeatable fault scenarios. Demonstrate bounded queues/memory and recovery after overload. Avoid an unqualified capacity guarantee based on worker count.

## TD-011: Prove installation, upgrade, backup, and restore

**Evidence:** Compose and release artifacts exist, but this review did not validate a full installation, upgrade, or disaster-recovery path. The production Compose file uses the moving `main` image tag; reviewed README/CI/Compose files also describe or use different PostgreSQL/TimescaleDB versions.

**Work:** Document a tested supported deployment combination and stable release upgrade path. Provide backup/restore instructions covering database data and separately protected encryption-key recovery. Test migrations against the advertised database versions, and describe recovery from an unsuccessful upgrade. Validate first-admin creation and first notification using the documented quick start.

**Acceptance:** A clean host can install from the docs, create a monitor, and receive a test alert. An independent restore recreates configuration/history and decrypts credentials. A prior supported release upgrades successfully with representative data. Document recovery duration observed in the exercise and version limitations. Reconcile documentation with actual artifacts.

## TD-012: Test cross-component contracts and user journeys

**Evidence:** Existing notification tests passed, yet the reviewed binding/reminder integration gaps remain. The PR workflow runs MCP checks only for `mcp/**` changes even though backend changes can affect the consumed contract.

**Work:** Add focused scheduler-to-history-to-incident-to-delivery tests and browser smoke coverage for setup, monitor creation, failure/recovery, and reconnect. Check OpenAPI and consumed MCP response projections when backend contracts change. Test against a pinned supported database image. Avoid treating test counts as evidence of lifecycle coverage.

**Acceptance:** The TD-001–008 regression scenarios fail against the affected behavior and pass after fixes. Backend contract changes trigger relevant MCP verification. A release smoke test exercises embedded frontend assets with the real API. Report skipped integration checks explicitly.

## Completion policy

An item is complete when its acceptance evidence, relevant tests, and operator-facing documentation are linked from its issue or PR. Endpoint/schema/error changes must update OpenAPI in the same change; consumed endpoints must also update MCP clients, tools, and projections as required by project guidelines. UI text must follow localization and theming rules.

The previous review's feature recommendations—maintenance windows, recovery thresholds, bulk management, heartbeat checks, status pages, remote probes, and team access—are covered in the [product roadmap](PRODUCT-ROADMAP.md). They are proposed capabilities, not bugs in an already promised implementation.
