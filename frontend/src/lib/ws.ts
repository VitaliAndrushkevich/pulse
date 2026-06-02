/**
 * WebSocket client — manages connection to /ws with reconnection backoff,
 * message dispatch, and auth-expired handling.
 *
 * Requirements: 5.1, 5.4, 5.7, 5.8, 5.9
 */

import { getToken, clearToken } from '$lib/stores/auth.svelte';
import { monitorStore } from '$lib/stores/monitors.svelte';
import { connectionStore, type ConnectionStatus } from '$lib/stores/connection.svelte';
import type { MonitorPatch, WsEnvelope } from '$lib/types';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface WsClientOptions {
	url: string;
	onMessage?: (msg: WsMessage) => void;
	onStatusChange?: (status: ConnectionStatus) => void;
	connectTimeout?: number; // ms, default 5000
}

export interface WsMessage {
	type: 'connected' | 'monitor_status';
	payload: ConnectedPayload | MonitorStatusPayload;
}

export interface ConnectedPayload {
	client_id: string;
	timestamp: string;
}

export interface MonitorStatusPayload {
	monitor_id: string;
	state: 'up' | 'down' | 'unknown';
	latency_ms: number;
	status_code?: number;
	ssl_days_remaining?: number;
	error?: string;
	checked_at: string;
	timestamp: string;
}

export interface WsClient {
	connect: () => void;
	disconnect: (code?: number) => void;
	readonly status: ConnectionStatus;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Close code sent by server when authentication has expired */
const AUTH_EXPIRED_CODE = 4401;

/** Default connect timeout in milliseconds */
const DEFAULT_CONNECT_TIMEOUT = 5000;

/** Normal closure code */
const NORMAL_CLOSURE = 1000;

// ---------------------------------------------------------------------------
// Backoff Algorithm
// ---------------------------------------------------------------------------

/**
 * Compute exponential backoff delay with ±25% jitter.
 *
 * - Base: 1000ms (1 second)
 * - Multiplier: 2x per attempt
 * - Max: 30000ms (30 seconds)
 * - Jitter: ±25% to prevent thundering herd
 *
 * Exported for testing.
 */
export function getBackoffDelay(attempt: number): number {
	const base = 1000;
	const max = 30000;
	const multiplier = 2;
	const delay = Math.min(base * Math.pow(multiplier, attempt), max);
	const jitter = delay * 0.25 * (Math.random() * 2 - 1);
	return delay + jitter;
}

// ---------------------------------------------------------------------------
// WebSocket Client Factory
// ---------------------------------------------------------------------------

/**
 * Creates a WebSocket client that:
 * - Connects to /ws?token=<jwt>
 * - Implements 5s connect timeout
 * - Parses incoming JSON messages and dispatches by type
 * - Reconnects with exponential backoff on unexpected close
 * - Handles close code 4401 (auth expired → redirect to login, no reconnect)
 * - Updates ConnectionStore status on connect/disconnect
 * - Dispatches monitor_status payloads to MonitorStore.applyPatch
 * - Disconnects cleanly with close code 1000 on logout
 */
export function createWsClient(options: WsClientOptions): WsClient {
	const connectTimeout = options.connectTimeout ?? DEFAULT_CONNECT_TIMEOUT;

	let ws: WebSocket | null = null;
	let currentStatus: ConnectionStatus = 'disconnected';
	let reconnectAttempt = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let connectTimer: ReturnType<typeof setTimeout> | null = null;
	let intentionalClose = false;

	function setStatus(status: ConnectionStatus): void {
		currentStatus = status;
		connectionStore.setStatus(status);
		options.onStatusChange?.(status);
	}

	function connect(): void {
		// Guard: WebSocket not available (SSR)
		if (typeof WebSocket === 'undefined') {
			return;
		}

		// Guard: already connected or connecting
		if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
			return;
		}

		// Build URL with token
		const token = getToken();
		if (!token) {
			setStatus('disconnected');
			return;
		}

		intentionalClose = false;
		setStatus('connecting');

		const url = buildWsUrl(options.url, token);
		ws = new WebSocket(url);

		// Connect timeout — if we don't get the 'connected' message within timeout,
		// treat as failed and trigger reconnection
		connectTimer = setTimeout(() => {
			if (currentStatus === 'connecting') {
				// Force close the socket to trigger onclose → reconnection
				ws?.close();
			}
		}, connectTimeout);

		ws.onopen = () => {
			// Connection opened, but we wait for the 'connected' message
			// before marking as fully connected (timeout still active)
		};

		ws.onmessage = (event: MessageEvent) => {
			handleMessage(event.data);
		};

		ws.onclose = (event: CloseEvent) => {
			clearConnectTimeout();
			ws = null;

			if (event.code === AUTH_EXPIRED_CODE) {
				// Auth expired: clear token, redirect to login, NO reconnect
				setStatus('auth_expired');
				clearToken();
				if (typeof window !== 'undefined') {
					window.location.href = '/login';
				}
				return;
			}

			if (intentionalClose) {
				// Clean disconnect (user logout)
				setStatus('disconnected');
				return;
			}

			// Unexpected close — start reconnection
			setStatus('disconnected');
			scheduleReconnect();
		};

		ws.onerror = () => {
			// The close event will fire after error, so reconnection is handled there
		};
	}

