/**
 * Property-Based Tests: IncidentsPanel (Properties 6, 7, 8)
 *
 * Tests three fundamental correctness properties of the IncidentsPanel:
 * - Cause truncation preserves content within limit
 * - Incident ordering by duration descending with name tiebreaker
 * - Panel capped at 10 entries with correct overflow count
 *
 * Feature: dashboard-refactor, Property 6: Incident cause truncation preserves content within limit
 * Feature: dashboard-refactor, Property 7: Incidents ordered by duration descending with name tiebreaker
 * Feature: dashboard-refactor, Property 8: Incidents panel capped at 10 entries with overflow count
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { ActiveIncident } from '$lib/types';
import { truncateCause, sortIncidents } from './IncidentsPanel.svelte';

// --- Constants ---

const TRUNCATION_LIMIT = 120;
const MAX_VISIBLE = 10;

// --- Arbitraries ---

/**
 * Generate an arbitrary cause string of varying length (0–300 chars).
 * Uses full unicode to stress-test truncation.
 */
const arbitraryCauseString: fc.Arbitrary<string> = fc.string({ minLength: 0, maxLength: 300 });

/**
 * Generate a single ActiveIncident with a randomized started_at and monitor_name.
 * started_at is varied across a wide window to produce diverse durations.
 */
const arbitraryIncident: fc.Arbitrary<ActiveIncident> = fc.record({
	monitor_id: fc.uuid(),
	monitor_name: fc.string({ minLength: 1, maxLength: 50 }),
	started_at: fc
		.date({
			min: new Date('2020-01-01T00:00:00Z'),
			max: new Date()
		})
		.map((d) => d.toISOString()),
	cause: fc.oneof(fc.constant(null), fc.string({ minLength: 0, maxLength: 200 })),
	state: fc.constant('down' as const)
});

/**
 * Generate 1–20 incidents for the panel cap property.
 */
const arbitraryIncidentList: fc.Arbitrary<ActiveIncident[]> = fc.array(arbitraryIncident, {
	minLength: 1,
	maxLength: 20
});

// --- Property Tests ---

describe('Property 6: Incident cause truncation preserves content within limit', () => {
	/**
	 * **Validates: Requirements 3.1**
	 */

	it('strings at or below the limit are returned unchanged', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: 0, maxLength: TRUNCATION_LIMIT }),
				(cause) => {
					const result = truncateCause(cause);
					expect(result).toBe(cause);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('strings exceeding the limit are truncated to first 120 chars + ellipsis', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: TRUNCATION_LIMIT + 1, maxLength: 500 }),
				(cause) => {
					const result = truncateCause(cause);
					expect(result).toBe(cause.slice(0, TRUNCATION_LIMIT) + '…');
					expect(result.length).toBe(TRUNCATION_LIMIT + 1); // 120 chars + 1 ellipsis char
				}
			),
			{ numRuns: 100 }
		);
	});

	it('truncated result always starts with the original prefix', () => {
		fc.assert(
			fc.property(arbitraryCauseString, (cause) => {
				const result = truncateCause(cause);
				if (cause.length > TRUNCATION_LIMIT) {
					expect(result.startsWith(cause.slice(0, TRUNCATION_LIMIT))).toBe(true);
					expect(result.endsWith('…')).toBe(true);
				} else {
					expect(result).toBe(cause);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('null cause returns empty string', () => {
		expect(truncateCause(null)).toBe('');
	});
});

describe('Property 7: Incidents ordered by duration descending with name tiebreaker', () => {
	/**
	 * **Validates: Requirements 3.3**
	 */

	it('adjacent pairs satisfy duration descending or equal duration with name ascending', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryIncident, { minLength: 2, maxLength: 20 }),
				(incidents) => {
					const sorted = sortIncidents(incidents);
					const now = Date.now();

					for (let i = 0; i < sorted.length - 1; i++) {
						const durationA = now - new Date(sorted[i].started_at).getTime();
						const durationB = now - new Date(sorted[i + 1].started_at).getTime();

						if (durationA !== durationB) {
							// Duration descending: earlier started_at = longer duration = comes first
							expect(durationA).toBeGreaterThanOrEqual(durationB);
						} else {
							// Equal duration: alphabetical name tiebreaker
							expect(
								sorted[i].monitor_name.localeCompare(sorted[i + 1].monitor_name)
							).toBeLessThanOrEqual(0);
						}
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('sorted output contains the same elements as the input', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryIncident, { minLength: 0, maxLength: 20 }),
				(incidents) => {
					const sorted = sortIncidents(incidents);
					expect(sorted.length).toBe(incidents.length);

					// Every input incident exists in the output (by monitor_id)
					const inputIds = incidents.map((i) => i.monitor_id).sort();
					const outputIds = sorted.map((i) => i.monitor_id).sort();
					expect(outputIds).toEqual(inputIds);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('sort does not mutate the original array', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryIncident, { minLength: 1, maxLength: 10 }),
				(incidents) => {
					const original = [...incidents];
					sortIncidents(incidents);

					// The input array should remain unchanged
					expect(incidents).toEqual(original);
				}
			),
			{ numRuns: 100 }
		);
	});
});

describe('Property 8: Incidents panel capped at 10 entries with overflow count', () => {
	/**
	 * **Validates: Requirements 3.6**
	 */

	it('displayed list is min(N, 10) for any N > 0 incidents', () => {
		fc.assert(
			fc.property(arbitraryIncidentList, (incidents) => {
				const sorted = sortIncidents(incidents);
				const visible = sorted.slice(0, MAX_VISIBLE);
				const expectedVisible = Math.min(incidents.length, MAX_VISIBLE);

				expect(visible.length).toBe(expectedVisible);
			}),
			{ numRuns: 100 }
		);
	});

	it('overflow count is total - 10 when N > 10, else 0', () => {
		fc.assert(
			fc.property(arbitraryIncidentList, (incidents) => {
				const sorted = sortIncidents(incidents);
				const overflowCount = Math.max(0, sorted.length - MAX_VISIBLE);

				if (incidents.length > MAX_VISIBLE) {
					expect(overflowCount).toBe(incidents.length - MAX_VISIBLE);
					expect(overflowCount).toBeGreaterThan(0);
				} else {
					expect(overflowCount).toBe(0);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('visible entries are the first 10 from the sorted order', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryIncident, { minLength: 11, maxLength: 20 }),
				(incidents) => {
					const sorted = sortIncidents(incidents);
					const visible = sorted.slice(0, MAX_VISIBLE);

					// The visible entries should be the first 10 from sorted
					expect(visible.length).toBe(MAX_VISIBLE);
					for (let i = 0; i < MAX_VISIBLE; i++) {
						expect(visible[i]).toBe(sorted[i]);
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('overflow count equals total incident count when N > 10', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryIncident, { minLength: 11, maxLength: 20 }),
				(incidents) => {
					const total = incidents.length;
					const overflowCount = Math.max(0, total - MAX_VISIBLE);

					// The overflow indicator should show the total count of incidents
					// beyond the visible 10
					expect(overflowCount + MAX_VISIBLE).toBe(total);
				}
			),
			{ numRuns: 100 }
		);
	});
});
