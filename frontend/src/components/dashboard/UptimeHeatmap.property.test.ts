/**
 * Property-Based Tests: UptimeHeatmap (Properties 14, 15)
 *
 * Property 14: Heatmap always produces exactly 24 hourly blocks
 * For any heatmap data (including empty arrays, partial data, or full 24-entry
 * arrays), the rendered heatmap SHALL produce exactly 24 blocks representing
 * consecutive hours.
 *
 * Property 15: Heatmap block color follows worst-state priority
 * For any hourly block with counts (up_count, down_count, unknown_count), the
 * color SHALL be red if down_count > 0, amber if unknown_count > 0 and
 * down_count == 0, green if up_count > 0 and down_count == 0 and
 * unknown_count == 0, and grey if all counts are 0.
 *
 * **Validates: Requirements 6.1, 6.2**
 *
 * Feature: dashboard-refactor, Property 14: Heatmap always produces exactly 24 hourly blocks
 * Feature: dashboard-refactor, Property 15: Heatmap block color follows worst-state priority
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { normalizeHeatmapData, getBlockColor } from './UptimeHeatmap.svelte';
import type { HeatmapHour } from '$lib/types';

// --- Arbitraries ---

/**
 * Generate a valid ISO date string for an hour_start field.
 * Uses a recent date range to stay realistic.
 */
const arbitraryHourStart: fc.Arbitrary<string> = fc
	.integer({ min: 0, max: 23 })
	.map((hoursAgo) => {
		const date = new Date('2024-01-15T00:00:00Z');
		date.setHours(date.getHours() + hoursAgo);
		return date.toISOString();
	});

/**
 * Generate a single HeatmapHour with random non-negative counts.
 */
const arbitraryHeatmapHour: fc.Arbitrary<HeatmapHour> = fc
	.tuple(
		arbitraryHourStart,
		fc.integer({ min: 0, max: 500 }),
		fc.integer({ min: 0, max: 500 }),
		fc.integer({ min: 0, max: 500 })
	)
	.map(([hour_start, up_count, down_count, unknown_count]) => ({
		hour_start,
		up_count,
		down_count,
		unknown_count
	}));

/**
 * Generate an array of 0–30 HeatmapHour items to test normalization
 * across empty, partial, exact, and over-full inputs.
 */
const arbitraryHeatmapData: fc.Arbitrary<HeatmapHour[]> = fc.array(arbitraryHeatmapHour, {
	minLength: 0,
	maxLength: 30
});

/**
 * Generate count combinations for color testing.
 * Includes zero and positive values to cover all 4 color states.
 */
const arbitraryCounts: fc.Arbitrary<{ up_count: number; down_count: number; unknown_count: number }> = fc
	.tuple(
		fc.integer({ min: 0, max: 1000 }),
		fc.integer({ min: 0, max: 1000 }),
		fc.integer({ min: 0, max: 1000 })
	)
	.map(([up_count, down_count, unknown_count]) => ({
		up_count,
		down_count,
		unknown_count
	}));

/**
 * Generate a HeatmapHour with specific count combination for color testing.
 */
function makeHour(up_count: number, down_count: number, unknown_count: number): HeatmapHour {
	return {
		hour_start: '2024-01-15T12:00:00Z',
		up_count,
		down_count,
		unknown_count
	};
}

// --- Property Tests ---

describe('Property 14: Heatmap always produces exactly 24 hourly blocks', () => {
	it('normalizeHeatmapData always returns exactly 24 items for any input length (0–30)', () => {
		fc.assert(
			fc.property(arbitraryHeatmapData, (data) => {
				const result = normalizeHeatmapData(data);
				expect(result).toHaveLength(24);
			}),
			{ numRuns: 100 }
		);
	});

	it('normalizeHeatmapData returns 24 blocks for empty input', () => {
		fc.assert(
			fc.property(fc.constant([]), (data: HeatmapHour[]) => {
				const result = normalizeHeatmapData(data);
				expect(result).toHaveLength(24);
			}),
			{ numRuns: 100 }
		);
	});

	it('normalizeHeatmapData returns 24 blocks for exactly 24 items', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryHeatmapHour, { minLength: 24, maxLength: 24 }),
				(data) => {
					const result = normalizeHeatmapData(data);
					expect(result).toHaveLength(24);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('normalizeHeatmapData returns 24 blocks for over-full arrays (>24 items)', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryHeatmapHour, { minLength: 25, maxLength: 30 }),
				(data) => {
					const result = normalizeHeatmapData(data);
					expect(result).toHaveLength(24);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('every block in normalized output has valid HeatmapHour structure', () => {
		fc.assert(
			fc.property(arbitraryHeatmapData, (data) => {
				const result = normalizeHeatmapData(data);
				for (const block of result) {
					expect(block).toHaveProperty('hour_start');
					expect(block).toHaveProperty('up_count');
					expect(block).toHaveProperty('down_count');
					expect(block).toHaveProperty('unknown_count');
					expect(typeof block.hour_start).toBe('string');
					expect(typeof block.up_count).toBe('number');
					expect(typeof block.down_count).toBe('number');
					expect(typeof block.unknown_count).toBe('number');
				}
			}),
			{ numRuns: 100 }
		);
	});
});

describe('Property 15: Heatmap block color follows worst-state priority', () => {
	it('returns red (--color-error) when down_count > 0 regardless of other counts', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 1, max: 1000 }),
				fc.integer({ min: 0, max: 1000 }),
				fc.integer({ min: 0, max: 1000 }),
				(down_count, up_count, unknown_count) => {
					const hour = makeHour(up_count, down_count, unknown_count);
					expect(getBlockColor(hour)).toBe('var(--color-error)');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('returns amber (--color-warning) when unknown_count > 0 and down_count == 0', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 1, max: 1000 }),
				fc.integer({ min: 0, max: 1000 }),
				(unknown_count, up_count) => {
					const hour = makeHour(up_count, 0, unknown_count);
					expect(getBlockColor(hour)).toBe('var(--color-warning)');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('returns green (--color-success) when up_count > 0 and down == 0 and unknown == 0', () => {
		fc.assert(
			fc.property(fc.integer({ min: 1, max: 1000 }), (up_count) => {
				const hour = makeHour(up_count, 0, 0);
				expect(getBlockColor(hour)).toBe('var(--color-success)');
			}),
			{ numRuns: 100 }
		);
	});

	it('returns grey (--color-border) when all counts are 0', () => {
		const hour = makeHour(0, 0, 0);
		expect(getBlockColor(hour)).toBe('var(--color-border)');
	});

	it('color priority is exhaustive: every count combination maps to exactly one valid color', () => {
		fc.assert(
			fc.property(arbitraryCounts, ({ up_count, down_count, unknown_count }) => {
				const hour = makeHour(up_count, down_count, unknown_count);
				const color = getBlockColor(hour);
				const validColors = [
					'var(--color-error)',
					'var(--color-warning)',
					'var(--color-success)',
					'var(--color-border)'
				];
				expect(validColors).toContain(color);

				// Verify correct priority ordering
				if (down_count > 0) {
					expect(color).toBe('var(--color-error)');
				} else if (unknown_count > 0) {
					expect(color).toBe('var(--color-warning)');
				} else if (up_count > 0) {
					expect(color).toBe('var(--color-success)');
				} else {
					expect(color).toBe('var(--color-border)');
				}
			}),
			{ numRuns: 100 }
		);
	});
});
