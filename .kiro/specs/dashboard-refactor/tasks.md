# Implementation Plan: Dashboard Refactor

## Overview

Transform the Dashboard page from a redundant monitor list into an operational health overview. Implementation follows a backend-first approach: new aggregate endpoint, then frontend store, then widget components, and finally wiring the page together with real-time updates. Each task builds incrementally so nothing is orphaned.

## Tasks

- [x] 1. Backend: Dashboard Summary Endpoint
  - [x] 1.1 Define Go structs and handler for `GET /api/v1/dashboard/summary`
    - Create `backend/internal/api/handlers/dashboard.go` with the handler
    - Define response structs: `DashboardSummaryResponse`, `HealthScoreData`, `StatusDistribution`, `ActiveIncident`, `TopLatencyMonitor`, `SSLExpiryEntry`, `HeatmapHour`, `RecentEvent`
    - Register route in `backend/internal/api/router.go` under authenticated group
    - Return placeholder/empty JSON for now — queries wired in next task
    - _Requirements: 1.1, 2.1, 3.1, 4.1, 5.1, 6.1, 7.1, 8.3_

  - [x] 1.2 Implement parallel database queries with errgroup
    - Add query functions for: health score + distribution, active incidents, top latency, SSL expiry, heatmap (TimescaleDB time_bucket), recent events
    - Execute all queries in parallel using `golang.org/x/sync/errgroup`
    - Assemble partial results into the response struct
    - Handle partial failures: if a sub-query fails, return partial data with appropriate flags
    - _Requirements: 1.1, 1.5, 2.1, 3.1, 3.3, 3.6, 4.1, 4.4, 5.1, 5.3, 6.1, 7.1, 7.4, 8.3_

  - [x] 1.3 Update OpenAPI spec with new endpoint schema
    - Add `GET /api/v1/dashboard/summary` to `backend/api/openapi.yaml`
    - Define all response schemas matching the Go structs
    - Include 401 and 500 error responses
    - _Requirements: 1.1, 2.1, 3.1, 4.1, 5.1, 6.1, 7.1_

  - [x] 1.4 Write backend handler tests
    - Test successful aggregate response with mock data
    - Test partial failure returns partial results with `partial_data: true`
    - Test 401 without valid token
    - Test empty monitors returns appropriate defaults
    - _Requirements: 1.1, 1.5, 8.3_

- [x] 2. Frontend: TypeScript Types and API Client
  - [x] 2.1 Add Dashboard TypeScript interfaces to `frontend/src/lib/types.ts`
    - Add `DashboardSummary`, `HealthScoreData`, `StatusDistribution`, `ActiveIncident`, `TopLatencyMonitor`, `SSLExpiryEntry`, `HeatmapHour`, `RecentEvent`, `WidgetId` types
    - Match the backend response schema exactly
    - _Requirements: 1.1, 2.1, 3.1, 4.1, 5.1, 6.1, 7.1_

  - [x] 2.2 Add `getDashboardSummary()` function to `frontend/src/lib/api.ts`
    - Call `GET /api/v1/dashboard/summary` using existing `apiRequest` pattern
    - Return typed `DashboardSummary` response
    - _Requirements: 8.3_

- [x] 3. Frontend: Dashboard Store
  - [x] 3.1 Create `frontend/src/lib/stores/dashboard.svelte.ts`
    - Implement `DashboardState` using Svelte 5 runes (`$state`, `$derived`)
    - Implement `load()` method that calls `getDashboardSummary()` and populates state
    - Implement per-widget `widgetErrors` and `widgetLoading` maps using `$state`
    - Use `Promise.allSettled` pattern for error isolation
    - _Requirements: 8.3, 8.4, 8.6, 9.2_

  - [x] 3.2 Implement `applyPatch()` for real-time WebSocket updates
    - Accept `MonitorPatch` from patchBus
    - Incrementally update: healthScore recalculation, statusDistribution counts, incidents list add/remove, events feed prepend
    - Update `lastUpdated` timestamp on each patch
    - _Requirements: 1.4, 2.5, 3.4, 3.5, 7.3, 9.1_

  - [x] 3.3 Implement staleness management (`markStale()` / `clearStale()`)
    - Track time since last WebSocket message
    - `markStale()` sets stale flag after 60s of silence
    - `clearStale()` resets on new message receipt
    - On WS reconnect, call `load()` to refresh all data
    - _Requirements: 9.2, 9.3, 9.4, 9.5_

  - [x] 3.4 Write property test: Widget error isolation (Property 19)
    - **Property 19: Widget error isolation preserves other widgets**
    - For any subset of widgets that fail, remaining widgets retain their data independently
    - **Validates: Requirements 8.4**

