# Status Timeline Failure Details Bugfix Design

## Overview

The `StatusTimeline.svelte` component discards `status_code` and `error` fields when building display segments from `HistoryPoint[]` data. Users hovering or clicking "down" segments see only a generic "Unhealthy — time range (N checks)" tooltip with no indication of *why* the monitor failed. The fix retains failure metadata in the `Segment` interface, enhances the hover tooltip to show representative failure info, and adds a click popover for full details. This is a frontend-only change — the backend already returns the data.

## Glossary

- **Bug_Condition (C)**: A "down" segment is rendered without any failure detail information (status codes and error messages are discarded during segment construction)
- **Property (P)**: When a "down" segment is hovered or clicked, the user sees HTTP status codes and/or error messages from the underlying checks
- **Preservation**: Existing "up" segment tooltips, segment geometry (boundaries, widths, point counts), empty-state rendering, and overall timeline layout must remain unchanged
- **Segment**: A contiguous group of `HistoryPoint` entries sharing the same `state`, rendered as a colored bar section in the timeline
- **HistoryPoint**: A single check result from `GET /monitors/{id}/history` containing `state`, `latency_ms`, `status_code`, `error`, `checked_at`
- **StatusTimeline.svelte**: The Svelte 5 component at `frontend/src/components/StatusTimeline.svelte` responsible for rendering the timeline bar

## Bug Details

### Bug Condition

The bug manifests when the `StatusTimeline` component processes `HistoryPoint[]` data that includes "down" entries with non-null `status_code` and/or `error` fields. The segment-building logic only extracts `state`, `startTime`, `endTime`, `widthPercent`, and `points` — all failure detail fields are dropped. As a result, no failure information is available for display in tooltips or click interactions.

**Formal Specification:**
```
FUNCTION isBugCondition(input)
  INPUT: input of type { segment: Segment, historyPoints: HistoryPoint[] }
  OUTPUT: boolean
  
  RETURN input.segment.state == 'down'
         AND input.historyPoints.some(p => p.status_code != null OR p.error != null)
         AND input.segment.failureDetails == undefined
END FUNCTION
```

### Examples

- User hovers a red segment containing 5 checks that all returned HTTP 503 → sees "Unhealthy — Mar 5, 14:00 → 14:25 (5 checks)" with no mention of 503
- User hovers a red segment where error is "connection refused" → sees generic tooltip, no error message
- User clicks a red segment expecting detailed breakdown → nothing happens, no popover appears
- Edge case: A "down" segment where `status_code` is null but `error` is "DNS resolution failed" → error should still be shown

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- "Up" segment tooltips must continue displaying "Healthy — [time range] (N checks)" without any failure detail fields
- Segment geometry (boundaries, widths, point counts) must remain identical for the same input data
- The empty-state placeholder ("No check data available") must continue to render when `data` is empty
- The uptime percentage calculation and display must remain unchanged
- Time range labels below the timeline must remain unchanged
- The timeline bar dimensions, border radius, and color scheme for "up" segments must not change

**Scope:**
All inputs that do NOT involve "down" segments should be completely unaffected by this fix. This includes:
- All "up" segment rendering and tooltip behavior
- Empty data rendering
- Segment boundary calculation algorithm
- Uptime percentage computation
- Time range label formatting

## Hypothesized Root Cause

Based on the code analysis of `StatusTimeline.svelte`, the root cause is clear:

1. **Segment interface is too narrow**: The `Segment` interface only defines `state`, `startTime`, `endTime`, `widthPercent`, and `points`. There are no fields for `status_code` or `error` data.

2. **Segment-building loop discards fields**: The `$derived.by` block that iterates over sorted `HistoryPoint[]` entries only tracks `state` changes and point counts. It never reads `pt.status_code` or `pt.error` from the history points.

3. **No click interaction exists**: The segment `<div>` elements only have a `title` attribute for native browser tooltips. There is no `onclick` handler or popover component.

4. **Tooltip uses string interpolation with no failure data**: The `title` attribute is built from `segment.state`, `formatDateTime(segment.startTime)`, `formatDateTime(segment.endTime)`, and `segment.points` — no failure data is available to include.

## Correctness Properties

Property 1: Bug Condition - Down Segments Display Failure Details

_For any_ "down" segment built from HistoryPoint data where at least one point has a non-null `status_code` or `error`, the fixed component SHALL retain those failure details in the Segment data structure and display representative failure information (status code and/or error message) in the hover tooltip.

**Validates: Requirements 2.1, 2.2**

Property 2: Preservation - Up Segments and Geometry Unchanged

_For any_ segment where state is "up", or for any computation of segment boundaries, widths, point counts, uptime percentage, or time range labels, the fixed component SHALL produce exactly the same output as the original component, preserving all existing non-failure-detail behavior.

**Validates: Requirements 3.1, 3.2, 3.3, 3.4**

## Fix Implementation

### Changes Required

**File**: `frontend/src/components/StatusTimeline.svelte`

**Specific Changes**:

1. **Extend the Segment interface**: Add optional failure detail fields to carry information from constituent HistoryPoints:
   ```typescript
   interface FailureDetail {
     status_code: number | null;
     error: string | null;
     count: number; // how many points had this specific combination
   }

   interface Segment {
     state: 'up' | 'down';
     startTime: number;
     endTime: number;
     widthPercent: number;
     points: number;
     failures: FailureDetail[]; // populated only for 'down' segments
   }
   ```

