import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getBackoffDelay, createWsClient, type WsMessage } from '../ws';
import type { ConnectionStatus } from '../stores/connection.svelte';

// ---------------------------------------------------------------------------
// Mock stores
// ---------------------------------------------------------------------------

let mockToken: string | null = 'test-jwt-token';
const mockClearToken = vi.fn(() => {
	mockToken = null;
});

vi.mock('$lib/stores/auth.svelte', () => ({
	getToken: () => mockToken,
	clearToken: (...args: unknown[]) => mockClearToken(...args)
}));

const mockApplyPatch = vi.fn();
vi.mock('$lib/stores/monitors.svelte', () => ({
	monitorStore: {
		applyPatch: (...args: unknown[]) => mockApplyPatch(...args)
	}
}));

const mockSetStatus = vi.fn();
vi.mock('$lib/stores/connection.svelte', () => ({
	connectionStore: {
		setStatus: (...args: unknown[]) => mockSetStatus(...args)
	}
}));

// ---------------------------------------------------------------------------
// Mock WebSocket
// ---------------------------------------------------------------------------

class MockWebSocket {
	static CONNECTING = 0;
	static OPEN = 1;
	static CLOSING = 2;
	static CLOSED = 3;

	static instances: MockWebSocket[] = [];

	url: string;
	readyState: number = MockWebSocket.CONNECTING;
	onopen: ((event: Event) => void) | null = null;
	onmessage: ((event: MessageEvent) => void) | null = null;
	onclose: ((event: CloseEvent) => void) | null = null;
	onerror: ((event: Event) => void) | null = null;
	closeCode: number | undefined;
	closeReason: string | undefined;

	constructor(url: string) {
		this.url = url;
		MockWebSocket.instances.push(this);
	}

	close(code?: number, reason?: string): void {
		this.closeCode = code;
		this.closeReason = reason;
		this.readyState = MockWebSocket.CLOSED;
		if (this.onclose) {
			this.onclose(new CloseEvent('close', { code: code ?? 1005, reason }));
		}
	}

	send(_data: string): void {
		// no-op for tests
	}

	// Test helpers
	simulateOpen(): void {
		this.readyState = MockWebSocket.OPEN;
		if (this.onopen) {
			this.onopen(new Event('open'));
		}
	}

	simulateMessage(data: string): void {
		if (this.onmessage) {
			this.onmessage(new MessageEvent('message', { data }));
		}
	}

	simulateClose(code: number = 1006, reason: string = ''): void {
		this.readyState = MockWebSocket.CLOSED;
		if (this.onclose) {
			this.onclose(new CloseEvent('close', { code, reason }));
		}
	}

	simulateError(): void {
		if (this.onerror) {
			this.onerror(new Event('error'));
		}
	}
}

// ---------------------------------------------------------------------------
// Test Setup
// ---------------------------------------------------------------------------

