/**
 * Property-Based Preservation Tests for StatusTimeline
 *
 * These tests capture baseline behavior of the UNFIXED code to detect regressions
 * after the failure-details fix is applied. They follow observation-first methodology:
 * observe current behavior, then assert it.
 *
 * **Validates: Requirements 3.1, 3.2, 3.3, 3.4**
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import * as fc from 'fast-check';
import type { HistoryPoint } from '$lib/types';
import { buildSegments, computeUptimePercent, formatSegmentTooltip } from '../statusTimeline.utils';
import StatusTimeline from '../StatusTimeline.svelte';

// --- Arbitraries ---

/**
 * Generate a valid ISO timestamp within a reasonable range (24h window).
 */
const arbitraryTimestamp = fc.integer({ min: 1700000000000, max: 1700086400000 }).map((ms) => {
  return new Date(ms).toISOString();
});

/**
 * Generate a single HistoryPoint with arbitrary state and timestamps.
 */
const arbitraryHistoryPoint: fc.Arbitrary<HistoryPoint> = fc.record({
  state: fc.constantFrom('up' as const, 'down' as const),
  latency_ms: fc.option(fc.integer({ min: 1, max: 5000 }), { nil: null }),
  status_code: fc.option(fc.integer({ min: 100, max: 599 }), { nil: null }),
  error: fc.option(fc.string({ minLength: 1, maxLength: 50 }), { nil: null }),
  ssl_days_remaining: fc.option(fc.integer({ min: 0, max: 365 }), { nil: null }),
  checked_at: arbitraryTimestamp
});

/**
 * Generate a non-empty array of HistoryPoints (1 to 50 points).
 */
const arbitraryHistoryPoints = fc.array(arbitraryHistoryPoint, { minLength: 1, maxLength: 50 });

/**
 * Generate HistoryPoints that are all "up" state.
 */
const arbitraryUpHistoryPoints = fc.array(
  fc.record({
    state: fc.constant('up' as const),
    latency_ms: fc.option(fc.integer({ min: 1, max: 5000 }), { nil: null }),
    status_code: fc.option(fc.constantFrom(200, 201, 204), { nil: null }),
    error: fc.constant(null),
    ssl_days_remaining: fc.option(fc.integer({ min: 0, max: 365 }), { nil: null }),
    checked_at: arbitraryTimestamp
  }),
  { minLength: 1, maxLength: 50 }
);



// --- Property Tests ---