2. **Collect failure details during segment building**: In the segment-construction loop, accumulate `status_code` and `error` from each HistoryPoint into a map, then flatten into the `failures` array when closing a segment:
   - Track a `Map<string, FailureDetail>` keyed by `${status_code}|${error}`
   - Increment `count` for duplicate combinations
   - Attach the collected failures when pushing a completed segment
   - For "up" segments, set `failures` to an empty array

3. **Enhance hover tooltip for "down" segments**: Replace the native `title` attribute with a richer tooltip that includes:
   - The time range and check count (existing)
   - The most common status code (e.g., "HTTP 503")
   - The first error message (truncated if long)

4. **Add click popover for "down" segments**: Implement a click-triggered popover that shows:
   - Full list of distinct failure reasons (status code + error) with occurrence counts
   - Time range of the segment
   - Total check count
   - Use Svelte 5 `$state` for popover visibility, positioned relative to the clicked segment

5. **Preserve "up" segment behavior**: The tooltip for "up" segments remains unchanged — "Healthy — [time range] (N checks)" with no failure fields shown.

## Testing Strategy

### Validation Approach

The testing strategy follows a two-phase approach: first, surface counterexamples that demonstrate the bug on unfixed code, then verify the fix works correctly and preserves existing behavior.

### Exploratory Bug Condition Checking

**Goal**: Surface counterexamples that demonstrate the bug BEFORE implementing the fix. Confirm that the current Segment interface lacks failure data and that tooltips contain no error information.

**Test Plan**: Write component tests that render `StatusTimeline` with "down" HistoryPoints containing `status_code` and `error` values. Assert that failure information is NOT present in the rendered output. Run these on the UNFIXED code to confirm the defect.

**Test Cases**:
1. **Missing status code in tooltip**: Render timeline with a "down" point having `status_code: 503` — assert tooltip does NOT contain "503" (will pass on unfixed code, confirming bug)
2. **Missing error in tooltip**: Render timeline with a "down" point having `error: "connection refused"` — assert tooltip does NOT contain "connection refused" (will pass on unfixed code)
3. **No click handler**: Click a "down" segment — assert no popover element appears (will pass on unfixed code)
4. **Segment interface check**: Inspect rendered segment data — assert no `failures` property exists (will pass on unfixed code)

**Expected Counterexamples**:
- Failure details are absent from all rendered tooltip text
- No interactive elements respond to clicks on "down" segments
- The Segment data structure contains only geometric/state data

### Fix Checking

**Goal**: Verify that for all inputs where the bug condition holds, the fixed component displays failure information.

**Pseudocode:**
```
FOR ALL input WHERE isBugCondition(input) DO
  result := renderStatusTimeline_fixed(input.historyPoints)
  segment := findDownSegment(result)
  ASSERT segment.failures.length > 0
  ASSERT tooltipContains(segment, statusCodeOrError)
  ASSERT clickRevealsPopover(segment)
END FOR
```

### Preservation Checking

**Goal**: Verify that for all inputs where the bug condition does NOT hold, the fixed component produces the same output as the original.

**Pseudocode:**
```
FOR ALL input WHERE NOT isBugCondition(input) DO
  ASSERT renderStatusTimeline_original(input) = renderStatusTimeline_fixed(input)
END FOR
```

**Testing Approach**: Property-based testing is recommended for preservation checking because:
- It generates many random `HistoryPoint[]` arrays with varying states, timestamps, and point counts
- It catches edge cases in segment boundary calculation that manual tests might miss
- It provides strong guarantees that geometry and "up" tooltip behavior is unchanged

**Test Plan**: Capture the segment-building output of the UNFIXED code for "up"-only inputs, then write property-based tests verifying the fixed code produces identical segments for the same inputs.

**Test Cases**:
1. **Up segment tooltip preservation**: Generate random all-"up" HistoryPoint arrays — verify tooltips match original format exactly
2. **Segment geometry preservation**: Generate random mixed HistoryPoint arrays — verify `startTime`, `endTime`, `widthPercent`, `points` are identical between original and fixed
3. **Empty state preservation**: Verify empty `data` prop still renders the placeholder message
4. **Uptime percentage preservation**: Generate random data — verify uptime calculation is identical

### Unit Tests

- Segment building with "down" points correctly populates `failures` array
- Segment building with "up" points produces empty `failures` array
- Multiple distinct error types in one segment are grouped and counted
- Tooltip text includes status code for "down" segments
- Tooltip text includes error message for "down" segments
- Click on "down" segment opens popover with full failure details
- Click on "up" segment does NOT open popover
- Popover closes when clicking outside or pressing Escape

### Property-Based Tests

- Generate random `HistoryPoint[]` with arbitrary states, status codes, and errors — verify all "down" segments have non-empty `failures` when source points have error data
- Generate random all-"up" `HistoryPoint[]` — verify segment geometry (boundaries, widths, counts) is bit-for-bit identical to the original algorithm
- Generate random mixed arrays — verify uptime percentage matches `upCount / total * 100`

### Integration Tests

- Render full monitor detail page with history containing failures — verify timeline shows failure info on hover
- Render timeline, hover a "down" segment, verify tooltip content includes HTTP status code
- Render timeline, click a "down" segment, verify popover appears with failure breakdown
- Render timeline with only "up" data, verify no failure detail UI elements exist
