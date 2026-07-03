# Implementation Plan: Notification Channels

## Overview

This plan implements asynchronous notification dispatch for Pulse monitors. The work is phased: database schema first, then core backend packages (dispatcher, SMTP, webhook), API handlers, frontend UI, and finally integration wiring. Each task builds incrementally — no orphaned code.

## Tasks

- [x] 1. Database schema and migrations
  - [x] 1.1 Create migration 014_notification_channels up/down SQL
    - Create `backend/migrations/014_notification_channels.up.sql` with tables: `notification_channels`, `channel_bindings`, `delivery_logs`, `smtp_settings`
    - UUID primary keys with `DEFAULT gen_random_uuid()`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
    - Foreign keys with `ON DELETE CASCADE` from `channel_bindings` to `notification_channels` and `monitors`
    - Indexes on `channel_bindings(monitor_id)` and `delivery_logs(channel_id, created_at)`
    - CHECK constraints: `channel_type IN ('email', 'webhook')`, `status IN ('success', 'failure')`, `port BETWEEN 1 AND 65535`
    - UNIQUE constraint on `channel_bindings(channel_id, monitor_id)`
    - Create `backend/migrations/014_notification_channels.down.sql` that fully reverses the up migration
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5_

  - [x] 1.2 Add sqlc queries for notification tables
    - Define queries in `backend/internal/store/postgres/` for: channel CRUD, binding CRUD, delivery log insert/list, SMTP settings CRUD
    - Follow existing sqlc patterns (parameterized queries, proper NULL handling)
    - Include paginated list queries for channels and delivery logs
    - _Requirements: 2.1, 13.1_

- [x] 2. Core notification package — types and interfaces
  - [x] 2.1 Create notification package with core types
    - Create `backend/internal/notification/` package
    - Define types: `DispatcherConfig`, `DeliveryJob`, `TemplateData`, `MonitorData`, `IncidentData`
    - Define `MonitorNotifState` and `StateTracker` struct with `sync.RWMutex`
    - Define `Metrics` struct with Prometheus counter/gauge registrations
    - _Requirements: 14.1, 14.2, 4.6_

  - [x] 2.2 Implement template validation logic
    - Create `backend/internal/notification/webhook/` package
    - Implement `ValidateWebhookTemplate(tmplStr string) error` using `text/template`
    - Implement `extractTemplateVars` to walk template AST and verify against known variable set
    - Implement `isKnownTemplateVar` with the full variable set: monitor.Name, monitor.URL, monitor.Target, Status, PreviousStatus, ResponseTime, Incident.StartedAt, Incident.Duration, Incident.ID, Timestamp
    - _Requirements: 1.4, 1.5, 9.1, 9.2_

  - [ ]* 2.3 Write property test for template validation (Property 2)
    - **Property 2: Webhook template validation round-trip**
    - Use `pgregory.net/rapid` to generate valid/invalid template strings
    - Verify: templates with only known variables pass; templates with unknown variables are rejected with the variable name in error
    - **Validates: Requirements 1.4, 1.5**

  - [x] 2.4 Implement trigger threshold and type validation
    - Create validation functions for trigger types: only accept {monitor_down, monitor_up, degraded, ssl_expiring, n_failures_in_row}
    - Validate threshold ranges: degraded [1, 60000], ssl_expiring [1, 365], n_failures_in_row [1, 100]
    - Return field-level validation errors
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.10_

  - [ ]* 2.5 Write property tests for trigger validation (Properties 5, 6)
    - **Property 5: Trigger threshold boundary validation**
    - **Property 6: Trigger type validation**
    - Use `rapid` to generate threshold values inside/outside valid ranges
    - Verify: valid values accepted, out-of-range rejected with field-level error
    - **Validates: Requirements 3.2, 3.3, 3.4, 3.5, 3.10**

