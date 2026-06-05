# Bugfix Requirements Document

## Introduction

This document covers two bugs in the status timeline on the monitor detail page:

1. **Missing failure details**: The timeline does not surface failure reasons when users hover or click on "down" (red) segments. The backend already returns `status_code` and `error` fields in the history API response, and the frontend `HistoryPoint` type includes these fields. However, the `StatusTimeline.svelte` component discards this information when building display segments and only shows generic "Unhealthy — time range (N checks)" in the tooltip. Users cannot determine **why** a monitor went down without navigating away to the incidents or history list.

2. **No real-time timeline updates**: The status timeline does not update when new check results arrive via WebSocket. The `history` array that feeds the `StatusTimeline` component is only loaded once on page mount via `getMonitorHistory()`. When a `monitor_status` WebSocket message arrives, `monitorStore.applyPatch()` updates the monitor's state badge, but the timeline data remains stale until the user manually reloads the page.

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN a user hovers over a "down" segment in the status timeline THEN the system only displays "Unhealthy — [time range] (N checks)" with no failure reason information

1.2 WHEN the StatusTimeline component builds segments from HistoryPoint data THEN the system discards the `status_code` and `error` fields, making them unavailable for display

1.3 WHEN a user clicks on a "down" segment in the status timeline THEN the system does not reveal any failure details (HTTP status code or error message)

1.4 WHEN a WebSocket `monitor_status` message arrives for the currently viewed monitor THEN the system only updates the monitor's state badge but does NOT append the new data point to the status timeline

1.5 WHEN multiple check results arrive via WebSocket while the user is viewing a monitor detail page THEN the status timeline becomes stale and shows outdated 24h history until the page is reloaded

### Expected Behavior (Correct)

2.1 WHEN a user hovers over a "down" segment in the status timeline THEN the system SHALL display failure details including the HTTP status code (e.g. "503 Service Unavailable") and/or error message from the checks in that segment

2.2 WHEN the StatusTimeline component builds segments from HistoryPoint data THEN the system SHALL retain representative `status_code` and `error` information from the underlying HistoryPoints for each "down" segment

2.3 WHEN a user clicks on a "down" segment in the status timeline THEN the system SHALL show a popover or expanded tooltip with the full failure details (status codes and error messages from the checks in that segment)

2.4 WHEN a WebSocket `monitor_status` message arrives for the currently viewed monitor THEN the system SHALL append the new check result as a HistoryPoint to the timeline data and the StatusTimeline component SHALL re-render to reflect the updated state

2.5 WHEN new check results are appended to the timeline in real-time THEN the system SHALL maintain the 24h sliding window by dropping points older than 24 hours from the displayed data

### Unchanged Behavior (Regression Prevention)

3.1 WHEN a user hovers over an "up" segment in the status timeline THEN the system SHALL CONTINUE TO display "Healthy — [time range] (N checks)" without failure detail fields

3.2 WHEN all history points have state "up" THEN the system SHALL CONTINUE TO render a fully green timeline bar with no error information

3.3 WHEN the timeline data is empty THEN the system SHALL CONTINUE TO display the "No check data available" placeholder message

3.4 WHEN the StatusTimeline component builds segments THEN the system SHALL CONTINUE TO correctly compute segment boundaries, widths, time ranges, and point counts as before

3.5 WHEN a WebSocket `monitor_status` message arrives for a DIFFERENT monitor (not the one being viewed) THEN the system SHALL CONTINUE TO only update that monitor's state in the store without affecting the current timeline view

3.6 WHEN the WebSocket connection is lost and reconnects THEN the system SHALL CONTINUE TO use the existing reconnection backoff logic and the timeline SHALL show whatever data was last loaded plus any new WS points received after reconnection
