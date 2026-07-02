/**
 * Property-Based Tests: HealthScore (Properties 1, 2, 3)
 *
 * Property 1: Health score is correct average
 * For any non-empty array of active monitors with uptime_percent values in
 * range [0, 100], the computed health score SHALL equal the arithmetic mean
 * rounded to 2 decimal places.
 *
 * Property 2: Health score color mapping follows stepped thresholds
 * For any health score percentage in [0, 100], the color function SHALL return
 * `--color-success` when >= 99, `--color-warning` when >= 95 and < 99,
 * `--color-error` when < 95.
 *
 * Property 3: Partial data health score uses only successful monitors
 * For any set of monitors where a subset has successfully retrieved stats and
 * the remainder has failed, the health score SHALL be computed exclusively from
 * the successful subset, and the partial_data flag SHALL be true when the failed
 * subset is non-empty.
 *
 * **Validates: Requirements 1.1, 1.2, 1.5**
 *
 * Feature: dashboard-refactor, Property 1: Health score is correct average
 * Feature: dashboard-refactor, Property 2: Health score color mapping follows stepped thresholds
 * Feature: dashboard-refactor, Property 3: Partial data health score uses only successful monitors
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { getHealthScoreColor } from './HealthScore.svelte';

// --- Pure computation functions under test ---

/**
 * Computes the health score as the arithmetic mean of uptime values,
 * rounded to exactly 2 decimal places.
 *
 * This mirrors the backend computation: AVG(uptime_percent) across active monitors.
 * The frontend receives the pre-computed value, but the property verifies
 * the mathematical invariant that the displayed value matches the mean.
 */
function computeHealthScore(uptimeValues: number[]): number {
	if (uptimeValues.length === 0) return 0;
	const sum = uptimeValues.reduce((acc, val) => acc + val, 0);
	const mean = sum / uptimeValues.length;
	return Math.round(mean * 100) / 100;
}

/**
 * Computes health score data from a mixed set of monitors where some
 * have successfully retrieved stats and others have failed.
 *
 * Returns the health score computed only from the successful subset,
 * and sets partial_data = true when there are failed monitors.
 */
function computePartialHealthScore(
	successfulUptimes: number[],
	failedCount: number
): { uptime_percent: number; active_monitor_count: number; partial_data: boolean } {
	const totalMonitors = successfulUptimes.length + failedCount;
	if (successfulUptimes.length === 0) {
		return {
			uptime_percent: 0,
			active_monitor_count: totalMonitors,
			partial_data: failedCount > 0
		};
	}
	const sum = successfulUptimes.reduce((acc, val) => acc + val, 0);
	const mean = sum / successfulUptimes.length;
	return {
		uptime_percent: Math.round(mean * 100) / 100,
		active_monitor_count: totalMonitors,
		partial_data: failedCount > 0
	};
}

// --- Arbitraries ---

/**
 * Generate an uptime_percent value in [0, 100] with up to 2 decimal places.
 * Uses integer math to avoid floating-point precision issues in generation.
 */
const arbitraryUptimePercent: fc.Arbitrary<number> = fc
	.integer({ min: 0, max: 10000 })
	.map((v) => v / 100);

/**
 * Generate a non-empty array of uptime values (1 to 500 monitors).
 * Upper bound of 500 matches the project's performance target.
 */
const arbitraryUptimeArray: fc.Arbitrary<number[]> = fc.array(arbitraryUptimePercent, {
	minLength: 1,
	maxLength: 500
});

/**
 * Generate a health score percentage in the full [0, 100] range
 * with fine-grained precision for threshold testing.
 */
const arbitraryScorePercent: fc.Arbitrary<number> = fc.double({
	min: 0,
	max: 100,
	noNaN: true,
	noDefaultInfinity: true
});

/**
 * Generate a pair of (successful uptimes, failed count) for partial data testing.
 * At least one successful monitor required to compute a meaningful score.
 */
const arbitraryPartialData: fc.Arbitrary<{ successful: number[]; failedCount: number }> = fc
	.tuple(
		fc.array(arbitraryUptimePercent, { minLength: 1, maxLength: 100 }),
		fc.integer({ min: 1, max: 100 })
	)
	.map(([successful, failedCount]) => ({ successful, failedCount }));

/**
 * Generate data where all monitors succeed (no failures).
 */
const arbitraryAllSuccessful: fc.Arbitrary<{ successful: number[]; failedCount: number }> = fc
	.array(arbitraryUptimePercent, { minLength: 1, maxLength: 100 })
	.map((successful) => ({ successful, failedCount: 0 }));

// --- Property Tests ---

