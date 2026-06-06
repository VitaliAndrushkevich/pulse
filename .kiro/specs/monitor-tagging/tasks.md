# Implementation Plan: Monitor Tagging

## Overview

Add key-value tag pairs to monitors in Pulse. Tags are stored in a normalized `monitor_tags` table, exposed through existing monitor CRUD endpoints and dedicated autocomplete APIs, promoted as Prometheus metric labels, and filterable from the frontend via a collapsible FilterBar component. Implementation spans database migration, sqlc queries, validation logic, API handlers, Prometheus dynamic metrics, WebSocket notifications, and frontend components.

## Tasks

- [x] 1. Database migration and sqlc query layer
  - [x] 1.1 Create database migration 008_monitor_tags
    - Create `backend/migrations/008_monitor_tags.up.sql`: CREATE TABLE `monitor_tags` with id (UUID PK), monitor_id (FK CASCADE), key (TEXT), value (TEXT), UNIQUE(monitor_id, key, value)
    - Add indexes: `idx_monitor_tags_monitor_id`, `idx_monitor_tags_key_value`, `idx_monitor_tags_key`
    - Create `backend/migrations/008_monitor_tags.down.sql`: DROP TABLE monitor_tags
    - _Requirements: 1.1, 1.2, 1.3, 1.4_

  - [x] 1.2 Create sqlc queries for tag operations
    - Create `backend/internal/store/postgres/queries/monitor_tags.sql` with:
    - `SetMonitorTags`: DELETE existing + batch INSERT new tags for a monitor
    - `ListTagsByMonitor`: SELECT tags WHERE monitor_id = $1
    - `ListAllTagKeys`: SELECT DISTINCT key FROM monitor_tags
    - `ListTagValues`: SELECT DISTINCT value FROM monitor_tags WHERE key = $1
    - `ListMonitorsFiltered`: SELECT monitors with optional type filter and AND-semantics tag filter using HAVING COUNT approach, paginated, ORDER BY created_at DESC
    - `CountMonitorsFiltered`: COUNT query matching ListMonitorsFiltered criteria
    - Run `sqlc generate` to produce Go code
    - _Requirements: 1.1, 1.4, 4.1, 4.2, 5.1, 5.2, 5.3, 5.4, 5.6, 10.1_

- [x] 2. Tag validation logic
  - [x] 2.1 Implement ValidateTags pure function
    - Create `backend/internal/monitor/tags.go` with `ValidateTags(tags []TagRequest) error`
    - Validate: key matches `^[a-z][a-z0-9_-]{0,63}$`, key does not start with `__`
    - Validate: value is 1–256 chars, printable UTF-8 (no control characters)
    - Validate: max 20 tags per monitor, no duplicate (key, value) pairs
    - Return descriptive error messages identifying specific failures
    - Pure function — no side effects or external state
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_

  - [x] 2.2 Write property test for tag key validation
    - **Property 2: Tag Key Validation Correctness**
    - Generate random strings; verify accepted iff matches `^[a-z][a-z0-9_-]{0,63}$` AND not `__` prefix
    - **Validates: Requirements 2.1, 2.2**

  - [x] 2.3 Write property test for tag value validation
    - **Property 3: Tag Value Validation Correctness**
    - Generate random strings; verify accepted iff length 1–256 and no control characters
    - **Validates: Requirements 2.3, 2.4**

  - [x] 2.4 Write property test for tag set constraints
    - **Property 4: Tag Set Constraint Validation**
    - Generate random tag sets; verify rejected iff len > 20 or contains duplicate (key, value) pairs
    - **Validates: Requirements 2.5, 2.6**

  - [x] 2.5 Write property test for validation determinism
    - **Property 5: Tag Validation Determinism**
    - Generate random tag sets; call ValidateTags twice; verify identical results
    - **Validates: Requirements 2.8**

