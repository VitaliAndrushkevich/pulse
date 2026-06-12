# Requirements Document

## Introduction

Monitor History Explorer enables operators to investigate historical uptime and performance data for any monitor over configurable time periods. The feature addresses the use case where customers report service issues during specific time windows, and operators need to verify the monitor's state during that period directly from the Pulse UI — similar to a Grafana dashboard experience. Each monitor can define its own retention duration, with a system-wide default of 30 days.

## Glossary

- **History_Explorer**: The dedicated tab on the monitor detail page that displays historical check data with time range selection and graph visualization.
- **Retention_Period**: The duration (in days) for which check result data is stored for a given monitor before automatic deletion.
- **Time_Range_Picker**: A UI control that allows operators to select a start and end timestamp for querying historical data, supporting both preset ranges and custom date-time input.
- **Check_Result**: A single data point from a monitor probe execution, containing state (up/down), latency, status code, error, and timestamp.
- **Downsampling**: The process of aggregating raw Check_Result data into fewer points for efficient rendering over large time ranges.
- **Operator**: An authenticated user of the Pulse platform with access to monitor detail pages.

## Requirements

### Requirement 1: Per-Monitor Retention Period Configuration

**User Story:** As an operator, I want to configure how long historical data is stored for each monitor, so that I can balance storage costs with investigation needs per service.

#### Acceptance Criteria

1. THE Monitor_API SHALL accept an optional `history_retention_days` field as an integer when creating or updating a monitor
2. WHEN `history_retention_days` is not specified for a monitor, THE System SHALL use a default retention period of 30 days
3. IF `history_retention_days` is specified with a value between 1 and 365 inclusive, THEN THE System SHALL store the value as the monitor's configured retention period
4. IF `history_retention_days` is outside the range 1–365 or is not a valid integer, THEN THE Monitor_API SHALL return a validation error with code `INVALID_RETENTION_PERIOD`
5. THE Retention_Service SHALL execute a deletion cycle at least once every 24 hours, removing Check_Result rows whose creation timestamp is older than the configured retention period for each monitor
6. WHEN a monitor's `history_retention_days` is reduced via update, THE Retention_Service SHALL apply the new retention period to existing Check_Result rows on the next deletion cycle
7. IF the Retention_Service fails to delete rows for a given monitor during a deletion cycle, THEN THE Retention_Service SHALL log the failure and retry on the next scheduled cycle without affecting other monitors

### Requirement 2: Automated Data Retention Enforcement

**User Story:** As an operator, I want expired historical data to be automatically removed, so that storage usage remains bounded without manual intervention.

#### Acceptance Criteria

1. THE Retention_Service SHALL execute a cleanup job on a periodic schedule (configurable via environment variable `PULSE_RETENTION_CHECK_INTERVAL` as a Go duration string, default `1h`, minimum `1m`, maximum `168h`)
2. IF `PULSE_RETENTION_CHECK_INTERVAL` is set to an unparseable or out-of-range value, THEN THE Retention_Service SHALL fail to start and log an error indicating the invalid configuration
3. WHEN the cleanup job runs, THE Retention_Service SHALL remove Check_Result rows where `checked_at` is older than the monitor's configured Retention_Period
4. THE Retention_Service SHALL process monitors in batches of up to 100 monitors per transaction and delete a maximum of 10,000 rows per batch to avoid long-running transactions that block writes
5. IF a cleanup cycle is still in progress when the next scheduled cycle triggers, THEN THE Retention_Service SHALL skip the new cycle and log a warning
6. IF the cleanup job encounters a database error during a batch, THEN THE Retention_Service SHALL log the error, skip the failing batch, continue with remaining batches, and retry the failed batch on the next scheduled cycle
7. THE Retention_Service SHALL expose a Prometheus metric `pulse_retention_rows_deleted_total` recording the cumulative count of rows removed across all cycles

### Requirement 3: History Explorer Tab on Monitor Detail Page

**User Story:** As an operator, I want a dedicated History tab on the monitor detail page, so that I can investigate historical performance without leaving the monitor context.

#### Acceptance Criteria

1. THE History_Explorer SHALL appear as a tab labeled "History" on the monitor detail page, positioned after a tab labeled "Overview" that contains the existing monitor detail content
2. WHEN the operator selects the History tab, THE History_Explorer SHALL display the Time_Range_Picker with preset options (1 hour, 6 hours, 24 hours, 7 days) and a response-time graph for the selected period
3. WHEN the History tab is first displayed after navigating to the monitor detail page, THE History_Explorer SHALL default the Time_Range_Picker to the "24 hours" preset and display data for the last 24 hours
4. THE History_Explorer SHALL display both response time (latency) as a line chart and uptime state (up/down) as color-coded regions in the graph visualization
5. THE History_Explorer SHALL preserve the selected time range when the operator switches to the Overview tab and returns to the History tab without reloading the page
6. WHEN the operator changes the selected time range, THE History_Explorer SHALL display a loading indicator while fetching data and render the updated graph within 2 seconds of receiving the response
7. IF the History_Explorer receives no data points for the selected time range, THEN THE History_Explorer SHALL display a message indicating no data is available for the selected period

### Requirement 4: Time Range Picker

**User Story:** As an operator, I want to select specific time periods for viewing monitor history, so that I can investigate customer-reported incidents at precise times.

#### Acceptance Criteria

