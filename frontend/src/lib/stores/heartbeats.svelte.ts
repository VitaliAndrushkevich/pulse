/**
 * HeartbeatStore — reactive heartbeat collection with patch-merge logic.
 *
 * Uses Svelte 5 runes ($state, $derived) for fine-grained reactivity.
 * Internal state is a Map<string, Heartbeat> for O(1) lookups by ID.
 *
 * Patch merge algorithm (mirrors monitors.svelte.ts):
 * - Only `state`, `last_success_at`, and `next_expected_deadline` are merged
 *   from a HeartbeatStatusPayload into a Heartbeat.
 * - Patches for unknown heartbeat_ids are silently discarded.
 * - Merge is deterministic and non-destructive.
 *
 * Design: §9.2, R8.4/R9.9.
 */

import type { Heartbeat, HeartbeatStatusPayload } from '$lib/types';

// --- Pure patch-merge function ---

/**
 * Merges a HeartbeatStatusPayload into an existing Heartbeat object.
 * Returns a new Heartbeat with patched fields applied.
 *
 * Rules:
 * 1. Only `state`, `last_success_at`, and `next_expected_deadline` are merged.
 * 2. heartbeat_id and timestamp are metadata — not merged into Heartbeat fields.
 * 3. A patch field that is omitted (undefined) leaves the existing value intact;
 *    an explicit null clears it. This keeps the merge a strict field-subset update.
 * 4. The merge is non-destructive: original fields not in the patch are preserved.
 * 5. The merge is deterministic: same heartbeat + same patch = same result.
 */
export function applyHeartbeatPatch(
	heartbeat: Heartbeat,
	patch: HeartbeatStatusPayload
): Heartbeat {
	return {
		...heartbeat,
		state: patch.state,
		last_success_at:
			patch.last_success_at === undefined ? heartbeat.last_success_at : patch.last_success_at,
		next_expected_deadline:
			patch.next_expected_deadline === undefined
				? heartbeat.next_expected_deadline
				: patch.next_expected_deadline
	};
}

// --- Reactive state (Svelte 5 runes) ---

let heartbeats = $state<Map<string, Heartbeat>>(new Map());

// --- Derived stats ---

const list = $derived<Heartbeat[]>(Array.from(heartbeats.values()));
const totalCount = $derived<number>(heartbeats.size);
const upCount = $derived<number>(
	Array.from(heartbeats.values()).filter((h) => h.state === 'up').length
);
const downCount = $derived<number>(
	Array.from(heartbeats.values()).filter((h) => h.state === 'down').length
);

// --- Actions ---

/**
 * Bulk replace all heartbeats (used on initial fetch and reconnect).
 * Rebuilds the map from scratch.
 */
function setHeartbeats(heartbeatList: Heartbeat[]): void {
	const next = new Map<string, Heartbeat>();
	for (const h of heartbeatList) {
		next.set(h.id, h);
	}
	heartbeats = next;
}

/**
 * Apply a WebSocket patch to an existing heartbeat.
 * Discards patches for unknown heartbeat_ids (R8.4, R9.9).
 */
function applyPatch(patch: HeartbeatStatusPayload): void {
	const existing = heartbeats.get(patch.heartbeat_id);
	if (!existing) {
		// Discard patch for unknown heartbeat_id
		return;
	}
	const updated = applyHeartbeatPatch(existing, patch);
	const next = new Map(heartbeats);
	next.set(patch.heartbeat_id, updated);
	heartbeats = next;
}

/**
 * Add or replace a single heartbeat in the store.
 * Used after create/update API calls.
 */
function updateHeartbeat(heartbeat: Heartbeat): void {
	const next = new Map(heartbeats);
	next.set(heartbeat.id, heartbeat);
	heartbeats = next;
}

/**
 * Remove a heartbeat from the store by ID.
 * Used after delete API calls.
 */
function removeHeartbeat(id: string): void {
	if (!heartbeats.has(id)) return;
	const next = new Map(heartbeats);
	next.delete(id);
	heartbeats = next;
}

/**
 * Clear all heartbeats from the store.
 */
function clear(): void {
	heartbeats = new Map();
}

/**
 * Lookup a heartbeat by ID from the reactive state.
 * Returns the Heartbeat if found, or undefined if not in the store.
 */
function getById(id: string): Heartbeat | undefined {
	return heartbeats.get(id);
}

// --- Exported singleton store ---

export const heartbeatStore = {
	get list(): Heartbeat[] {
		return list;
	},
	get totalCount(): number {
		return totalCount;
	},
	get upCount(): number {
		return upCount;
	},
	get downCount(): number {
		return downCount;
	},
	setHeartbeats,
	applyPatch,
	updateHeartbeat,
	removeHeartbeat,
	getById,
	clear
};
