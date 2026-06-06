# Requirements Document

## Introduction

Monitor Tagging extends Pulse with key-value tag pairs on monitors. Tags enable Prometheus metric filtering/grouping via label promotion, and power a minimalistic frontend filter bar for the monitor list view. Tags are stored in a normalized `monitor_tags` table and exposed through existing monitor CRUD endpoints plus a dedicated autocomplete API.

## Glossary

- **Tag_Storage_Layer**: The PostgreSQL `monitor_tags` table and associated sqlc queries responsible for persisting, retrieving, and filtering tags
- **API_Handler**: The Go gin HTTP handler layer that processes tag-related requests within monitor CRUD and dedicated tag endpoints
- **Tag_Validator**: The pure validation function that enforces tag key format, value length, count limits, and reserved prefix rules
- **Prometheus_Label_Promoter**: The DynamicMetrics component that promotes tag keys as `tag_`-prefixed labels on Prometheus gauge vectors
- **FilterBar**: The Svelte frontend component providing type pills and tag chip filtering for the monitor list
- **WebSocket_Hub**: The existing WebSocket broadcast system extended with `monitor_tags_changed` message type
- **Monitor**: An uptime check target (HTTP, TCP, UDP, or WebSocket) stored in the `monitors` table
- **Tag**: A key-value pair associated with a monitor, where the key identifies the category and the value identifies the specific label
- **AND_Semantics**: Filtering logic where a monitor must match ALL specified tag filters to appear in results
- **Label_Promotion**: The process of converting stored tag keys into Prometheus metric label names prefixed with `tag_`

## Requirements

### Requirement 1: Tag Persistence

**User Story:** As a platform operator, I want tags stored in a normalized table with referential integrity, so that tag data remains consistent and queryable without impacting monitor table performance.

#### Acceptance Criteria

1. THE Tag_Storage_Layer SHALL store each tag as a separate row with monitor_id, key, and value columns in the `monitor_tags` table
2. THE Tag_Storage_Layer SHALL enforce a UNIQUE constraint on the combination of monitor_id, key, and value
3. WHEN a monitor is deleted, THEN THE Tag_Storage_Layer SHALL cascade-delete all associated tags
4. THE Tag_Storage_Layer SHALL maintain indexes on monitor_id, key, and the (key, value) pair for efficient lookups

### Requirement 2: Tag Validation

**User Story:** As a platform operator, I want strict validation on tag keys and values, so that tags remain consistent, safe for Prometheus labels, and do not degrade system performance.

#### Acceptance Criteria

1. THE Tag_Validator SHALL reject any tag key that does not match the pattern `^[a-z][a-z0-9_-]{0,63}$`
2. THE Tag_Validator SHALL reject any tag key that starts with the reserved prefix `__`
3. THE Tag_Validator SHALL reject any tag value that is empty or exceeds 256 characters
4. THE Tag_Validator SHALL reject any tag value containing control characters (non-printable UTF-8)
5. THE Tag_Validator SHALL reject a tag set containing more than 20 tags for a single monitor
6. THE Tag_Validator SHALL reject a tag set containing duplicate key-value pairs
7. THE Tag_Validator SHALL return a descriptive error message identifying the specific validation failure
8. THE Tag_Validator SHALL be a pure function with no side effects or external state dependencies

### Requirement 3: Monitor CRUD with Tags

**User Story:** As an API consumer, I want to include tags in monitor create and update payloads, so that I can manage tags alongside monitor configuration in a single operation.

#### Acceptance Criteria

1. WHEN a monitor is created with a tags array in the request body, THEN THE API_Handler SHALL persist the monitor and all tags within a single database transaction
2. WHEN a monitor is updated with a tags array in the request body, THEN THE API_Handler SHALL replace all existing tags for that monitor with the new tag set within a single transaction
3. WHEN a monitor create or update request contains invalid tags, THEN THE API_Handler SHALL return HTTP 400 with error code `INVALID_TAGS` and a descriptive message
4. WHEN a monitor is retrieved (GET), THEN THE API_Handler SHALL include the full tags array in the response body
5. WHEN monitors are listed (GET /api/v1/monitors), THEN THE API_Handler SHALL include the tags array for each monitor in the response

### Requirement 4: Tag Autocomplete API

**User Story:** As a frontend developer, I want dedicated endpoints to list available tag keys and values, so that the filter bar can offer autocomplete suggestions to users.

#### Acceptance Criteria

1. WHEN a GET request is made to `/api/v1/tags`, THEN THE API_Handler SHALL return a list of all distinct tag keys currently in use
2. WHEN a GET request is made to `/api/v1/tags/:key`, THEN THE API_Handler SHALL return a list of all distinct values for the specified tag key
3. THE API_Handler SHALL require authentication (JWT or API token) for tag autocomplete endpoints
4. THE API_Handler SHALL share existing API rate limits for tag autocomplete endpoints

### Requirement 5: Filtered Monitor Listing

