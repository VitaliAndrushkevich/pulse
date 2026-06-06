/**
 * Property-Based Tests: Tag Persistence Round-Trip
 *
 * Tests the fundamental property that for any valid tag set T,
 * storing and retrieving it produces exactly T (same keys, values — set equality holds).
 *
 * Since we cannot hit a real API in unit tests, we validate the round-trip through:
 * 1. MonitorStore: set a monitor with tags → read back → verify tags match exactly
 * 2. Serialization cycle: JSON.stringify (request body) → JSON.parse (response) → verify identical
 * 3. API request/response shape: CreateMonitorRequest.tags → Monitor.tags round-trip
 *
 * **Validates: Requirements 3.1, 3.2, 3.4, 3.5**
 */
import { describe, it, expect, beforeEach } from 'vitest';
import * as fc from 'fast-check';
import type { Monitor, Tag } from '$lib/types';
import type { CreateMonitorRequest, UpdateMonitorRequest } from '$lib/api';
import { monitorStore } from '$lib/stores/monitors.svelte';

// --- Arbitraries ---

/**
 * Generate a valid tag key: matches ^[a-z][a-z0-9_-]{0,63}$, not starting with __
 */
const arbitraryTagKey: fc.Arbitrary<string> = fc
	.tuple(
		fc.constantFrom(...'abcdefghijklmnopqrstuvwxyz'.split('')),
		fc.stringOf(
			fc.constantFrom(...'abcdefghijklmnopqrstuvwxyz0123456789_-'.split('')),
			{ minLength: 0, maxLength: 62 }
		)
	)
	.map(([first, rest]) => first + rest)
	.filter((key) => !key.startsWith('__'));

/**
 * Generate a valid tag value: 1-256 printable characters (no control chars).
 * Uses printable ASCII subset for simplicity — covers core round-trip semantics.
 */
const arbitraryTagValue: fc.Arbitrary<string> = fc.stringOf(
	fc.integer({ min: 32, max: 126 }).map((cp) => String.fromCharCode(cp)),
	{ minLength: 1, maxLength: 256 }
);

/**
 * Generate a single valid Tag.
 */
const arbitraryTag: fc.Arbitrary<Tag> = fc.record({
	key: arbitraryTagKey,
	value: arbitraryTagValue
});

/**
 * Generate a valid tag set: 0-20 tags, no duplicate (key, value) pairs.
 */
const arbitraryTagSet: fc.Arbitrary<Tag[]> = fc
	.array(arbitraryTag, { minLength: 0, maxLength: 20 })
	.map((tags) => {
		// Deduplicate by (key, value) pair
		const seen = new Set<string>();
		return tags.filter((tag) => {
			const fingerprint = `${tag.key}::${tag.value}`;
			if (seen.has(fingerprint)) return false;
			seen.add(fingerprint);
			return true;
		});
	});

/**
 * Generate a complete Monitor object with a given tag set.
 */
function makeMonitorWithTags(id: string, tags: Tag[]): Monitor {
	return {
		id,
		name: 'Test Monitor',
		type: 'http',
		target: 'https://example.com',
		interval_seconds: 60,
		timeout_seconds: 10,
		status: 'active',
		state: 'up',
		last_checked_at: '2024-01-01T00:00:00Z',
		next_check_at: '2024-01-01T00:01:00Z',
		settings: {},
		tags,
		created_at: '2024-01-01T00:00:00Z',
		updated_at: '2024-01-01T00:00:00Z'
	};
}

// --- Helpers ---

/**
 * Compare two tag sets for equality (order-independent).
 * Returns true if both sets contain exactly the same (key, value) pairs.
 */
function tagSetsEqual(a: Tag[], b: Tag[]): boolean {
	if (a.length !== b.length) return false;
	const setA = new Set(a.map((t) => `${t.key}::${t.value}`));
	const setB = new Set(b.map((t) => `${t.key}::${t.value}`));
	if (setA.size !== setB.size) return false;
	for (const item of setA) {
		if (!setB.has(item)) return false;
	}
	return true;
}

// --- Property Tests ---