describe('WebSocket Client', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		MockWebSocket.instances = [];
		mockToken = 'test-jwt-token';
		mockApplyPatch.mockClear();
		mockSetStatus.mockClear();
		mockClearToken.mockClear();

		// Install mock WebSocket globally
		vi.stubGlobal('WebSocket', MockWebSocket);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	// -----------------------------------------------------------------------
	// getBackoffDelay
	// -----------------------------------------------------------------------

	describe('getBackoffDelay()', () => {
		it('returns ~1000ms for attempt 0', () => {
			// With jitter ±25%, range is [750, 1250]
			const delay = getBackoffDelay(0);
			expect(delay).toBeGreaterThanOrEqual(750);
			expect(delay).toBeLessThanOrEqual(1250);
		});

		it('returns ~2000ms for attempt 1', () => {
			const delay = getBackoffDelay(1);
			expect(delay).toBeGreaterThanOrEqual(1500);
			expect(delay).toBeLessThanOrEqual(2500);
		});

		it('returns ~4000ms for attempt 2', () => {
			const delay = getBackoffDelay(2);
			expect(delay).toBeGreaterThanOrEqual(3000);
			expect(delay).toBeLessThanOrEqual(5000);
		});

		it('caps at 30000ms max base (with jitter up to 37500)', () => {
			const delay = getBackoffDelay(100);
			expect(delay).toBeGreaterThanOrEqual(22500); // 30000 - 25%
			expect(delay).toBeLessThanOrEqual(37500); // 30000 + 25%
		});

		it('never returns a negative value', () => {
			for (let i = 0; i < 20; i++) {
				expect(getBackoffDelay(i)).toBeGreaterThan(0);
			}
		});
	});

	// -----------------------------------------------------------------------
	// createWsClient - connection
	// -----------------------------------------------------------------------

	describe('createWsClient()', () => {
		it('creates a WebSocket connection with token in URL', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			expect(MockWebSocket.instances).toHaveLength(1);
			expect(MockWebSocket.instances[0].url).toContain('token=test-jwt-token');
		});

		it('sets status to "connecting" when connect() is called', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			expect(mockSetStatus).toHaveBeenCalledWith('connecting');
		});

		it('sets status to "connected" when server sends connected message', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'abc', timestamp: '2024-01-01T00:00:00Z' } }));

			expect(mockSetStatus).toHaveBeenCalledWith('connected');
		});

		it('does not connect if no token is available', () => {
			mockToken = null;
			const client = createWsClient({ url: '/ws' });
			client.connect();

			expect(MockWebSocket.instances).toHaveLength(0);
			expect(mockSetStatus).toHaveBeenCalledWith('disconnected');
		});

		it('does not connect when WebSocket is not available (SSR)', () => {
			vi.stubGlobal('WebSocket', undefined);
			const client = createWsClient({ url: '/ws' });
			client.connect();

			expect(MockWebSocket.instances).toHaveLength(0);
		});

		it('does not create duplicate connections', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.readyState = MockWebSocket.OPEN;

			client.connect(); // Should be a no-op
			expect(MockWebSocket.instances).toHaveLength(1);
		});
	});

	// -----------------------------------------------------------------------
	// Message handling
	// -----------------------------------------------------------------------

	describe('message handling', () => {
		it('dispatches monitor_status to monitorStore.applyPatch', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();

			const patch = {
				monitor_id: 'mon-1',
				state: 'down',
				latency_ms: 250,
				checked_at: '2024-01-01T00:00:00Z',
				timestamp: '2024-01-01T00:00:00Z'
			};
			ws.simulateMessage(JSON.stringify({ type: 'monitor_status', payload: patch }));

			expect(mockApplyPatch).toHaveBeenCalledWith(patch);
		});

		it('calls onMessage callback for received messages', () => {
			const onMessage = vi.fn();
			const client = createWsClient({ url: '/ws', onMessage });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'c1', timestamp: '2024-01-01' } }));

			expect(onMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'connected' }));
		});

		it('discards invalid JSON messages without crashing', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();

			// Should not throw
			expect(() => {
				ws.simulateMessage('not valid json {{{');
			}).not.toThrow();
		});

		it('discards messages without a type field', () => {
			const onMessage = vi.fn();
			const client = createWsClient({ url: '/ws', onMessage });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ payload: { data: 'something' } }));

			expect(onMessage).not.toHaveBeenCalled();
		});
	});

	// -----------------------------------------------------------------------
	// Connect timeout
	// -----------------------------------------------------------------------

	describe('connect timeout', () => {
		it('closes socket if connected message not received within timeout', () => {
			const client = createWsClient({ url: '/ws', connectTimeout: 5000 });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			// Don't send 'connected' message

			// Advance past the timeout
			vi.advanceTimersByTime(5000);

			// The socket should have been closed (triggering reconnection)
			expect(ws.readyState).toBe(MockWebSocket.CLOSED);
		});

		it('does not close socket if connected message arrives before timeout', () => {
			const client = createWsClient({ url: '/ws', connectTimeout: 5000 });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));

			// Advance past timeout — should be safe
			vi.advanceTimersByTime(5000);
			expect(ws.readyState).toBe(MockWebSocket.OPEN);
		});
	});

	// -----------------------------------------------------------------------
	// Close code 4401 (auth expired)
	// -----------------------------------------------------------------------

	describe('auth expired (close code 4401)', () => {
		it('sets status to auth_expired and does not reconnect', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));

			// Simulate auth expired close
			ws.simulateClose(4401);

			expect(mockSetStatus).toHaveBeenCalledWith('auth_expired');

			// Advance time — should not attempt reconnection
			vi.advanceTimersByTime(60000);
			expect(MockWebSocket.instances).toHaveLength(1); // No new connections
		});

		it('clears the auth token on 4401', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateClose(4401);

			expect(mockClearToken).toHaveBeenCalled();
		});
	});

	// -----------------------------------------------------------------------
	// Reconnection
	// -----------------------------------------------------------------------

	describe('reconnection', () => {
		it('schedules reconnection after unexpected close', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));
			ws.simulateClose(1006); // Abnormal closure

			expect(mockSetStatus).toHaveBeenCalledWith('disconnected');

			// First reconnect attempt after ~1000ms (±25% jitter)
			vi.advanceTimersByTime(1300); // Upper bound of first attempt
			expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(2);
		});

		it('resets reconnect attempt counter on successful connection', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			// First connection succeeds
			let ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));

			// Connection drops
			ws.simulateClose(1006);

			// Reconnect
			vi.advanceTimersByTime(1300);
			ws = MockWebSocket.instances[1];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'y', timestamp: '2024-01-02' } }));

			// Drop again — backoff should start from 0 again (~1000ms, not 2000ms)
			ws.simulateClose(1006);
			vi.advanceTimersByTime(1300);
			expect(MockWebSocket.instances.length).toBeGreaterThanOrEqual(3);
		});

		it('does not reconnect after intentional disconnect', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));

			client.disconnect();

			// Wait a long time — no new connections
			vi.advanceTimersByTime(60000);
			expect(MockWebSocket.instances).toHaveLength(1);
		});
	});

	// -----------------------------------------------------------------------
	// Clean disconnect
	// -----------------------------------------------------------------------

	describe('disconnect()', () => {
		it('sends close code 1000 by default', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.readyState = MockWebSocket.OPEN;

			client.disconnect();
			expect(ws.closeCode).toBe(1000);
		});

		it('allows custom close code', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.readyState = MockWebSocket.OPEN;

			client.disconnect(4000);
			expect(ws.closeCode).toBe(4000);
		});

		it('sets status to disconnected', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.readyState = MockWebSocket.OPEN;

			client.disconnect();
			expect(mockSetStatus).toHaveBeenCalledWith('disconnected');
		});

		it('cancels any pending reconnection', () => {
			const client = createWsClient({ url: '/ws' });
			client.connect();

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateClose(1006); // Trigger reconnection scheduling

			// Disconnect before reconnection fires
			client.disconnect();

			vi.advanceTimersByTime(60000);
			expect(MockWebSocket.instances).toHaveLength(1); // No new connections
		});
	});

	// -----------------------------------------------------------------------
	// onStatusChange callback
	// -----------------------------------------------------------------------

	describe('onStatusChange callback', () => {
		it('is called on status transitions', () => {
			const onStatusChange = vi.fn();
			const client = createWsClient({ url: '/ws', onStatusChange });
			client.connect();

			expect(onStatusChange).toHaveBeenCalledWith('connecting');

			const ws = MockWebSocket.instances[0];
			ws.simulateOpen();
			ws.simulateMessage(JSON.stringify({ type: 'connected', payload: { client_id: 'x', timestamp: '2024-01-01' } }));

			expect(onStatusChange).toHaveBeenCalledWith('connected');
		});
	});
});
