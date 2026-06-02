# Requirements Document

## Introduction

This document specifies requirements for the Pulse frontend product layer (Milestone G). The feature covers the complete interactive frontend: a virtualized monitor dashboard, monitor CRUD forms, detail views with response-time charts and incident timelines, real-time WebSocket state synchronization via patch merge, and login/session management. The frontend is a static SvelteKit build (Svelte 5, TypeScript strict, Tailwind CSS 3.4) served embedded in the Go binary and communicating with the existing `/api/v1` REST endpoints and `/ws` WebSocket endpoint.

## Glossary

- **Dashboard**: The primary page (`/`) displaying all monitors in a scrollable, virtualized list with summary statistics.
- **Monitor_Store**: The Svelte reactive store at `frontend/src/lib/stores/monitors.ts` holding the canonical collection of monitor objects and supporting deterministic patch-merge updates.
- **API_Client**: The HTTP client module at `frontend/src/lib/api.ts` responsible for all REST communication with `/api/v1`.
- **WS_Client**: The WebSocket client module at `frontend/src/lib/ws.ts` managing the connection to `/ws`, reconnection, and message dispatch.
- **Patch_Payload**: A `monitor_status` WebSocket message containing only changed fields for a single monitor, conforming to `{ "type": "monitor_status", "payload": { monitor_id, ...changed_fields } }`.
- **Virtual_List**: A UI component that renders only the visible subset of monitor rows plus a buffer, avoiding DOM allocation for off-screen items.
- **Session**: The authenticated state represented by a JWT stored in localStorage enabling access to protected routes and the WebSocket endpoint.
- **Secret_Reference**: A UUID pointer to a secret resource; the UI displays only the secret name and UUID, never the decrypted value.
- **History_Chart**: A uPlot-based time-series chart rendering response-time data points from the monitor history API.
- **Incident_Timeline**: A visual representation of incident start/resolution events for a specific monitor.

## Requirements

### Requirement 1: Virtualized Monitor Dashboard

**User Story:** As an operator, I want the dashboard to display all monitors without UI freezes, so that I can monitor large deployments without degraded browser performance.

#### Acceptance Criteria

1. WHEN the Dashboard mounts and Monitor_Store emits its initial collection, THE Virtual_List SHALL render only the visible monitor rows plus a configurable buffer (minimum 5, maximum 20 rows above and below the viewport).
2. WHEN 500 monitors are present in Monitor_Store, THE Dashboard SHALL render the first visible rows within 500ms measured from SvelteKit page-navigation start to the first contentful paint of the monitor list, without dropping frames below 30fps during that interval.
3. WHEN the user scrolls through 500 monitors, THE Virtual_List SHALL recycle DOM nodes so that the total rendered row count remains below 60 at any point.
4. WHEN Monitor_Store updates, THE Dashboard SHALL display summary statistics (total monitors, healthy count, unhealthy count, incidents in last 24 hours) reflecting the current store state within the same reactive tick.
5. WHEN a Patch_Payload arrives via WS_Client, THE Dashboard SHALL update the affected monitor row in place within 100ms of payload receipt without re-rendering the entire list.
6. IF the Virtual_List receives an empty monitor collection, THEN THE Dashboard SHALL display an empty state message with a link to create a monitor.
7. IF WS_Client loses connection, THEN THE Dashboard SHALL display a visible stale-data indicator within 2 seconds of disconnection and remove it when the connection is re-established.

### Requirement 2: Monitor Create and Edit Forms

**User Story:** As an operator, I want to create and edit monitors through forms, so that I can manage my monitoring targets without using the API directly.

#### Acceptance Criteria

1. WHEN the user submits the create monitor form, THE API_Client SHALL send a POST request to `/api/v1/monitors` with the form values and navigate to the created monitor's detail view on success.
2. WHEN the user submits the edit monitor form, THE API_Client SHALL send a PUT request to `/api/v1/monitors/{id}` with the updated values and reflect changes in Monitor_Store on success.
3. THE monitor form SHALL validate that `name` is non-empty and at most 255 characters, `type` is one of `[http, https, tcp, udp, websocket]`, `target` is non-empty and at most 2048 characters, `interval_seconds` is between 10 and 86400, and `timeout_seconds` is between 1 and 300, before enabling submission.
4. WHEN the form includes a secret reference field, THE form SHALL present a dropdown of available secrets showing only name and UUID (Secret_Reference), never the secret value.
5. IF the API returns a validation error (HTTP 400), THEN THE form SHALL display the error message from the response body's `error.message` field in a visible error summary above the form fields.
6. IF the API returns a non-validation error (network failure, HTTP 401, or HTTP 500), THEN THE form SHALL display a dismissible error notification indicating the failure reason and preserve all entered form values.
7. THE monitor form SHALL provide type-specific settings fields based on the selected monitor type: expected status codes for http/https, payload for udp, and handshake message for websocket.
8. WHEN the user opens the edit form for an existing monitor, THE form SHALL pre-populate all fields with the current monitor values retrieved from Monitor_Store.
9. WHEN the user cancels form editing, THE form SHALL discard unsaved changes and navigate back without modifying Monitor_Store.

