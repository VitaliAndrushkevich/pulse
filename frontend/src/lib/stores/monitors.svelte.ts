/**
 * MonitorStore — reactive monitor collection with patch-merge logic.
 *
 * Uses Svelte 5 runes ($state, $derived) for fine-grained reactivity.
 * Internal state is a Map<string, Monitor> for O(1) lookups by ID.
 *
 * Patch merge algorithm:
 * - Only `state` and `last_checked_at` are merged from MonitorPatch into Monitor
 * - Patches for unknown monitor_ids are silently discarded
 * - Merge is deterministic and non-destructive
 *
 * Requirements: 1.4, 5.2, 5.3, 5.6
 */

import type { Monitor, MonitorPatch } from '$lib/types';

// --- Pure patch-merge function ---

/**
 * Merges a MonitorPatch into an existing Monitor object.
 * Returns a new Monitor with patched fields applied.
 *
 * Rules:
 * 1. Only `state` and `last_checked_at` (from patch.checked_at) are merged
 * 2. monitor_id and timestamp are metadata — not merged into Monitor fields
 * 3. The merge is non-destructive: original fields not in the patch are preserved
 * 4. The merge is deterministic: same monitor + same patch = same result
 */
export function applyMonitorPatch(monitor: Monitor, patch: MonitorPatch): Monitor {
	return {
		...monitor,
		state: patch.state,
		last_checked_at: patch.checked_at
	};
}

// --- Reactive state (Svelte 5 runes) ---

let monitors = $state<Map<string, Monitor>>(new Map());

// --- Derived stats ---

const list = $derived<Monitor[]>(Array.from(monitors.values()));
const totalCount = $derived<number>(monitors.size);
const healthyCount = $derived<number>(
	Array.from(monitors.values()).filter((m) => m.state === 'up').length
);
const unhealthyCount = $derived<number>(
	Array.from(monitors.values()).filter((m) => m.state === 'down').length
);

// --- Actions ---

/**
 * Bulk replace all monitors (used on initial fetch and reconnect).
 * Rebuilds the map from scratch.
 */
function setMonitors(monitorList: Monitor[]): void {
	const next = new Map<string, Monitor>();
	for (const m of monitorList) {
		next.set(m.id, m);
	}
	monitors = next;
}

/**
 * Apply a WebSocket patch to an existing monitor.
 * Discards patches for unknown monitor_ids (Requirement 5.6).
 */
function applyPatch(patch: MonitorPatch): void {
	const existing = monitors.get(patch.monitor_id);
	if (!existing) {
		// Discard patch for unknown monitor_id
		return;
	}
	const updated = applyMonitorPatch(existing, patch);
	const next = new Map(monitors);
	next.set(patch.monitor_id, updated);
	monitors = next;
}

/**
 * Add or replace a single monitor in the store.
 * Used after create/update API calls.
 */
function updateMonitor(monitor: Monitor): void {
	const next = new Map(monitors);
	next.set(monitor.id, monitor);
	monitors = next;
}

/**
 * Remove a monitor from the store by ID.
 * Used after delete API calls.
 */
function removeMonitor(id: string): void {
	if (!monitors.has(id)) return;
	const next = new Map(monitors);
	next.delete(id);
	monitors = next;
}

/**
 * Clear all monitors from the store.
 */
function clear(): void {
	monitors = new Map();
}

/**
 * Lookup a monitor by ID from the reactive state.
 * Returns the Monitor if found, or undefined if not in the store.
 * Because `monitors` is reactive ($state), any component reading this
 * will re-render when the underlying map entry changes (e.g., from a WS patch).
 */
function getById(id: string): Monitor | undefined {
	return monitors.get(id);
}

// --- Exported singleton store ---

export const monitorStore = {
	get list(): Monitor[] {
		return list;
	},
	get totalCount(): number {
		return totalCount;
	},
	get healthyCount(): number {
		return healthyCount;
	},
	get unhealthyCount(): number {
		return unhealthyCount;
	},
	setMonitors,
	applyPatch,
	updateMonitor,
	removeMonitor,
	getById,
	clear
};
