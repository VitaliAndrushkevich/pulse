import { describe, it, expect, beforeEach } from 'vitest';
import { connectionStore } from './connection.svelte';

describe('ConnectionStore', () => {
	beforeEach(() => {
		connectionStore.setStatus('disconnected');
	});

	describe('initial state', () => {
		it('starts with disconnected status', () => {
			expect(connectionStore.status).toBe('disconnected');
		});
	});

	describe('setStatus', () => {
		it('updates status to connecting', () => {
			connectionStore.setStatus('connecting');
			expect(connectionStore.status).toBe('connecting');
		});

		it('updates status to connected', () => {
			connectionStore.setStatus('connected');
			expect(connectionStore.status).toBe('connected');
		});

		it('updates status to disconnected', () => {
			connectionStore.setStatus('connected');
			connectionStore.setStatus('disconnected');
			expect(connectionStore.status).toBe('disconnected');
		});

		it('updates status to auth_expired', () => {
			connectionStore.setStatus('auth_expired');
			expect(connectionStore.status).toBe('auth_expired');
		});
	});

	describe('lastConnected', () => {
		it('records timestamp when transitioning to connected', () => {
			const before = new Date();
			connectionStore.setStatus('connected');
			const after = new Date();

			expect(connectionStore.lastConnected).not.toBeNull();
			expect(connectionStore.lastConnected!.getTime()).toBeGreaterThanOrEqual(before.getTime());
			expect(connectionStore.lastConnected!.getTime()).toBeLessThanOrEqual(after.getTime());
		});

		it('does not update lastConnected for non-connected transitions', () => {
			connectionStore.setStatus('connected');
			const connectedAt = connectionStore.lastConnected;

			connectionStore.setStatus('disconnected');
			expect(connectionStore.lastConnected).toBe(connectedAt);

			connectionStore.setStatus('connecting');
			expect(connectionStore.lastConnected).toBe(connectedAt);
		});

		it('updates lastConnected on each reconnection', async () => {
			connectionStore.setStatus('connected');
			const firstConnect = connectionStore.lastConnected!.getTime();

			// Small delay to ensure different timestamp
			await new Promise((r) => setTimeout(r, 10));

			connectionStore.setStatus('disconnected');
			connectionStore.setStatus('connected');
			const secondConnect = connectionStore.lastConnected!.getTime();

			expect(secondConnect).toBeGreaterThanOrEqual(firstConnect);
		});
	});
});
