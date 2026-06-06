import { describe, it, expect, beforeEach } from 'vitest';
import { monitorStore, applyMonitorPatch } from '../stores/monitors.svelte';
import type { Monitor, MonitorPatch } from '../types';

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
		last_checked_at: '2024-01-01T00:00:00Z',
		next_check_at: '2024-01-01T00:01:00Z',
		settings: {},
		tags: [],
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
		checked_at: '2024-01-01T00:02:00Z',
		timestamp: '2024-01-01T00:02:00Z',
		...overrides
	};
}

describe('MonitorStore', () => {
	beforeEach(() => {
		monitorStore.clear();
	});

	describe('setMonitors()', () => {
		it('replaces all monitors in the store', () => {
			const monitors = [makeMonitor({ id: 'a' }), makeMonitor({ id: 'b' })];
			monitorStore.setMonitors(monitors);
			expect(monitorStore.totalCount).toBe(2);
			expect(monitorStore.list.map((m) => m.id).sort()).toEqual(['a', 'b']);
		});

		it('clears previous monitors on set', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'old' })]);
			monitorStore.setMonitors([makeMonitor({ id: 'new' })]);
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].id).toBe('new');
		});

		it('handles empty array', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'x' })]);
			monitorStore.setMonitors([]);
			expect(monitorStore.totalCount).toBe(0);
			expect(monitorStore.list).toEqual([]);
		});

		it('deduplicates monitors by id (last wins)', () => {
			const monitors = [
				makeMonitor({ id: 'dup', name: 'First' }),
				makeMonitor({ id: 'dup', name: 'Second' })
			];
			monitorStore.setMonitors(monitors);
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].name).toBe('Second');
		});
	});

	describe('applyPatch()', () => {
		it('updates state and last_checked_at for a known monitor', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'mon-1', state: 'up' })]);
			monitorStore.applyPatch(makePatch({ monitor_id: 'mon-1', state: 'down', checked_at: '2024-06-01T00:00:00Z' }));
			const updated = monitorStore.list.find((m) => m.id === 'mon-1')!;
			expect(updated.state).toBe('down');
			expect(updated.last_checked_at).toBe('2024-06-01T00:00:00Z');
		});

		it('preserves non-patched fields', () => {
			const original = makeMonitor({ id: 'mon-1', name: 'Keep Me', target: 'https://keep.com' });
			monitorStore.setMonitors([original]);
			monitorStore.applyPatch(makePatch({ monitor_id: 'mon-1' }));
			const updated = monitorStore.list.find((m) => m.id === 'mon-1')!;
			expect(updated.name).toBe('Keep Me');
			expect(updated.target).toBe('https://keep.com');
			expect(updated.interval_seconds).toBe(original.interval_seconds);
			expect(updated.timeout_seconds).toBe(original.timeout_seconds);
			expect(updated.status).toBe(original.status);
			expect(updated.settings).toEqual(original.settings);
			expect(updated.created_at).toBe(original.created_at);
			expect(updated.updated_at).toBe(original.updated_at);
		});

		it('discards patches for unknown monitor_ids', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'known' })]);
			monitorStore.applyPatch(makePatch({ monitor_id: 'unknown' }));
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].id).toBe('known');
		});

		it('does not create new entries for unknown monitor_ids', () => {
			monitorStore.setMonitors([]);
			monitorStore.applyPatch(makePatch({ monitor_id: 'ghost' }));
			expect(monitorStore.totalCount).toBe(0);
		});
	});

	describe('updateMonitor()', () => {
		it('adds a new monitor', () => {
			monitorStore.updateMonitor(makeMonitor({ id: 'new-mon' }));
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].id).toBe('new-mon');
		});

		it('replaces an existing monitor', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'x', name: 'Old' })]);
			monitorStore.updateMonitor(makeMonitor({ id: 'x', name: 'New' }));
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].name).toBe('New');
		});
	});

	describe('removeMonitor()', () => {
		it('removes an existing monitor by id', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'a' }), makeMonitor({ id: 'b' })]);
			monitorStore.removeMonitor('a');
			expect(monitorStore.totalCount).toBe(1);
			expect(monitorStore.list[0].id).toBe('b');
		});

		it('does nothing for unknown ids', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'a' })]);
			monitorStore.removeMonitor('nonexistent');
			expect(monitorStore.totalCount).toBe(1);
		});
	});

	describe('clear()', () => {
		it('empties the store', () => {
			monitorStore.setMonitors([makeMonitor({ id: 'a' }), makeMonitor({ id: 'b' })]);
			monitorStore.clear();
			expect(monitorStore.totalCount).toBe(0);
			expect(monitorStore.list).toEqual([]);
		});
	});

	describe('derived stats', () => {
		it('computes totalCount correctly', () => {
			monitorStore.setMonitors([
				makeMonitor({ id: '1' }),
				makeMonitor({ id: '2' }),
				makeMonitor({ id: '3' })
			]);
			expect(monitorStore.totalCount).toBe(3);
		});

		it('computes healthyCount (state === up)', () => {
			monitorStore.setMonitors([
				makeMonitor({ id: '1', state: 'up' }),
				makeMonitor({ id: '2', state: 'down' }),
				makeMonitor({ id: '3', state: 'up' }),
				makeMonitor({ id: '4', state: 'unknown' })
			]);
			expect(monitorStore.healthyCount).toBe(2);
		});

		it('computes unhealthyCount (state === down)', () => {
			monitorStore.setMonitors([
				makeMonitor({ id: '1', state: 'up' }),
				makeMonitor({ id: '2', state: 'down' }),
				makeMonitor({ id: '3', state: 'down' }),
				makeMonitor({ id: '4', state: 'unknown' })
			]);
			expect(monitorStore.unhealthyCount).toBe(2);
		});

		it('unknown state counts neither as healthy nor unhealthy', () => {
			monitorStore.setMonitors([
				makeMonitor({ id: '1', state: 'unknown' }),
				makeMonitor({ id: '2', state: 'unknown' })
			]);
			expect(monitorStore.healthyCount).toBe(0);
			expect(monitorStore.unhealthyCount).toBe(0);
			expect(monitorStore.totalCount).toBe(2);
		});

		it('all zeros for empty store', () => {
			expect(monitorStore.totalCount).toBe(0);
			expect(monitorStore.healthyCount).toBe(0);
			expect(monitorStore.unhealthyCount).toBe(0);
		});

		it('stats update after applyPatch changes state', () => {
			monitorStore.setMonitors([
				makeMonitor({ id: 'a', state: 'up' }),
				makeMonitor({ id: 'b', state: 'up' })
			]);
			expect(monitorStore.healthyCount).toBe(2);
			expect(monitorStore.unhealthyCount).toBe(0);

			monitorStore.applyPatch(makePatch({ monitor_id: 'a', state: 'down' }));
			expect(monitorStore.healthyCount).toBe(1);
			expect(monitorStore.unhealthyCount).toBe(1);
		});
	});
});

