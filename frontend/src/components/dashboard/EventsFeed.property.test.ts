/**
 * Property-Based Tests: EventsFeed (Properties 16, 17, 18)
 *
 * Tests three fundamental correctness properties of the EventsFeed:
 * - Events feed bounded at 10 in reverse chronological order
 * - Initial events feed filtered to 24-hour window
 * - Relative timestamp formatting produces correct human-readable duration
 *
 * **Validates: Requirements 7.1, 7.3, 7.4, 7.2, 9.3**
 *
 * Feature: dashboard-refactor, Property 16: Events feed bounded at 10 in reverse chronological order
 * Feature: dashboard-refactor, Property 17: Initial events feed filtered to 24-hour window
 * Feature: dashboard-refactor, Property 18: Relative timestamp formatting produces correct human-readable duration
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { RecentEvent } from '$lib/types';
import { formatRelativeTime, filterInitialEvents, prependEvent } from './EventsFeed.svelte';

// --- Constants ---

const MAX_FEED_SIZE = 10;
const TWENTY_FOUR_HOURS_MS = 24 * 60 * 60 * 1000;

// --- Arbitraries ---

/**
 * Generate a valid monitor state ('up' | 'down' | 'unknown').
 */
const arbitraryState: fc.Arbitrary<'up' | 'down' | 'unknown'> = fc.oneof(
	fc.constant('up' as const),
	fc.constant('down' as const),
	fc.constant('unknown' as const)
);

/**
 * Generate a RecentEvent with a specific occurred_at timestamp.
 */
function arbitraryEventAt(dateArb: fc.Arbitrary<Date>): fc.Arbitrary<RecentEvent> {
	return fc.record({
		monitor_id: fc.uuid(),
		monitor_name: fc.string({ minLength: 1, maxLength: 50 }),
		from_state: arbitraryState,
		to_state: arbitraryState,
		occurred_at: dateArb.map((d) => d.toISOString())
	});
}

/**
 * Generate a RecentEvent with a timestamp within the last 24 hours.
 */
const arbitraryRecentEvent: fc.Arbitrary<RecentEvent> = arbitraryEventAt(
	fc.date({
		min: new Date(Date.now() - TWENTY_FOUR_HOURS_MS + 1000),
		max: new Date(),
		noInvalidDate: true
	})
);

/**
 * Generate a RecentEvent with a timestamp older than 24 hours.
 */
const arbitraryOldEvent: fc.Arbitrary<RecentEvent> = arbitraryEventAt(
	fc.date({
		min: new Date('2020-01-01T00:00:00Z'),
		max: new Date(Date.now() - TWENTY_FOUR_HOURS_MS - 1000),
		noInvalidDate: true
	})
);

/**
 * Generate a mixed list of recent and old events (1–30 total).
 */
const arbitraryMixedEvents: fc.Arbitrary<RecentEvent[]> = fc
	.tuple(
		fc.array(arbitraryRecentEvent, { minLength: 0, maxLength: 15 }),
		fc.array(arbitraryOldEvent, { minLength: 0, maxLength: 15 })
	)
	.filter(([recent, old]) => recent.length + old.length > 0)
	.map(([recent, old]) => [...recent, ...old]);

/**
 * Generate a list of events (1–20) with distinct timestamps for ordering tests.
 */
const arbitraryEventList: fc.Arbitrary<RecentEvent[]> = fc.array(arbitraryRecentEvent, {
	minLength: 1,
	maxLength: 20
});

/**
 * Generate a past timestamp as milliseconds offset from "now".
 * Covers from 0ms (just happened) to 30 days ago.
 */
const arbitraryPastOffsetMs: fc.Arbitrary<number> = fc.integer({
	min: 0,
	max: 30 * 24 * 60 * 60 * 1000
});

/**
 * Generate a reference "now" timestamp (reasonable range).
 */
