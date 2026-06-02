# Implementation Plan: Frontend Product (Milestone G)

## Overview

This plan implements the complete Pulse frontend product layer as a static SvelteKit 5 application with TypeScript. Tasks are ordered by dependency chain: foundational modules (types, API client, stores) → reusable components (VirtualList, MonitorForm) → page routes → integration wiring. Property-based tests validate correctness properties defined in the design using fast-check.

## Tasks

- [x] 1. Set up testing infrastructure and core types
  - [x] 1.1 Install dependencies and configure Vitest with fast-check
    - Add `vitest`, `@testing-library/svelte`, `jsdom`, `fast-check`, and `uplot` to `package.json`
    - Configure `vitest.config.ts` with jsdom environment and Svelte plugin
    - Update `package.json` test script to use `vitest --run`
    - _Requirements: Design Testing Strategy_

  - [x] 1.2 Define core TypeScript types and interfaces
    - Create `frontend/src/lib/types.ts` with Monitor, HistoryPoint, Incident, Secret, MonitorPatch, PaginatedList, WsEnvelope interfaces
    - Create `frontend/src/lib/validation.ts` with monitor form validation functions (name, type, target, interval, timeout)
    - Create `frontend/src/lib/format.ts` with date formatting and secret reference formatting utilities
    - _Requirements: 2.3, 7.2, 7.4_

- [x] 2. Implement API client module
  - [x] 2.1 Implement fetch wrapper with auth, timeout, and error handling
    - Implement `apiRequest<T>()` in `frontend/src/lib/api.ts` with Bearer token injection, 15s AbortController timeout, error envelope parsing, X-Request-ID extraction
    - Implement `onUnauthorized` callback (clear JWT, redirect to login)
    - Implement typed endpoint functions: `login()`, `getMonitors()`, `getMonitor()`, `createMonitor()`, `updateMonitor()`, `deleteMonitor()`, `getMonitorHistory()`, `getMonitorIncidents()`, `getSecrets()`
    - _Requirements: 6.1, 6.2, 8.1, 8.2, 8.3, 8.4, 8.5_

  - [x]* 2.2 Write property tests for API client auth header and error handling
    - **Property 11: API client attaches Bearer token**
    - **Property 14: Error toast content from API responses**
    - **Validates: Requirements 6.2, 8.2, 8.4, 8.5**

- [x] 3. Implement reactive stores
  - [x] 3.1 Implement AuthStore with JWT lifecycle
    - Create `frontend/src/lib/stores/auth.ts` using Svelte 5 `$state` rune
    - Implement `getToken()`, `setToken()`, `clearToken()`, `isAuthenticated` derived state
    - Read/write JWT from localStorage on init and changes
    - _Requirements: 6.1, 6.2, 6.5, 6.7, 6.9_

  - [x] 3.2 Implement MonitorStore with patch-merge logic
    - Create `frontend/src/lib/stores/monitors.ts` using Svelte 5 runes
    - Implement `Map<string, Monitor>` state with `setMonitors()`, `applyPatch()`, `updateMonitor()`, `removeMonitor()`, `clear()`
    - Implement `$derived` getters: `list`, `totalCount`, `healthyCount`, `unhealthyCount`
    - Implement `applyMonitorPatch()` pure function for deterministic field-level merge
    - Discard patches for unknown monitor_ids
    - _Requirements: 1.4, 5.2, 5.3, 5.6_

  - [x]* 3.3 Write property tests for patch-merge logic
    - **Property 7: Patch merge preserves non-patched fields**
    - **Property 8: Patch merge commutativity for distinct fields**
    - **Property 10: Patch for unknown monitor_id is discarded**
    - **Validates: Requirements 5.2, 5.3, 5.6**

  - [x]* 3.4 Write property tests for dashboard statistics derivation
    - **Property 2: Dashboard statistics derivation**
    - **Validates: Requirements 1.4**

  - [x] 3.5 Implement ToastStore and ConnectionStore
    - Create `frontend/src/lib/stores/toast.ts` with Toast array state, `addToast()`, `dismissToast()`, auto-dismiss timers (4s success, 8s error), max 5 visible
    - Create `frontend/src/lib/stores/connection.ts` with ConnectionStatus state and `setStatus()`, `lastConnected` timestamp
    - _Requirements: 5.8, 8.1, 8.2_

- [x] 4. Implement WebSocket client
  - [x] 4.1 Implement WebSocket client with reconnection backoff
    - Implement `createWsClient()` in `frontend/src/lib/ws.ts`
    - Build connection URL as `/ws?token=<jwt>`
    - Implement 5s connect timeout, message JSON parsing, type dispatch
    - Implement exponential backoff reconnection (1s base, 2x multiplier, 30s max, ±25% jitter)
    - Handle close code 4401 (auth expired → redirect to login, no reconnect)
    - Dispatch `monitor_status` payloads to MonitorStore.applyPatch
    - Update ConnectionStore status on connect/disconnect
    - Implement clean disconnect with close code 1000 on logout
    - _Requirements: 5.1, 5.4, 5.7, 5.8, 5.9_

  - [x]* 4.2 Write property tests for reconnection backoff delay bounds
    - **Property 9: Reconnection backoff delay bounds**
    - **Validates: Requirements 5.4**