describe('applyMonitorPatch() pure function', () => {
	it('merges state and last_checked_at from patch', () => {
		const monitor = makeMonitor({ state: 'up', last_checked_at: '2024-01-01T00:00:00Z' });
		const patch = makePatch({ state: 'down', checked_at: '2024-06-01T12:00:00Z' });
		const result = applyMonitorPatch(monitor, patch);
		expect(result.state).toBe('down');
		expect(result.last_checked_at).toBe('2024-06-01T12:00:00Z');
	});

	it('preserves all non-patched fields', () => {
		const monitor = makeMonitor({
			id: 'keep-id',
			name: 'Keep Name',
			type: 'tcp',
			target: 'keep-target:443',
			interval_seconds: 30,
			timeout_seconds: 5,
			status: 'paused',
			settings: { foo: 'bar' },
			next_check_at: '2024-01-01T00:01:00Z',
			created_at: '2023-01-01T00:00:00Z',
			updated_at: '2023-06-01T00:00:00Z'
		});
		const patch = makePatch({ state: 'up', checked_at: '2024-07-01T00:00:00Z' });
		const result = applyMonitorPatch(monitor, patch);

		expect(result.id).toBe('keep-id');
		expect(result.name).toBe('Keep Name');
		expect(result.type).toBe('tcp');
		expect(result.target).toBe('keep-target:443');
		expect(result.interval_seconds).toBe(30);
		expect(result.timeout_seconds).toBe(5);
		expect(result.status).toBe('paused');
		expect(result.settings).toEqual({ foo: 'bar' });
		expect(result.next_check_at).toBe('2024-01-01T00:01:00Z');
		expect(result.created_at).toBe('2023-01-01T00:00:00Z');
		expect(result.updated_at).toBe('2023-06-01T00:00:00Z');
	});

	it('does not mutate the original monitor', () => {
		const monitor = makeMonitor({ state: 'up' });
		const patch = makePatch({ state: 'down' });
		const result = applyMonitorPatch(monitor, patch);
		expect(result).not.toBe(monitor);
		expect(monitor.state).toBe('up');
	});

	it('is deterministic — same inputs produce same output', () => {
		const monitor = makeMonitor();
		const patch = makePatch();
		const result1 = applyMonitorPatch(monitor, patch);
		const result2 = applyMonitorPatch(monitor, patch);
		expect(result1).toEqual(result2);
	});
});