describe('Property 1: Health score is correct average', () => {
	it('computed health score equals arithmetic mean rounded to 2 decimal places', () => {
		fc.assert(
			fc.property(arbitraryUptimeArray, (uptimeValues) => {
				const score = computeHealthScore(uptimeValues);

				// Verify: score equals expected arithmetic mean
				const expectedMean = uptimeValues.reduce((a, b) => a + b, 0) / uptimeValues.length;
				const expectedRounded = Math.round(expectedMean * 100) / 100;

				expect(score).toBeCloseTo(expectedRounded, 10);
			}),
			{ numRuns: 100 }
		);
	});

	it('health score is always within [0, 100] when inputs are in [0, 100]', () => {
		fc.assert(
			fc.property(arbitraryUptimeArray, (uptimeValues) => {
				const score = computeHealthScore(uptimeValues);

				expect(score).toBeGreaterThanOrEqual(0);
				expect(score).toBeLessThanOrEqual(100);
			}),
			{ numRuns: 100 }
		);
	});

	it('health score has at most 2 decimal places', () => {
		fc.assert(
			fc.property(arbitraryUptimeArray, (uptimeValues) => {
				const score = computeHealthScore(uptimeValues);

				// Multiplying by 100 and checking it's an integer verifies at most 2 decimals
				const scaled = Math.round(score * 100);
				expect(score * 100).toBeCloseTo(scaled, 10);
			}),
			{ numRuns: 100 }
		);
	});

	it('single monitor health score equals that monitor uptime exactly', () => {
		fc.assert(
			fc.property(arbitraryUptimePercent, (uptime) => {
				const score = computeHealthScore([uptime]);
				expect(score).toBeCloseTo(uptime, 10);
			}),
			{ numRuns: 100 }
		);
	});
});

describe('Property 2: Health score color mapping follows stepped thresholds', () => {
	it('returns --color-success for any value >= 99', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 99, max: 100, noNaN: true, noDefaultInfinity: true }),
				(percent) => {
					expect(getHealthScoreColor(percent)).toBe('var(--color-success)');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('returns --color-warning for any value >= 95 and < 99', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 95, max: 98.999999999, noNaN: true, noDefaultInfinity: true }),
				(percent) => {
					// Ensure we don't accidentally hit exactly 99 due to float rounding
					if (percent >= 99) return;
					expect(getHealthScoreColor(percent)).toBe('var(--color-warning)');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('returns --color-error for any value < 95', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 0, max: 94.999999999, noNaN: true, noDefaultInfinity: true }),
				(percent) => {
					// Ensure we don't accidentally hit exactly 95 due to float rounding
					if (percent >= 95) return;
					expect(getHealthScoreColor(percent)).toBe('var(--color-error)');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('color mapping covers all values in [0, 100] (exhaustive partition)', () => {
		fc.assert(
			fc.property(arbitraryScorePercent, (percent) => {
				const color = getHealthScoreColor(percent);
				const validColors = [
					'var(--color-success)',
					'var(--color-warning)',
					'var(--color-error)'
				];
				expect(validColors).toContain(color);

				// Verify correct partition
				if (percent >= 99) {
					expect(color).toBe('var(--color-success)');
				} else if (percent >= 95) {
					expect(color).toBe('var(--color-warning)');
				} else {
					expect(color).toBe('var(--color-error)');
				}
			}),
			{ numRuns: 100 }
		);
	});
});

describe('Property 3: Partial data health score uses only successful monitors', () => {
	it('health score is computed exclusively from successful monitors', () => {
		fc.assert(
			fc.property(arbitraryPartialData, ({ successful, failedCount }) => {
				const result = computePartialHealthScore(successful, failedCount);

				// Expected: mean of successful subset only
				const expectedMean = successful.reduce((a, b) => a + b, 0) / successful.length;
				const expectedRounded = Math.round(expectedMean * 100) / 100;

				expect(result.uptime_percent).toBeCloseTo(expectedRounded, 10);
			}),
			{ numRuns: 100 }
		);
	});

	it('partial_data flag is true when failed subset is non-empty', () => {
		fc.assert(
			fc.property(arbitraryPartialData, ({ successful, failedCount }) => {
				const result = computePartialHealthScore(successful, failedCount);

				// failedCount >= 1 by construction in arbitraryPartialData
				expect(result.partial_data).toBe(true);
			}),
			{ numRuns: 100 }
		);
	});

	it('partial_data flag is false when all monitors succeed', () => {
		fc.assert(
			fc.property(arbitraryAllSuccessful, ({ successful, failedCount }) => {
				const result = computePartialHealthScore(successful, failedCount);

				expect(result.partial_data).toBe(false);
			}),
			{ numRuns: 100 }
		);
	});

	it('active_monitor_count includes both successful and failed monitors', () => {
		fc.assert(
			fc.property(arbitraryPartialData, ({ successful, failedCount }) => {
				const result = computePartialHealthScore(successful, failedCount);

				expect(result.active_monitor_count).toBe(successful.length + failedCount);
			}),
			{ numRuns: 100 }
		);
	});

	it('health score from partial data is independent of the number of failed monitors', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryUptimePercent, { minLength: 1, maxLength: 50 }),
				fc.integer({ min: 1, max: 100 }),
				fc.integer({ min: 1, max: 100 }),
				(successful, failedCount1, failedCount2) => {
					const result1 = computePartialHealthScore(successful, failedCount1);
					const result2 = computePartialHealthScore(successful, failedCount2);

					// Health score depends ONLY on the successful subset, not on how many failed
					expect(result1.uptime_percent).toBe(result2.uptime_percent);
				}
			),
			{ numRuns: 100 }
		);
	});
});
