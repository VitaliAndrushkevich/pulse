/**
 * Bug Condition Exploration Test: Down Segments Missing Failure Details
 *
 * **Validates: Requirements 1.1, 1.2, 1.3, 2.1, 2.2**
 *
 * This test encodes the EXPECTED (correct) behavior: when "down" segments have
 * status_code or error data, that information should be visible in the rendered
 * tooltip/popover content. On unfixed code, this test will FAIL — confirming
 * the bug exists because the current implementation discards failure details.
 */
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import fc from 'fast-check';
import StatusTimeline from '../StatusTimeline.svelte';
import type { HistoryPoint } from '$lib/types';

/**
 * Arbitrary generator for "down" HistoryPoint arrays where at least one point
 * has a non-null status_code or error field.
 */
const downHistoryPointWithError = fc.record({
  state: fc.constant('down' as const),
  latency_ms: fc.option(fc.integer({ min: 1, max: 30000 }), { nil: null }),
  status_code: fc.option(fc.constantFrom(400, 401, 403, 404, 500, 502, 503, 504), { nil: null }),
  error: fc.option(
    fc.constantFrom(
      'connection refused',
      'timeout exceeded',
      'DNS resolution failed',
      'TLS handshake error',
      'connection reset by peer'
    ),
    { nil: null }
  ),
  ssl_days_remaining: fc.option(fc.integer({ min: 0, max: 365 }), { nil: null }),
  checked_at: fc.date({
    min: new Date('2024-01-01T00:00:00Z'),
    max: new Date('2024-01-01T23:59:59Z')
  }).map((d) => d.toISOString())
});

/**
 * Ensures at least one point has a non-null status_code or error.
 * We generate 2-10 "down" points, then filter to guarantee the constraint.
 */
const downHistoryPointsWithErrors: fc.Arbitrary<HistoryPoint[]> = fc
  .array(downHistoryPointWithError, { minLength: 2, maxLength: 10 })
  .filter((points) => points.some((p) => p.status_code !== null || p.error !== null));

describe('StatusTimeline - Bug Condition: Down Segments Missing Failure Details', () => {
  it('Property 1: down segments with failure data MUST display status_code or error in rendered content', () => {
    fc.assert(
      fc.property(downHistoryPointsWithErrors, (points) => {
        const { container } = render(StatusTimeline, { props: { data: points } });

        // Gather all title attributes from segment divs (tooltip content)
        const timelineBar = container.querySelector('[data-testid="timeline-bar"]');
        expect(timelineBar).not.toBeNull();

        const segmentDivs = timelineBar!.querySelectorAll('[title]');
        expect(segmentDivs.length).toBeGreaterThan(0);

        // Collect all tooltip text and any popover/visible text content
        const allRenderedText = Array.from(segmentDivs)
          .map((el) => el.getAttribute('title') || '')
          .join(' ');

        // Also check any visible text in the timeline bar area (for popovers/custom tooltips)
        const allVisibleText = timelineBar!.textContent || '';
        const combinedText = `${allRenderedText} ${allVisibleText}`;

        // The property: at least one failure detail (status_code number or error message)
        // from the input points MUST appear in the rendered tooltip/popover content
        const hasFailureDetailDisplayed = points.some((p) => {
          if (p.status_code !== null) {
            return combinedText.includes(String(p.status_code));
          }
          if (p.error !== null) {
            return combinedText.includes(p.error);
          }
          return false;
        });

        expect(hasFailureDetailDisplayed).toBe(true);
      }),
      { numRuns: 50 }
    );
  });
});