1. THE Time_Range_Picker SHALL provide preset ranges: Last 1 hour, Last 6 hours, Last 24 hours, Last 7 days, Last 30 days
2. THE Time_Range_Picker SHALL provide a custom range mode where the operator specifies start and end date-time values with minute-level granularity in the monitor's local timezone
3. WHEN the operator selects a preset range, THE Time_Range_Picker SHALL compute start and end timestamps relative to the current time and immediately trigger a history query
4. IF the operator selects a range that extends beyond the monitor's configured Retention_Period, THEN THE History_Explorer SHALL display a notice indicating data may be incomplete for the requested period
5. IF the operator sets a start time that is after the end time, THEN THE Time_Range_Picker SHALL display a validation error and prevent the query
6. IF the operator sets an end time in the future, THEN THE Time_Range_Picker SHALL clamp the end time to the current time before executing the query
7. WHEN the operator confirms a time range selection, THE History_Explorer SHALL display a loading indicator, query the History_API, and update the graph within 5 seconds for ranges up to 30 days
8. IF the History_API query fails or times out, THEN THE History_Explorer SHALL display an error message indicating the failure reason and allow the operator to retry without re-entering the time range
9. WHEN the History_Explorer tab is first opened, THE Time_Range_Picker SHALL default to the "Last 24 hours" preset and automatically load the corresponding data

### Requirement 5: History API Extension

**User Story:** As an operator, I want the history API to support querying longer time ranges with efficient data delivery, so that the UI remains responsive even when displaying weeks of data.

#### Acceptance Criteria

1. THE History_API SHALL accept an optional `step` query parameter specifying the desired aggregation interval in seconds, with a minimum value of 60 and a maximum value of 86400
2. WHEN `step` is provided, THE History_API SHALL return downsampled data points aggregated over the specified interval, where each bucket includes min_latency_ms, max_latency_ms, avg_latency_ms, total_checks, and uptime_ratio (number of checks with state "up" divided by total checks in that bucket, expressed as a value between 0.0 and 1.0)
3. WHEN `step` is not provided and the requested range exceeds 24 hours, THE History_API SHALL auto-select a step value equal to the requested range in seconds divided by 1000 (rounded up to the nearest whole second) to limit the response to a maximum of 1000 data points
4. THE History_API SHALL accept queries with time ranges up to the monitor's configured Retention_Period (default 30 days) without enforcing the previous 7-day hardcoded limit
5. IF the requested range exceeds the monitor's Retention_Period, THEN THE History_API SHALL clamp the start time to the retention boundary and include a `truncated: true` flag in the response metadata alongside the adjusted `from` timestamp
6. IF `step` is provided with a value less than 60, greater than 86400, zero, or negative, THEN THE History_API SHALL reject the request with a 400 error response indicating the valid range for the step parameter
7. WHEN the History_API returns aggregated (downsampled) data, THE response SHALL include a `step` field indicating the aggregation interval used in seconds, allowing the client to distinguish aggregated responses from raw data point responses

### Requirement 6: Graph Visualization

**User Story:** As an operator, I want a clear, interactive graph of monitor performance over time, so that I can visually identify outage periods and latency patterns.

#### Acceptance Criteria

1. THE History_Explorer SHALL render response-time data as a line chart using the existing uPlot library with the Y-axis labeled in milliseconds and the X-axis displaying time-of-day labels
2. THE History_Explorer SHALL render uptime-state data as a colored band below the latency chart, using green for "up", red for "down", and gray for "unknown" states
3. WHEN the operator hovers over the graph, THE History_Explorer SHALL display a tooltip showing the timestamp in the format "YYYY-MM-DD HH:mm:ss" (local time), the latency value in milliseconds (or "N/A" if null), and the monitor state ("up", "down", or "unknown") for that data point
4. THE History_Explorer SHALL support zoom via click-and-drag on the graph to narrow the visible time range, with a minimum zoom window of 1 minute
5. WHEN the graph is zoomed, THE History_Explorer SHALL display a visible "Reset zoom" button that, when activated, restores the graph to the full selected time range
6. WHILE data is loading from the History_API, THE History_Explorer SHALL display a loading skeleton placeholder matching the chart's rendered height (250px) in place of the graph
7. IF the History_API returns an empty dataset for the selected time range, THEN THE History_Explorer SHALL display a text message indicating no data is available instead of rendering an empty chart

### Requirement 7: OpenAPI Specification Update

**User Story:** As a developer, I want the OpenAPI specification to accurately reflect all API changes, so that API consumers have reliable documentation.

#### Acceptance Criteria

1. THE OpenAPI_Spec SHALL document the `history_retention_days` field on the Monitor schema with type integer, minimum 1, maximum 365, nullable true, and default 30, and SHALL include the same field (with identical constraints) on the CreateMonitorRequest and PutMonitorRequest schemas
2. THE OpenAPI_Spec SHALL document the `step` query parameter on the history endpoint with type integer, minimum 1, maximum 86400, unit in seconds, and a description stating that responses are aggregated into time buckets of the specified duration when the parameter is provided
3. THE OpenAPI_Spec SHALL document the extended HistoryResponse schema including a `truncated` field of type boolean indicating whether results were capped, and SHALL define an AggregatedHistoryPoint schema (used in place of HistoryPoint when `step` is provided) containing fields: `min_latency_ms` (integer, nullable), `max_latency_ms` (integer, nullable), `avg_latency_ms` (integer, nullable), `uptime_ratio` (number, format float, range 0.0 to 1.0), `timestamp` (string, format date-time), and `check_count` (integer, minimum 1)
4. THE OpenAPI_Spec SHALL document the `INVALID_RETENTION_PERIOD` error code in the ErrorResponse schema description and in the 400 responses for the createMonitor and putMonitor operations
