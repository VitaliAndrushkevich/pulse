# Implementation Plan

## Overview

Bugfix implementation covering two bugs in the StatusTimeline component on the monitor detail page:

1. **Bug #1 — Missing failure details**: "Down" segments discard `status_code` and `error` fields, showing only generic "Unhealthy — time range (N checks)" in tooltips with no failure reason.
2. **Bug #2 — No real-time timeline updates**: The `history` array that feeds `StatusTimeline` is loaded once on mount. WebSocket `monitor_status` messages update the monitor state badge but do NOT append new data points to the timeline.

Follows the bug condition methodology for each bug: explore with property tests → write preservation tests → implement fix → verify.

## Tasks

---

### Bug #1: Missing Failure Details in Timeline Tooltips/Popovers

- [x] 1. Write bug condition exploration test for failure details
  - **Property 1: Bug Condition** - Down Segments Missing Failure Details
  - **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
  - **DO NOT attempt to fix the test or the code when it fails**
  - **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes after implementation
  - **GOAL**: Surface counterexamples that demonstrate the bug exists
  - **Scoped PBT Approach**: Scope the property to concrete failing cases: "down" HistoryPoints with non-null `status_code` or `error` fields
  - Create test file `frontend/src/components/StatusTimeline.test.ts` (or extend existing)
  - Use `fast-check` to generate arbitrary "down" HistoryPoint arrays where at least one point has `status_code != null` or `error != null`
  - Property: for all such inputs, the rendered component MUST contain failure detail text (status code number or error message substring) in tooltip or popover content
  - Formally: `fc.assert(fc.property(downHistoryPointsWithErrors, (points) => { render StatusTimeline with points; assert segment tooltip or popover includes status_code or error text }))`
  - Run test on UNFIXED code
  - **EXPECTED OUTCOME**: Test FAILS (this is correct - it proves the bug exists because current code discards failure details)
  - Document counterexamples found (e.g., "rendered tooltip for segment with status_code=503 contains only 'Unhealthy — time range (N checks)', no '503' present")
  - Mark task complete when test is written, run, and failure is documented
  - _Requirements: 1.1, 1.2, 1.3, 2.1, 2.2_

- [x] 2. Write preservation property tests for failure details fix (BEFORE implementing fix)
  - **Property 2: Preservation** - Up Segments and Geometry Unchanged
  - **IMPORTANT**: Follow observation-first methodology
  - **IMPORTANT**: Write these tests BEFORE implementing any fix
  - Observe behavior on UNFIXED code for non-buggy inputs (all-"up" HistoryPoint arrays, empty arrays, mixed arrays where we only check "up" segment behavior)
  - Write property-based tests using `fast-check` capturing observed behavior patterns:
    - **Segment geometry preservation**: Generate random `HistoryPoint[]` arrays with arbitrary states and timestamps — verify `startTime`, `endTime`, `widthPercent`, and `points` values are deterministic and match the current algorithm output
    - **Up segment tooltip preservation**: For all "up" segments, verify tooltip text matches format "Healthy — [time range] (N checks)" with no failure detail content
    - **Empty state preservation**: Verify empty `data` prop renders "No check data available" placeholder
    - **Uptime percentage preservation**: For random HistoryPoint arrays, verify uptime equals `upCount / total * 100` formatted to 1 decimal place
  - Extract the segment-building logic into a testable pure function (or test via component rendering) to enable property-based testing of geometry
  - Formally: `fc.assert(fc.property(arbitraryHistoryPoints, (points) => { segments = buildSegments(points); for each "up" segment: tooltip matches "Healthy — ..." format; geometry fields are consistent with algorithm }))`
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS (this confirms baseline behavior to preserve)
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.1, 3.2, 3.3, 3.4_