- [x] 5. Checkpoint - Core modules complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Implement validation and formatting utilities
  - [x] 6.1 Implement form validation logic
    - Implement validation functions in `frontend/src/lib/validation.ts`: `validateName()`, `validateTarget()`, `validateInterval()`, `validateTimeout()`, `validateType()`, `validateEmail()`, `validatePassword()`
    - Return structured validation results with error messages
    - _Requirements: 2.3, 6.6_

  - [x]* 6.2 Write property tests for form validation
    - **Property 3: Monitor form field validation**
    - **Property 12: Login form input validation**
    - **Validates: Requirements 2.3, 6.6**

  - [x]* 6.3 Write property tests for secret reference display format
    - **Property 13: Secret reference display format**
    - **Validates: Requirements 7.2, 7.4**

- [x] 7. Implement reusable components
  - [x] 7.1 Implement VirtualList component with DOM recycling
    - Create `frontend/src/components/VirtualList.svelte` with fixed-height row strategy
    - Implement scroll handler with requestAnimationFrame throttling
    - Implement top/bottom spacer elements for scroll position
    - Implement configurable buffer (5–20), max 60 rendered rows
    - Expose `scrollToIndex()` and `scrollToTop()` API
    - _Requirements: 1.1, 1.3_

  - [x]* 7.2 Write property tests for VirtualList row count invariant
    - **Property 1: Virtual list rendered row count invariant**
    - **Validates: Requirements 1.1, 1.3**

  - [x] 7.3 Implement MonitorRow component
    - Create `frontend/src/components/MonitorRow.svelte` displaying name, type badge, target, state indicator (color-coded up/down/unknown), last_checked_at (formatted or "Not checked yet" placeholder)
    - Fixed height matching VirtualList itemHeight
    - _Requirements: 3.2_

  - [x]* 7.4 Write property tests for MonitorRow required fields
    - **Property 4: Monitor list row displays all required fields**
    - **Validates: Requirements 3.2**

  - [x] 7.5 Implement MonitorForm component
    - Create `frontend/src/components/MonitorForm.svelte` with create/edit modes
    - Implement field inputs: name, type selector, target, interval_seconds, timeout_seconds, status toggle
    - Implement type-specific settings (expected_status_codes for http/https, payload for udp, handshake_message for websocket)
    - Integrate validation from `validation.ts`, disable submit until valid
    - Implement secret reference dropdown (name + UUID only)
    - Implement error summary display for API errors
    - Pre-populate fields in edit mode from initialValues
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9_

  - [x] 7.6 Implement Pagination component
    - Create `frontend/src/components/Pagination.svelte` with previous/next controls
    - Disable previous on page 1, disable next on last page based on total_pages
    - Emit page change events
    - _Requirements: 3.5_

  - [x]* 7.7 Write property tests for pagination boundary controls
    - **Property 5: Pagination boundary controls**
    - **Validates: Requirements 3.5**

  - [x] 7.8 Implement HistoryChart component with uPlot
    - Create `frontend/src/components/HistoryChart.svelte`
    - Initialize uPlot with time x-axis and millisecond y-axis
    - Handle zero data points with placeholder message
    - Clean up uPlot instance on component destroy
    - _Requirements: 4.2, 4.3_

  - [x] 7.9 Implement Toast notification component
    - Create `frontend/src/components/Toast.svelte` rendering toast stack from ToastStore
    - Implement dismiss button, persistent vs auto-dismiss behavior
    - Display X-Request-ID in error toasts
    - Max 5 visible toasts
    - _Requirements: 8.1, 8.2, 8.4_

  - [x] 7.10 Implement ConnectionBadge component
    - Create `frontend/src/components/ConnectionBadge.svelte` showing live/paused status from ConnectionStore
    - Visible in header when disconnected
    - _Requirements: 1.7, 5.8_

