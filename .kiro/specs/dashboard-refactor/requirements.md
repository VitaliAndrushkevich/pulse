# Requirements Document

## Introduction

Refactor the main Dashboard page (`frontend/src/routes/+page.svelte`) from a duplicate monitor list into an operational health overview. The current Dashboard shows the same content as the Monitors page with stat cards on top, providing no unique value. The new Dashboard answers "is everything OK right now?" at a glance through aggregated signals, visual patterns, and active incident awareness — without listing individual monitors for management.

## Glossary

- **Dashboard**: The root page (`/`) of the Pulse application serving as an operational health overview
- **Health_Score**: A single aggregated uptime percentage computed across all active monitors for the last 24 hours
- **Status_Ring**: A donut/ring chart visualizing the ratio of monitors in each state (up, down, unknown)
- **Incidents_Panel**: A section displaying only monitors currently in the `down` state with contextual information
- **Sparkline**: A compact inline chart showing response time trends without axes or labels
- **Heatmap**: A grid of colored time blocks representing monitor state over a time period
- **Events_Feed**: A chronological list of recent state transitions (up→down, down→up) across all monitors
- **Monitor_State**: One of `up`, `down`, or `unknown` as reported by the checking engine
- **SSL_Warning**: A notification that a monitor's TLS certificate expires within a configurable threshold

## Requirements

### Requirement 1: Global Health Score Display

**User Story:** As an operator, I want to see a single aggregated uptime percentage for all monitors, so that I can instantly assess overall system health.

#### Acceptance Criteria

1. WHEN the Dashboard loads, THE Dashboard SHALL display the Health_Score as a percentage computed from the average `uptime_24h.uptime_percent` across all monitors with `status: 'active'`, rounded to two decimal places (e.g., "99.95%")
2. THE Dashboard SHALL render the Health_Score using a stepped color scheme: green (`--color-success`) for values >= 99%, amber (`--color-warning`) for values >= 95% and < 99%, and red (`--color-error`) for values < 95%
3. WHEN no active monitors exist, THE Dashboard SHALL display the Health_Score as "—" with the `--color-text-secondary` color
4. WHEN a WebSocket `monitor_status` patch changes a monitor's state, THE Dashboard SHALL recalculate the Health_Score within the current render cycle using the updated monitor data already held in the reactive store
5. IF the stats fetch for one or more active monitors fails during Dashboard load, THEN THE Dashboard SHALL compute the Health_Score from the monitors whose stats were successfully retrieved and display a warning indicator that partial data is shown

### Requirement 2: Status Ring Visualization

**User Story:** As an operator, I want to see a visual shape showing the ratio of up/down/unknown monitors, so that I can perceive the distribution without reading numbers.

#### Acceptance Criteria

1. THE Dashboard SHALL display a Status_Ring (SVG donut chart) with arc angles proportional to the count of monitors in each Monitor_State, where each segment's arc angle equals (state_count / total_count) × 360 degrees
2. THE Status_Ring SHALL use CSS custom properties for segment fill: `--color-success` for `up`, `--color-error` for `down`, `--color-secondary` for `unknown`
3. THE Status_Ring SHALL display the total monitor count as a plain integer in the center of the ring
4. WHEN all monitors share a single state, THE Status_Ring SHALL render as a complete ring filled with that state's color
5. WHEN a WebSocket `monitor_status` patch changes a monitor state, THE Status_Ring SHALL update segment proportions within the same Svelte reactive render cycle without requiring manual user interaction
6. IF the total monitor count is zero, THEN THE Status_Ring SHALL render an empty ring using `--color-secondary` fill and display "0" in the center
7. THE Status_Ring SHALL include an accessible group label describing the distribution (e.g., an ARIA label conveying each state name and its count) so that screen readers can announce the status breakdown

### Requirement 3: Active Incidents Panel

**User Story:** As an operator, I want to see which monitors are currently down with duration and cause, so that I can prioritize response without navigating away.

#### Acceptance Criteria

