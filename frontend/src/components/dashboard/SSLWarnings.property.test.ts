/**
 * Property-Based Tests: SSLWarnings (Properties 11, 12, 13)
 *
 * Tests three fundamental correctness properties of the SSLWarnings widget:
 * - SSL expiry filter includes only monitors with days_remaining <= 30
 * - SSL entries ordered by days remaining ascending with name tiebreaker
 * - SSL urgency tier correctly categorizes days remaining
 *
 * Feature: dashboard-refactor, Property 11: SSL expiry filter includes only monitors with days_remaining <= 30
 * Feature: dashboard-refactor, Property 12: SSL entries ordered by days remaining ascending with name tiebreaker
 * Feature: dashboard-refactor, Property 13: SSL urgency tier correctly categorizes days remaining
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { SSLExpiryEntry } from '$lib/types';
import { filterSSLEntries, sortSSLEntries, getUrgencyTier } from './SSLWarnings.svelte';

// --- Arbitraries ---

/**
 * Generate a single SSLExpiryEntry with days_remaining in a wide range
 * covering negative (expired), zero, within threshold (1–30), and beyond (31+).
 */
const arbitrarySSLEntry: fc.Arbitrary<SSLExpiryEntry> = fc.record({
	monitor_id: fc.uuid(),
	monitor_name: fc.string({ minLength: 1, maxLength: 50 }),
	days_remaining: fc.integer({ min: -30, max: 90 }),
	expires_at: fc
		.date({
			min: new Date('2024-01-01T00:00:00Z'),
			max: new Date('2026-12-31T00:00:00Z'),
			noInvalidDate: true
		})
		.map((d) => d.toISOString())
});

/**
 * Generate an SSLExpiryEntry that is within the 30-day threshold (days_remaining <= 30).
 */
const arbitrarySSLEntryWithinThreshold: fc.Arbitrary<SSLExpiryEntry> = fc.record({
	monitor_id: fc.uuid(),
	monitor_name: fc.string({ minLength: 1, maxLength: 50 }),
	days_remaining: fc.integer({ min: -30, max: 30 }),
	expires_at: fc
		.date({
			min: new Date('2024-01-01T00:00:00Z'),
			max: new Date('2026-12-31T00:00:00Z'),
			noInvalidDate: true
		})
		.map((d) => d.toISOString())
});

/**
 * Generate an SSLExpiryEntry that is beyond the 30-day threshold (days_remaining > 30).
 */
const arbitrarySSLEntryBeyondThreshold: fc.Arbitrary<SSLExpiryEntry> = fc.record({
	monitor_id: fc.uuid(),
	monitor_name: fc.string({ minLength: 1, maxLength: 50 }),
	days_remaining: fc.integer({ min: 31, max: 365 }),
	expires_at: fc
		.date({
			min: new Date('2024-01-01T00:00:00Z'),
			max: new Date('2026-12-31T00:00:00Z'),
			noInvalidDate: true
		})
		.map((d) => d.toISOString())
});

/**
 * Generate a mixed list of SSL entries (some within threshold, some beyond).
 */
const arbitraryMixedSSLList: fc.Arbitrary<SSLExpiryEntry[]> = fc.array(arbitrarySSLEntry, {
	minLength: 1,
	maxLength: 20
});

/**
 * Generate a days_remaining value covering all urgency tiers and boundary cases.
 */
const arbitraryDaysRemaining: fc.Arbitrary<number> = fc.integer({ min: -365, max: 365 });

// --- Property Tests ---

