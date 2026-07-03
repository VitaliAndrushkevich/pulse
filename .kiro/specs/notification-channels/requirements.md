# Requirements Document

## Introduction

Notification channels enable Pulse users to receive alerts when monitors change state. The module introduces a channel-and-binding architecture: users create reusable notification channels (email or webhook), bind them to monitors, and configure per-monitor trigger conditions. This supports alerting for downtime, recovery, degraded performance, SSL expiry, and consecutive failure thresholds with optional reminder policies.

## Glossary

- **Notification_Channel**: A reusable configuration entity representing a delivery method (email or webhook) with its connection parameters.
- **Channel_Binding**: A many-to-many relationship linking a Notification_Channel to a Monitor with per-binding trigger conditions.
- **Trigger_Condition**: A rule specifying which monitor state transitions or thresholds cause a notification to fire (e.g., monitor_down, monitor_up, degraded, ssl_expiring, n_failures_in_row).
- **Reminder_Policy**: A per-binding configuration that repeats notifications at a fixed interval while the triggering condition persists.
- **Notification_Dispatcher**: The backend component responsible for asynchronously sending notifications without blocking the scheduler.
- **Webhook_Template**: A Go template string used to render the HTTP body of a webhook notification, with access to predefined template variables.
- **Template_Variable**: A named data point available inside webhook templates (e.g., `monitor.Name`, `Status`, `ResponseTime`).
- **SMTP_Settings**: Instance-level SMTP server configuration used by all email channels.
- **Delivery_Log**: A record of each notification dispatch attempt, including success/failure status and retry metadata.
- **Notification_API**: The REST API surface under `/api/v1` for managing channels, bindings, and testing notifications.

## Requirements

### Requirement 1: Create Notification Channel

**User Story:** As a Pulse administrator, I want to create notification channels with type-specific configuration, so that I can define reusable alert delivery endpoints.

#### Acceptance Criteria

1. WHEN a valid channel creation request is received, THE Notification_API SHALL create a Notification_Channel with a stable UUID, require a channel name between 1 and 100 characters, and return the created resource.
2. WHEN the channel type is "email", THE Notification_API SHALL require between 1 and 50 recipient email addresses, each conforming to RFC 5322 format and not exceeding 254 characters.
3. WHEN the channel type is "webhook", THE Notification_API SHALL require a URL with an http or https scheme (maximum 2048 characters), an HTTP method from the set (GET, POST, PUT, PATCH, DELETE), and a body template in the configuration.
4. WHEN a webhook body template is provided, THE Notification_API SHALL validate the template by parsing it as a Go template and verifying all referenced variables exist in the Template_Variable set.
5. IF an invalid template is submitted, THEN THE Notification_API SHALL return a descriptive error identifying the parsing failure or unknown variable.
6. IF a required field is missing or invalid, THEN THE Notification_API SHALL return a validation error with field-level detail.
7. THE Notification_API SHALL accept up to 20 optional custom HTTP headers for webhook channels, each with a header name no longer than 128 characters and a header value no longer than 8192 characters.
8. IF the channel type is not "email" or "webhook", THEN THE Notification_API SHALL return a validation error indicating the unsupported channel type.

### Requirement 2: Manage Notification Channels

**User Story:** As a Pulse administrator, I want to list, view, update, and delete notification channels, so that I can maintain my alert configuration over time.

#### Acceptance Criteria

1. THE Notification_API SHALL provide a paginated list endpoint for all Notification_Channels with default values of page=1 and limit=20, accepting a maximum limit of 100.
2. WHEN a channel update request is received, THE Notification_API SHALL treat the request as a full replacement of the channel configuration and validate it using the same rules as creation.
3. WHEN a channel is deleted, THE Notification_API SHALL remove the channel and all associated Channel_Bindings in a single cascading operation without requiring separate unbind calls.
4. THE Notification_API SHALL return the full channel configuration on GET requests, replacing any encrypted header values with a fixed redacted placeholder string.
5. IF a GET, update, or delete request references a channel UUID that does not exist, THEN THE Notification_API SHALL return an error indicating the channel was not found.
6. IF a channel update request fails validation, THEN THE Notification_API SHALL return a validation error with field-level detail and leave the existing channel unchanged.

### Requirement 3: Bind Channels to Monitors

**User Story:** As a Pulse administrator, I want to bind notification channels to specific monitors with trigger conditions, so that I receive alerts only for the events I care about on each monitor.

#### Acceptance Criteria