- [x] 3. Fix for down segments missing failure details

  - [x] 3.1 Extend Segment interface and add FailureDetail type
    - Add `FailureDetail` interface: `{ status_code: number | null; error: string | null; count: number }`
    - Add `failures: FailureDetail[]` field to `Segment` interface
    - For "up" segments, `failures` is always an empty array
    - _Bug_Condition: isBugCondition(input) where segment.state == 'down' AND historyPoints have status_code or error AND segment.failureDetails == undefined_
    - _Expected_Behavior: segment.failures contains aggregated FailureDetail entries from constituent HistoryPoints_
    - _Preservation: "up" segments have failures = [], geometry fields unchanged_
    - _Requirements: 2.2, 3.4_

  - [x] 3.2 Update segment-building loop to collect failure details
    - Track a `Map<string, FailureDetail>` keyed by `${status_code}|${error}` during segment construction
    - Increment `count` for duplicate status_code + error combinations
    - When closing a "down" segment, attach collected failures array sorted by count descending
    - When closing an "up" segment, set failures to empty array
    - Reset the failure map when starting a new segment
    - _Bug_Condition: segment-building loop discards status_code and error fields_
    - _Expected_Behavior: failures array populated for "down" segments from HistoryPoint data_
    - _Preservation: segment boundaries, widths, point counts, start/end times remain identical_
    - _Requirements: 2.2, 3.4_

  - [x] 3.3 Enhance hover tooltip for "down" segments
    - Replace native `title` attribute with a custom Svelte tooltip (or enhanced title) for "down" segments
    - Show: time range, check count (existing), plus most common status code (e.g., "HTTP 503") and first error message (truncated to ~60 chars if longer)
    - Keep "up" segment tooltip unchanged: "Healthy — [time range] (N checks)"
    - _Bug_Condition: tooltip shows only generic "Unhealthy — time range" with no failure reason_
    - _Expected_Behavior: tooltip includes representative status_code and/or error from failures array_
    - _Preservation: "up" segment tooltips remain "Healthy — [time range] (N checks)"_
    - _Requirements: 2.1, 3.1_

  - [x] 3.4 Add click popover for "down" segments
    - Add `onclick` handler to "down" segment divs
    - Implement a popover component (positioned relative to clicked segment) using Svelte 5 `$state` for visibility
    - Popover shows: full list of distinct failure reasons (status code + error) with occurrence counts, time range, total check count
    - Close popover on click-outside or Escape key press
    - "Up" segments do NOT get click handlers or popovers
    - _Bug_Condition: clicking a "down" segment does nothing_
    - _Expected_Behavior: click opens popover with full failure breakdown_
    - _Preservation: "up" segments have no click interaction_
    - _Requirements: 2.3, 3.1_

  - [x] 3.5 Verify bug condition exploration test now passes
    - **Property 1: Expected Behavior** - Down Segments Display Failure Details
    - **IMPORTANT**: Re-run the SAME test from task 1 - do NOT write a new test
    - The test from task 1 encodes the expected behavior (failure details visible in tooltip/popover)
    - When this test passes, it confirms the expected behavior is satisfied
    - Run bug condition exploration test from step 1
    - **EXPECTED OUTCOME**: Test PASSES (confirms bug is fixed)
    - _Requirements: 2.1, 2.2, 2.3_

  - [x] 3.6 Verify preservation tests still pass
    - **Property 2: Preservation** - Up Segments and Geometry Unchanged
    - **IMPORTANT**: Re-run the SAME tests from task 2 - do NOT write new tests
    - Run preservation property tests from step 2
    - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions)
    - Confirm all preservation tests still pass after fix (no regressions to "up" segment behavior, geometry, empty state, or uptime percentage)
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

---

### Bug #2: No Real-Time Timeline Updates via WebSocket