1. THE Incidents_Panel SHALL display each monitor currently in the `down` state with its name, the elapsed duration since the incident `started_at` timestamp formatted as a human-readable string (e.g., "2h 15m", "3d 4h"), and the last error cause truncated to 120 characters with an ellipsis if longer
2. WHEN no monitors are in the `down` state, THE Incidents_Panel SHALL display an empty state message indicating all systems are operational
3. THE Incidents_Panel SHALL order entries by incident duration descending (longest-running first), using alphabetical monitor name as a tiebreaker when durations are equal
4. WHEN a monitor transitions from `down` to `up` via a WebSocket patch, THE Incidents_Panel SHALL remove that monitor from the list within the current render cycle
5. WHEN a monitor transitions from `up` to `down` via a WebSocket patch, THE Incidents_Panel SHALL add that monitor to the list within the current render cycle
6. THE Incidents_Panel SHALL display a maximum of 10 active incidents; IF more than 10 monitors are in the `down` state, THEN THE Incidents_Panel SHALL display a summary indicator showing the total count of active incidents
7. THE Incidents_Panel SHALL update each displayed duration value every 60 seconds without requiring a page reload or new data fetch

### Requirement 4: Response Time Sparklines

**User Story:** As an operator, I want to see the top slowest monitors with their latency trends, so that I can identify performance degradation patterns.

#### Acceptance Criteria

1. WHEN the Dashboard loads, THE Dashboard SHALL retrieve the 24-hour average latency for all monitors via the stats endpoint, select up to 5 monitors with the highest non-null `avg_latency_ms` values, and display a Sparkline chart for each
2. THE Dashboard SHALL display each Sparkline entry with the monitor name (truncated to 40 characters with ellipsis if longer) and the 24-hour average latency rendered as a whole-number integer suffixed with "ms" (e.g., "142 ms")
3. THE Sparkline SHALL render latency data points from the monitor history API using a step of 900 seconds (15-minute buckets, yielding up to 96 data points for 24 hours), plotting each bucket's `avg_latency_ms` value
4. THE Dashboard SHALL order the Sparkline entries by 24-hour average latency descending (highest value first)
5. IF fewer than 5 monitors have non-null `avg_latency_ms` in their 24-hour stats, THEN THE Dashboard SHALL display Sparkline entries only for the monitors that have non-null latency data, without placeholder entries
6. WHEN no history data points are returned from the history API for a monitor (empty `aggregated_points` array), THE Dashboard SHALL omit that monitor from the Sparkline section and select the next-highest-latency monitor as a replacement if available
7. IF a history data point has a null `avg_latency_ms` value within an otherwise populated series, THEN THE Sparkline SHALL skip that data point without rendering a plotted value for that time bucket (gap in the line)

### Requirement 5: SSL Expiry Warnings

**User Story:** As an operator, I want to see which monitors have certificates expiring soon, so that I can renew them before they cause outages.

#### Acceptance Criteria

1. THE Dashboard SHALL display a list of monitors whose SSL certificate expires within 30 days or has already expired (days remaining ≤ 0), limited to monitors that report SSL data (HTTP/HTTPS monitors with TLS only)
2. THE Dashboard SHALL display each SSL_Warning entry with the monitor name, days remaining as a whole integer, and expiration date formatted in the user's locale (using the browser's `Intl.DateTimeFormat` default)
3. THE Dashboard SHALL order SSL_Warning entries by days remaining ascending (most urgent first); IF two entries have the same days remaining, THEN THE Dashboard SHALL order them alphabetically by monitor name
4. WHEN no monitors have certificates expiring within 30 days and no monitors have already-expired certificates, THE Dashboard SHALL hide the SSL expiry section entirely
5. THE Dashboard SHALL distinguish urgency levels: expired (0 days or fewer remaining) with red styling, critical (1–7 days remaining) with red styling, and warning (8–30 days remaining) with amber styling
6. WHEN a WebSocket `monitor_status` patch containing `ssl_days_remaining` is received, THE Dashboard SHALL update the SSL expiry list within 2 seconds to reflect the new value, adding, removing, or reordering entries as appropriate
7. IF a monitor's SSL data becomes unavailable (ssl field absent from stats response), THEN THE Dashboard SHALL remove that monitor from the SSL expiry list

### Requirement 6: Uptime Heatmap

**User Story:** As an operator, I want to see a visual timeline of system state over the last 24 hours, so that I can recognize patterns and recurring issues.

#### Acceptance Criteria

1. THE Dashboard SHALL display a Heatmap showing aggregated state of all enabled monitors as colored blocks spanning the last 24 hours, where each block represents one hour (24 blocks total)
2. THE Heatmap SHALL color each block based on the worst state observed during that hour: green if all monitors were `up`, red if any monitor was `down`, amber if some were `unknown` but none `down`, and grey if no check data exists for that hour
3. WHEN the user hovers over a Heatmap block, THE Dashboard SHALL display a tooltip showing the time range (start and end hour) and the count of monitors in each state (`up`, `down`, `unknown`) during that period
4. THE Heatmap SHALL label the time axis with hour markers in the user's local timezone, displaying a label every 3 hours (8 labels total) to avoid visual clutter
5. IF the history data request fails, THEN THE Dashboard SHALL display the Heatmap area with an error message indicating data could not be loaded and a retry control
6. WHEN new check results arrive via WebSocket, THE Dashboard SHALL update the current hour's Heatmap block within 5 seconds without requiring a full page reload