- [x] 3. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Notification dispatcher — state tracking and dispatch
  - [x] 4.1 Implement StateTracker for deduplication
    - Implement `StateTracker` with thread-safe state management
    - Track: `IsDegraded`, `SSLWarned`, `ConsecFailuresFired`, `LastReminderSent` per monitor
    - Implement `Evaluate` logic: check state transitions, fire exactly once per transition, prevent duplicates for ongoing conditions
    - Implement state reset on recovery (down→up clears all flags)
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_

  - [ ]* 4.2 Write property tests for state transitions (Properties 8, 9)
    - **Property 8: State transition notification — exactly once**
    - **Property 9: Ongoing condition deduplication**
    - Use `rapid` to generate sequences of check results with state transitions
    - Verify: exactly one notification per transition, no duplicates for ongoing conditions, re-trigger after recovery
    - **Validates: Requirements 4.1, 4.2, 4.3, 4.4, 4.5**

  - [x] 4.3 Implement Dispatcher with buffered channel and worker pool
    - Implement `Dispatcher` struct with configurable worker count (`PULSE_NOTIFICATION_WORKERS`) and buffer size (256)
    - Implement non-blocking enqueue with drop semantics when buffer full
    - Implement worker goroutine loop: dequeue job, dispatch, log result
    - Register Prometheus metrics: `pulse_notification_deliveries_total`, `pulse_notification_dropped_total`, `pulse_notification_in_flight`, `pulse_notification_retry_queue_size`
    - _Requirements: 14.1, 14.2, 14.3, 4.6, 7.4_

  - [ ]* 4.4 Write property test for non-blocking dispatch (Property 10)
    - **Property 10: Non-blocking dispatch**
    - Verify: `Evaluate` returns within bounded time (<1ms) regardless of buffer state
    - Test with full buffer: notification dropped, counter incremented
    - **Validates: Requirements 4.6, 14.2**

  - [x] 4.5 Implement retry logic with exponential backoff
    - Implement retry queue with in-memory timer
    - Backoff delays: 30s, 60s, 120s (multiplier 2), max 3 retries
    - Classify retryable vs non-retryable failures
    - Mark permanently failed after all retries exhausted
    - _Requirements: 7.1, 7.3, 7.6_

  - [ ]* 4.6 Write property tests for retry and failure classification (Properties 15, 18)
    - **Property 15: Exponential backoff retry**
    - **Property 18: Non-retryable failure classification**
    - Verify: retry delays follow 30s/60s/120s pattern; non-retryable errors skip retry; 3 retries then permanent failure
    - **Validates: Requirements 7.1, 7.3, 7.6**

  - [x] 4.7 Implement delivery logging
    - Record each delivery attempt in `delivery_logs` table via sqlc
    - Include: timestamp, channel_id, monitor_id, trigger_type, attempt number, status, error_detail (truncated to 1024 chars)
    - Implement panic recovery in workers: recover, log, record failure, continue
    - _Requirements: 7.2, 7.5_

  - [ ]* 4.8 Write property tests for delivery logging and panic recovery (Properties 16, 17)
    - **Property 16: Delivery log completeness**
    - **Property 17: Panic recovery continuity**
    - Verify: log entries match total attempts; panics are recovered and logged; processing continues after panic
    - **Validates: Requirements 7.2, 7.5**

  - [x] 4.9 Implement reminder scheduler
    - Implement `ReminderScheduler` with a ticker goroutine
    - Scan active reminders at each tick, re-enqueue notifications at configured interval (5/10/15/30/60 min)
    - Stop reminders when triggering condition resolves within one interval period
    - _Requirements: 4.7, 4.8_

  - [ ]* 4.10 Write property test for reminder intervals (Property 11)
    - **Property 11: Reminder interval correctness**
    - Verify: reminders fire at intervals ≥ I and ≤ I + tick; no reminders after condition resolves
    - **Validates: Requirements 4.7, 4.8**

  - [x] 4.11 Implement graceful shutdown with drain
    - On shutdown signal: stop accepting new jobs, drain buffered channel, wait for in-flight deliveries
    - Configurable drain timeout via `PULSE_NOTIFICATION_DRAIN_TIMEOUT` (default 30s)
    - If timeout expires: cancel remaining, log abandoned count, proceed with shutdown
    - _Requirements: 14.4, 14.5_

  - [ ]* 4.12 Write property test for graceful shutdown (Property 21)
    - **Property 21: Graceful shutdown drain**
    - Verify: pending notifications drained within timeout; timeout expiry cancels remaining and logs count
    - **Validates: Requirements 14.4, 14.5**