- [x] 4. Checkpoint - Backend + Store foundation
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Frontend: WidgetShell and Core Widgets (Part 1)
  - [x] 5.1 Create `WidgetShell.svelte` shared wrapper component
    - Accept `loading`, `error`, `onRetry` props
    - Render loading skeleton (`animate-pulse` Tailwind) when loading
    - Render error message with retry button when error
    - Render slot content when ready
    - _Requirements: 8.4, 8.6_

  - [x] 5.2 Create `HealthScore.svelte` component
    - Display percentage with stepped color (green >= 99%, amber >= 95%, red < 95%)
    - Handle empty state ("—" with secondary color)
    - Handle partial data warning indicator
    - Use `t()` for all labels
    - _Requirements: 1.1, 1.2, 1.3, 1.5_

  - [x] 5.3 Write property tests for HealthScore (Properties 1, 2, 3)
    - **Property 1: Health score is correct average**
    - **Property 2: Health score color mapping follows stepped thresholds**
    - **Property 3: Partial data health score uses only successful monitors**
    - **Validates: Requirements 1.1, 1.2, 1.5**

  - [x] 5.4 Create `StatusRing.svelte` component
    - SVG donut chart with proportional arc angles per state
    - Center displays total count as integer
    - Colors via CSS custom properties (`--color-success`, `--color-error`, `--color-secondary`)
    - ARIA label describing each state and count
    - Handle zero monitors (empty ring with secondary fill)
    - Use `t()` for ARIA label template
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.6, 2.7_

  - [x] 5.5 Write property tests for StatusRing (Properties 4, 5)
    - **Property 4: Status ring arc angles are proportional and sum to 360°**
    - **Property 5: Status ring ARIA label describes all states and counts**
    - **Validates: Requirements 2.1, 2.4, 2.6, 2.7**

  - [x] 5.6 Create `IncidentsPanel.svelte` component
    - Display down monitors with name, elapsed duration (human-readable), and truncated cause (120 chars + ellipsis)
    - Order by duration descending with name tiebreaker
    - Cap at 10 entries with overflow count indicator
    - Empty state message when no incidents
    - Auto-refresh durations every 60 seconds
    - Use `t()` for labels and empty state
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_

  - [x] 5.7 Write property tests for IncidentsPanel (Properties 6, 7, 8)
    - **Property 6: Incident cause truncation preserves content within limit**
    - **Property 7: Incidents ordered by duration descending with name tiebreaker**
    - **Property 8: Incidents panel capped at 10 entries with overflow count**
    - **Validates: Requirements 3.1, 3.3, 3.6**

- [x] 6. Frontend: Core Widgets (Part 2)
  - [x] 6.1 Create `ResponseSparklines.svelte` component
    - Display top-5 monitors by latency with canvas sparkline charts
    - Monitor name truncated to 40 chars, latency as integer + "ms"
    - Fetch history data (`step=900`) for each top monitor
    - Handle null data points as gaps in the line
    - Order by latency descending
    - Use `t()` for labels
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_

  - [x] 6.2 Write property tests for Sparklines (Properties 9, 10)
    - **Property 9: Top-5 latency selection picks highest non-null values in descending order**
    - **Property 10: Sparkline entry formatting applies truncation and integer suffix**
    - **Validates: Requirements 4.1, 4.2, 4.4, 4.5, 4.6**

  - [x] 6.3 Create `SSLWarnings.svelte` component
    - Display monitors with SSL expiring within 30 days (or already expired)
    - Show name, days remaining, expiry date (locale-formatted via `Intl.DateTimeFormat`)
    - Order by days remaining ascending with name tiebreaker
    - Urgency styling: expired/critical (red) for ≤7 days, warning (amber) for 8–30 days
    - Hide section entirely when no entries
    - Use `t()` for labels
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_

  - [x] 6.4 Write property tests for SSLWarnings (Properties 11, 12, 13)
    - **Property 11: SSL expiry filter includes only monitors with days_remaining <= 30**
    - **Property 12: SSL entries ordered by days remaining ascending with name tiebreaker**
    - **Property 13: SSL urgency tier correctly categorizes days remaining**
    - **Validates: Requirements 5.1, 5.3, 5.5**

  - [x] 6.5 Create `UptimeHeatmap.svelte` component
    - 24 hourly blocks with worst-state coloring (red > amber > green > grey)
    - Tooltip on hover showing time range and per-state counts
    - Time axis labels every 3 hours in user's local timezone
    - Error state with retry control if data fails
    - Use `t()` for labels and tooltips
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6_

  - [x] 6.6 Write property tests for Heatmap (Properties 14, 15)
    - **Property 14: Heatmap always produces exactly 24 hourly blocks**
    - **Property 15: Heatmap block color follows worst-state priority**
    - **Validates: Requirements 6.1, 6.2**