- [x] 4. Write bug condition exploration test for missing real-time updates
  - **Property 3: Bug Condition** - WebSocket Messages Not Appended to Timeline
  - **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
  - **DO NOT attempt to fix the test or the code when it fails**
  - **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes after implementation
  - **GOAL**: Surface counterexamples that demonstrate the timeline does not update in real-time
  - **Scoped PBT Approach**: Scope the property to concrete failing cases: a rendered monitor detail page receives a `monitor_status` WS message for the currently viewed monitor, but the `history` array length does not increase
  - Create test file `frontend/src/routes/monitors/[id]/MonitorDetail.timeline-ws.test.ts` (or extend existing)
  - Use `fast-check` to generate arbitrary `MonitorPatch` payloads (varying `state`, `latency_ms`, `status_code`, `error`, `checked_at`) for the current monitor
  - Property: after a WS `monitor_status` message arrives for the viewed monitor, the timeline's `data` prop length increases by 1 and the last point matches the WS payload fields (`state`, `latency_ms`, `status_code`, `error`, `checked_at`)
  - Simulate: render detail page with initial history, fire `onMessage` callback with a `monitor_status` message matching current `monitorId`, assert timeline data updated
  - Run test on UNFIXED code
  - **EXPECTED OUTCOME**: Test FAILS (this is correct - it proves the bug exists because the page does not listen for WS messages to update timeline)
  - Document counterexamples found (e.g., "after WS message {monitor_id: 'abc', state: 'down', ...}, history array length remains N instead of N+1")
  - Mark task complete when test is written, run, and failure is documented
  - _Requirements: 1.4, 1.5, 2.4_

- [x] 5. Write preservation property tests for real-time updates (BEFORE implementing fix)
  - **Property 4: Preservation** - WS Messages for Other Monitors Do Not Affect Timeline
  - **IMPORTANT**: Follow observation-first methodology
  - **IMPORTANT**: Write these tests BEFORE implementing any fix
  - Observe behavior on UNFIXED code for non-buggy inputs (WS messages for DIFFERENT monitors, reconnection scenarios)
  - Write property-based tests using `fast-check` capturing observed behavior patterns:
    - **Different-monitor WS messages**: Generate WS `monitor_status` payloads where `monitor_id != currentMonitorId` — verify the timeline `history` array is unchanged (length, content)
    - **Reconnection preserves existing data**: Simulate WS disconnect+reconnect — verify the timeline still contains the previously loaded history (no data loss)
    - **Initial load behavior**: Verify that `getMonitorHistory()` data is correctly loaded into the timeline on mount (existing behavior preserved)
  - Formally: `fc.assert(fc.property(otherMonitorPatches, (patch) => { fire WS message for different monitor; assert history array unchanged }))`
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS (this confirms baseline behavior to preserve)
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.5, 3.6_

- [x] 6. Fix for missing real-time timeline updates via WebSocket

  - [x] 6.1 Add WS message listener on monitor detail page for timeline updates
    - In `frontend/src/routes/monitors/[id]/+page.svelte`, subscribe to WS messages using the `onMessage` callback
    - Use a `$effect` that registers a listener (or use an existing global WS client's `onMessage` hook) filtering for `type === 'monitor_status'` and `payload.monitor_id === monitorId`
    - When a matching message arrives, convert `MonitorPatch` to `HistoryPoint`:
      ```
      { state: patch.state (cast to 'up'|'down'), latency_ms: patch.latency_ms, status_code: patch.status_code ?? null, error: patch.error ?? null, checked_at: patch.checked_at }
      ```
    - Note: `MonitorPatch.state` includes `'unknown'` but `HistoryPoint.state` only allows `'up'|'down'` — map `'unknown'` to `'down'` (or skip the point)
    - Append the new `HistoryPoint` to the `history` array (reactive update via `history = [...history, newPoint]`)
    - _Bug_Condition: WS monitor_status message for current monitor arrives but history array is not updated_
    - _Expected_Behavior: new HistoryPoint appended to history, StatusTimeline re-renders_
    - _Preservation: WS messages for other monitors do not touch timeline data_
    - _Requirements: 1.4, 1.5, 2.4, 3.5_

  - [x] 6.2 Implement 24h sliding window pruning
    - After appending a new point, compute the 24h cutoff: `Date.now() - 24 * 60 * 60 * 1000`
    - Filter out any points with `checked_at` earlier than the cutoff
    - Apply this as: `history = history.filter(p => new Date(p.checked_at).getTime() >= cutoff)`
    - This maintains a rolling 24h window so the timeline doesn't grow unbounded
    - _Bug_Condition: timeline shows stale data beyond 24h window_
    - _Expected_Behavior: points older than 24h from current time are pruned on each new WS point arrival_
    - _Preservation: existing points within the 24h window are not removed_
    - _Requirements: 2.5_

  - [x] 6.3 Clean up WS listener on page navigation
    - Ensure the WS message listener is removed when the detail page component unmounts (user navigates away)
    - Use `$effect` return cleanup function or an explicit unsubscribe pattern
    - Prevent memory leaks and stale updates to unmounted components
    - _Bug_Condition: listener remains active after navigation, causing stale state updates_
    - _Expected_Behavior: listener removed on unmount, no updates to history after navigation_
    - _Preservation: WS reconnection logic and monitorStore.applyPatch behavior unchanged_
    - _Requirements: 3.6_

  - [x] 6.4 Verify bug condition exploration test now passes
    - **Property 3: Expected Behavior** - WebSocket Messages Append to Timeline
    - **IMPORTANT**: Re-run the SAME test from task 4 - do NOT write a new test
    - The test from task 4 encodes the expected behavior (WS messages update timeline data)
    - When this test passes, it confirms the expected behavior is satisfied
    - Run bug condition exploration test from step 4
    - **EXPECTED OUTCOME**: Test PASSES (confirms bug is fixed)
    - _Requirements: 2.4, 2.5_

  - [x] 6.5 Verify preservation tests still pass
    - **Property 4: Preservation** - WS Messages for Other Monitors Do Not Affect Timeline
    - **IMPORTANT**: Re-run the SAME tests from task 5 - do NOT write new tests
    - Run preservation property tests from step 5
    - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions)
    - Confirm WS messages for other monitors still have no effect on timeline, reconnection still works
    - _Requirements: 3.5, 3.6_