- [x] 5. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Email and webhook delivery clients
  - [x] 6.1 Implement SMTP client for email delivery
    - Create `backend/internal/notification/smtp/` package
    - Implement SMTP connection with 30s send timeout, TLS support
    - Load SMTP settings from DB (singleton row), decrypt password with AES-256-GCM
    - Implement HTML email rendering with Pulse-branded template (ECG motif, logo, brand colors)
    - Include in email body: monitor name, target URL, status change, response time, incident ID, started-at, duration
    - Set subject to include monitor name and event type
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.7_

  - [ ]* 6.2 Write property test for email rendering (Property 13)
    - **Property 13: Email rendering completeness**
    - Use `rapid` to generate `TemplateData` with non-empty fields
    - Verify: rendered HTML contains all required fields; subject contains monitor name and event type
    - **Validates: Requirements 5.2, 5.3, 5.7**

  - [x] 6.3 Implement webhook HTTP delivery client
    - Implement webhook delivery: render body template, enforce 1MB max body, send HTTP request
    - Use configured method, include custom headers (decrypted from AES-256-GCM), 10s request timeout
    - Set `Content-Type: application/json` when no explicit Content-Type header configured
    - Handle template render failures: log error, skip delivery, no retry, record in delivery_log
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7_

  - [ ]* 6.4 Write property tests for webhook request construction (Property 14)
    - **Property 14: Webhook request construction**
    - Use `rapid` to generate channel configs and `TemplateData`
    - Verify: correct method used, headers included, Content-Type defaulting, body ≤ 1MB
    - **Validates: Requirements 6.1, 6.2**

  - [x] 6.5 Implement independent fan-out delivery
    - When multiple bindings match a trigger, dispatch to all independently
    - Failure in one binding must not prevent delivery to others
    - _Requirements: 4.9_

  - [ ]* 6.6 Write property test for fan-out (Property 12)
    - **Property 12: Independent fan-out delivery**
    - Verify: N bindings → N delivery attempts; failure in binding K does not block others
    - **Validates: Requirements 4.9**

- [x] 7. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. API handlers — channel and binding CRUD
  - [x] 8.1 Implement notification channel CRUD handlers
    - Create `backend/internal/api/handlers/notification_channels.go`
    - Implement: Create (POST), List (GET paginated), Get (GET by ID), Update (PUT full replacement), Delete (DELETE with cascade)
    - Validate channel name (1-100 chars), type (email/webhook), type-specific config
    - Email: validate 1-50 recipients, RFC 5322 format, max 254 chars each
    - Webhook: validate URL (http/https, max 2048), method (GET/POST/PUT/PATCH/DELETE), body template, 0-20 headers (name ≤128, value ≤8192)
    - Encrypt webhook header values with AES-256-GCM on create/update
    - On GET: redact header values with `[REDACTED]` placeholder
    - Return field-level validation errors on failure
    - _Requirements: 1.1, 1.2, 1.3, 1.6, 1.7, 1.8, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

  - [x] 8.2 Implement notification binding CRUD handlers
    - Create `backend/internal/api/handlers/notification_bindings.go`
    - Implement: Create (POST), List (GET), Update (PUT), Delete (DELETE)
    - Validate: at least one trigger required, valid trigger types and thresholds
    - Validate channel and monitor existence, return 404 if not found
    - Enforce unique channel-monitor constraint, return 409 on duplicate
    - Support optional reminder_interval_minutes (5/10/15/30/60 or null for disabled)
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10_

  - [x] 8.3 Implement test notification endpoint
    - Create `POST /api/v1/notifications/channels/:id/test` handler
    - Dispatch with sample data (all Template_Variables populated with type-correct values, monitor name prefixed with "[TEST]")
    - Return synchronous result within 10s timeout
    - Return failure cause on error (connection error, auth failure, timeout, non-2xx)
    - Do NOT record in delivery_logs, do NOT queue for retry
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [ ]* 8.4 Write property test for test notification isolation (Property 19)
    - **Property 19: Test notification isolation**
    - Verify: test notifications do not create delivery_log records; failed tests not queued for retry
    - **Validates: Requirements 8.6**

  - [x] 8.5 Implement template variables reference endpoint
    - Create `GET /api/v1/notifications/template-variables` handler
    - Return all Template_Variables with: name, Go type, description, example value
    - Group by dot-notation prefix (monitor, Incident, top-level)
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [x] 8.6 Implement SMTP settings CRUD and test handlers
    - Create handlers for: `GET /api/v1/notifications/smtp-settings`, `PUT`, `DELETE`, `POST .../test`
    - GET: return host, port, username, from_address, tls_enabled, `password_set: bool` — never return raw password
    - PUT: validate port (1-65535), from_address (RFC 5322), encrypt password with AES-256-GCM
    - DELETE: remove SMTP settings row, disabling email notifications
    - POST test: validate connectivity (TCP + EHLO + optional AUTH), return synchronous result
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.9_

  - [ ]* 8.7 Write property test for SMTP status redaction (Property 20)
    - **Property 20: SMTP status redaction**
    - Verify: GET response never includes username or password values; includes only host, port, from_address when configured
    - **Validates: Requirements 12.3**

  - [x] 8.8 Register notification handlers in API router
    - Register `NotificationChannelHandler` and `NotificationBindingHandler` in `backend/internal/api/router.go`
    - Pass required dependencies: queries, pool, dispatcher, secretKey
    - All endpoints under `/api/v1` prefix with combined auth middleware
    - _Requirements: 1.1, 3.1_