1. WHEN a binding request is received with valid Notification_Channel and Monitor references, THE Notification_API SHALL create a Channel_Binding linking the specified Notification_Channel to the specified Monitor and return the created resource.
2. THE Notification_API SHALL require at least one Trigger_Condition per Channel_Binding, accepting only the defined trigger types: monitor_down, monitor_up, degraded, ssl_expiring, and n_failures_in_row.
3. WHEN the trigger "degraded" is configured, THE Notification_API SHALL require a response time threshold as an integer between 1 and 60000 milliseconds.
4. WHEN the trigger "ssl_expiring" is configured, THE Notification_API SHALL require a days-before-expiry threshold as an integer between 1 and 365.
5. WHEN the trigger "n_failures_in_row" is configured, THE Notification_API SHALL require a consecutive failure count threshold as an integer between 1 and 100.
6. THE Notification_API SHALL accept an optional Reminder_Policy with interval selection (5, 10, 15, 30, or 60 minutes, or disabled).
7. WHEN a monitor is deleted, THE Notification_API SHALL remove all Channel_Bindings associated with that monitor.
8. THE Notification_API SHALL allow multiple channels to be bound to the same monitor, and the same channel to be bound to multiple monitors, while preventing duplicate bindings for the same channel-monitor-trigger combination.
9. IF a binding request references a non-existent Notification_Channel or Monitor, THEN THE Notification_API SHALL return a validation error indicating which referenced resource was not found.
10. IF a binding request contains a threshold value outside the allowed range or a missing required threshold for its trigger type, THEN THE Notification_API SHALL return a validation error with field-level detail.

### Requirement 4: Notification Dispatch on State Change

**User Story:** As a Pulse administrator, I want notifications dispatched automatically when a monitor changes state, so that I am alerted without manual intervention.

#### Acceptance Criteria

1. WHEN a monitor transitions to a "down" state from any non-down state and one or more Channel_Bindings with "monitor_down" trigger exist, THE Notification_Dispatcher SHALL send a notification through each bound channel.
2. WHEN a monitor transitions from "down" to "up" and one or more Channel_Bindings with "monitor_up" trigger exist, THE Notification_Dispatcher SHALL send a recovery notification through each bound channel.
3. WHEN a check result response time first exceeds the configured "degraded" threshold (transitioning from non-degraded to degraded state) and a Channel_Binding with "degraded" trigger exists, THE Notification_Dispatcher SHALL send a single degradation notification and SHALL NOT send additional degradation notifications for subsequent checks while the monitor remains in the degraded state.
4. WHEN an SSL certificate days-remaining first falls below the configured "ssl_expiring" threshold and a Channel_Binding with "ssl_expiring" trigger exists, THE Notification_Dispatcher SHALL send a single SSL expiry warning notification and SHALL NOT repeat the notification on subsequent checks while the certificate remains below the threshold.
5. WHEN a monitor accumulates consecutive failures reaching the configured "n_failures_in_row" threshold for the first time since last recovery, THE Notification_Dispatcher SHALL send a single consecutive-failure notification and SHALL NOT repeat the notification for additional consecutive failures beyond the threshold until the monitor recovers.
6. THE Notification_Dispatcher SHALL execute notification delivery asynchronously without blocking the scheduler or checker goroutines.
7. WHILE a Reminder_Policy is active and the triggering condition persists, THE Notification_Dispatcher SHALL repeat the notification at the configured interval, measured from the last successful or attempted delivery regardless of delivery outcome.
8. WHEN the triggering condition resolves, THE Notification_Dispatcher SHALL stop sending reminder notifications for that condition within one interval period.
9. WHEN a monitor has multiple Channel_Bindings matching the same trigger condition, THE Notification_Dispatcher SHALL dispatch notifications to all matching bindings independently so that a delivery failure on one binding does not prevent delivery to other bindings.

### Requirement 5: Email Notification Delivery

**User Story:** As a Pulse administrator, I want email notifications sent via the instance-level SMTP configuration with Pulse branding, so that alert emails are recognizable and professionally formatted.

#### Acceptance Criteria

1. WHEN an email notification is triggered, THE Notification_Dispatcher SHALL establish an SMTP connection using the instance-level SMTP_Settings with a send timeout of 30 seconds, and deliver the email as HTML.
2. THE Notification_Dispatcher SHALL use the fixed Pulse-branded HTML email template with the ECG motif, logo, and brand colors.
3. THE Notification_Dispatcher SHALL include the monitor name, target URL, status change description, response time, incident ID, incident started-at timestamp, and incident duration in the email body.
4. THE Notification_Dispatcher SHALL send the email to all recipient addresses configured in the Notification_Channel.
5. IF SMTP_Settings are not configured, THEN THE Notification_Dispatcher SHALL log a warning and skip email delivery without crashing.
6. IF the SMTP server rejects the email or the connection times out, THEN THE Notification_Dispatcher SHALL log the failure and queue the delivery for retry.
7. WHEN an email notification is triggered, THE Notification_Dispatcher SHALL set the email subject to include the monitor name and the event type that triggered the notification.