### Requirement 3: Monitor List View

**User Story:** As an operator, I want a paginated list of monitors with status indicators, so that I can find and manage specific monitors.

#### Acceptance Criteria

1. WHEN the user navigates to `/monitors`, THE API_Client SHALL fetch the first page of monitors from `GET /api/v1/monitors` with default pagination (page=1, limit=20).
2. THE monitor list SHALL display each monitor's name, type, target, current state (up/down/unknown), and last checked timestamp; IF `last_checked_at` is null, THEN THE monitor list SHALL display a placeholder label indicating the monitor has not been checked yet.
3. WHEN the user clicks a monitor row, THE application SHALL navigate to the monitor detail page at `/monitors/{id}`.
4. WHEN the user clicks the "Create Monitor" action, THE application SHALL navigate to the monitor creation form.
5. WHEN the user requests the next or previous page via pagination controls, THE API_Client SHALL fetch the corresponding page and replace the displayed list content; THE pagination controls SHALL be disabled for directions where no further pages exist (previous on page 1, next on the last page) based on `total_pages` from the API response.
6. IF the API request to `GET /api/v1/monitors` fails, THEN THE monitor list SHALL display an error message indicating the failure and provide a retry action to re-fetch the current page.
7. IF the API response returns zero monitors (total=0), THEN THE monitor list SHALL display an empty state message and a prompt to create the first monitor.

### Requirement 4: Monitor Detail View with Charts

**User Story:** As an operator, I want to see response-time history and incident timeline for a monitor, so that I can diagnose performance trends and outage patterns.

#### Acceptance Criteria

1. WHEN the user navigates to `/monitors/{id}`, THE API_Client SHALL fetch the monitor details from `GET /api/v1/monitors/{id}` and the history from `GET /api/v1/monitors/{id}/history` with query parameters `from` (24 hours before now, RFC 3339) and `to` (now, RFC 3339).
2. WHEN the history response contains one or more data points, THE History_Chart SHALL render latency_ms values against checked_at timestamps using uPlot with a time-formatted x-axis and a millisecond-formatted y-axis, completing rendering within 200ms of data receipt for up to 1440 points.
3. IF the history response contains zero data points, THEN THE History_Chart SHALL display a placeholder message indicating no check data is available for the selected time range.
4. WHEN the user navigates to `/monitors/{id}`, THE detail view SHALL fetch incidents from `GET /api/v1/monitors/{id}/incidents` with page 1 and limit 20, and render the Incident_Timeline showing each incident's started_at timestamp and resolved_at timestamp (or an "ongoing" indicator when resolved_at is null).
5. WHEN a WebSocket message of type `monitor_status` arrives with a `monitor_id` matching the displayed monitor, THE detail view SHALL merge the payload fields (state, latency_ms, checked_at) into the displayed monitor data without a full page reload.
6. THE detail view SHALL display the following monitor fields: name, type, target, interval_seconds, timeout_seconds, status, state, last_checked_at, next_check_at, settings, created_at, and updated_at.
7. IF the monitor detail or history API returns an HTTP error other than 404, THEN THE detail view SHALL display an error message indicating the failure and allow the user to retry the failed request.
8. IF the monitor does not exist (API returns 404), THEN THE detail view SHALL display a "Monitor not found" message and provide navigation back to the monitor list.

### Requirement 5: WebSocket Client and Store Patch Merge

**User Story:** As an operator, I want live monitor status updates without manual refresh, so that I see outages the moment they are detected.

#### Acceptance Criteria

