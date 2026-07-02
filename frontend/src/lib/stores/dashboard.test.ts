import { describe, it, expect, beforeEach, vi } from 'vitest';
import { dashboardStore } from './dashboard.svelte';
import { monitorStore } from './monitors.svelte';
import type { MonitorPatch, Monitor } from '$lib/types';

// Mock the API module so load() doesn't make real HTTP requests
vi.mock('$lib/api', () => ({
	getDashboardSummary: vi.fn().mockResolvedValue({
		health_score: { uptime_percent: 99.5, active_monitor_count: 10, partial_data: false },
		status_distribution: { up: 8, down: 1, unknown: 1, total: 10 },
		active_incidents: [],
		top_latency_monitors: [],
		ssl_expiry: [],
		heatmap: [],
		recent_events: [],
		generated_at: '2024-01-15T14:30:00Z'
	})
}));

// --- Test helpers ---

function makeMonitor(overrides: Partial<Monitor> = {}): Monitor {
	return {
		id: 'mon-1',
		name: 'Test Monitor',
		type: 'http',
		target: 'https://example.com',
		interval_seconds: 60,
		timeout_seconds: 10,
		status: 'active',
		state: 'up',
		last_checked_at: '2024-01-15T14:00:00Z',
		next_check_at: '2024-01-15T14:01:00Z',
		settings: {},
		tags: [],
		history_retention_days: 7,
		created_at: '2024-01-01T00:00:00Z',
		updated_at: '2024-01-01T00:00:00Z',
		...overrides
	};
}

function makePatch(overrides: Partial<MonitorPatch> = {}): MonitorPatch {
	return {
		monitor_id: 'mon-1',
		state: 'down',
		latency_ms: 150,
		checked_at: '2024-01-15T14:30:00Z',
		timestamp: '2024-01-15T14:30:00Z',
		...overrides
	};
}

describe('DashboardStore — staleness management', () => {
	beforeEach(() => {
		dashboardStore.reset();
	});

	describe('initial state', () => {
		it('starts with stale = false', () => {
			expect(dashboardStore.stale).toBe(false);
		});
	});

	describe('markStale()', () => {
		it('sets stale flag to true', () => {
			dashboardStore.markStale();
			expect(dashboardStore.stale).toBe(true);
		});

		it('is idempotent — calling multiple times stays true', () => {
			dashboardStore.markStale();
			dashboardStore.markStale();
			expect(dashboardStore.stale).toBe(true);
		});
	});

	describe('clearStale()', () => {
		it('sets stale flag to false', () => {
			dashboardStore.markStale();
			expect(dashboardStore.stale).toBe(true);

			dashboardStore.clearStale();
			expect(dashboardStore.stale).toBe(false);
		});

		it('updates lastUpdated timestamp', () => {
			expect(dashboardStore.lastUpdated).toBeNull();

			const before = new Date();
			dashboardStore.clearStale();
			const after = new Date();

			expect(dashboardStore.lastUpdated).not.toBeNull();
			expect(dashboardStore.lastUpdated!.getTime()).toBeGreaterThanOrEqual(before.getTime());
			expect(dashboardStore.lastUpdated!.getTime()).toBeLessThanOrEqual(after.getTime());
		});

		it('updates lastUpdated on each call', async () => {
			dashboardStore.clearStale();
			const first = dashboardStore.lastUpdated!.getTime();

			await new Promise((r) => setTimeout(r, 10));

			dashboardStore.clearStale();
			const second = dashboardStore.lastUpdated!.getTime();

			expect(second).toBeGreaterThanOrEqual(first);
		});

		it('is safe to call when already not stale', () => {
			expect(dashboardStore.stale).toBe(false);
			dashboardStore.clearStale();
			expect(dashboardStore.stale).toBe(false);
			expect(dashboardStore.lastUpdated).not.toBeNull();
		});
	});

	describe('reset()', () => {
		it('clears stale flag', () => {
			dashboardStore.markStale();
			expect(dashboardStore.stale).toBe(true);

			dashboardStore.reset();
			expect(dashboardStore.stale).toBe(false);
		});
	});

	describe('load() on reconnect', () => {
		it('refreshes all data and updates lastUpdated', async () => {
			dashboardStore.markStale();
			expect(dashboardStore.stale).toBe(true);

			await dashboardStore.load();

			// load() sets lastUpdated on success
			expect(dashboardStore.lastUpdated).not.toBeNull();
			// Data should be populated from the mock
			expect(dashboardStore.healthScore).not.toBeNull();
			expect(dashboardStore.healthScore!.uptime_percent).toBe(99.5);
		});
	});
});


