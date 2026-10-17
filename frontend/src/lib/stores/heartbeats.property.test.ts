/**
 * Property-Based Tests: Heartbeat patch-merge semantics
 *
 * Validates the fundamental properties of applyHeartbeatPatch and the
 * heartbeatStore.applyPatch merge path:
 *   - Field-subset merge: only state / last_success_at / next_expected_deadline
 *     change; every other heartbeat field is preserved untouched.
 *   - Idempotency: applying the same patch twice yields the same result as once.
 *   - Unknown-id discard: a patch for an id absent from the store is ignored.
 *
 * Mirrors frontend/src/lib/stores/dashboard.property.test.ts conventions.
 *
 * Design: §9.2, Requirements R8.4 / R9.9.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import * as fc from 'fast-check';
import type { Heartbeat, HeartbeatStatusPayload, HeartbeatState } from '$lib/types';
import { applyHeartbeatPatch, heartbeatStore } from '$lib/stores/heartbeats.svelte';

// --- Arbitraries ---

const arbState: fc.Arbitrary<HeartbeatState> = fc.constantFrom('up', 'down', 'paused', 'new');

const arbTimestamp: fc.Arbitrary<string> = fc
  .date({
    min: new Date('2020-01-01T00:00:00Z'),
    max: new Date('2030-01-01T00:00:00Z'),
    noInvalidDate: true
  })
  .map((d) => d.toISOString());

const arbTag = fc.record({
  key: fc.string({ minLength: 1, maxLength: 12 }),
  value: fc.string({ minLength: 1, maxLength: 12 })
});

const arbHeartbeat: fc.Arbitrary<Heartbeat> = fc.record({
  id: fc.uuid(),
  name: fc.string({ minLength: 1, maxLength: 40 }),
  expected_interval_seconds: fc.integer({ min: 1, max: 31536000 }),
  grace_seconds: fc.integer({ min: 0, max: 31536000 }),
  status: fc.constantFrom<'active' | 'paused'>('active', 'paused'),
  state: arbState,
  last_success_at: fc.option(arbTimestamp, { nil: null }),
  next_expected_deadline: fc.option(arbTimestamp, { nil: null }),
  tags: fc.array(arbTag, { maxLength: 4 }),
  token_prefix: fc.string({ minLength: 8, maxLength: 8 }),
  token_created_at: arbTimestamp,
  created_at: arbTimestamp,
  updated_at: arbTimestamp
});

// Build a status payload for a given heartbeat id. last_success_at /
// next_expected_deadline may be present, explicit-null, or omitted (undefined).
function arbPatchFor(id: string): fc.Arbitrary<HeartbeatStatusPayload> {
  return fc.record(
    {
      heartbeat_id: fc.constant(id),
      state: arbState,
      last_success_at: fc.oneof(arbTimestamp, fc.constant(null), fc.constant(undefined)),
      next_expected_deadline: fc.oneof(arbTimestamp, fc.constant(null), fc.constant(undefined)),
      timestamp: arbTimestamp
    },
    { requiredKeys: ['heartbeat_id', 'state', 'timestamp'] }
  );
}

// Fields that applyHeartbeatPatch is allowed to change.
const MERGED_FIELDS = new Set(['state', 'last_success_at', 'next_expected_deadline']);

describe('applyHeartbeatPatch — field-subset merge', () => {
  it('only state / last_success_at / next_expected_deadline may change', () => {
    fc.assert(
      fc.property(
        arbHeartbeat.chain((hb) => fc.tuple(fc.constant(hb), arbPatchFor(hb.id))),
        ([hb, patch]) => {
          const result = applyHeartbeatPatch(hb, patch);

          for (const key of Object.keys(hb) as (keyof Heartbeat)[]) {
            if (!MERGED_FIELDS.has(key)) {
              expect(result[key]).toEqual(hb[key]);
            }
          }

          // state always takes the patch value
          expect(result.state).toBe(patch.state);

          // omitted (undefined) timestamp fields preserve the original value;
          // present (including explicit null) ones override
          expect(result.last_success_at).toBe(
            patch.last_success_at === undefined ? hb.last_success_at : patch.last_success_at
          );
          expect(result.next_expected_deadline).toBe(
            patch.next_expected_deadline === undefined
              ? hb.next_expected_deadline
              : patch.next_expected_deadline
          );
        }
      ),
      { numRuns: 200 }
    );
  });

  it('is idempotent — applying the same patch twice equals applying it once', () => {
    fc.assert(
      fc.property(
        arbHeartbeat.chain((hb) => fc.tuple(fc.constant(hb), arbPatchFor(hb.id))),
        ([hb, patch]) => {
          const once = applyHeartbeatPatch(hb, patch);
          const twice = applyHeartbeatPatch(once, patch);
          expect(twice).toEqual(once);
        }
      ),
      { numRuns: 200 }
    );
  });
});

describe('heartbeatStore.applyPatch — unknown-id discard', () => {
  beforeEach(() => {
    heartbeatStore.clear();
  });

  it('discards patches for ids not present in the store', () => {
    fc.assert(
      fc.property(arbHeartbeat, arbState, (hb, patchState) => {
        heartbeatStore.clear();
        heartbeatStore.setHeartbeats([hb]);

        // A patch for a different id must not create or alter any entry.
        const unknownId = hb.id === 'x' ? 'y' : `${hb.id}-unknown`;
        heartbeatStore.applyPatch({
          heartbeat_id: unknownId,
          state: patchState,
          timestamp: '2025-01-01T00:00:00Z'
        });

        expect(heartbeatStore.getById(unknownId)).toBeUndefined();
        expect(heartbeatStore.totalCount).toBe(1);
        // The known heartbeat is untouched by the unknown-id patch.
        expect(heartbeatStore.getById(hb.id)?.state).toBe(hb.state);
      }),
      { numRuns: 100 }
    );
  });

  it('applies a matching-id patch and preserves non-merged fields', () => {
    fc.assert(
      fc.property(
        arbHeartbeat.chain((hb) => fc.tuple(fc.constant(hb), arbPatchFor(hb.id))),
        ([hb, patch]) => {
          heartbeatStore.clear();
          heartbeatStore.setHeartbeats([hb]);

          heartbeatStore.applyPatch(patch);
          const updated = heartbeatStore.getById(hb.id);

          expect(updated).toBeDefined();
          expect(updated?.state).toBe(patch.state);
          expect(updated?.name).toBe(hb.name);
          expect(updated?.expected_interval_seconds).toBe(hb.expected_interval_seconds);
          expect(updated?.token_prefix).toBe(hb.token_prefix);
        }
      ),
      { numRuns: 100 }
    );
  });
});