### Requirement 6: Webhook Notification Delivery

**User Story:** As a Pulse administrator, I want webhook notifications rendered using my custom template and delivered to my configured endpoint, so that I can integrate with any external system (Slack, Telegram, Discord, PagerDuty, etc.).

#### Acceptance Criteria

1. WHEN a webhook notification is triggered, THE Notification_Dispatcher SHALL render the body template using the current Template_Variables, enforce a maximum rendered body size of 1 MB, and send an HTTP request to the configured URL.
2. WHEN a webhook notification is triggered, THE Notification_Dispatcher SHALL use the configured HTTP method, include any configured custom headers, and set a default Content-Type of "application/json" when no Content-Type header is explicitly configured.
3. THE Notification_Dispatcher SHALL make the following Template_Variables available: monitor.Name, monitor.URL, monitor.Target, Status, PreviousStatus, ResponseTime, Incident.StartedAt, Incident.Duration, Incident.ID, Timestamp.
4. IF the webhook endpoint returns a non-2xx status code, THEN THE Notification_Dispatcher SHALL log the failure and queue the delivery for retry.
5. IF the webhook endpoint is unreachable, THEN THE Notification_Dispatcher SHALL log the connection error and queue the delivery for retry.
6. THE Notification_Dispatcher SHALL set a request timeout of 10 seconds for webhook HTTP calls.
7. IF the body template fails to render at dispatch time, THEN THE Notification_Dispatcher SHALL log an error indicating the template rendering failure, skip the delivery without queuing for retry, and record the failure in the Delivery_Log.

### Requirement 7: Delivery Retry and Logging

**User Story:** As a Pulse administrator, I want failed notification deliveries retried with exponential backoff and all delivery attempts logged, so that transient failures are handled gracefully and I can audit notification history.

#### Acceptance Criteria

1. WHEN a notification delivery fails with a retryable error, THE Notification_Dispatcher SHALL retry with exponential backoff using a multiplier of 2 (delays: 30s, 60s, 120s) for a maximum of 3 retry attempts.
2. THE Notification_Dispatcher SHALL record each delivery attempt (including the initial attempt and all retries) in the Delivery_Log with timestamp, channel ID, monitor ID, trigger type, attempt number, status (success/failure), and error detail (truncated to 1024 characters).
3. IF all retry attempts are exhausted, THEN THE Notification_Dispatcher SHALL mark the delivery as permanently failed in the Delivery_Log and not attempt further retries for that notification.
4. THE Notification_Dispatcher SHALL expose a Prometheus counter metric named `pulse_notification_deliveries_total` labeled by channel type (email/webhook) and outcome (success/failure).
5. IF a notification delivery panics, THEN THE Notification_Dispatcher SHALL recover the panic, record the panic error in the Delivery_Log with status "failure", and continue processing other pending notifications without interruption.
6. IF a delivery failure is non-retryable (invalid channel configuration or template rendering error), THEN THE Notification_Dispatcher SHALL mark the delivery as permanently failed immediately without retrying.

### Requirement 8: Test Notification

**User Story:** As a Pulse administrator, I want to send a test notification through a configured channel, so that I can verify the channel works before relying on it for real alerts.

#### Acceptance Criteria

1. WHEN a test notification request is received for a Notification_Channel, THE Notification_API SHALL dispatch a notification with sample data through that channel and return the result synchronously within a maximum timeout of 10 seconds.
2. THE Notification_API SHALL populate all Template_Variables in the test payload with type-correct sample values that match production data formats and include a clear test indicator (e.g., monitor name prefixed with "[TEST]").
3. THE Notification_API SHALL return a synchronous response indicating whether the test delivery succeeded or failed, including the channel type and channel ID in the response.
4. IF the test delivery fails, THEN THE Notification_API SHALL return the failure cause (connection error, authentication failure, timeout, or non-2xx response code) in the response body.
5. IF the specified Notification_Channel does not exist, THEN THE Notification_API SHALL return a validation error indicating the channel was not found.
6. THE Notification_API SHALL NOT record test deliveries in the Delivery_Log and SHALL NOT queue failed test deliveries for retry.

### Requirement 9: Webhook Template Variable Reference

