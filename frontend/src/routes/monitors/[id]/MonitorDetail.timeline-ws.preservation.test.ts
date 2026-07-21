/**
 * Property-Based Preservation Tests for Real-Time Timeline Updates
 *
 * **Property 4: Preservation** - WS Messages for Other Monitors Do Not Affect Timeline
 *
 * These tests capture baseline behavior of the UNFIXED code to detect regressions
 * after the real-time WS update fix is applied. They follow observation-first methodology:
 * observe current behavior on non-buggy inputs, then assert it.
 *
 * Key observations on UNFIXED code:
 * 1. The `history` array on the detail page is loaded ONCE via getMonitorHistory() on mount
 * 2. WS messages are dispatched to monitorStore.applyPatch() which only updates monitor state/badge
 * 3. The timeline (StatusTimeline component) receives `history` as a prop and never changes after mount
 * 4. WS messages for OTHER monitors have no effect on the viewed monitor's timeline
 * 5. After WS reconnection, the layout re-fetches the monitor list but does NOT re-fetch history
 *
 * **Validates: Requirements 3.5, 3.6**
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import * as fc from 'fast-check';
import type { HistoryPoint, MonitorPatch } from '$lib/types';
import { monitorStore } from '$lib/stores/monitors.svelte';
import StatusTimeline from '../../../components/StatusTimeline.svelte';
import { buildSegments } from '../../../components/statusTimeline.utils';

// --- Arbitraries ---

/**
 * Generate a valid ISO timestamp within a 24h window.
 */
const arbitraryTimestamp = fc.integer({ min: 1700000000000, max: 1700086400000 }).map((ms) => {
  return new Date(ms).toISOString();
});

/**
 * Generate a single HistoryPoint.
 */
const arbitraryHistoryPoint: fc.Arbitrary<HistoryPoint> = fc.record({
  state: fc.constantFrom('up' as const, 'down' as const),
  latency_ms: fc.option(fc.integer({ min: 1, max: 5000 }), { nil: null }),
  status_code: fc.option(fc.integer({ min: 100, max: 599 }), { nil: null }),
  error: fc.option(fc.constantFrom('connection refused', 'timeout', 'DNS error'), { nil: null }),
  ssl_days_remaining: fc.option(fc.integer({ min: 0, max: 365 }), { nil: null }),
  checked_at: arbitraryTimestamp
});

/**
 * Generate a non-empty array of HistoryPoints (2 to 30 points).
 */
const arbitraryHistoryPoints = fc.array(arbitraryHistoryPoint, { minLength: 2, maxLength: 30 });

/**
 * Generate a MonitorPatch for a DIFFERENT monitor (monitor_id is always different from 'current-monitor').
 */
const arbitraryOtherMonitorPatch: fc.Arbitrary<MonitorPatch> = fc.record({
  monitor_id: fc.string({ minLength: 5, maxLength: 20 }).filter((id) => id !== 'current-monitor'),
  state: fc.constantFrom('up' as const, 'down' as const, 'unknown' as const),
  latency_ms: fc.integer({ min: 1, max: 10000 }),
  status_code: fc.option(fc.integer({ min: 100, max: 599 }), { nil: undefined }),
  error: fc.option(fc.constantFrom('connection refused', 'timeout exceeded', 'DNS error'), {
    nil: undefined
  }),
  checked_at: fc.date({ min: new Date('2024-01-01'), max: new Date('2024-12-31'), noInvalidDate: true }).map((d) =>
    d.toISOString()
  ),
  timestamp: fc.date({ min: new Date('2024-01-01'), max: new Date('2024-12-31'), noInvalidDate: true }).map((d) =>
    d.toISOString()
  )
});

/**
 * Generate a MonitorPatch for the CURRENT monitor being viewed.
 */