- [x] 8. Checkpoint - Components complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Implement page routes
  - [x] 9.1 Implement login page
    - Create `frontend/src/routes/login/+page.svelte` with email/password form
    - Integrate validation (email format, non-empty password)
    - Call `login()` API, store JWT via AuthStore on success
    - Display "invalid credentials" on 401, "service unavailable" on network/other errors
    - Redirect to `/` on successful login
    - _Requirements: 6.1, 6.4, 6.6, 6.8_

  - [x] 9.2 Implement auth guard in layout
    - Update `frontend/src/routes/+layout.svelte` to check AuthStore.isAuthenticated
    - Redirect to `/login` if no JWT and route is not `/login`
    - Include Header with nav links and ConnectionBadge
    - Include Toast component at layout level
    - Implement logout action in nav
    - _Requirements: 6.3, 6.7, 5.7_

  - [x] 9.3 Implement dashboard page with VirtualList
    - Update `frontend/src/routes/+page.svelte` with StatsBar (total, healthy, unhealthy counts from MonitorStore)
    - Integrate VirtualList with MonitorRow rendering monitor list from store
    - Fetch monitors on mount, establish WS connection
    - Show empty state with link to create monitor when collection is empty
    - _Requirements: 1.1, 1.4, 1.5, 1.6, 1.7_

  - [x] 9.4 Implement monitor list page with pagination
    - Create `frontend/src/routes/monitors/+page.svelte`
    - Fetch paginated monitors from API (page=1, limit=20 default)
    - Render monitor rows with name, type, target, state, last_checked_at
    - Integrate Pagination component
    - Navigate to `/monitors/{id}` on row click
    - Show "Create Monitor" action navigating to `/monitors/create`
    - Handle empty state and API errors with retry
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_

  - [x] 9.5 Implement monitor detail page with chart and incidents
    - Create `frontend/src/routes/monitors/[id]/+page.svelte`
    - Fetch monitor details, history (24h window), and incidents (page 1, limit 20)
    - Display all monitor fields per design (name, type, target, interval, timeout, status, state, last_checked_at, next_check_at, settings, created_at, updated_at)
    - Integrate HistoryChart with history data
    - Render IncidentTimeline with started_at, resolved_at (or "ongoing")
    - Subscribe to WS patches for the displayed monitor_id
    - Handle 404 (not found message + back link), other errors with retry
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

  - [x]* 9.6 Write property tests for monitor detail field display
    - **Property 6: Monitor detail displays all specified fields**
    - **Validates: Requirements 4.6**

  - [x] 9.7 Implement monitor create page
    - Create `frontend/src/routes/monitors/create/+page.svelte`
    - Integrate MonitorForm in create mode
    - On submit: call `createMonitor()` API, navigate to `/monitors/{id}` on success
    - Handle validation errors (display inline) and other errors (toast)
    - _Requirements: 2.1, 2.5, 2.6_

  - [x] 9.8 Implement monitor edit page
    - Create `frontend/src/routes/monitors/[id]/edit/+page.svelte`
    - Fetch monitor data from store or API, pass as initialValues to MonitorForm in edit mode
    - On submit: call `updateMonitor()` API, update MonitorStore on success
    - On cancel: navigate back without changes
    - _Requirements: 2.2, 2.5, 2.6, 2.8, 2.9_

  - [x] 9.9 Implement settings page with secrets management
    - Create `frontend/src/routes/settings/+page.svelte`
    - Fetch and list secrets (name, UUID, created_at only)
    - Implement secret create form (name + masked value input)
    - Clear value from state immediately after successful creation
    - Handle creation errors (display message, retain name, clear value)
    - _Requirements: 7.1, 7.2, 7.3, 7.5_

- [x] 10. Integration and WebSocket wiring
  - [x] 10.1 Wire WebSocket lifecycle to app layout
    - Connect WS on authenticated layout mount, disconnect on logout/unmount
    - Re-fetch full monitor list on WS reconnect (reconcile missed patches)
    - Display connection badge from ConnectionStore
    - _Requirements: 5.1, 5.5, 5.7, 5.8_

  - [x] 10.2 Wire real-time updates to dashboard and detail views
    - Route incoming `monitor_status` patches through MonitorStore.applyPatch
    - Dashboard updates single rows in-place without full list re-render
    - Detail view merges patches for the currently viewed monitor
    - _Requirements: 1.5, 4.5, 5.2_

- [x] 11. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate the 14 universal correctness properties from the design
- Unit tests validate specific examples and edge cases
- All code is TypeScript with Svelte 5 runes-based reactivity
- Testing uses Vitest + fast-check + @testing-library/svelte
- The frontend communicates with the existing backend REST API (`/api/v1`) and WebSocket (`/ws`)

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "3.1", "3.5", "6.1"] },
    { "id": 3, "tasks": ["2.2", "3.2", "6.2", "6.3"] },
    { "id": 4, "tasks": ["3.3", "3.4", "4.1"] },
    { "id": 5, "tasks": ["4.2", "7.1", "7.3", "7.6", "7.8", "7.9", "7.10"] },
    { "id": 6, "tasks": ["7.2", "7.4", "7.5", "7.7"] },
    { "id": 7, "tasks": ["9.1", "9.2"] },
    { "id": 8, "tasks": ["9.3", "9.4", "9.9"] },
    { "id": 9, "tasks": ["9.5", "9.6", "9.7", "9.8"] },
    { "id": 10, "tasks": ["10.1", "10.2"] }
  ]
}
```
