# Implementation Plan: Monitor History Explorer

## Overview

This plan implements per-monitor retention configuration, automated data lifecycle management via a RetentionService, an extended History API with downsampling, and a tabbed History Explorer UI with time range picker and interactive uPlot chart. Tasks are structured to build backend foundations first, then extend the API, then deliver the frontend, with tests alongside each implementation step.

## Tasks

- [x] 1. Database migration and model layer
  - [x] 1.1 Create migration 011_monitor_retention
    - Create `backend/migrations/011_monitor_retention.up.sql` adding `history_retention_days INTEGER NOT NULL DEFAULT 30` column with `CHECK (history_retention_days >= 1 AND history_retention_days <= 365)` constraint to the `monitors` table
    - Create `backend/migrations/011_monitor_retention.down.sql` dropping the column
    - _Requirements: 1.1, 1.2, 1.3_

  - [x] 1.2 Update sqlc queries and generated models
    - Add `history_retention_days` to monitor SELECT, INSERT, and UPDATE queries in the sqlc query files
    - Regenerate sqlc output to include `HistoryRetentionDays int32` in the Monitor struct
    - _Requirements: 1.1, 1.2, 1.3_

  - [x] 1.3 Add retention validation to monitor handler
    - Extend `CreateMonitorRequest` and `UpdateMonitorRequest` structs with optional `HistoryRetentionDays *int32` field
    - Add validation: nil → default 30, [1,365] → stored, else → `INVALID_RETENTION_PERIOD` error
    - Wire into existing create/update handlers in `backend/internal/api/handlers/monitors.go`
    - _Requirements: 1.1, 1.2, 1.3, 1.4_

  - [x] 1.4 Write property tests for retention validation (Go, rapid)
    - **Property 1: Retention period storage round-trip**
    - **Property 2: Retention period validation rejects invalid values**
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.4**

- [x] 2. Retention Service
  - [x] 2.1 Implement RetentionService core
    - Create `backend/internal/retention/service.go` with the `RetentionService` struct
    - Implement `Start(ctx)` for periodic execution using a ticker
    - Implement `runCycle(ctx)` iterating monitors in pages of 100, deleting expired rows with LIMIT 10,000
    - Add overlap guard (`atomic.Bool`) to skip overlapping cycles with warning log
    - Add Prometheus counter `pulse_retention_rows_deleted_total`
    - _Requirements: 2.1, 2.3, 2.4, 2.5, 2.7, 1.5, 1.6_

  - [x] 2.2 Implement retention interval config parsing
    - Parse `PULSE_RETENTION_CHECK_INTERVAL` as Go duration (default `1h`, min `1m`, max `168h`)
    - Fail startup with descriptive log if invalid or out of range
    - _Requirements: 2.1, 2.2_

  - [x] 2.3 Add batch error handling and retry logic
    - On batch DB error: log, skip batch, continue remaining monitors
    - Failed batches retry automatically on the next cycle
    - _Requirements: 2.6, 1.7_

  - [x] 2.4 Wire RetentionService into application startup
    - Instantiate RetentionService in `backend/cmd/pulse/main.go`
    - Start it as a goroutine with the application context
    - Ensure graceful shutdown on context cancellation
    - _Requirements: 2.1_

  - [x] 2.5 Write property tests for RetentionService (Go, rapid)
    - **Property 3: Retention cleanup removes only expired rows**
    - **Property 4: Retention interval config validation**
    - **Validates: Requirements 1.5, 1.6, 2.1, 2.2, 2.3**

- [x] 3. Checkpoint — Backend retention layer
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. History API extension
  - [x] 4.1 Add aggregation query to TimescaleDB store
    - Create `QueryHistoryAggregated(ctx, monitorID, start, end, stepSeconds)` method on `timescale.Store`
    - Implement SQL using `time_bucket($4::interval, checked_at)` with MIN/MAX/AVG latency, COUNT, and uptime_ratio
    - Define `AggregatedPoint` struct with `Timestamp`, `MinLatency`, `MaxLatency`, `AvgLatency`, `CheckCount`, `UptimeRatio`
    - _Requirements: 5.2_

  - [x] 4.2 Extend history handler with step parameter and retention clamping
    - Accept optional `step` query param (int, seconds, [60, 86400])
    - Remove the 7-day max window validation
    - Implement retention boundary clamping: if `from < now - retention_days`, clamp and set `truncated: true`
    - Implement auto-step: when `step` absent and range > 24h, compute `ceil(range_seconds / 1000)`
    - Route to `QueryHistory` (raw) or `QueryHistoryAggregated` (downsampled) based on step
    - Return extended response with `step`, `truncated`, `aggregated_points` fields
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_

  - [x] 4.3 Write property tests for History API logic (Go, rapid)
    - **Property 5: Step parameter validation**
    - **Property 6: Aggregation bucket correctness**
    - **Property 7: Auto-step calculation bounds response size**
    - **Property 8: Retention boundary enforcement**
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7**