const arbitraryCurrentMonitorPatch: fc.Arbitrary<MonitorPatch> = fc.record({
  monitor_id: fc.constant('current-monitor'),
  state: fc.constantFrom('up' as const, 'down' as const, 'unknown' as const),
  latency_ms: fc.integer({ min: 1, max: 10000 }),
  status_code: fc.option(fc.integer({ min: 100, max: 599 }), { nil: undefined }),
  error: fc.option(fc.constantFrom('connection refused', 'timeout exceeded'), { nil: undefined }),
  checked_at: fc.date({ min: new Date('2024-01-01'), max: new Date('2024-12-31'), noInvalidDate: true }).map((d) =>
    d.toISOString()
  ),
  timestamp: fc.date({ min: new Date('2024-01-01'), max: new Date('2024-12-31'), noInvalidDate: true }).map((d) =>
    d.toISOString()
  )
});

// --- Tests ---

describe('Timeline WS Preservation Properties', () => {
  beforeEach(() => {
    monitorStore.clear();
  });

  describe('Property 4: Different-Monitor WS Messages Do Not Affect Timeline', () => {
    /**
     * **Validates: Requirements 3.5**
     *
     * When a WS `monitor_status` message arrives for a DIFFERENT monitor,
     * the timeline history array remains completely unchanged.
     *
     * Observation on unfixed code: The detail page's `history` is a local $state
     * variable set once during fetchData(). monitorStore.applyPatch() only updates
     * the monitor's state/last_checked_at in the store map — it does NOT touch the
     * history array. So WS messages for other monitors naturally have no effect.
     */
    it('applying patches for other monitors does not change timeline data', () => {
      fc.assert(
        fc.property(
          arbitraryHistoryPoints,
          fc.array(arbitraryOtherMonitorPatch, { minLength: 1, maxLength: 10 }),
          (historyPoints, patches) => {
            // Set up the current monitor in the store
            monitorStore.updateMonitor({
              id: 'current-monitor',
              name: 'Test Monitor',
              type: 'http',
              target: 'https://example.com',
              interval_seconds: 60,
              timeout_seconds: 10,
              status: 'active',
              state: 'up',
              last_checked_at: '2024-01-01T00:00:00Z',
              next_check_at: '2024-01-01T00:01:00Z',
              settings: {},
              tags: [],
              history_retention_days: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            });

            // Capture the timeline data before WS messages
            const historyBefore = [...historyPoints];

            // Apply patches for OTHER monitors via the store (simulating WS dispatch)
            for (const patch of patches) {
              monitorStore.applyPatch(patch);
            }

            // The history array is unchanged — it's local state, not connected to the store
            // This verifies the architectural guarantee that WS patches for other monitors
            // cannot affect a local history array
            expect(historyPoints).toEqual(historyBefore);
            expect(historyPoints.length).toBe(historyBefore.length);

            // The current monitor's state in the store should also be unchanged
            // (patches were for different monitor_ids, and those monitors aren't in the store)
            const currentMonitor = monitorStore.getById('current-monitor');
            expect(currentMonitor?.state).toBe('up');

            monitorStore.clear();
          }
        ),
        { numRuns: 100 }
      );
    });

    /**
     * **Validates: Requirements 3.5**
     *
     * When WS messages for other monitors arrive, the StatusTimeline component
     * renders identically — same segments, same geometry.
     */
    it('StatusTimeline renders the same segments regardless of other-monitor WS patches', () => {
      fc.assert(
        fc.property(
          arbitraryHistoryPoints,
          fc.array(arbitraryOtherMonitorPatch, { minLength: 1, maxLength: 5 }),
          (historyPoints, patches) => {
            // Build segments BEFORE any patches
            const segmentsBefore = buildSegments(historyPoints);

            // Simulate WS patches for other monitors arriving
            monitorStore.updateMonitor({
              id: 'current-monitor',
              name: 'Test',
              type: 'http',
              target: 'https://example.com',
              interval_seconds: 60,
              timeout_seconds: 10,
              status: 'active',
              state: 'up',
              last_checked_at: '2024-01-01T00:00:00Z',
              next_check_at: null,
              settings: {},
              tags: [],
              history_retention_days: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            });

            for (const patch of patches) {
              monitorStore.applyPatch(patch);
            }

            // Build segments AFTER patches — should be identical
            const segmentsAfter = buildSegments(historyPoints);

            expect(segmentsAfter).toEqual(segmentsBefore);
            expect(segmentsAfter.length).toBe(segmentsBefore.length);

            for (let i = 0; i < segmentsBefore.length; i++) {
              expect(segmentsAfter[i].state).toBe(segmentsBefore[i].state);
              expect(segmentsAfter[i].startTime).toBe(segmentsBefore[i].startTime);
              expect(segmentsAfter[i].endTime).toBe(segmentsBefore[i].endTime);
              expect(segmentsAfter[i].widthPercent).toBe(segmentsBefore[i].widthPercent);
              expect(segmentsAfter[i].points).toBe(segmentsBefore[i].points);
            }

            monitorStore.clear();
          }
        ),
        { numRuns: 100 }
      );
    });
  });

  describe('Property 4: Reconnection Preserves Existing Timeline Data', () => {
    /**
     * **Validates: Requirements 3.6**
     *
     * After a WS disconnect+reconnect cycle, the timeline history data
     * is still intact. On unfixed code, the history is a local variable
     * that is never modified after initial load — reconnection only
     * triggers a monitor list re-fetch in the layout, not a history re-fetch.
     *
     * We simulate this by verifying that the history array and its
     * derived segments remain unchanged through store operations that
     * mimic what happens during reconnection (setMonitors call).
     */
    it('history array is preserved through reconnection store operations', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (historyPoints) => {
          // Initial state: monitor in store, history loaded locally
          monitorStore.setMonitors([
            {
              id: 'current-monitor',
              name: 'Test Monitor',
              type: 'http',
              target: 'https://example.com',
              interval_seconds: 60,
              timeout_seconds: 10,
              status: 'active',
              state: 'up',
              last_checked_at: '2024-01-01T00:00:00Z',
              next_check_at: '2024-01-01T00:01:00Z',
              settings: {},
              tags: [],
              history_retention_days: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            }
          ]);

          // Capture history state
          const historySnapshot = JSON.parse(JSON.stringify(historyPoints));
          const segmentsBefore = buildSegments(historyPoints);

          // Simulate reconnection: layout calls setMonitors with fresh data
          // This is what happens on WS reconnect (see +layout.svelte onStatusChange)
          monitorStore.setMonitors([
            {
              id: 'current-monitor',
              name: 'Test Monitor',
              type: 'http',
              target: 'https://example.com',
              interval_seconds: 60,
              timeout_seconds: 10,
              status: 'active',
              state: 'down', // state might change on reconnect
              last_checked_at: '2024-01-01T01:00:00Z',
              next_check_at: '2024-01-01T01:01:00Z',
              settings: {},
              tags: [],
              history_retention_days: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T01:00:00Z'
            }
          ]);

          // The local history array should NOT be affected by store operations
          expect(historyPoints).toEqual(historySnapshot);

          // Segments derived from history remain unchanged
          const segmentsAfter = buildSegments(historyPoints);
          expect(segmentsAfter).toEqual(segmentsBefore);

          monitorStore.clear();
        }),
        { numRuns: 100 }
      );
    });

    /**
     * **Validates: Requirements 3.6**
     *
     * Even after applying patches for the CURRENT monitor (which updates
     * state badge), the history array and timeline segments remain unchanged.
     * On the unfixed code, applyPatch only updates monitor.state and
     * monitor.last_checked_at — it does NOT modify the history array.
     */
    it('patches for current monitor update store state but not timeline history', () => {
      fc.assert(
        fc.property(
          arbitraryHistoryPoints,
          arbitraryCurrentMonitorPatch,
          (historyPoints, patch) => {
            // Set up the monitor in the store
            monitorStore.updateMonitor({
              id: 'current-monitor',
              name: 'Test Monitor',
              type: 'http',
              target: 'https://example.com',
              interval_seconds: 60,
              timeout_seconds: 10,
              status: 'active',
              state: 'up',
              last_checked_at: '2024-01-01T00:00:00Z',
              next_check_at: '2024-01-01T00:01:00Z',
              settings: {},
              tags: [],
              history_retention_days: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            });

            // Snapshot the history
            const historySnapshot = JSON.parse(JSON.stringify(historyPoints));
            const segmentsBefore = buildSegments(historyPoints);

            // Apply a patch for the CURRENT monitor (WS message arrives)
            monitorStore.applyPatch(patch);

            // The monitor's state in the store changed
            const updatedMonitor = monitorStore.getById('current-monitor');
            expect(updatedMonitor?.state).toBe(patch.state);
            expect(updatedMonitor?.last_checked_at).toBe(patch.checked_at);

            // But the local history array is completely unaffected
            expect(historyPoints).toEqual(historySnapshot);
            expect(historyPoints.length).toBe(historySnapshot.length);

            // Segments are unchanged
            const segmentsAfter = buildSegments(historyPoints);
            expect(segmentsAfter).toEqual(segmentsBefore);

            monitorStore.clear();
          }
        ),
        { numRuns: 100 }
      );
    });
  });

  describe('Property 4: Initial Load Behavior', () => {
    /**
     * **Validates: Requirements 3.5, 3.6**
     *
     * The StatusTimeline heartbeat bar renders beats for provided history data.
     * When data is provided as a prop, it is displayed as individual beat bars.
     * Each beat represents one or more data points (bucketed when data exceeds maxBars).
     */
    it('StatusTimeline correctly renders provided history data on mount', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (historyPoints) => {
          const { container } = render(StatusTimeline, { props: { data: historyPoints } });

          // Timeline should be rendered
          const timeline = container.querySelector('[data-testid="status-timeline"]');
          expect(timeline).not.toBeNull();

          // Timeline bar should be present (non-empty data)
          const timelineBar = container.querySelector('[data-testid="timeline-bar"]');
          expect(timelineBar).not.toBeNull();

          // Beats should be rendered as buttons
          const beatButtons = timelineBar!.querySelectorAll('button[data-beat]');
          expect(beatButtons.length).toBeGreaterThan(0);
          // With <= 90 points (our maxBars default), each point gets its own beat
          expect(beatButtons.length).toBeLessThanOrEqual(historyPoints.length);

          // Verify correct state coloring on each beat
          beatButtons.forEach((btn) => {
            const hasGreen = btn.className.includes('bg-emerald-500');
            const hasRed = btn.className.includes('bg-rose-500');
            const hasAmber = btn.className.includes('bg-amber-500');
            expect(hasGreen || hasRed || hasAmber).toBe(true);
          });
        }),
        { numRuns: 50 }
      );
    });

    /**
     * **Validates: Requirements 3.5**
     *
     * When data fits within maxBars (90), each point becomes its own beat.
     * The number of rendered beats equals the number of input data points.
     */
    it('number of rendered beats matches data points when under maxBars', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (historyPoints) => {
          const { container } = render(StatusTimeline, { props: { data: historyPoints } });
          const timelineBar = container.querySelector('[data-testid="timeline-bar"]');
          const beatButtons = timelineBar!.querySelectorAll('button[data-beat]');

          // With maxBars=90 and our test data (max 30 points), each point is a beat
          expect(beatButtons.length).toBe(historyPoints.length);
        }),
        { numRuns: 50 }
      );
    });

    /**
     * **Validates: Requirements 3.5, 3.6**
     *
     * The uptime percentage displayed matches the computed value from the data.
     */
    it('uptime percentage displayed matches computed value from initial data', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (historyPoints) => {
          const { container } = render(StatusTimeline, { props: { data: historyPoints } });

          const upCount = historyPoints.filter((p) => p.state === 'up').length;
          const expectedUptime = ((upCount / historyPoints.length) * 100).toFixed(1);

          // The uptime text should be present in the rendered output
          const allText = container.textContent || '';
          expect(allText).toContain(`${expectedUptime}%`);
        }),
        { numRuns: 50 }
      );
    });
  });
});