- [x] 7. Checkpoint - All widgets implemented
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Frontend: Events Feed, Data Freshness, and Page Assembly
  - [x] 8.1 Create `EventsFeed.svelte` component
    - Display 10 most recent state transitions in reverse chronological order
    - Show monitor name, transition (from → to), relative timestamp
    - Auto-update relative timestamps every 30 seconds
    - Visually distinguish recovery (to `up`) vs failure (to `down`) with color tokens
    - Prepend new events from WebSocket, remove oldest when exceeding 10
    - Initial population from incident data (24h window)
    - Session-scoped: cleared on refresh
    - Use `t()` for labels and empty state
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

  - [x] 8.2 Write property tests for EventsFeed (Properties 16, 17, 18)
    - **Property 16: Events feed bounded at 10 in reverse chronological order**
    - **Property 17: Initial events feed filtered to 24-hour window**
    - **Property 18: Relative timestamp formatting produces correct human-readable duration**
    - **Validates: Requirements 7.1, 7.3, 7.4, 7.2, 9.3**

  - [x] 8.3 Create `DataFreshness.svelte` component
    - Display "last updated" relative timestamp refreshing every 5 seconds
    - Show stale indicator badge after 60s without WebSocket messages
    - Clear stale indicator on new message
    - Use `t()` for labels
    - _Requirements: 9.3, 9.4, 9.5_

  - [x] 8.4 Rewrite `frontend/src/routes/+page.svelte` as the new Dashboard
    - Remove existing MonitorRow/VirtualList-based content
    - Import and compose all widget components in responsive grid layout
    - 3-column grid at >= 768px, single-column below 768px
    - HealthScore and IncidentsPanel first in DOM order
    - Call `dashboardStore.load()` on mount
    - Subscribe to patchBus and call `dashboardStore.applyPatch()` on `monitor_status` messages
    - Set up staleness timer (60s without WS message → markStale)
    - Handle WS reconnect: re-fetch all data via `load()`
    - No horizontal overflow at 320px–768px
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 9.1, 9.2_

  - [x] 8.5 Add i18n keys for all Dashboard widgets to locale files
    - Add new keys to `frontend/src/locales/en.json` under a `dashboard` section
    - Add placeholder keys to `ru.json`, `es.json`, and other locale files
    - Covers: widget titles, empty states, error messages, labels, ARIA text
    - _Requirements: 1.3, 3.2, 5.4, 7.5_

- [x] 9. Final Checkpoint - Full integration verified
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties (19 total mapped to design)
- Unit tests validate specific examples and edge cases
- Backend queries use `errgroup` for parallel execution — partial failures are handled gracefully
- All frontend strings use `t()` from `$lib/i18n` per project convention
- CSS custom properties used for theming — no hardcoded Tailwind colors
- The existing `patchBus` and WebSocket infrastructure is reused; no new WS protocol changes needed

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1"] },
    { "id": 1, "tasks": ["1.2", "1.3", "2.2"] },
    { "id": 2, "tasks": ["1.4", "3.1"] },
    { "id": 3, "tasks": ["3.2", "3.3", "5.1"] },
    { "id": 4, "tasks": ["3.4", "5.2", "5.4", "5.6"] },
    { "id": 5, "tasks": ["5.3", "5.5", "5.7", "6.1", "6.3", "6.5"] },
    { "id": 6, "tasks": ["6.2", "6.4", "6.6", "8.1", "8.3"] },
    { "id": 7, "tasks": ["8.2", "8.4", "8.5"] }
  ]
}
```
dct