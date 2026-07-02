/**
 * Property-Based Tests: StatusRing (Properties 4, 5)
 *
 * Property 4: Status ring arc angles are proportional and sum to 360°
 * For any distribution of monitors across states where total > 0,
 * each state's arc angle SHALL equal (state_count / total_count) × 360,
 * and all angles SHALL sum to exactly 360°.
 *
 * Property 5: Status ring ARIA label describes all states and counts
 * For any non-zero distribution, the ARIA label SHALL contain each
 * state name with non-zero count paired with its numeric value.
 *
 * **Validates: Requirements 2.1, 2.4, 2.6, 2.7**
 *
 * Feature: dashboard-refactor, Property 4: Status ring arc angles are proportional and sum to 360°
 * Feature: dashboard-refactor, Property 5: Status ring ARIA label describes all states and counts
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { computeArcAngles, buildAriaDescription } from './StatusRing.svelte';
import type { StatusDistribution } from '$lib/types';

// --- Arbitraries ---

/**
 * Generate a StatusDistribution with total > 0.
 * At least one of up, down, unknown must be positive.
 * total = up + down + unknown.
 */
const arbitraryNonZeroDistribution: fc.Arbitrary<StatusDistribution> = fc
	.tuple(
		fc.integer({ min: 0, max: 1000 }),
		fc.integer({ min: 0, max: 1000 }),
		fc.integer({ min: 0, max: 1000 })
	)
	.filter(([up, down, unknown]) => up + down + unknown > 0)
	.map(([up, down, unknown]) => ({
		up,
		down,
		unknown,
		total: up + down + unknown
	}));

/**
 * Generate a StatusDistribution where at least one state has a non-zero count.
 * This is the same constraint but explicitly named for Property 5.
 */
const arbitraryDistributionWithNonZeroState: fc.Arbitrary<StatusDistribution> = arbitraryNonZeroDistribution;

// --- Property 4 Tests ---

describe('Property 4: Status ring arc angles are proportional and sum to 360°', () => {
	it('each arc angle equals (state_count / total_count) × 360', () => {
		fc.assert(
			fc.property(arbitraryNonZeroDistribution, (distribution) => {
				const segments = computeArcAngles(distribution);

				for (const segment of segments) {
					const expectedAngle = (segment.count / distribution.total) * 360;
					expect(segment.angle).toBeCloseTo(expectedAngle, 10);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('all arc angles sum to exactly 360°', () => {
		fc.assert(
			fc.property(arbitraryNonZeroDistribution, (distribution) => {
				const segments = computeArcAngles(distribution);
				const sum = segments.reduce((acc, seg) => acc + seg.angle, 0);

				expect(sum).toBeCloseTo(360, 10);
			}),
			{ numRuns: 100 }
		);
	});

	it('single-state distribution produces one segment with 360° angle', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 1, max: 1000 }),
				fc.constantFrom('up' as const, 'down' as const, 'unknown' as const),
				(count, state) => {
					const distribution: StatusDistribution = {
						up: state === 'up' ? count : 0,
						down: state === 'down' ? count : 0,
						unknown: state === 'unknown' ? count : 0,
						total: count
					};

					const segments = computeArcAngles(distribution);
					const nonZeroSegments = segments.filter((s) => s.count > 0);

					// Only one segment should have a non-zero angle
					expect(nonZeroSegments).toHaveLength(1);
					expect(nonZeroSegments[0].state).toBe(state);
					expect(nonZeroSegments[0].angle).toBeCloseTo(360, 10);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('segment angles are non-negative', () => {
		fc.assert(
			fc.property(arbitraryNonZeroDistribution, (distribution) => {
				const segments = computeArcAngles(distribution);

				for (const segment of segments) {
					expect(segment.angle).toBeGreaterThanOrEqual(0);
				}
			}),
			{ numRuns: 100 }
		);
	});
});

// --- Property 5 Tests ---

describe('Property 5: Status ring ARIA label describes all states and counts', () => {
	it('ARIA label contains every non-zero state name with its count', () => {
		fc.assert(
			fc.property(arbitraryDistributionWithNonZeroState, (distribution) => {
				const label = buildAriaDescription(distribution);

				if (distribution.up > 0) {
					expect(label).toContain(`${distribution.up} up`);
				}
				if (distribution.down > 0) {
					expect(label).toContain(`${distribution.down} down`);
				}
				if (distribution.unknown > 0) {
					expect(label).toContain(`${distribution.unknown} unknown`);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('ARIA label does NOT contain state names with zero counts', () => {
		fc.assert(
			fc.property(arbitraryDistributionWithNonZeroState, (distribution) => {
				const label = buildAriaDescription(distribution);

				if (distribution.up === 0) {
					expect(label).not.toMatch(/\d+ up/);
				}
				if (distribution.down === 0) {
					expect(label).not.toMatch(/\d+ down/);
				}
				if (distribution.unknown === 0) {
					expect(label).not.toMatch(/\d+ unknown/);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('ARIA label is non-empty for any non-zero distribution', () => {
		fc.assert(
			fc.property(arbitraryDistributionWithNonZeroState, (distribution) => {
				const label = buildAriaDescription(distribution);
				expect(label.length).toBeGreaterThan(0);
			}),
			{ numRuns: 100 }
		);
	});
});