### Requirement 7: Recent Events Feed

**User Story:** As an operator, I want to see the last state transitions across all monitors, so that I can understand what changed recently without checking each monitor individually.

#### Acceptance Criteria

1. THE Events_Feed SHALL display the 10 most recent state transitions across all monitors in reverse chronological order, where a state transition is defined as a change between any two distinct states (`up`, `down`, `unknown`)
2. THE Events_Feed SHALL display each event with the monitor name (resolved from the monitor store), the transition (previous state → new state), and a relative timestamp formatted as a human-readable duration (e.g., "3 min ago", "2 hours ago") that updates at least every 30 seconds
3. WHEN a WebSocket `monitor_status` patch arrives and the patch `state` differs from the monitor's current state in the store, THE Events_Feed SHALL prepend the new event to the list and remove the oldest event if the list exceeds 10 entries
4. WHEN the Events_Feed is first rendered, THE Events_Feed SHALL populate the list from available incident data (resolved and open incidents across all monitors), limited to the 10 most recent transitions occurring within the last 24 hours
5. WHEN no state transitions exist in the feed (no incidents in the last 24 hours and no real-time transitions observed since page load), THE Events_Feed SHALL display a message indicating no recent activity
6. THE Events_Feed SHALL visually distinguish recovery events (transition to `up`) from failure events (transition to `down`) using the project's CSS custom property color tokens
7. THE Events_Feed SHALL be session-scoped: real-time events accumulated via WebSocket are retained while the application session is active, and cleared on logout or page refresh

### Requirement 8: Dashboard Layout and Responsiveness

**User Story:** As an operator, I want the Dashboard to be usable on both desktop and tablet screens, so that I can check operational health from different devices.

#### Acceptance Criteria

1. THE Dashboard SHALL arrange widgets in a responsive grid of 3 columns at viewport widths of 768px and above, collapsing to a single-column stacked layout at viewport widths below 768px
2. THE Dashboard SHALL render the Health_Score widget and the Incidents_Panel widget as the first and second elements in DOM order, ensuring they appear as the topmost visible widgets regardless of viewport width
3. THE Dashboard SHALL initiate all widget data requests in parallel so that no widget request blocks another widget from loading
4. IF any individual widget data request fails, THEN THE Dashboard SHALL display an error indicator within that widget's boundaries describing the failure, and provide a retry action that re-fetches that widget's data, without affecting the rendering or data of other widgets
5. THE Dashboard SHALL not render a monitor list or individual MonitorRow components
6. WHILE any widget data request is in progress, THE Dashboard SHALL display an animated loading skeleton placeholder within that widget's boundaries until data arrives or the request fails
7. THE Dashboard SHALL remain scrollable and interactive at viewport widths from 768px down to 320px with no horizontal overflow

### Requirement 9: Real-Time Data Freshness

**User Story:** As an operator, I want the Dashboard to reflect the current state of my infrastructure without manual refresh, so that I can trust the information is up-to-date.

#### Acceptance Criteria

1. WHILE a WebSocket connection is active, THE Dashboard SHALL update the Health_Score, Status_Ring, Incidents_Panel, and Events_Feed within 500 milliseconds of receiving a `monitor_status` patch
2. WHEN the WebSocket connection transitions from disconnected to connected, THE Dashboard SHALL reload all widget data from the REST API and replace local state with the fetched data before resuming patch application
3. THE Dashboard SHALL display a single global "last updated" timestamp in relative format (e.g., "5s ago", "2m ago") that reflects the most recent WebSocket message receipt or REST API fetch completion, and SHALL refresh the displayed relative value at least every 5 seconds
4. IF the Dashboard has not received any WebSocket message for 60 seconds, THEN THE Dashboard SHALL display a non-modal stale-data indicator (badge or text style change) adjacent to the "last updated" timestamp
5. WHEN the Dashboard receives a WebSocket message after the stale-data indicator is visible, THE Dashboard SHALL remove the stale-data indicator and reset the staleness timer