- [x] 9. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 10. Scheduler integration and SMTP startup
  - [x] 10.1 Integrate dispatcher with scheduler
    - Add `Dispatcher` field to scheduler struct
    - Call `dispatcher.Evaluate(ctx, monitorID, previousState, result)` after `UpdateMonitorState` and hub broadcast
    - Ensure call is non-blocking (Evaluate writes to buffered channel)
    - _Requirements: 4.1, 4.6_

  - [x] 10.2 Implement SMTP startup validation
    - On startup: read SMTP settings from DB
    - If configured: validate connectivity (EHLO handshake, 10s timeout), log result
    - If not configured: log info "email notifications disabled"
    - If validation fails: log warning, mark unavailable, continue startup without termination
    - _Requirements: 12.6, 12.7, 12.8_

  - [x] 10.3 Wire dispatcher initialization in main.go
    - Initialize `Dispatcher` with config from env vars (`PULSE_NOTIFICATION_WORKERS`, `PULSE_NOTIFICATION_DRAIN_TIMEOUT`)
    - Start worker pool and reminder scheduler
    - Register graceful shutdown hook for dispatcher drain
    - Pass dispatcher to scheduler and API handlers
    - _Requirements: 14.1, 14.4_

- [x] 11. OpenAPI specification update
  - [x] 11.1 Update backend/api/openapi.yaml with notification endpoints
    - Add path entries for: channel CRUD, binding CRUD, test notification, template variables, SMTP settings
    - Define schemas: NotificationChannel, ChannelBinding, DeliveryLog, TriggerCondition, ReminderPolicy, TemplateVariable
    - Document error responses using existing error envelope schema
    - Add pagination parameters on list endpoints
    - Include X-Request-ID response header on all endpoints
    - Ensure spec passes OpenAPI 3.0.3 validation
    - _Requirements: 15.1, 15.2, 15.3, 15.4, 15.5_

- [x] 12. Frontend — notifications page and channel management
  - [x] 12.1 Create notification stores and API client methods
    - Add notification API methods to `frontend/src/lib/api.ts`: channel CRUD, test, template variables, binding CRUD, SMTP settings
    - Create `frontend/src/lib/stores/notifications.svelte.ts` with reactive state for channels list, pagination, loading states
    - _Requirements: 10.1, 10.2_

  - [x] 12.2 Add i18n keys for notification UI
    - Add all notification-related keys to `frontend/src/locales/en.json` under `notifications.*` namespace
    - Include keys for: page title, channel list, form fields, triggers, reminders, empty states, toast messages, SMTP settings
    - Propagate keys to all 12 other locale files with English placeholders
    - _Requirements: 10.7, 11.7_

  - [x] 12.3 Create Notifications page with channel list
    - Create `frontend/src/routes/notifications/+page.svelte`
    - Add "Notifications" entry in main navigation after "Monitors"
    - Render paginated channel list with name, type, creation date (default page size 20)
    - Display empty state with create prompt when no channels exist
    - Include edit, delete (with confirmation dialog), and test actions per channel
    - Show test result as toast notification (success/failure with error detail)
    - Use `t()` for all user-visible strings
    - _Requirements: 10.1, 10.2, 10.5, 10.6, 10.8_

  - [x] 12.4 Create channel form component (create/edit)
    - Create `frontend/src/components/NotificationChannelForm.svelte`
    - Type selector: email or webhook
    - Email fields: recipient email addresses (multi-input, 1-50)
    - Webhook fields: URL, HTTP method dropdown, custom headers (add/remove, max 20), body template editor (monospace)
    - Show template variable reference panel when webhook type selected
    - Client-side validation matching API constraints
    - _Requirements: 10.3, 10.4_

  - [ ]* 12.5 Write frontend property tests for channel validation (Property 1)
    - **Property 1: Channel creation validation**
    - Use `fast-check` to generate valid/invalid channel payloads
    - Verify: form → payload mapping produces correct structure; invalid inputs show validation errors
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.6, 1.7, 1.8**