**User Story:** As a platform operator, I want to filter the monitor list by type and tags with AND semantics, so that I can quickly locate specific monitors in a large deployment.

#### Acceptance Criteria

1. WHEN a GET request to `/api/v1/monitors` includes `tag` query parameters in `key:value` format, THEN THE API_Handler SHALL return only monitors possessing ALL specified tags
2. WHEN a GET request to `/api/v1/monitors` includes a `type` query parameter, THEN THE API_Handler SHALL return only monitors of that type
3. WHEN both `type` and `tag` query parameters are provided, THEN THE API_Handler SHALL apply both filters with AND semantics
4. THE API_Handler SHALL paginate filtered results using `page` and `limit` query parameters
5. THE API_Handler SHALL return pagination metadata including total count, current page, limit, and total pages
6. THE API_Handler SHALL order filtered results by creation date descending

### Requirement 6: Prometheus Label Promotion

**User Story:** As a DevOps engineer, I want monitor tags promoted as Prometheus metric labels, so that I can filter and group uptime metrics in Grafana by environment, team, or other dimensions.

#### Acceptance Criteria

1. THE Prometheus_Label_Promoter SHALL prefix all promoted tag keys with `tag_` when used as Prometheus label names
2. THE Prometheus_Label_Promoter SHALL apply identical label sets to both `pulse_monitor_up` and `pulse_monitor_response_time_seconds` gauge vectors for the same monitor
3. WHEN a monitor has tags not present in the current label set, THEN THE Prometheus_Label_Promoter SHALL fill missing label values with an empty string
4. THE Prometheus_Label_Promoter SHALL enforce a maximum of 10 promoted tag keys to prevent label cardinality explosion
5. WHEN the set of distinct tag keys changes across all monitors, THEN THE Prometheus_Label_Promoter SHALL rebuild metric vectors with the updated label set
6. WHEN metric vectors are rebuilt, THEN THE Prometheus_Label_Promoter SHALL unregister old vectors and register new ones with the Prometheus registry
7. IF the Prometheus registry rejects re-registration of metrics, THEN THE Prometheus_Label_Promoter SHALL log the error and retain previous metric vectors

### Requirement 7: Frontend FilterBar

**User Story:** As a Pulse user, I want a compact, collapsible filter bar on the monitor list page, so that I can narrow down monitors by type and tags without cluttering the interface.

#### Acceptance Criteria

1. THE FilterBar SHALL render monitor type options as horizontal pill toggles
2. THE FilterBar SHALL render tag filters as compact chip selectors displaying key:value pairs
3. WHEN no filters are active, THEN THE FilterBar SHALL collapse to a single "Filter" button
4. WHEN the user selects or deselects a filter, THEN THE FilterBar SHALL emit the updated filter state to the parent component
5. WHEN the FilterBar mounts, THEN THE FilterBar SHALL fetch available tag keys and values from the autocomplete API
6. THE FilterBar SHALL cache fetched tag options in memory to avoid repeated network requests
7. WHEN filter state changes, THEN THE FilterBar SHALL trigger a server-side paginated query with the new filter parameters

### Requirement 8: WebSocket Tag Change Notifications

**User Story:** As a frontend developer, I want real-time notifications when monitor tags change, so that connected clients can update their local state without polling.

#### Acceptance Criteria

1. WHEN tags are modified via the API, THEN THE WebSocket_Hub SHALL broadcast a `monitor_tags_changed` message to all connected clients
2. THE WebSocket_Hub SHALL include the monitor_id, updated tags array, and timestamp in the `monitor_tags_changed` payload
3. THE WebSocket_Hub SHALL NOT include tags in regular `monitor_status` check messages
4. WHEN a `monitor_tags_changed` message is received, THEN THE frontend monitor store SHALL merge the new tags into local state for the affected monitor

### Requirement 9: Concurrent Tag Modification Safety

**User Story:** As a platform operator running automation scripts, I want tag modifications to be safe under concurrent access, so that simultaneous updates do not corrupt tag state.

#### Acceptance Criteria

1. WHEN two concurrent requests modify tags for the same monitor, THEN THE API_Handler SHALL use transaction isolation to ensure one modification completes fully before the other
2. IF a concurrent modification conflict is detected, THEN THE API_Handler SHALL return HTTP 409 Conflict to the losing request
3. THE Tag_Storage_Layer SHALL use a DELETE-then-INSERT approach for tag replacement within a single transaction

### Requirement 10: Performance at Scale

**User Story:** As a platform operator with 500+ monitors, I want tag filtering to remain efficient, so that the monitor list page stays responsive.

#### Acceptance Criteria

1. THE Tag_Storage_Layer SHALL use database indexes to support tag-filtered queries without full table scans
2. THE API_Handler SHALL enforce a maximum page size of 100 monitors per request
3. THE Prometheus_Label_Promoter SHALL load monitor tags via JOIN in the scheduler query to avoid N+1 query patterns
4. THE FilterBar SHALL delegate all filtering to server-side queries rather than performing client-side filtering