describe('DashboardStore — applyPatch()', () => {
	beforeEach(() => {
		dashboardStore.reset();
		monitorStore.clear();
	});

	/**
	 * Helper: set up dashboard with initial state and populate monitorStateCache.
	 * Simulates what happens after load() populates the store.
	 */
	async function setupWithMonitors(monitors: Monitor[]): Promise<void> {
		monitorStore.setMonitors(monitors);
		// Call load() to populate state and monitorStateCache
		await dashboardStore.load();
	}

	describe('discards patches for unknown monitors', () => {
		it('silently ignores patch for monitor not in the cache', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			const beforeDist = { ...dashboardStore.statusDistribution! };
			dashboardStore.applyPatch(makePatch({ monitor_id: 'unknown-id', state: 'down' }));

			// Distribution unchanged
			expect(dashboardStore.statusDistribution).toEqual(beforeDist);
		});
	});

	describe('statusDistribution updates', () => {
		it('decrements old state and increments new state on transition', async () => {
			await setupWithMonitors([
				makeMonitor({ id: 'mon-1', state: 'up' }),
				makeMonitor({ id: 'mon-2', state: 'up' }),
				makeMonitor({ id: 'mon-3', state: 'down' })
			]);

			// mon-1 transitions from up → down
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));

			const dist = dashboardStore.statusDistribution!;
			// Was: up=8(mock), now adjusted: up-1, down+1
			// But since load() uses the mock response (up:8, down:1, unknown:1),
			// after patch: up=7, down=2, unknown=1
			expect(dist.up).toBe(7);
			expect(dist.down).toBe(2);
			expect(dist.unknown).toBe(1);
		});

		it('does not change distribution when state is the same', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			const beforeDist = { ...dashboardStore.statusDistribution! };
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));

			expect(dashboardStore.statusDistribution).toEqual(beforeDist);
		});

		it('floors count at 0 to prevent negative values', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'unknown' })]);

			// Mock returns unknown: 1. Transitioning from unknown to up should yield unknown: 0 (not -1)
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));

			const dist = dashboardStore.statusDistribution!;
			expect(dist.unknown).toBe(0);
			expect(dist.up).toBe(9); // was 8, now +1
		});
	});

	describe('activeIncidents updates', () => {
		it('adds incident when monitor transitions TO down', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', name: 'API Server', state: 'up' })]);

			dashboardStore.applyPatch(
				makePatch({ monitor_id: 'mon-1', state: 'down', error: 'Connection refused' })
			);

			const incidents = dashboardStore.activeIncidents;
			expect(incidents.length).toBe(1);
			expect(incidents[0].monitor_id).toBe('mon-1');
			expect(incidents[0].monitor_name).toBe('API Server');
			expect(incidents[0].cause).toBe('Connection refused');
			expect(incidents[0].state).toBe('down');
		});

		it('removes incident when monitor transitions FROM down', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			// First go down
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));
			expect(dashboardStore.activeIncidents.length).toBe(1);

			// Then recover
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));
			expect(dashboardStore.activeIncidents.length).toBe(0);
		});

		it('does not add incident when state stays down', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			// Go down
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));
			expect(dashboardStore.activeIncidents.length).toBe(1);

			// Patch with same state (still down)
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));
			expect(dashboardStore.activeIncidents.length).toBe(1);
		});
	});

	describe('recentEvents updates', () => {
		it('prepends new event on state change', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', name: 'Web App', state: 'up' })]);

			dashboardStore.applyPatch(
				makePatch({ monitor_id: 'mon-1', state: 'down', checked_at: '2024-01-15T15:00:00Z' })
			);

			const events = dashboardStore.recentEvents;
			expect(events.length).toBe(1);
			expect(events[0].monitor_id).toBe('mon-1');
			expect(events[0].monitor_name).toBe('Web App');
			expect(events[0].from_state).toBe('up');
			expect(events[0].to_state).toBe('down');
			expect(events[0].occurred_at).toBe('2024-01-15T15:00:00Z');
		});

		it('does not add event when state is unchanged', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));
			expect(dashboardStore.recentEvents.length).toBe(0);
		});

		it('caps events at 10 entries (removes oldest)', async () => {
			const monitors = Array.from({ length: 12 }, (_, i) =>
				makeMonitor({ id: `mon-${i}`, name: `Monitor ${i}`, state: 'up' })
			);
			await setupWithMonitors(monitors);

			// Generate 12 state transitions
			for (let i = 0; i < 12; i++) {
				dashboardStore.applyPatch(
					makePatch({
						monitor_id: `mon-${i}`,
						state: 'down',
						checked_at: `2024-01-15T15:${String(i).padStart(2, '0')}:00Z`
					})
				);
			}

			expect(dashboardStore.recentEvents.length).toBe(10);
			// Most recent event should be first
			expect(dashboardStore.recentEvents[0].monitor_id).toBe('mon-11');
		});
	});

	describe('healthScore recalculation', () => {
		it('recalculates uptime_percent based on updated distribution', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			// Initial mock: up=8, down=1, unknown=1, total=10 → uptime = 80%
			expect(dashboardStore.healthScore!.uptime_percent).toBe(99.5); // from mock

			// Transition mon-1 from up to down: up becomes 7, total stays 10
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));

			// New: up=7/10 = 70%
			expect(dashboardStore.healthScore!.uptime_percent).toBe(70);
		});
	});

	describe('lastUpdated timestamp', () => {
		it('updates lastUpdated on each patch (even without state change)', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			const before = new Date();
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));
			const after = new Date();

			expect(dashboardStore.lastUpdated!.getTime()).toBeGreaterThanOrEqual(before.getTime());
			expect(dashboardStore.lastUpdated!.getTime()).toBeLessThanOrEqual(after.getTime());
		});
	});

	describe('monitorStateCache tracking', () => {
		it('tracks state across multiple patches for the same monitor', async () => {
			await setupWithMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);

			// up → down
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down' }));
			expect(dashboardStore.activeIncidents.length).toBe(1);

			// down → unknown (not up)
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'unknown' }));
			// Should be removed from incidents (transition FROM down)
			expect(dashboardStore.activeIncidents.length).toBe(0);

			// unknown → up
			dashboardStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'up' }));
			// Should add event but not add incident (not transitioning TO down)
			expect(dashboardStore.activeIncidents.length).toBe(0);
			// 3 state changes = 3 events
			expect(dashboardStore.recentEvents.length).toBe(3);
		});
	});
});
