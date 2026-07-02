/**
 * Property-Based Tests: ResponseSparklines (Properties 9, 10)
 *
 * Property 9: Top-5 latency selection picks highest non-null values in descending order
 * For any set of monitors with varying avg_latency_ms values (some null, some non-null),
 * the sparkline selection SHALL contain at most 5 monitors, all with non-null latency,
 * ordered by latency descending, and these SHALL be the highest-latency monitors available.
 *
 * Property 10: Sparkline entry formatting applies truncation and integer suffix
 * For any monitor name string and avg_latency_ms float, the displayed name SHALL be
 * truncated to 40 characters with ellipsis if longer, and the latency SHALL be displayed
 * as a rounded integer followed by " ms".
 *
 * **Validates: Requirements 4.1, 4.2, 4.4, 4.5, 4.6**
 *
 * Feature: dashboard-refactor, Property 9: Top-5 latency selection picks highest non-null values in descending order
 * Feature: dashboard-refactor, Property 10: Sparkline entry formatting applies truncation and integer suffix
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { TopLatencyMonitor } from '$lib/types';
import {
	truncateMonitorName,
	formatLatency,
	selectTopMonitors
} from './ResponseSparklines.svelte';

// --- Arbitraries ---

/**
 * Generate a valid monitor_id (UUID-like string).
 */
const arbitraryMonitorId: fc.Arbitrary<string> = fc.uuid();

/**
 * Generate a monitor name of variable length (1 to 80 characters).
 * Uses printable unicode to exercise truncation logic.
 */
const arbitraryMonitorName: fc.Arbitrary<string> = fc.string({ minLength: 1, maxLength: 80 });

/**
 * Generate a non-null avg_latency_ms value (positive float, typical range).
 */
const arbitraryLatency: fc.Arbitrary<number> = fc.double({
	min: 0.01,
	max: 10000,
	noNaN: true,
	noDefaultInfinity: true
});

/**
 * Generate avg_latency_ms that can be null, undefined, NaN, or a valid number.
 * This exercises the filtering logic in selectTopMonitors.
 */
const arbitraryNullableLatency: fc.Arbitrary<number | null | undefined> = fc.oneof(
	{ weight: 5, arbitrary: arbitraryLatency },
	{ weight: 1, arbitrary: fc.constant(null) },
	{ weight: 1, arbitrary: fc.constant(undefined) },
	{ weight: 1, arbitrary: fc.constant(NaN) }
);

/**
 * Generate a monitor with a potentially null/undefined/NaN latency value.
 * The type assertion is needed because TypeScript interface says number,
 * but real-world data can have null/undefined from API responses.
 */
const arbitraryMonitor: fc.Arbitrary<TopLatencyMonitor> = fc
	.tuple(arbitraryMonitorId, arbitraryMonitorName, arbitraryNullableLatency)
	.map(([id, name, latency]) => ({
		monitor_id: id,
		monitor_name: name,
		avg_latency_ms: latency as unknown as number
	}));

/**
 * Generate a monitor with a guaranteed non-null latency.
 */
const arbitraryValidMonitor: fc.Arbitrary<TopLatencyMonitor> = fc
	.tuple(arbitraryMonitorId, arbitraryMonitorName, arbitraryLatency)
	.map(([id, name, latency]) => ({
		monitor_id: id,
		monitor_name: name,
		avg_latency_ms: latency
	}));

/**
 * Generate an array of monitors (0 to 20) with mixed latency values.
 */
const arbitraryMonitorArray: fc.Arbitrary<TopLatencyMonitor[]> = fc.array(arbitraryMonitor, {
	minLength: 0,
	maxLength: 20
});

/**
 * Generate an array with more than 5 valid monitors to exercise the limit.
 */
const arbitraryLargeValidArray: fc.Arbitrary<TopLatencyMonitor[]> = fc.array(arbitraryValidMonitor, {
	minLength: 6,
	maxLength: 30
});

// --- Property 9 Tests ---