const arbitraryNow: fc.Arbitrary<number> = fc
	.date({
		min: new Date('2024-01-01T00:00:00Z'),
		max: new Date('2025-12-31T23:59:59Z'),
		noInvalidDate: true
	})
	.map((d) => d.getTime());

// --- Property Tests ---

describe('Property 16: Events feed bounded at 10 in reverse chronological order', () => {
	it('prependEvent never exceeds maxSize entries', () => {
		fc.assert(
			fc.property(
				arbitraryEventList,
				arbitraryRecentEvent,
				fc.integer({ min: 1, max: 20 }),
				(existingEvents, newEvent, maxSize) => {
					const result = prependEvent(existingEvents, newEvent, maxSize);

					expect(result.length).toBeLessThanOrEqual(maxSize);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('prependEvent always places the new event first (most recent)', () => {
		fc.assert(
			fc.property(arbitraryEventList, arbitraryRecentEvent, (existingEvents, newEvent) => {
				const result = prependEvent(existingEvents, newEvent, MAX_FEED_SIZE);

				expect(result[0]).toEqual(newEvent);
			}),
			{ numRuns: 100 }
		);
	});

	it('prependEvent preserves relative order of existing events', () => {
		fc.assert(
			fc.property(arbitraryEventList, arbitraryRecentEvent, (existingEvents, newEvent) => {
				const result = prependEvent(existingEvents, newEvent, MAX_FEED_SIZE);

				// Existing events that remain should be in the same relative order
				const remaining = result.slice(1);
				const expectedRemaining = existingEvents.slice(0, MAX_FEED_SIZE - 1);

				expect(remaining).toEqual(expectedRemaining);
			}),
			{ numRuns: 100 }
		);
	});

	it('prependEvent removes oldest (last) event when list exceeds maxSize', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryRecentEvent, { minLength: MAX_FEED_SIZE, maxLength: 20 }),
				arbitraryRecentEvent,
				(existingEvents, newEvent) => {
					const result = prependEvent(existingEvents, newEvent, MAX_FEED_SIZE);

					// Result must be exactly MAX_FEED_SIZE
					expect(result.length).toBe(MAX_FEED_SIZE);

					// The new event is first
					expect(result[0]).toEqual(newEvent);

					// The oldest entries from original are dropped
					const keptFromOriginal = result.slice(1);
					expect(keptFromOriginal).toEqual(existingEvents.slice(0, MAX_FEED_SIZE - 1));
				}
			),
			{ numRuns: 100 }
		);
	});

	it('prependEvent returns a new array (immutable)', () => {
		fc.assert(
			fc.property(arbitraryEventList, arbitraryRecentEvent, (existingEvents, newEvent) => {
				const originalRef = [...existingEvents];
				const result = prependEvent(existingEvents, newEvent, MAX_FEED_SIZE);

				// Original array must not be mutated
				expect(existingEvents).toEqual(originalRef);
				// Result is a different reference
				expect(result).not.toBe(existingEvents);
			}),
			{ numRuns: 100 }
		);
	});

	it('sequential prepends maintain bounded size and newest-first order', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryRecentEvent, { minLength: 5, maxLength: 25 }),
				(allEvents) => {
					let feed: RecentEvent[] = [];

					// Simulate adding events one by one
					for (const event of allEvents) {
						feed = prependEvent(feed, event, MAX_FEED_SIZE);
					}

					// Feed never exceeds max size
					expect(feed.length).toBeLessThanOrEqual(MAX_FEED_SIZE);

					// The most recently added event is first
					expect(feed[0]).toEqual(allEvents[allEvents.length - 1]);
				}
			),
			{ numRuns: 100 }
		);
	});
});