- [x] 3. Checkpoint - Ensure validation tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. API handlers for monitor CRUD with tags
  - [x] 4.1 Update monitor create handler to accept and persist tags
    - Modify `POST /api/v1/monitors` handler to parse `tags` from request body
    - Call `ValidateTags` on input; return 400 INVALID_TAGS on failure
    - Persist monitor + tags in a single transaction (INSERT monitor, then SetMonitorTags)
    - Return tags in the 201 response
    - _Requirements: 3.1, 3.3, 3.4_

  - [x] 4.2 Update monitor update handler to replace tags
    - Modify `PUT /api/v1/monitors/:id` handler to parse `tags` from request body
    - Call `ValidateTags`; return 400 INVALID_TAGS on failure
    - Replace all tags within the same transaction as the monitor update
    - Handle concurrent modification with row-level locking; return 409 on conflict
    - _Requirements: 3.2, 3.3, 9.1, 9.2, 9.3_

  - [x] 4.3 Update monitor GET and LIST responses to include tags
    - Modify GET `/api/v1/monitors/:id` to JOIN tags and include in response
    - Modify GET `/api/v1/monitors` to include tags for each monitor
    - _Requirements: 3.4, 3.5_

  - [x] 4.4 Implement filtered monitor listing with type and tag query params
    - Parse `type` query param and `tag` query params (format: `key:value`) from GET `/api/v1/monitors`
    - Call `ListMonitorsFiltered` / `CountMonitorsFiltered` with AND semantics
    - Return paginated response with total, page, limit, total_pages metadata
    - Enforce max page size of 100
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 10.2_

  - [x] 4.5 Implement tag autocomplete endpoints
    - Add `GET /api/v1/tags` → returns distinct tag keys (authenticated)
    - Add `GET /api/v1/tags/:key` → returns distinct values for a key (authenticated)
    - Register routes in router with existing auth middleware
    - _Requirements: 4.1, 4.2, 4.3, 4.4_

  - [x] 4.6 Write property test for filter completeness and soundness
    - **Property 6: Filter Completeness and Soundness (AND Semantics)**
    - Generate random monitor sets with tags and random filters; verify a monitor appears in results iff it matches type AND possesses ALL specified tags
    - **Validates: Requirements 5.1, 5.2, 5.3**

  - [x] 4.7 Write property test for pagination correctness
    - **Property 7: Pagination Correctness**
    - Generate random page/limit params; verify returned count ≤ limit and metadata consistent with total
    - **Validates: Requirements 5.4, 5.5**

  - [x] 4.8 Write property test for filtered result ordering
    - **Property 8: Filtered Result Ordering**
    - Generate random filtered result sets; verify created_at is descending
    - **Validates: Requirements 5.6**

- [x] 5. Checkpoint - Ensure API handler tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Prometheus dynamic metrics with tag label promotion
  - [x] 6.1 Implement DynamicMetrics struct with label rebuild
    - Create `backend/internal/monitor/dynamic_metrics.go` with `DynamicMetrics` struct
    - Implement `RebuildLabels(allTagKeys []string)`: unregister old vectors, build new label set with `tag_` prefix, register new gauge vectors
    - Cap promoted tag keys at 10 (configurable via `PULSE_MAX_METRIC_TAG_KEYS`)
    - Log error and retain previous vectors if registry rejects re-registration
    - _Requirements: 6.1, 6.4, 6.5, 6.6, 6.7_

  - [x] 6.2 Implement RecordCheck with tag-aware labels
    - Implement `RecordCheck(monitorID, name, mType, url string, tags map[string]string, up bool, latencySeconds float64)`
    - Build label values with `tag_`-prefixed keys; fill missing keys with empty string
    - Set `pulse_monitor_up` and `pulse_monitor_response_time_seconds` with identical label sets
    - Implement `CleanupMonitor` for stale series removal on delete/update
    - _Requirements: 6.1, 6.2, 6.3, 10.3_

  - [x] 6.3 Integrate DynamicMetrics into scheduler
    - Replace static metrics with DynamicMetrics in scheduler
    - Load tags via JOIN in `ListActiveMonitorsDue` query (no N+1)
    - Call `RebuildLabels` when tag keys change (on monitor create/update)
    - _Requirements: 6.5, 10.3_

  - [x] 6.4 Write property test for label prefix transformation
    - **Property 9: Prometheus Label Prefix Transformation**
    - Generate random valid tag keys; verify label name equals `"tag_" + key`
    - **Validates: Requirements 6.1**

  - [x] 6.5 Write property test for label set invariant
    - **Property 10: Prometheus Label Set Invariant**
    - Generate random monitors with tags; verify `pulse_monitor_up` and `pulse_monitor_response_time_seconds` have identical label sets
    - **Validates: Requirements 6.2**

  - [x] 6.6 Write property test for missing tag fill
    - **Property 11: Prometheus Missing Tag Fill**
    - Generate monitors with subset of global tag keys; verify missing values are empty string
    - **Validates: Requirements 6.3**

  - [x] 6.7 Write property test for label cardinality cap
    - **Property 12: Prometheus Label Cardinality Cap**
    - Generate random tag key sets of varying size; verify promoted count never exceeds 10
    - **Validates: Requirements 6.4**