- [x] 5. OpenAPI specification update
  - [x] 5.1 Update OpenAPI spec for all API changes
    - Add `history_retention_days` field to Monitor schema (integer, min 1, max 365, nullable, default 30)
    - Add same field to CreateMonitorRequest and PutMonitorRequest schemas
    - Add `step` query parameter to history endpoint (integer, min 60, max 86400)
    - Define `AggregatedHistoryPoint` schema (min_latency_ms, max_latency_ms, avg_latency_ms, uptime_ratio, timestamp, check_count)
    - Extend HistoryResponse with `truncated` boolean, `step` integer, `aggregated_points` array
    - Document `INVALID_RETENTION_PERIOD` error code in 400 responses for createMonitor and putMonitor
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

- [x] 6. Checkpoint — Backend API complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Frontend API client and types
  - [x] 7.1 Extend API client with aggregated history support
    - Add `AggregatedHistoryPoint` interface to `frontend/src/lib/api.ts`
    - Add `HistoryResponseExtended` interface with `points`, `aggregated_points`, `step`, `truncated` fields
    - Create `getMonitorHistoryExtended(id, from, to, step?)` function returning the extended response
    - Add `history_retention_days` to the Monitor type in `frontend/src/lib/types.ts`
    - _Requirements: 5.1, 5.2, 5.7_

- [x] 8. Frontend UI components
  - [x] 8.1 Create TimeRangePicker component
    - Create `frontend/src/components/TimeRangePicker.svelte`
    - Implement preset buttons: 1h, 6h, 24h, 7d, 30d
    - Implement custom range mode with `<input type="datetime-local">` (minute granularity)
    - Add validation: start < end, clamp end to current time if in future
    - Show retention notice when selected range exceeds monitor's `history_retention_days`
    - Emit `onchange` callback with computed `{ from, to }` timestamps
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [x] 8.2 Create HistoryExplorer component
    - Create `frontend/src/components/HistoryExplorer.svelte`
    - Accept `monitorId` and `retentionDays` props
    - Manage selected time range state (default: "24 hours" preset)
    - Fetch data via `getMonitorHistoryExtended` with appropriate `step`
    - Render TimeRangePicker + HistoryChart
    - Handle loading (250px skeleton), error (with retry button), and empty states
    - _Requirements: 3.2, 3.3, 3.4, 3.6, 3.7, 4.7, 4.8, 4.9_

  - [x] 8.3 Create enhanced HistoryChart for explorer
    - Create `frontend/src/components/HistoryChartExplorer.svelte`
    - Render latency line chart (Y-axis: ms) and uptime color band (green/red/gray) using uPlot
    - Support both raw points and aggregated data (min/max band with avg line)
    - Implement tooltip with "YYYY-MM-DD HH:mm:ss", latency (or min/max/avg), and state
    - Implement click-and-drag zoom with 1-minute minimum window
    - Show "Reset zoom" button when zoomed
    - Display 250px skeleton loader while data is loading
    - Display "No data available" message for empty datasets
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7_

  - [x] 8.4 Refactor monitor detail page with tabs
    - Modify `frontend/src/routes/monitors/[id]/+page.svelte` to add tab system
    - Move existing content into an "Overview" tab
    - Add "History" tab rendering the `HistoryExplorer` component
    - Manage tab state with `$state` (preserve selected time range on tab switch)
    - _Requirements: 3.1, 3.5_

  - [x] 8.5 Write property tests for frontend (Vitest + fast-check)
    - **Property 9: Time range preset computation**
    - **Property 10: Start-before-end validation**
    - **Property 11: Future end-time clamping**
    - **Property 12: Retention notice visibility**
    - **Validates: Requirements 4.3, 4.4, 4.5, 4.6**

  - [x] 8.6 Write unit tests for frontend components
    - Test tab rendering and switching (Overview/History)
    - Test TimeRangePicker preset buttons and custom mode
    - Test loading skeleton display during fetch
    - Test error state rendering with retry button
    - Test empty state message display
    - Test zoom reset button visibility
    - Test default "24 hours" selection on first open
    - _Requirements: 3.1, 3.2, 3.3, 3.6, 3.7, 6.4, 6.5, 6.6, 6.7_

- [x] 9. Final checkpoint — Full feature verification
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation between backend and frontend phases
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- Migration number is 011 (next after existing 010_http3_monitor_type)
- Backend uses existing `timescale.Store` pattern — extend with new `QueryHistoryAggregated` method
- Frontend extends existing `HistoryChart.svelte` pattern with new `HistoryChartExplorer.svelte`
- The existing `getMonitorHistory` function is preserved for backward compatibility; new `getMonitorHistoryExtended` is added

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "5.1"] },
    { "id": 1, "tasks": ["1.2", "7.1"] },
    { "id": 2, "tasks": ["1.3", "4.1"] },
    { "id": 3, "tasks": ["1.4", "2.1", "2.2"] },
    { "id": 4, "tasks": ["2.3", "4.2"] },
    { "id": 5, "tasks": ["2.4", "2.5", "4.3"] },
    { "id": 6, "tasks": ["8.1", "8.3"] },
    { "id": 7, "tasks": ["8.2", "8.4"] },
    { "id": 8, "tasks": ["8.5", "8.6"] }
  ]
}
```
