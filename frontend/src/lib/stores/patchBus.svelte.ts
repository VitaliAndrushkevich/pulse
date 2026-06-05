/**
 * Patch Event Bus — lightweight pub/sub for MonitorPatch events.
 *
 * The WS client publishes patches here (in addition to calling monitorStore.applyPatch).
 * Components that need to react to specific monitor patches (e.g., the detail page
 * updating its local history array) subscribe via this bus.
 *
 * This is intentionally a separate module from the monitor store so that:
 * 1. The store remains focused on global monitor state
 * 2. Page-level reactions (local history updates) are decoupled
 * 3. Tests can mock the store independently without breaking patch subscriptions
 *
 * Note: listeners is a plain array (not $state) because mutations to it should NOT
 * trigger Svelte reactivity — it's purely an imperative callback registry.
 */

import type { MonitorPatch } from '$lib/types';

type PatchListener = (patch: MonitorPatch) => void;

const listeners: PatchListener[] = [];

/**
 * Subscribe to monitor patch events.
 * Returns an unsubscribe function.
 */
function subscribe(listener: PatchListener): () => void {
	listeners.push(listener);
	return () => {
		const idx = listeners.indexOf(listener);
		if (idx >= 0) {
			listeners.splice(idx, 1);
		}
	};
}

/**
 * Publish a patch to all subscribers.
 * Called by the WS client when a monitor_status message arrives.
 */
function publish(patch: MonitorPatch): void {
	for (const listener of listeners) {
		listener(patch);
	}
}

export const patchBus = {
	subscribe,
	publish
};