**User Story:** As a Pulse administrator, I want to see all available template variables with descriptions when editing a webhook channel, so that I can construct my payload correctly.

#### Acceptance Criteria

1. THE Notification_API SHALL provide an endpoint that returns a list of all available Template_Variables, where each entry includes the variable name, Go type, a human-readable description, and an example value.
2. THE Notification_API SHALL include every variable defined in the Template_Variable set (monitor.Name, monitor.URL, monitor.Target, Status, PreviousStatus, ResponseTime, Incident.StartedAt, Incident.Duration, Incident.ID, Timestamp) in the reference response.
3. THE Notification_API SHALL group Template_Variables by their dot-notation prefix (e.g., "monitor", "Incident") in the response, with ungrouped variables listed under a top-level group.
4. IF a client requests the template variable reference endpoint, THEN THE Notification_API SHALL return the complete variable list within 500 milliseconds under normal operating conditions.

### Requirement 10: Notification UI — Channel Management

**User Story:** As a Pulse administrator, I want a dedicated Notifications page in the main navigation to manage channels, so that notification configuration is accessible and organized.

#### Acceptance Criteria

1. THE Frontend SHALL display a "Notifications" entry in the main navigation after the "Monitors" entry.
2. THE Frontend SHALL render a paginated list of all Notification_Channels with name, type, and creation date, using a default page size of 20 items.
3. WHEN the user clicks "Create Channel", THE Frontend SHALL display a form with a type selector (email/webhook) and type-specific configuration fields: recipient email addresses for email type; URL, HTTP method, custom headers, and body template for webhook type.
4. WHEN the "webhook" type is selected, THE Frontend SHALL display a body editor with monospace font and a variable reference panel showing all available Template_Variables.
5. THE Frontend SHALL provide edit and delete actions for each channel in the list, where delete requires a confirmation dialog before submission.
6. WHEN the user clicks "Test" on a channel, THE Frontend SHALL display a loading indicator during the request and show the delivery result as a toast notification indicating success or failure with error detail.
7. THE Frontend SHALL use the t() function for all user-visible strings.
8. IF no Notification_Channels exist, THEN THE Frontend SHALL display an empty state with a prompt to create the first channel.

### Requirement 11: Notification UI — Per-Monitor Binding Configuration

**User Story:** As a Pulse administrator, I want to configure which channels and triggers apply to each monitor from the monitor detail view, so that I can customize alerting per monitor.

#### Acceptance Criteria

1. THE Frontend SHALL display a "Notifications" section in the monitor detail/edit view.
2. THE Frontend SHALL allow selecting one or more Notification_Channels to bind to the current monitor.
3. THE Frontend SHALL display trigger condition options for each binding with appropriate input fields: numeric input (1–60000 ms) for degraded threshold, numeric input (1–365 days) for ssl_expiring threshold, numeric input (1–100) for n_failures_in_row threshold, and checkboxes for monitor_down and monitor_up.
4. THE Frontend SHALL allow configuring the Reminder_Policy interval per binding with a dropdown offering values: disabled, 5, 10, 15, 30, or 60 minutes.
5. WHEN a binding is saved, THE Frontend SHALL validate that at least one trigger condition is selected and display a validation error if none are selected.
6. THE Frontend SHALL provide add and remove actions for bindings, where remove requires a confirmation dialog.
7. THE Frontend SHALL use the t() function for all user-visible strings.
8. IF no Notification_Channels have been created yet, THEN THE Frontend SHALL display a message directing the user to create channels from the Notifications page first.

### Requirement 12: Instance-Level SMTP Configuration

**User Story:** As a Pulse administrator, I want to configure SMTP settings at the instance level, so that all email channels use a single outgoing mail server.

#### Acceptance Criteria

1. THE System SHALL read SMTP configuration from environment variables (PULSE_SMTP_HOST, PULSE_SMTP_PORT, PULSE_SMTP_USERNAME, PULSE_SMTP_PASSWORD, PULSE_SMTP_FROM, PULSE_SMTP_TLS), where PULSE_SMTP_HOST, PULSE_SMTP_PORT, and PULSE_SMTP_FROM are required for SMTP to be considered configured, and PULSE_SMTP_USERNAME and PULSE_SMTP_PASSWORD are optional (supporting unauthenticated relay).
2. WHEN all required SMTP environment variables are set, THE System SHALL attempt an SMTP connection (TCP connect and EHLO handshake) with a timeout of 10 seconds at startup and log the result as success or failure.
3. IF SMTP connectivity validation fails at startup, THEN THE System SHALL log a warning with the connection error, mark SMTP as unavailable, and continue startup without terminating the process.
4. IF any required SMTP environment variable (PULSE_SMTP_HOST, PULSE_SMTP_PORT, or PULSE_SMTP_FROM) is not set, THEN THE System SHALL log a warning indicating email notifications are disabled and skip connectivity validation.
5. THE Notification_API SHALL expose an endpoint to check SMTP configuration status returning the configured state (configured/not configured), the host, port, and sender address when configured, without including PULSE_SMTP_USERNAME or PULSE_SMTP_PASSWORD values in the response.
6. THE System SHALL validate that PULSE_SMTP_PORT is a numeric value between 1 and 65535 and that PULSE_SMTP_FROM is a valid RFC 5322 email address, logging an error and treating SMTP as not configured if validation fails.