	function disconnect(code: number = NORMAL_CLOSURE): void {
		intentionalClose = true;
		clearReconnectTimer();
		clearConnectTimeout();

		if (ws) {
			if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
				ws.close(code);
			}
			ws = null;
		}

		setStatus('disconnected');
	}

	function handleMessage(data: unknown): void {
		if (typeof data !== 'string') return;

		let envelope: WsEnvelope;
		try {
			envelope = JSON.parse(data);
		} catch {
			// Invalid JSON — discard message, don't crash
			return;
		}

		if (!envelope || typeof envelope.type !== 'string') {
			return;
		}

		const msg: WsMessage = {
			type: envelope.type as WsMessage['type'],
			payload: envelope.payload as WsMessage['payload']
		};

		switch (envelope.type) {
			case 'connected':
				clearConnectTimeout();
				reconnectAttempt = 0;
				setStatus('connected');
				break;

			case 'monitor_status':
				monitorStore.applyPatch(envelope.payload as MonitorPatch);
				break;
		}

		// Forward to optional external handler
		options.onMessage?.(msg);
	}

	function scheduleReconnect(): void {
		clearReconnectTimer();
		const delay = getBackoffDelay(reconnectAttempt);
		reconnectAttempt++;

		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			connect();
		}, delay);
	}

	function clearReconnectTimer(): void {
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
	}

	function clearConnectTimeout(): void {
		if (connectTimer !== null) {
			clearTimeout(connectTimer);
			connectTimer = null;
		}
	}

	return {
		connect,
		disconnect,
		get status(): ConnectionStatus {
			return currentStatus;
		}
	};
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Builds the full WebSocket URL from the base path and token.
 * Handles both relative paths and absolute URLs.
 */
function buildWsUrl(basePath: string, token: string): string {
	// If it's already a full URL, append token
	if (basePath.startsWith('ws://') || basePath.startsWith('wss://')) {
		const separator = basePath.includes('?') ? '&' : '?';
		return `${basePath}${separator}token=${encodeURIComponent(token)}`;
	}

	// For relative paths, construct from window.location
	if (typeof window === 'undefined') {
		// SSR fallback — shouldn't normally happen since connect() guards for WebSocket
		return basePath;
	}

	const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	const host = window.location.host;
	const path = basePath.startsWith('/') ? basePath : `/${basePath}`;
	const separator = path.includes('?') ? '&' : '?';
	return `${protocol}//${host}${path}${separator}token=${encodeURIComponent(token)}`;
}