---

### Final Checkpoint

- [x] 7. Checkpoint - Ensure all tests pass
  - Run full frontend test suite: `pnpm test` from `frontend/` directory
  - Ensure all property-based tests pass:
    - Property 1 (Bug Condition → Expected Behavior): failure details shown in down segments
    - Property 2 (Preservation): up segments, geometry, empty state unchanged
    - Property 3 (Bug Condition → Expected Behavior): WS messages append to timeline
    - Property 4 (Preservation): other-monitor WS messages don't affect timeline
  - Ensure all existing 141 frontend unit tests continue to pass
  - Ask the user if questions arise

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1", "2", "4", "5"] },
    { "id": 1, "tasks": ["3.1"] },
    { "id": 2, "tasks": ["3.2"] },
    { "id": 3, "tasks": ["3.3"] },
    { "id": 4, "tasks": ["3.4"] },
    { "id": 5, "tasks": ["3.5", "3.6"] },
    { "id": 6, "tasks": ["6.1"] },
    { "id": 7, "tasks": ["6.2"] },
    { "id": 8, "tasks": ["6.3"] },
    { "id": 9, "tasks": ["6.4", "6.5"] },
    { "id": 10, "tasks": ["7"] }
  ]
}
```

## Notes

- Tasks 1, 2, 4, and 5 are independent and can be done in parallel (all run on UNFIXED code)
- Tasks 1 and 4 are expected to FAIL — this confirms both bugs exist. Do not attempt to fix them.
- Tasks 2 and 5 are expected to PASS — this captures baseline behavior to preserve.
- Bug #1 implementation (tasks 3.1–3.4) depends on tasks 1 and 2 being complete.
- Bug #2 implementation (tasks 6.1–6.3) depends on tasks 4 and 5 being complete.
- Bug #1 and Bug #2 implementations are independent of each other and could theoretically be parallelized, but the dependency graph sequences them for clarity.
- Verification tasks (3.5, 3.6, 6.4, 6.5) re-run existing tests — no new test code needed.
- For Bug #2, the WS client already dispatches to `monitorStore.applyPatch()` — the detail page needs to ALSO capture these events for timeline updates via the `onMessage` callback.
- `MonitorPatch.state` includes `'unknown'` which `HistoryPoint.state` does not — implementation must handle this mapping.
- Test command: `pnpm test` from `frontend/` directory (uses Vitest with fast-check).