1. WHEN the user is authenticated and the dashboard is loaded, THE WS_Client SHALL establish a WebSocket connection to `/ws?token=<jwt>` and wait up to 5 seconds for the `connected` message before treating the connection as failed and triggering reconnection.
2. WHEN a `monitor_status` message is received, THE Monitor_Store SHALL merge the Patch_Payload into the existing monitor object by overwriting only the fields present in the payload, preserving all other fields unchanged.
3. THE Monitor_Store SHALL ensure that applying any valid Patch_Payload to a monitor object produces an object containing all original fields plus the patched fields (merge is non-destructive and deterministic regardless of patch order for distinct fields).
4. IF the WS_Client connection is lost, THEN THE WS_Client SHALL attempt reconnection with exponential backoff (initial delay 1 second, maximum delay 30 seconds, multiplier 2) indefinitely until the connection is restored or the user logs out.
5. WHEN the WS_Client reconnects successfully and receives the `connected` message, THE application SHALL re-fetch the full monitor list from the REST API to reconcile any patches missed during disconnection.
6. IF a Patch_Payload references a monitor_id not present in Monitor_Store, THEN THE Monitor_Store SHALL discard the patch without creating a partial monitor object.
7. WHEN the user logs out or navigates away from the application, THE WS_Client SHALL send a WebSocket close frame (code 1000) and release the connection within 3 seconds.
8. WHILE the WS_Client is disconnected, THE Dashboard SHALL display a connection-status badge indicating that live updates are paused, visible without scrolling.
9. IF the server closes the WebSocket connection with close code 4401 (authentication expired), THEN THE WS_Client SHALL not attempt reconnection and SHALL redirect the user to the login page.

### Requirement 6: Login and Session Management

**User Story:** As an operator, I want to log in with my credentials and have my session persist across page refreshes, so that I can use the application without repeated authentication.

#### Acceptance Criteria

1. WHEN the user submits valid credentials on the login page, THE API_Client SHALL send a POST request to `/api/v1/auth/login`, store the returned JWT in localStorage, and use it for subsequent authenticated requests.
2. WHEN the JWT is stored, THE API_Client SHALL include it as a Bearer token in the Authorization header of all subsequent API requests.
3. WHEN a user without a stored JWT navigates to any route other than the login page, THE application SHALL redirect the user to the login page.
4. IF the login API returns 401, THEN THE login form SHALL display an error message indicating invalid credentials without revealing which field (email or password) was incorrect.
5. WHEN an API request returns 401, THE application SHALL clear the stored JWT from localStorage, close the WebSocket connection if open, and redirect the user to the login page.
6. THE login page SHALL validate that the email field contains a valid email format and that the password field is non-empty before enabling the submit button.
7. THE application SHALL provide a logout action that clears the stored JWT from localStorage, closes the WebSocket connection, and redirects to the login page.
8. IF the login API request fails due to a network error or returns a status code other than 200 or 401, THEN THE login form SHALL display an error message indicating the service is unavailable and keep the submitted field values intact.
9. WHEN the application loads in the browser, THE application SHALL check localStorage for an existing JWT and, if present, use it for authenticated requests without requiring the user to log in again.

### Requirement 7: Secret Security in UI

**User Story:** As an operator, I want assurance that secret values are never visible in the frontend, so that credentials remain protected even if someone views my screen.

#### Acceptance Criteria

1. THE API_Client SHALL interact with the secrets endpoints (`GET /api/v1/secrets`, `GET /api/v1/secrets/{id}`, `POST /api/v1/secrets`, `PUT /api/v1/secrets/{id}`, `DELETE /api/v1/secrets/{id}`) which return secret metadata only (id, name, created_at, updated_at) and SHALL never receive or store decrypted secret values.
2. WHEN displaying a secret reference in monitor forms or settings, THE application SHALL show only the secret name and UUID as read-only text, with no input field, placeholder, or DOM element that could contain or suggest a secret value.
3. THE application SHALL provide a form to create secrets with a name field (maximum 128 characters) and a value field rendered as a password-type input (masked characters), and WHEN the form is submitted successfully, THE application SHALL clear the value from the input field and component state within the same event cycle and display a confirmation indicating the secret was created.
4. IF a secret is referenced in monitor settings, THEN THE monitor detail view SHALL display the secret as "Secret: {name} ({uuid})" without any value or placeholder for the encrypted content.
5. IF secret creation fails due to a validation error, THEN THE application SHALL display an error message indicating the failure reason, retain the entered name for correction, and clear the value field from the input and component state.

### Requirement 8: API Client Error Handling

**User Story:** As an operator, I want clear feedback when API operations fail, so that I can understand and resolve issues.

#### Acceptance Criteria

1. WHEN an API request fails with a network error or does not receive a response within 15 seconds, THE API_Client SHALL display a toast notification indicating connectivity failure that remains visible until the operator dismisses it.
2. WHEN an API request returns an error response (4xx or 5xx) with a valid error envelope `{ "error": { "code", "message" } }`, THE API_Client SHALL display a toast notification containing the `message` value from the envelope.
3. IF an API request returns 401 on a protected endpoint, THEN THE API_Client SHALL clear the stored JWT, redirect the operator to the login page, and suppress any additional error notification for that response.
4. THE API_Client SHALL include the `X-Request-ID` header value in every error toast notification to aid in support and debugging.
5. IF an API error response does not conform to the standard error envelope structure, THEN THE API_Client SHALL display a toast notification with a generic connectivity error indication and the `X-Request-ID` value.