### Requirement 13: Database Schema and Migrations

**User Story:** As a developer, I want the notification data model stored in PostgreSQL with proper migrations, so that the schema is versioned and reproducible.

#### Acceptance Criteria

1. THE System SHALL create database tables for notification_channels, channel_bindings, and delivery_logs using golang-migrate migrations consisting of sequentially numbered up/down file pairs where the down migration fully reverses the corresponding up migration.
2. THE System SHALL use UUID primary keys with DEFAULT gen_random_uuid() for all notification tables, and include created_at TIMESTAMPTZ NOT NULL DEFAULT now() on each table, consistent with existing schema conventions.
3. THE System SHALL enforce foreign key constraints from channel_bindings to both notification_channels and monitors with ON DELETE CASCADE.
4. THE System SHALL add indexes on channel_bindings(monitor_id) and delivery_logs(channel_id, created_at) for query performance.
5. THE System SHALL enforce NOT NULL constraints on all foreign key columns and on delivery_logs status and trigger_type columns to prevent incomplete records.

### Requirement 14: Non-Blocking Architecture

**User Story:** As a developer, I want notification dispatch to be non-blocking and fault-tolerant, so that notification failures cannot degrade monitoring reliability.

#### Acceptance Criteria

1. THE Notification_Dispatcher SHALL process notifications in a dedicated goroutine pool whose size is controlled by the PULSE_NOTIFICATION_WORKERS environment variable (default: 10 in development, 50 in production), separate from the scheduler worker pool.
2. THE Notification_Dispatcher SHALL use a buffered channel with a capacity of 256 entries for notification enqueue, dropping notifications when the buffer is full rather than blocking the scheduler.
3. IF a notification is dropped due to buffer overflow, THEN THE Notification_Dispatcher SHALL increment a Prometheus counter metric (labeled by channel type) and log a warning including the monitor ID and trigger type of the dropped notification.
4. WHEN a shutdown signal is received, THE Notification_Dispatcher SHALL stop accepting new notifications, drain all pending notifications from the buffer, and wait for in-flight deliveries to complete within a configurable timeout controlled by the PULSE_NOTIFICATION_DRAIN_TIMEOUT environment variable (default: 30 seconds).
5. IF the drain timeout expires with notifications still pending or in-flight, THEN THE Notification_Dispatcher SHALL cancel remaining deliveries, log the count of abandoned notifications, and proceed with shutdown.

### Requirement 15: OpenAPI Specification Update

**User Story:** As a developer, I want the OpenAPI spec updated with all new notification endpoints, so that the API contract remains the source of truth.

#### Acceptance Criteria

1. THE OpenAPI specification at backend/api/openapi.yaml SHALL define path entries for: channel CRUD (`/notifications/channels`, `/notifications/channels/{id}`), channel binding CRUD (`/monitors/{id}/notification-bindings`, `/monitors/{id}/notification-bindings/{bindingId}`), test notification (`/notifications/channels/{id}/test`), template variable reference (`/notifications/template-variables`), and SMTP status (`/notifications/smtp-status`).
2. THE OpenAPI specification SHALL define schemas for NotificationChannel, ChannelBinding, DeliveryLog, TriggerCondition, ReminderPolicy, and TemplateVariable, each with property names, types, required/optional annotations, and format constraints matching the Glossary definitions.
3. THE OpenAPI specification SHALL document error responses on all notification endpoints using the existing error envelope schema (`{ "error": { "code": "...", "message": "..." } }`) and include the X-Request-ID response header.
4. THE OpenAPI specification SHALL include pagination parameters (page, limit) on list endpoints for channels and delivery logs, consistent with existing paginated endpoints in the spec.
5. WHEN the OpenAPI specification is updated, THE System SHALL pass OpenAPI 3.0.3 schema validation without errors.