- [x] 7. Checkpoint - Ensure metrics tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. WebSocket tag change notifications
  - [x] 8.1 Add monitor_tags_changed message type
    - Define `MonitorTagsChangedPayload` struct in `backend/internal/hub/messages.go`
    - Add `TypeMonitorTagsChanged = "monitor_tags_changed"` constant
    - Include monitor_id, tags array, and timestamp in payload
    - _Requirements: 8.1, 8.2, 8.3_

  - [x] 8.2 Broadcast tag changes from API handlers
    - After successful tag modification in create/update handlers, broadcast `monitor_tags_changed` to hub
    - Ensure regular `monitor_status` checks do NOT include tags
    - _Requirements: 8.1, 8.3_

  - [x] 8.3 Write property test for WebSocket patch minimality
    - **Property 15: WebSocket Patch Minimality**
    - Generate random monitor_status payloads; verify none contain a tags field
    - **Validates: Requirements 8.3**

  - [x] 8.4 Write property test for tag change notification completeness
    - **Property 16: Tag Change Notification Completeness**
    - Generate random tag modifications; verify broadcast contains monitor_id, full tags array, and timestamp
    - **Validates: Requirements 8.1, 8.2**

- [x] 9. OpenAPI specification updates
  - [x] 9.1 Update OpenAPI spec with tag schemas and endpoints
    - Add `Tag` schema (key, value) to `backend/api/openapi.yaml`
    - Add `tags` array to `Monitor`, `CreateMonitorRequest`, `PutMonitorRequest` schemas
    - Add `tag` query parameter (array) to GET `/monitors` operation
    - Add `GET /tags` and `GET /tags/{key}` operations with responses
    - Add `INVALID_TAGS` error code to error schema
    - Add `monitor_tags_changed` to WebSocket message types documentation
    - _Requirements: 3.3, 4.1, 4.2, 5.1_

- [x] 10. Checkpoint - Ensure backend is complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Frontend FilterBar component and store updates
  - [x] 11.1 Add tag types and extend monitor store
    - Add `Tag` interface and `MonitorFilters` interface to `frontend/src/lib/types.ts`
    - Extend `Monitor` interface with `tags: Tag[]`
    - Update monitor store to handle `monitor_tags_changed` WS messages (merge tags into local state)
    - _Requirements: 8.4_

  - [x] 11.2 Add tag API methods to API client
    - Add `getTags(): Promise<string[]>` to `frontend/src/lib/api.ts`
    - Add `getTagValues(key: string): Promise<string[]>` to api client
    - Update `listMonitors` to accept type and tag filter params
    - _Requirements: 4.1, 4.2, 7.5_

  - [x] 11.3 Implement FilterBar component
    - Create `frontend/src/components/FilterBar.svelte`
    - Render type filters as horizontal pill toggles
    - Render tag filters as compact chip selectors (key: value)
    - Collapse to single "Filter" button when no filters active
    - Emit filter changes via callback prop
    - Fetch and cache available tags on mount from autocomplete API
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6_

  - [x] 11.4 Integrate FilterBar into monitor list page
    - Add FilterBar to monitor list page
    - Wire filter changes to trigger server-side paginated re-fetch with new params
    - Pass available types and fetched tag options to FilterBar
    - _Requirements: 7.7, 10.4_

  - [x] 11.5 Write unit tests for FilterBar component
    - Test pill toggle selection/deselection
    - Test tag chip rendering and removal
    - Test collapse behavior when no filters active
    - Test filter change emission
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [x] 11.6 Write property test for tag persistence round-trip (frontend)
    - **Property 1: Tag Persistence Round-Trip**
    - Generate random valid tag sets; create/update monitor via API; verify retrieved tags match exactly
    - **Validates: Requirements 3.1, 3.2, 3.4, 3.5**

- [x] 12. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Backend property tests use `pgregory.net/rapid` (Go); frontend property tests use `fast-check` (TypeScript)
- Unit tests validate specific examples and edge cases
- The migration numbering (008) follows sequentially from existing migrations
- sqlc queries are in `backend/internal/store/postgres/queries/`; run `sqlc generate` after adding
- DynamicMetrics replaces the static Prometheus metrics for monitors to support tag-based labels
- Tag autocomplete endpoints share existing auth middleware and rate limits
- Frontend FilterBar delegates all filtering server-side — no client-side filtering of 500+ monitors

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "2.1"] },
    { "id": 2, "tasks": ["2.2", "2.3", "2.4", "2.5"] },
    { "id": 3, "tasks": ["4.1", "4.5"] },
    { "id": 4, "tasks": ["4.2", "4.3", "4.4"] },
    { "id": 5, "tasks": ["4.6", "4.7", "4.8", "6.1"] },
    { "id": 6, "tasks": ["6.2", "8.1"] },
    { "id": 7, "tasks": ["6.3", "8.2", "6.4", "6.5", "6.6", "6.7"] },
    { "id": 8, "tasks": ["8.3", "8.4", "9.1"] },
    { "id": 9, "tasks": ["11.1", "11.2"] },
    { "id": 10, "tasks": ["11.3"] },
    { "id": 11, "tasks": ["11.4", "11.5", "11.6"] }
  ]
}
```
