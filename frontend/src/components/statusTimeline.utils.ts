/**
 * Pure utility functions extracted from StatusTimeline.svelte
 * for property-based testing of segment-building logic.
 */
import type { HistoryPoint } from '$lib/types';

export interface FailureDetail {
  status_code: number | null;
  error: string | null;
  count: number;
}

export interface Segment {
  state: 'up' | 'down';
  startTime: number;
  endTime: number;
  widthPercent: number;
  points: number;
  failures: FailureDetail[];
}

/**
 * Collects failure details from a map into a sorted array (by count descending).
 */
function collectFailures(failureMap: Map<string, FailureDetail>): FailureDetail[] {
  return Array.from(failureMap.values()).sort((a, b) => b.count - a.count);
}

/**
 * Adds a HistoryPoint's failure info to the failure map.
 */
function trackFailure(
  failureMap: Map<string, FailureDetail>,
  pt: HistoryPoint
): void {
  const key = `${pt.status_code}|${pt.error}`;
  const existing = failureMap.get(key);
  if (existing) {
    existing.count++;
  } else {
    failureMap.set(key, {
      status_code: pt.status_code,
      error: pt.error,
      count: 1
    });
  }
}

/**
 * Builds contiguous segments from HistoryPoint data.
 * This is the exact same algorithm used in StatusTimeline.svelte.
 */
export function buildSegments(data: HistoryPoint[]): Segment[] {
  if (data.length === 0) return [];

  // Sort by time ascending
  const sorted = [...data].sort(
    (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
  );

  const totalStart = new Date(sorted[0].checked_at).getTime();
  const totalEnd = new Date(sorted[sorted.length - 1].checked_at).getTime();
  const totalDuration = totalEnd - totalStart;

  if (totalDuration === 0) {
    // Single point or all at the same time
    const failureMap = new Map<string, FailureDetail>();
    for (const pt of sorted) {
      if (pt.state === 'down') {
        trackFailure(failureMap, pt);
      }
    }
    return [
      {
        state: sorted[0].state,
        startTime: totalStart,
        endTime: totalEnd,
        widthPercent: 100,
        points: sorted.length,
        failures: sorted[0].state === 'down' ? collectFailures(failureMap) : []
      }
    ];
  }

  // Build contiguous segments of same state
  const result: Segment[] = [];
  let currentState = sorted[0].state;
  let segStart = totalStart;
  let pointCount = 1;
  let failureMap = new Map<string, FailureDetail>();

  // Track the first point
  if (sorted[0].state === 'down') {
    trackFailure(failureMap, sorted[0]);
  }

  for (let i = 1; i < sorted.length; i++) {
    const pt = sorted[i];
    const ptState = pt.state as 'up' | 'down';

    if (ptState !== currentState) {
      // End of current segment — boundary is midpoint between last and current
      const prevTime = new Date(sorted[i - 1].checked_at).getTime();
      const currTime = new Date(pt.checked_at).getTime();
      const boundary = prevTime + (currTime - prevTime) / 2;

      result.push({
        state: currentState,
        startTime: segStart,
        endTime: boundary,
        widthPercent: ((boundary - segStart) / totalDuration) * 100,
        points: pointCount,
        failures: currentState === 'down' ? collectFailures(failureMap) : []
      });

      // Reset for new segment
      currentState = ptState;
      segStart = boundary;
      pointCount = 1;
      failureMap = new Map<string, FailureDetail>();
      if (ptState === 'down') {
        trackFailure(failureMap, pt);
      }
    } else {
      pointCount++;
      if (ptState === 'down') {
        trackFailure(failureMap, pt);
      }
    }
  }

  // Final segment
  result.push({
    state: currentState,
    startTime: segStart,
    endTime: totalEnd,
    widthPercent: ((totalEnd - segStart) / totalDuration) * 100,
    points: pointCount,
    failures: currentState === 'down' ? collectFailures(failureMap) : []
  });

  return result;
}

/**
 * Computes uptime percentage from HistoryPoint data.
 * Returns null for empty data, otherwise "XX.X" string.
 */
export function computeUptimePercent(data: HistoryPoint[]): string | null {
  if (data.length === 0) return null;
  const upCount = data.filter((p) => p.state === 'up').length;
  return ((upCount / data.length) * 100).toFixed(1);
}

/**
 * Truncates a string to maxLen characters, appending "…" if truncated.
 */
function truncate(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str;
  return str.slice(0, maxLen) + '…';
}

/**
 * Formats tooltip text for a segment.
 * - "up" segments: "Healthy — [time range] (N checks)"
 * - "down" segments: includes most common status code and first error message
 */
export function formatSegmentTooltip(segment: Segment): string {
  const start = formatDateTime(segment.startTime);
  const end = formatDateTime(segment.endTime);
  const plural = segment.points > 1 ? 's' : '';

  if (segment.state === 'up') {
    return `Healthy — ${start} → ${end} (${segment.points} check${plural})`;
  }

  // "down" segment: include failure details
  let tooltip = `Unhealthy — ${start} → ${end} (${segment.points} check${plural})`;

  if (segment.failures.length > 0) {
    // Find the most common failure that has a status_code
    const withStatusCode = segment.failures.find((f) => f.status_code !== null);
    // Find the most common failure that has an error message
    const withError = segment.failures.find((f) => f.error !== null);

    if (withStatusCode) {
      tooltip += ` | HTTP ${withStatusCode.status_code}`;
    }

    if (withError) {
      tooltip += ` | ${truncate(withError.error!, 60)}`;
    }
  }

  return tooltip;
}

function formatDateTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  });
}