describe('Property 1: Tag Persistence Round-Trip', () => {
	beforeEach(() => {
		monitorStore.clear();
	});

	it('monitorStore.setMonitors preserves tags exactly (create round-trip)', () => {
		fc.assert(
			fc.property(arbitraryTagSet, (tags) => {
				monitorStore.clear();
				const monitor = makeMonitorWithTags('rt-1', tags);

				// Simulate: API returns monitor after creation → store receives it
				monitorStore.setMonitors([monitor]);

				// Retrieve from store
				const retrieved = monitorStore.getById('rt-1');
				expect(retrieved).toBeDefined();
				expect(tagSetsEqual(retrieved!.tags, tags)).toBe(true);
			}),
			{ numRuns: 200 }
		);
	});

	it('monitorStore.updateMonitor preserves tags exactly (update round-trip)', () => {
		fc.assert(
			fc.property(arbitraryTagSet, arbitraryTagSet, (initialTags, updatedTags) => {
				monitorStore.clear();

				// Create with initial tags
				const initial = makeMonitorWithTags('rt-2', initialTags);
				monitorStore.setMonitors([initial]);

				// Update with new tags (simulating PUT response)
				const updated = makeMonitorWithTags('rt-2', updatedTags);
				monitorStore.updateMonitor(updated);

				// Retrieve and verify updated tags
				const retrieved = monitorStore.getById('rt-2');
				expect(retrieved).toBeDefined();
				expect(tagSetsEqual(retrieved!.tags, updatedTags)).toBe(true);
			}),
			{ numRuns: 200 }
		);
	});

	it('JSON serialization round-trip preserves tag data exactly', () => {
		fc.assert(
			fc.property(arbitraryTagSet, (tags) => {
				// Simulate: request body serialization (what the API client sends)
				const requestBody: CreateMonitorRequest = {
					name: 'Test',
					type: 'http',
					target: 'https://example.com',
					interval_seconds: 60,
					timeout_seconds: 10,
					tags
				};
				const serialized = JSON.stringify(requestBody);

				// Simulate: response deserialization (what the API client receives)
				const parsed = JSON.parse(serialized) as CreateMonitorRequest;

				// Verify tags survived the serialization round-trip
				expect(parsed.tags).toBeDefined();
				expect(tagSetsEqual(parsed.tags!, tags)).toBe(true);

				// Verify individual key/value integrity
				for (let i = 0; i < tags.length; i++) {
					expect(parsed.tags![i].key).toBe(tags[i].key);
					expect(parsed.tags![i].value).toBe(tags[i].value);
				}
			}),
			{ numRuns: 200 }
		);
	});

	it('applyTagsChange WebSocket round-trip preserves tags exactly', () => {
		fc.assert(
			fc.property(arbitraryTagSet, arbitraryTagSet, (initialTags, wsTags) => {
				monitorStore.clear();

				// Set initial monitor with tags
				const monitor = makeMonitorWithTags('rt-3', initialTags);
				monitorStore.setMonitors([monitor]);

				// Simulate: monitor_tags_changed WS message updates tags
				monitorStore.applyTagsChange('rt-3', wsTags);

				// Retrieve and verify
				const retrieved = monitorStore.getById('rt-3');
				expect(retrieved).toBeDefined();
				expect(tagSetsEqual(retrieved!.tags, wsTags)).toBe(true);
			}),
			{ numRuns: 200 }
		);
	});

	it('tag set equality is independent of array order', () => {
		fc.assert(
			fc.property(
				arbitraryTagSet.filter((tags) => tags.length >= 2),
				(tags) => {
					monitorStore.clear();

					// Store with original order
					const monitor1 = makeMonitorWithTags('rt-4', tags);
					monitorStore.setMonitors([monitor1]);

					// Create reversed copy and update
					const reversed = [...tags].reverse();
					const monitor2 = makeMonitorWithTags('rt-4', reversed);
					monitorStore.updateMonitor(monitor2);

					// Both should represent the same tag set
					const retrieved = monitorStore.getById('rt-4');
					expect(retrieved).toBeDefined();
					expect(tagSetsEqual(retrieved!.tags, tags)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('empty tag set round-trips correctly', () => {
		monitorStore.clear();
		const monitor = makeMonitorWithTags('rt-empty', []);
		monitorStore.setMonitors([monitor]);

		const retrieved = monitorStore.getById('rt-empty');
		expect(retrieved).toBeDefined();
		expect(retrieved!.tags).toEqual([]);
	});

	it('multiple monitors preserve independent tag sets', () => {
		fc.assert(
			fc.property(arbitraryTagSet, arbitraryTagSet, (tagsA, tagsB) => {
				monitorStore.clear();

				const monitorA = makeMonitorWithTags('mon-a', tagsA);
				const monitorB = makeMonitorWithTags('mon-b', tagsB);
				monitorStore.setMonitors([monitorA, monitorB]);

				const retrievedA = monitorStore.getById('mon-a');
				const retrievedB = monitorStore.getById('mon-b');

				expect(retrievedA).toBeDefined();
				expect(retrievedB).toBeDefined();
				expect(tagSetsEqual(retrievedA!.tags, tagsA)).toBe(true);
				expect(tagSetsEqual(retrievedB!.tags, tagsB)).toBe(true);
			}),
			{ numRuns: 100 }
		);
	});
});