describe('Property 9: Top-5 latency selection picks highest non-null values in descending order', () => {
	it('result contains at most 5 monitors', () => {
		fc.assert(
			fc.property(arbitraryMonitorArray, (monitors) => {
				const selected = selectTopMonitors(monitors);
				expect(selected.length).toBeLessThanOrEqual(5);
			}),
			{ numRuns: 100 }
		);
	});

	it('all selected monitors have non-null, non-NaN latency', () => {
		fc.assert(
			fc.property(arbitraryMonitorArray, (monitors) => {
				const selected = selectTopMonitors(monitors);
				for (const m of selected) {
					expect(m.avg_latency_ms).not.toBeNull();
					expect(m.avg_latency_ms).not.toBeUndefined();
					expect(Number.isNaN(m.avg_latency_ms)).toBe(false);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('selected monitors are ordered by latency descending', () => {
		fc.assert(
			fc.property(arbitraryMonitorArray, (monitors) => {
				const selected = selectTopMonitors(monitors);
				for (let i = 1; i < selected.length; i++) {
					expect(selected[i - 1].avg_latency_ms).toBeGreaterThanOrEqual(
						selected[i].avg_latency_ms
					);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('selected monitors are the highest-latency ones available', () => {
		fc.assert(
			fc.property(arbitraryMonitorArray, (monitors) => {
				const selected = selectTopMonitors(monitors);

				// Get all valid (non-null, non-NaN) latency values
				const validMonitors = monitors.filter(
					(m) =>
						m.avg_latency_ms != null && !Number.isNaN(m.avg_latency_ms)
				);
				const validLatencies = validMonitors
					.map((m) => m.avg_latency_ms)
					.sort((a, b) => b - a);

				// The selected latencies should be the top N from the valid set
				const expectedTopLatencies = validLatencies.slice(0, 5);

				expect(selected.map((m) => m.avg_latency_ms)).toEqual(expectedTopLatencies);
			}),
			{ numRuns: 100 }
		);
	});

	it('with more than 5 valid monitors, exactly 5 are returned', () => {
		fc.assert(
			fc.property(arbitraryLargeValidArray, (monitors) => {
				const selected = selectTopMonitors(monitors);
				expect(selected.length).toBe(5);
			}),
			{ numRuns: 100 }
		);
	});

	it('with fewer than 5 valid monitors, all valid monitors are returned', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryValidMonitor, { minLength: 1, maxLength: 4 }),
				(monitors) => {
					const selected = selectTopMonitors(monitors);
					expect(selected.length).toBe(monitors.length);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('empty input returns empty selection', () => {
		const selected = selectTopMonitors([]);
		expect(selected).toEqual([]);
	});

	it('all-null latency monitors produce empty selection', () => {
		fc.assert(
			fc.property(
				fc.array(
					fc.tuple(arbitraryMonitorId, arbitraryMonitorName).map(([id, name]) => ({
						monitor_id: id,
						monitor_name: name,
						avg_latency_ms: null as unknown as number
					})),
					{ minLength: 1, maxLength: 10 }
				),
				(monitors) => {
					const selected = selectTopMonitors(monitors);
					expect(selected.length).toBe(0);
				}
			),
			{ numRuns: 100 }
		);
	});
});

// --- Property 10 Tests ---

describe('Property 10: Sparkline entry formatting applies truncation and integer suffix', () => {
	it('names <= 40 chars are returned unchanged', () => {
		fc.assert(
			fc.property(fc.string({ minLength: 0, maxLength: 40 }), (name) => {
				const result = truncateMonitorName(name);
				expect(result).toBe(name);
			}),
			{ numRuns: 100 }
		);
	});

	it('names > 40 chars are truncated to 40 chars + ellipsis', () => {
		fc.assert(
			fc.property(fc.string({ minLength: 41, maxLength: 200 }), (name) => {
				const result = truncateMonitorName(name);
				expect(result.length).toBe(41); // 40 chars + 1 char "…"
				expect(result.endsWith('…')).toBe(true);
				expect(result.slice(0, 40)).toBe(name.slice(0, 40));
			}),
			{ numRuns: 100 }
		);
	});

	it('truncation preserves original content prefix exactly', () => {
		fc.assert(
			fc.property(fc.string({ minLength: 41, maxLength: 200 }), (name) => {
				const result = truncateMonitorName(name);
				// The first 40 characters of the result match the first 40 of the input
				expect(result.slice(0, 40)).toBe(name.slice(0, 40));
			}),
			{ numRuns: 100 }
		);
	});

	it('latency is formatted as rounded integer followed by " ms"', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 0, max: 99999, noNaN: true, noDefaultInfinity: true }),
				(latencyMs) => {
					const result = formatLatency(latencyMs);
					const expectedInt = Math.round(latencyMs);
					expect(result).toBe(`${expectedInt} ms`);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('formatted latency always ends with " ms" suffix', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 0, max: 99999, noNaN: true, noDefaultInfinity: true }),
				(latencyMs) => {
					const result = formatLatency(latencyMs);
					expect(result.endsWith(' ms')).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('formatted latency value is always a whole integer (no decimals)', () => {
		fc.assert(
			fc.property(
				fc.double({ min: 0, max: 99999, noNaN: true, noDefaultInfinity: true }),
				(latencyMs) => {
					const result = formatLatency(latencyMs);
					const numericPart = result.replace(' ms', '');
					// Should parse as integer without decimal point
					expect(numericPart).toBe(String(parseInt(numericPart, 10)));
				}
			),
			{ numRuns: 100 }
		);
	});
});
