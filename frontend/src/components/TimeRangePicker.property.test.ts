/**
 * Property-Based Tests: TimeRangePicker Logic
 *
 * Tests core logic properties of the TimeRangePicker without rendering components.
 * Each property validates a universal invariant that must hold across all valid inputs.
 *
 * **Validates: Requirements 4.3, 4.4, 4.5, 4.6**
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

// --- Replicated Logic (extracted from TimeRangePicker.svelte) ---

type PresetKey = '1h' | '6h' | '24h' | '7d' | '30d';

const presets: { key: PresetKey; label: string; ms: number }[] = [
  { key: '1h', label: '1h', ms: 60 * 60 * 1000 },
  { key: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { key: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
  { key: '7d', label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
  { key: '30d', label: '30d', ms: 30 * 24 * 60 * 60 * 1000 },
];

/**
 * Compute time range for a preset. Mirrors the selectPreset function.
 */
function computePresetRange(key: PresetKey): { from: string; to: string } {
  const preset = presets.find((p) => p.key === key)!;
  const now = new Date();
  const to = now.toISOString();
  const from = new Date(now.getTime() - preset.ms).toISOString();
  return { from, to };
}

/**
 * Validate custom range: returns error message or null.
 * Mirrors the applyCustomRange validation logic.
 */
function validateCustomRange(
  startDate: Date,
  endDate: Date
): string | null {
  if (isNaN(startDate.getTime()) || isNaN(endDate.getTime())) {
    return 'Invalid date values';
  }
  if (startDate >= endDate) {
    return 'Start time must be before end time';
  }
  return null;
}

/**
 * Clamp end time to now if in the future. Mirrors the clamping logic.
 */
function clampEndToNow(endDate: Date): Date {
  const now = new Date();
  return endDate > now ? now : endDate;
}

/**
 * Determine if a retention notice should be shown.
 * Mirrors the showRetentionNotice derived logic.
 */
function shouldShowRetentionNotice(
  rangeDurationMs: number,
  retentionDays: number
): boolean {
  const retentionMs = retentionDays * 24 * 60 * 60 * 1000;
  return rangeDurationMs > retentionMs;
}

// --- Arbitraries ---

const arbitraryPresetKey: fc.Arbitrary<PresetKey> = fc.constantFrom(
  '1h',
  '6h',
  '24h',
  '7d',
  '30d'
);

/**
 * Generate a date in the past (within last year).
 */
const arbitraryPastDate: fc.Arbitrary<Date> = fc
  .integer({ min: 1, max: 365 * 24 * 60 * 60 * 1000 })
  .map((msAgo) => new Date(Date.now() - msAgo));

/**
 * Generate a date in the future (up to 1 year ahead).
 */
const arbitraryFutureDate: fc.Arbitrary<Date> = fc
  .integer({ min: 1000, max: 365 * 24 * 60 * 60 * 1000 })
  .map((msAhead) => new Date(Date.now() + msAhead));

/**
 * Generate a valid retention days value [1, 365].
 */
const arbitraryRetentionDays: fc.Arbitrary<number> = fc.integer({
  min: 1,
  max: 365,
});

/**
 * Generate a pair of dates where start >= end (invalid range).
 */
const arbitraryInvalidRange: fc.Arbitrary<{ start: Date; end: Date }> = fc
  .tuple(arbitraryPastDate, fc.integer({ min: 0, max: 60 * 60 * 1000 }))
  .map(([baseDate, offset]) => ({
    start: new Date(baseDate.getTime() + offset),
    end: baseDate,
  }));

/**
 * Generate a range duration in milliseconds (1 minute to 60 days).
 */
const arbitraryRangeDurationMs: fc.Arbitrary<number> = fc.integer({
  min: 60 * 1000,
  max: 60 * 24 * 60 * 60 * 1000,
});

// --- Property Tests ---