describe('Property 17: Initial events feed filtered to 24-hour window', () => {
	it('filterInitialEvents includes only events within 24 hours of now', () => {
		fc.assert(
			fc.property(arbitraryMixedEvents, (events) => {
				const now = Date.now();
				const result = filterInitialEvents(events, now);
				const cutoff = now - TWENTY_FOUR_HOURS_MS;

				for (const event of result) {
					const eventTime = new Date(event.occurred_at).getTime();
					expect(eventTime).toBeGreaterThanOrEqual(cutoff);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('filterInitialEvents excludes all events older than 24 hours', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryOldEvent, { minLength: 1, maxLength: 15 }),
				(oldEvents) => {
					const now = Date.now();
					const result = filterInitialEvents(oldEvents, now);

					expect(result).toHaveLength(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('filterInitialEvents caps results at 10 entries', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryRecentEvent, { minLength: 11, maxLength: 30 }),
				(manyRecentEvents) => {
					const now = Date.now();
					const result = filterInitialEvents(manyRecentEvents, now);

					expect(result.length).toBeLessThanOrEqual(MAX_FEED_SIZE);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('filterInitialEvents returns events in reverse chronological order', () => {
		fc.assert(
			fc.property(arbitraryMixedEvents, (events) => {
				const now = Date.now();
				const result = filterInitialEvents(events, now);

				// Each event should be >= the next one (descending by occurred_at)
				for (let i = 0; i < result.length - 1; i++) {
					const currentTime = new Date(result[i].occurred_at).getTime();
					const nextTime = new Date(result[i + 1].occurred_at).getTime();
					expect(currentTime).toBeGreaterThanOrEqual(nextTime);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('filterInitialEvents selects the 10 most recent when more qualify', () => {
		fc.assert(
			fc.property(
				fc.array(arbitraryRecentEvent, { minLength: 11, maxLength: 30 }),
				(manyRecentEvents) => {
					const now = Date.now();
					const result = filterInitialEvents(manyRecentEvents, now);

					// All qualifying events sorted by time descending
					const cutoff = now - TWENTY_FOUR_HOURS_MS;
					const allQualifying = manyRecentEvents
						.filter((e) => new Date(e.occurred_at).getTime() >= cutoff)
						.sort(
							(a, b) =>
								new Date(b.occurred_at).getTime() - new Date(a.occurred_at).getTime()
						);

					// Result should be the top 10 from all qualifying
					const expectedTop10 = allQualifying.slice(0, MAX_FEED_SIZE);
					expect(result).toEqual(expectedTop10);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('filterInitialEvents does not mutate the input array', () => {
		fc.assert(
			fc.property(arbitraryMixedEvents, (events) => {
				const originalCopy = [...events];
				const now = Date.now();
				filterInitialEvents(events, now);

				expect(events).toEqual(originalCopy);
			}),
			{ numRuns: 100 }
		);
	});
});

describe('Property 18: Relative timestamp formatting produces correct human-readable duration', () => {
	it('timestamps < 10 seconds ago produce "justNow" key', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 0, max: 9999 }),
				(now, offsetMs) => {
					const occurredAt = new Date(now - offsetMs).toISOString();
					const result = formatRelativeTime(occurredAt, now);

					expect(result.key).toBe('dashboard.events.justNow');
					expect(result.params).toBeUndefined();
				}
			),
			{ numRuns: 100 }
		);
	});

	it('timestamps 10–59 seconds ago produce "secondsAgo" key with correct count', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 10_000, max: 59_999 }),
				(now, offsetMs) => {
					const occurredAt = new Date(now - offsetMs).toISOString();
					const result = formatRelativeTime(occurredAt, now);

					const expectedSeconds = Math.floor(offsetMs / 1000);
					expect(result.key).toBe('dashboard.events.secondsAgo');
					expect(result.params).toEqual({ count: String(expectedSeconds) });
				}
			),
			{ numRuns: 100 }
		);
	});

	it('timestamps 1–59 minutes ago produce "minutesAgo" key with correct count', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 60_000, max: 3_599_999 }),
				(now, offsetMs) => {
					const occurredAt = new Date(now - offsetMs).toISOString();
					const result = formatRelativeTime(occurredAt, now);

					const expectedMinutes = Math.floor(Math.floor(offsetMs / 1000) / 60);
					expect(result.key).toBe('dashboard.events.minutesAgo');
					expect(result.params).toEqual({ count: String(expectedMinutes) });
				}
			),
			{ numRuns: 100 }
		);
	});

	it('timestamps 1–23 hours ago produce "hoursAgo" key with correct count', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 3_600_000, max: 86_399_999 }),
				(now, offsetMs) => {
					const occurredAt = new Date(now - offsetMs).toISOString();
					const result = formatRelativeTime(occurredAt, now);

					const totalSeconds = Math.floor(offsetMs / 1000);
					const expectedHours = Math.floor(Math.floor(totalSeconds / 60) / 60);
					expect(result.key).toBe('dashboard.events.hoursAgo');
					expect(result.params).toEqual({ count: String(expectedHours) });
				}
			),
			{ numRuns: 100 }
		);
	});

	it('timestamps >= 24 hours ago produce "daysAgo" key with correct count', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 86_400_000, max: 30 * 86_400_000 }),
				(now, offsetMs) => {
					const occurredAt = new Date(now - offsetMs).toISOString();
					const result = formatRelativeTime(occurredAt, now);

					const totalSeconds = Math.floor(offsetMs / 1000);
					const totalMinutes = Math.floor(totalSeconds / 60);
					const totalHours = Math.floor(totalMinutes / 60);
					const expectedDays = Math.floor(totalHours / 24);
					expect(result.key).toBe('dashboard.events.daysAgo');
					expect(result.params).toEqual({ count: String(expectedDays) });
				}
			),
			{ numRuns: 100 }
		);
	});

	it('formatRelativeTime always returns a valid key for any past timestamp', () => {
		fc.assert(
			fc.property(arbitraryNow, arbitraryPastOffsetMs, (now, offsetMs) => {
				const occurredAt = new Date(now - offsetMs).toISOString();
				const result = formatRelativeTime(occurredAt, now);

				const validKeys = [
					'dashboard.events.justNow',
					'dashboard.events.secondsAgo',
					'dashboard.events.minutesAgo',
					'dashboard.events.hoursAgo',
					'dashboard.events.daysAgo'
				];
				expect(validKeys).toContain(result.key);
			}),
			{ numRuns: 100 }
		);
	});

	it('formatRelativeTime unit transitions are monotonic (larger offset → same or larger unit)', () => {
		fc.assert(
			fc.property(
				arbitraryNow,
				fc.integer({ min: 0, max: 30 * 86_400_000 }),
				fc.integer({ min: 0, max: 30 * 86_400_000 }),
				(now, offsetA, offsetB) => {
					const smallerOffset = Math.min(offsetA, offsetB);
					const largerOffset = Math.max(offsetA, offsetB);

					const occurredAtSmaller = new Date(now - smallerOffset).toISOString();
					const occurredAtLarger = new Date(now - largerOffset).toISOString();

					const resultSmaller = formatRelativeTime(occurredAtSmaller, now);
					const resultLarger = formatRelativeTime(occurredAtLarger, now);

					const unitOrder = [
						'dashboard.events.justNow',
						'dashboard.events.secondsAgo',
						'dashboard.events.minutesAgo',
						'dashboard.events.hoursAgo',
						'dashboard.events.daysAgo'
					];

					const indexSmaller = unitOrder.indexOf(resultSmaller.key);
					const indexLarger = unitOrder.indexOf(resultLarger.key);

					// Larger offset should produce same or higher unit
					expect(indexLarger).toBeGreaterThanOrEqual(indexSmaller);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('formatRelativeTime count values are always positive integers when params present', () => {
		fc.assert(
			fc.property(arbitraryNow, arbitraryPastOffsetMs, (now, offsetMs) => {
				const occurredAt = new Date(now - offsetMs).toISOString();
				const result = formatRelativeTime(occurredAt, now);

				if (result.params?.count) {
					const count = Number(result.params.count);
					expect(Number.isInteger(count)).toBe(true);
					expect(count).toBeGreaterThan(0);
				}
			}),
			{ numRuns: 100 }
		);
	});
});