describe('Property 11: SSL expiry filter includes only monitors with days_remaining <= 30', () => {
	/**
	 * **Validates: Requirements 5.1, 5.3, 5.5**
	 */

	it('filtered result contains only entries with days_remaining <= 30', () => {
		fc.assert(
			fc.property(arbitraryMixedSSLList, (entries) => {
				const filtered = filterSSLEntries(entries);

				for (const entry of filtered) {
					expect(entry.days_remaining).toBeLessThanOrEqual(30);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('all entries with days_remaining <= 30 are included in the filtered result', () => {
		fc.assert(
			fc.property(arbitraryMixedSSLList, (entries) => {
				const filtered = filterSSLEntries(entries);
				const expectedIds = entries
					.filter((e) => e.days_remaining <= 30)
					.map((e) => e.monitor_id)
					.sort();
				const actualIds = filtered.map((e) => e.monitor_id).sort();

				expect(actualIds).toEqual(expectedIds);
			}),
			{ numRuns: 100 }
		);
	});

	it('entries with days_remaining > 30 are never included', () => {
		fc.assert(
			fc.property(
				fc.array(arbitrarySSLEntryBeyondThreshold, { minLength: 1, maxLength: 10 }),
				(entries) => {
					const filtered = filterSSLEntries(entries);
					expect(filtered.length).toBe(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('entries with days_remaining <= 30 are always included', () => {
		fc.assert(
			fc.property(
				fc.array(arbitrarySSLEntryWithinThreshold, { minLength: 1, maxLength: 10 }),
				(entries) => {
					const filtered = filterSSLEntries(entries);
					expect(filtered.length).toBe(entries.length);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('filter preserves original entry objects (no mutation)', () => {
		fc.assert(
			fc.property(arbitraryMixedSSLList, (entries) => {
				const original = entries.map((e) => ({ ...e }));
				filterSSLEntries(entries);

				expect(entries).toEqual(original);
			}),
			{ numRuns: 100 }
		);
	});
});

describe('Property 12: SSL entries ordered by days remaining ascending with name tiebreaker', () => {
	/**
	 * **Validates: Requirements 5.1, 5.3, 5.5**
	 */

	it('adjacent pairs satisfy days_remaining ascending or equal with name ascending', () => {
		fc.assert(
			fc.property(
				fc.array(arbitrarySSLEntryWithinThreshold, { minLength: 2, maxLength: 20 }),
				(entries) => {
					const sorted = sortSSLEntries(entries);

					for (let i = 0; i < sorted.length - 1; i++) {
						const a = sorted[i];
						const b = sorted[i + 1];

						if (a.days_remaining !== b.days_remaining) {
							expect(a.days_remaining).toBeLessThan(b.days_remaining);
						} else {
							expect(a.monitor_name.localeCompare(b.monitor_name)).toBeLessThanOrEqual(0);
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
				fc.array(arbitrarySSLEntryWithinThreshold, { minLength: 0, maxLength: 20 }),
				(entries) => {
					const sorted = sortSSLEntries(entries);
					expect(sorted.length).toBe(entries.length);

					const inputIds = entries.map((e) => e.monitor_id).sort();
					const outputIds = sorted.map((e) => e.monitor_id).sort();
					expect(outputIds).toEqual(inputIds);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('sort does not mutate the original array', () => {
		fc.assert(
			fc.property(
				fc.array(arbitrarySSLEntryWithinThreshold, { minLength: 1, maxLength: 10 }),
				(entries) => {
					const original = [...entries];
					sortSSLEntries(entries);

					expect(entries).toEqual(original);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('expired entries (negative days) sort before non-expired entries', () => {
		fc.assert(
			fc.property(
				fc.tuple(
					fc.record({
						monitor_id: fc.uuid(),
						monitor_name: fc.string({ minLength: 1, maxLength: 20 }),
						days_remaining: fc.integer({ min: -30, max: -1 }),
						expires_at: fc.constant('2024-06-01T00:00:00Z')
					}),
					fc.record({
						monitor_id: fc.uuid(),
						monitor_name: fc.string({ minLength: 1, maxLength: 20 }),
						days_remaining: fc.integer({ min: 1, max: 30 }),
						expires_at: fc.constant('2024-07-01T00:00:00Z')
					})
				),
				([expired, valid]) => {
					const sorted = sortSSLEntries([valid, expired]);
					expect(sorted[0].days_remaining).toBeLessThan(sorted[1].days_remaining);
				}
			),
			{ numRuns: 100 }
		);
	});
});

describe('Property 13: SSL urgency tier correctly categorizes days remaining', () => {
	/**
	 * **Validates: Requirements 5.1, 5.3, 5.5**
	 */

	it('days_remaining <= 0 always returns "expired"', () => {
		fc.assert(
			fc.property(fc.integer({ min: -365, max: 0 }), (days) => {
				expect(getUrgencyTier(days)).toBe('expired');
			}),
			{ numRuns: 100 }
		);
	});

	it('days_remaining 1–7 always returns "critical"', () => {
		fc.assert(
			fc.property(fc.integer({ min: 1, max: 7 }), (days) => {
				expect(getUrgencyTier(days)).toBe('critical');
			}),
			{ numRuns: 100 }
		);
	});

	it('days_remaining 8–30 always returns "warning"', () => {
		fc.assert(
			fc.property(fc.integer({ min: 8, max: 30 }), (days) => {
				expect(getUrgencyTier(days)).toBe('warning');
			}),
			{ numRuns: 100 }
		);
	});

	it('boundary values are correctly categorized', () => {
		// Exact boundary tests (deterministic but validating property edges)
		expect(getUrgencyTier(0)).toBe('expired');
		expect(getUrgencyTier(-1)).toBe('expired');
		expect(getUrgencyTier(1)).toBe('critical');
		expect(getUrgencyTier(7)).toBe('critical');
		expect(getUrgencyTier(8)).toBe('warning');
		expect(getUrgencyTier(30)).toBe('warning');
	});

	it('tier assignment is exhaustive for the SSL threshold range', () => {
		fc.assert(
			fc.property(arbitraryDaysRemaining, (days) => {
				const tier = getUrgencyTier(days);

				// The function always returns one of the three valid tiers
				expect(['expired', 'critical', 'warning']).toContain(tier);

				// Verify the mapping is correct
				if (days <= 0) {
					expect(tier).toBe('expired');
				} else if (days <= 7) {
					expect(tier).toBe('critical');
				} else {
					expect(tier).toBe('warning');
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('tier transitions happen at correct boundary points', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: -100, max: 100 }),
				(days) => {
					const tier = getUrgencyTier(days);
					const tierNext = getUrgencyTier(days + 1);

					// If we cross from 0 to 1: expired → critical
					if (days === 0) {
						expect(tier).toBe('expired');
						expect(tierNext).toBe('critical');
					}
					// If we cross from 7 to 8: critical → warning
					if (days === 7) {
						expect(tier).toBe('critical');
						expect(tierNext).toBe('warning');
					}
				}
			),
			{ numRuns: 100 }
		);
	});
});