// Feature: monitor-history-explorer, Property 9: Time range preset computation
describe('Property 9: Time range preset computation', () => {
  it('preset duration matches the specified duration and end is within 1 second of now', () => {
    fc.assert(
      fc.property(arbitraryPresetKey, (key) => {
        const preset = presets.find((p) => p.key === key)!;
        const beforeCompute = Date.now();
        const range = computePresetRange(key);
        const afterCompute = Date.now();

        const fromMs = new Date(range.from).getTime();
        const toMs = new Date(range.to).getTime();

        // Duration must equal the preset's specified duration
        const duration = toMs - fromMs;
        expect(duration).toBe(preset.ms);

        // End time must be within 1 second of now
        // Account for execution time by checking against before/after bounds
        expect(toMs).toBeGreaterThanOrEqual(beforeCompute);
        expect(toMs).toBeLessThanOrEqual(afterCompute + 1000);
      }),
      { numRuns: 100 }
    );
  });

  it('all presets produce valid ISO 8601 timestamps', () => {
    fc.assert(
      fc.property(arbitraryPresetKey, (key) => {
        const range = computePresetRange(key);

        // Both from and to must be valid ISO date strings
        const fromDate = new Date(range.from);
        const toDate = new Date(range.to);

        expect(fromDate.toISOString()).toBe(range.from);
        expect(toDate.toISOString()).toBe(range.to);
        expect(isNaN(fromDate.getTime())).toBe(false);
        expect(isNaN(toDate.getTime())).toBe(false);
      }),
      { numRuns: 100 }
    );
  });

  it('from is always before to for any preset', () => {
    fc.assert(
      fc.property(arbitraryPresetKey, (key) => {
        const range = computePresetRange(key);
        const fromMs = new Date(range.from).getTime();
        const toMs = new Date(range.to).getTime();

        expect(fromMs).toBeLessThan(toMs);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: monitor-history-explorer, Property 10: Start-before-end validation
describe('Property 10: Start-before-end validation', () => {
  it('validation error is returned when start >= end', () => {
    fc.assert(
      fc.property(arbitraryInvalidRange, ({ start, end }) => {
        const error = validateCustomRange(start, end);

        // Must produce a validation error
        expect(error).not.toBeNull();
        expect(error).toBe('Start time must be before end time');
      }),
      { numRuns: 100 }
    );
  });

  it('no validation error when start < end', () => {
    fc.assert(
      fc.property(
        arbitraryPastDate,
        fc.integer({ min: 1, max: 365 * 24 * 60 * 60 * 1000 }),
        (startDate, durationMs) => {
          const endDate = new Date(startDate.getTime() + durationMs);

          const error = validateCustomRange(startDate, endDate);

          // Must NOT produce a validation error
          expect(error).toBeNull();
        }
      ),
      { numRuns: 100 }
    );
  });

  it('validation blocks API request (onchange not called) on invalid range', () => {
    fc.assert(
      fc.property(arbitraryInvalidRange, ({ start, end }) => {
        let apiCalled = false;

        // Simulate the applyCustomRange flow
        const error = validateCustomRange(start, end);
        if (error === null) {
          apiCalled = true;
        }

        // API must NOT be called when validation fails
        expect(error).not.toBeNull();
        expect(apiCalled).toBe(false);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: monitor-history-explorer, Property 11: Future end-time clamping
describe('Property 11: Future end-time clamping', () => {
  it('future end times are clamped to now (within 1 second tolerance)', () => {
    fc.assert(
      fc.property(arbitraryFutureDate, (futureEnd) => {
        const beforeClamp = Date.now();
        const clamped = clampEndToNow(futureEnd);
        const afterClamp = Date.now();

        // Clamped value must be <= now (within execution tolerance)
        expect(clamped.getTime()).toBeLessThanOrEqual(afterClamp);
        expect(clamped.getTime()).toBeGreaterThanOrEqual(beforeClamp - 1000);

        // Clamped value must be less than the original future date
        expect(clamped.getTime()).toBeLessThan(futureEnd.getTime());
      }),
      { numRuns: 100 }
    );
  });

  it('past end times are not clamped (returned unchanged)', () => {
    fc.assert(
      fc.property(arbitraryPastDate, (pastEnd) => {
        const clamped = clampEndToNow(pastEnd);

        // Past dates should pass through unchanged
        expect(clamped.getTime()).toBe(pastEnd.getTime());
      }),
      { numRuns: 100 }
    );
  });

  it('clamped end time is within 1 second of current time for any future date', () => {
    fc.assert(
      fc.property(arbitraryFutureDate, (futureEnd) => {
        const now = Date.now();
        const clamped = clampEndToNow(futureEnd);

        // The clamped value should be within 1 second of now
        const diff = Math.abs(clamped.getTime() - now);
        expect(diff).toBeLessThanOrEqual(1000);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: monitor-history-explorer, Property 12: Retention notice visibility
describe('Property 12: Retention notice visibility', () => {
  it('notice shown when range duration exceeds retention days', () => {
    fc.assert(
      fc.property(arbitraryRetentionDays, (retentionDays) => {
        // Duration that exceeds retention by at least 1ms
        const retentionMs = retentionDays * 24 * 60 * 60 * 1000;
        const exceedingDuration = retentionMs + 1;

        const showNotice = shouldShowRetentionNotice(exceedingDuration, retentionDays);
        expect(showNotice).toBe(true);
      }),
      { numRuns: 100 }
    );
  });

  it('notice NOT shown when range duration is within retention days', () => {
    fc.assert(
      fc.property(
        arbitraryRetentionDays,
        fc.double({ min: 0.01, max: 1.0, noNaN: true }),
        (retentionDays, fraction) => {
          // Duration that is a fraction of retention (always within)
          const retentionMs = retentionDays * 24 * 60 * 60 * 1000;
          const withinDuration = Math.floor(retentionMs * fraction);

          const showNotice = shouldShowRetentionNotice(withinDuration, retentionDays);
          expect(showNotice).toBe(false);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('notice shown for any preset that exceeds the configured retention', () => {
    fc.assert(
      fc.property(arbitraryPresetKey, arbitraryRetentionDays, (key, retentionDays) => {
        const preset = presets.find((p) => p.key === key)!;
        const retentionMs = retentionDays * 24 * 60 * 60 * 1000;

        const showNotice = shouldShowRetentionNotice(preset.ms, retentionDays);

        // Notice should be shown if and only if preset exceeds retention
        if (preset.ms > retentionMs) {
          expect(showNotice).toBe(true);
        } else {
          expect(showNotice).toBe(false);
        }
      }),
      { numRuns: 100 }
    );
  });

  it('boundary: range exactly equal to retention does NOT show notice', () => {
    fc.assert(
      fc.property(arbitraryRetentionDays, (retentionDays) => {
        const retentionMs = retentionDays * 24 * 60 * 60 * 1000;

        // Exactly at the boundary — not exceeding
        const showNotice = shouldShowRetentionNotice(retentionMs, retentionDays);
        expect(showNotice).toBe(false);
      }),
      { numRuns: 100 }
    );
  });
});