- [x] 13. Frontend — per-monitor binding configuration
  - [x] 13.1 Create binding configuration section in monitor detail view
    - Add "Notifications" section to monitor detail/edit view
    - Allow selecting channels to bind with add/remove actions (remove with confirmation)
    - Display trigger condition options per binding: checkboxes for monitor_down/monitor_up, numeric inputs for degraded (1-60000ms), ssl_expiring (1-365 days), n_failures_in_row (1-100)
    - Reminder policy dropdown per binding: disabled, 5, 10, 15, 30, 60 minutes
    - Validate at least one trigger selected on save
    - Show message directing to Notifications page if no channels exist
    - Use `t()` for all user-visible strings
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7, 11.8_

  - [ ]* 13.2 Write frontend property tests for binding uniqueness (Property 7)
    - **Property 7: Binding uniqueness constraint**
    - Use `fast-check` to generate binding operations
    - Verify: duplicate channel-monitor pairs rejected; distinct pairs accepted
    - **Validates: Requirements 3.8**

  - [ ]* 13.3 Write frontend property tests for update behavior (Properties 3, 4)
    - **Property 3: Channel update is full replacement**
    - **Property 4: Failed update preserves state**
    - Verify: PUT followed by GET returns new config exactly; failed PUT leaves state unchanged
    - **Validates: Requirements 2.2, 2.4, 2.6**

- [x] 14. Frontend — SMTP settings page
  - [x] 14.1 Create SMTP settings section in Settings page
    - Add SMTP configuration form to Settings page: host, port, username, password, from address, TLS toggle
    - Show `password_set` indicator (never display raw password)
    - Include test connectivity button with synchronous result display
    - Include delete SMTP settings action with confirmation
    - Use `t()` for all user-visible strings
    - _Requirements: 12.1, 12.3, 12.4, 12.5_

- [x] 15. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 16. Integration wiring and final validation
  - [x] 16.1 Add environment variables to .env.example
    - Add `PULSE_NOTIFICATION_WORKERS` (default: 10 dev, 50 prod)
    - Add `PULSE_NOTIFICATION_DRAIN_TIMEOUT` (default: 30s)
    - Document in comments alongside existing env vars
    - _Requirements: 14.1, 14.4_

  - [x] 16.2 Ensure cascade delete behavior for monitor deletion
    - Verify that existing monitor DELETE handler cascades to channel_bindings (via FK ON DELETE CASCADE)
    - No code change needed if FK constraint is correct; verify with integration test
    - _Requirements: 3.7_

  - [ ]* 16.3 Write integration tests for full dispatch flow
    - Test: scheduler → dispatcher.Evaluate → state tracker → worker → delivery → delivery_log
    - Test: webhook delivery with httptest server (success and failure paths)
    - Test: channel/binding cascade delete
    - Test: migration up/down cycle
    - **Validates: Requirements 4.1, 4.6, 7.2, 3.7, 13.1**

- [x] 17. Final checkpoint
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation between phases
- Property tests use `pgregory.net/rapid` (Go backend) and `fast-check` (frontend) — both already established in the project
- The dispatcher integrates with the scheduler via a single non-blocking `Evaluate` call after check completion
- AES-256-GCM encryption uses the existing `PULSE_SECRET_KEY` — no new crypto infrastructure needed
- SMTP settings are stored in DB (not env vars) to support UI-based configuration
- All frontend strings use `t()` with keys propagated to all 13 locale files

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "2.1"] },
    { "id": 2, "tasks": ["2.2", "2.4"] },
    { "id": 3, "tasks": ["2.3", "2.5", "4.1"] },
    { "id": 4, "tasks": ["4.2", "4.3"] },
    { "id": 5, "tasks": ["4.4", "4.5", "4.7", "4.9"] },
    { "id": 6, "tasks": ["4.6", "4.8", "4.10", "4.11", "6.1", "6.3"] },
    { "id": 7, "tasks": ["4.12", "6.2", "6.4", "6.5"] },
    { "id": 8, "tasks": ["6.6", "8.1", "8.2"] },
    { "id": 9, "tasks": ["8.3", "8.5", "8.6"] },
    { "id": 10, "tasks": ["8.4", "8.7", "8.8"] },
    { "id": 11, "tasks": ["10.1", "10.2", "10.3", "11.1"] },
    { "id": 12, "tasks": ["12.1", "12.2"] },
    { "id": 13, "tasks": ["12.3", "12.4", "14.1"] },
    { "id": 14, "tasks": ["12.5", "13.1"] },
    { "id": 15, "tasks": ["13.2", "13.3", "16.1", "16.2"] },
    { "id": 16, "tasks": ["16.3"] }
  ]
}
```