describe('StatusTimeline Preservation Properties', () => {
  describe('Property 2: Segment Geometry Preservation', () => {
    /**
     * **Validates: Requirements 3.4**
     *
     * Segment geometry is deterministic: for the same input, buildSegments always
     * produces the same startTime, endTime, widthPercent, and points values.
     */
    it('segment building is deterministic for the same input', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const result1 = buildSegments(points);
          const result2 = buildSegments(points);

          expect(result1).toEqual(result2);
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Total widthPercent of all segments sums to ~100%.
     */
    it('segment widths sum to approximately 100%', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const segments = buildSegments(points);
          if (segments.length === 0) return;

          const totalWidth = segments.reduce((sum, s) => sum + s.widthPercent, 0);
          // Allow floating-point tolerance
          expect(totalWidth).toBeCloseTo(100, 5);
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Total point count across all segments equals input length.
     */
    it('total points across segments equals input data length', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const segments = buildSegments(points);
          const totalPoints = segments.reduce((sum, s) => sum + s.points, 0);
          expect(totalPoints).toBe(points.length);
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Segments are contiguous: each segment's endTime equals the next segment's startTime.
     */
    it('segments are contiguous (no gaps between segments)', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const segments = buildSegments(points);
          for (let i = 0; i < segments.length - 1; i++) {
            expect(segments[i].endTime).toBe(segments[i + 1].startTime);
          }
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Single point input produces exactly one segment with widthPercent = 100.
     */
    it('single point produces one segment with 100% width', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoint, (point) => {
          const segments = buildSegments([point]);
          expect(segments.length).toBe(1);
          expect(segments[0].widthPercent).toBe(100);
          expect(segments[0].points).toBe(1);
          expect(segments[0].state).toBe(point.state);
        }),
        { numRuns: 100 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Adjacent segments have different states (no two consecutive segments share the same state).
     */
    it('adjacent segments have different states', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const segments = buildSegments(points);
          for (let i = 0; i < segments.length - 1; i++) {
            expect(segments[i].state).not.toBe(segments[i + 1].state);
          }
        }),
        { numRuns: 200 }
      );
    });
  });

  describe('Property 2: Up Segment Tooltip Preservation', () => {
    /**
     * **Validates: Requirements 3.1**
     *
     * For all "up" segments, tooltip text matches format "Healthy — [time range] (N checks)"
     * with no failure detail content.
     */
    it('up segment tooltips match "Healthy — ..." format without failure details', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const segments = buildSegments(points);
          const upSegments = segments.filter((s) => s.state === 'up');

          for (const seg of upSegments) {
            const tooltip = formatSegmentTooltip(seg);
            // Must start with "Healthy"
            expect(tooltip.startsWith('Healthy')).toBe(true);
            // Must contain check count
            expect(tooltip).toContain(`${seg.points} check`);
            // Must NOT contain failure-related keywords
            expect(tooltip).not.toContain('HTTP');
            expect(tooltip).not.toContain('error');
            expect(tooltip).not.toContain('Error');
            expect(tooltip).not.toContain('status_code');
          }
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.1, 3.2**
     *
     * All-up arrays produce only "Healthy" tooltips.
     */
    it('all-up history produces only Healthy tooltips', () => {
      fc.assert(
        fc.property(arbitraryUpHistoryPoints, (points) => {
          const segments = buildSegments(points);
          // All segments should be "up"
          for (const seg of segments) {
            expect(seg.state).toBe('up');
            const tooltip = formatSegmentTooltip(seg);
            expect(tooltip.startsWith('Healthy')).toBe(true);
          }
        }),
        { numRuns: 100 }
      );
    });
  });

  describe('Property 2: Empty State Preservation', () => {
    /**
     * **Validates: Requirements 3.3**
     *
     * Empty data prop renders "No check data available" placeholder.
     */
    it('empty data renders placeholder message', () => {
      render(StatusTimeline, { props: { data: [] } });
      const placeholder = screen.getByText(/No check data available/);
      expect(placeholder).toBeTruthy();
    });

    /**
     * **Validates: Requirements 3.3**
     *
     * buildSegments returns empty array for empty input.
     */
    it('buildSegments returns empty array for empty input', () => {
      const segments = buildSegments([]);
      expect(segments).toEqual([]);
    });
  });

  describe('Property 2: Uptime Percentage Preservation', () => {
    /**
     * **Validates: Requirements 3.4**
     *
     * Uptime equals upCount / total * 100 formatted to 1 decimal place.
     */
    it('uptime percentage equals upCount / total * 100 to 1 decimal', () => {
      fc.assert(
        fc.property(arbitraryHistoryPoints, (points) => {
          const result = computeUptimePercent(points);
          expect(result).not.toBeNull();

          const upCount = points.filter((p) => p.state === 'up').length;
          const expected = ((upCount / points.length) * 100).toFixed(1);
          expect(result).toBe(expected);
        }),
        { numRuns: 200 }
      );
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * Uptime is null for empty data.
     */
    it('uptime is null for empty data', () => {
      expect(computeUptimePercent([])).toBeNull();
    });

    /**
     * **Validates: Requirements 3.4**
     *
     * All-up data produces 100.0% uptime.
     */
    it('all-up data produces 100.0% uptime', () => {
      fc.assert(
        fc.property(arbitraryUpHistoryPoints, (points) => {
          const result = computeUptimePercent(points);
          expect(result).toBe('100.0');
        }),
        { numRuns: 100 }
      );
    });
  });
});
