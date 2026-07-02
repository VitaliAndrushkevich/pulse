/**
 * DashboardStore — reactive dashboard state with per-widget error isolation.
 *
 * Uses Svelte 5 runes ($state, $derived) for fine-grained reactivity.
 * Fetches aggregated data from GET /api/v1/dashboard/summary and populates
 * widget-level state. Each widget tracks its own loading/error independently
 * via Maps keyed by WidgetId.
 *
 * The load() method uses Promise.allSettled-style error isolation so that
 * a failure in one widget's data processing does not crash the others.
 *
 * Requirements: 8.3, 8.4, 8.6, 9.2
 */

import { getDashboardSummary } from '$lib/api';
import { monitorStore } from '$lib/stores/monitors.svelte';
import type {
	HealthScoreData,
	StatusDistribution,
	ActiveIncident,
	TopLatencyMonitor,
	SSLExpiryEntry,
	HeatmapHour,
	RecentEvent,
	MonitorPatch,
	WidgetId
} from '$lib/types';

// --- Reactive state (Svelte 5 runes) ---

let healthScore = $state<HealthScoreData | null>(null);
let statusDistribution = $state<StatusDistribution | null>(null);
let activeIncidents = $state<ActiveIncident[]>([]);
let topLatencyMonitors = $state<TopLatencyMonitor[]>([]);
let sslExpiry = $state<SSLExpiryEntry[]>([]);
let heatmap = $state<HeatmapHour[]>([]);
let recentEvents = $state<RecentEvent[]>([]);
let lastUpdated = $state<Date | null>(null);
let stale = $state<boolean>(false);
let widgetErrors = $state<Map<WidgetId, string>>(new Map());
let widgetLoading = $state<Map<WidgetId, boolean>>(new Map());

// --- Internal per-monitor state cache ---
// Tracks the last known state per monitor from the dashboard's perspective.
// Populated on load() from monitorStore.list, updated on each applyPatch().
// This allows applyPatch to determine the OLD state even though monitorStore
// has already been updated by the time patchBus delivers the patch.
let monitorStateCache = new Map<string, 'up' | 'down' | 'unknown'>();

// --- Derived values ---

const isLoading = $derived<boolean>(
	Array.from(widgetLoading.values()).some((v) => v === true)
);

const hasErrors = $derived<boolean>(widgetErrors.size > 0);

// --- Widget ID constants for internal use ---

const ALL_WIDGETS: WidgetId[] = [
	'health-score',
	'status-ring',
	'incidents',
	'sparklines',
	'ssl-expiry',
	'heatmap',
	'events-feed'
];

// --- Internal helpers ---

function setWidgetLoading(widgetId: WidgetId, loading: boolean): void {
	const next = new Map(widgetLoading);
	if (loading) {
		next.set(widgetId, true);
	} else {
		next.delete(widgetId);
	}
	widgetLoading = next;
}

function setWidgetError(widgetId: WidgetId, error: string | null): void {
	const next = new Map(widgetErrors);
	if (error) {
		next.set(widgetId, error);
	} else {
		next.delete(widgetId);
	}
	widgetErrors = next;
}

function clearAllErrors(): void {
	widgetErrors = new Map();
}

function setAllWidgetsLoading(loading: boolean): void {
	if (loading) {
		const next = new Map<WidgetId, boolean>();
		for (const id of ALL_WIDGETS) {
			next.set(id, true);
		}
		widgetLoading = next;
	} else {
		widgetLoading = new Map();
	}
}

// --- Actions ---

/**
 * Fetch dashboard summary from the API and populate state.
 * Uses error isolation: the single API call returns all widget data,
 * but each widget's data population is wrapped independently so that
 * a parsing or assignment error in one widget does not affect others.
 *
 * On WS reconnect, this should be called to replace local state (Requirement 9.2).
 */
async function load(): Promise<void> {
	setAllWidgetsLoading(true);
	clearAllErrors();

	try {
		const summary = await getDashboardSummary();

		// Use Promise.allSettled pattern for per-widget error isolation.
		// Each widget population is independent — one failure won't crash others.
		const results = await Promise.allSettled([
			// health-score
			Promise.resolve().then(() => {
				healthScore = summary.health_score;
			}),
			// status-ring
			Promise.resolve().then(() => {
				statusDistribution = summary.status_distribution;
			}),
			// incidents
			Promise.resolve().then(() => {
				activeIncidents = summary.active_incidents;
			}),
			// sparklines
			Promise.resolve().then(() => {
				topLatencyMonitors = summary.top_latency_monitors;
			}),
			// ssl-expiry
			Promise.resolve().then(() => {
				sslExpiry = summary.ssl_expiry;
			}),
			// heatmap
			Promise.resolve().then(() => {
				heatmap = summary.heatmap;
			}),
			// events-feed
			Promise.resolve().then(() => {
				recentEvents = summary.recent_events;
			})
		]);

		// Map results back to widget IDs and record errors
		results.forEach((result, index) => {
			const widgetId = ALL_WIDGETS[index];
			if (result.status === 'rejected') {
				setWidgetError(widgetId, result.reason?.message ?? 'Failed to load widget data');
			}
		});

		lastUpdated = new Date();

		// Populate per-monitor state cache from monitorStore for future applyPatch calls
		monitorStateCache = new Map();
		for (const monitor of monitorStore.list) {
			monitorStateCache.set(monitor.id, monitor.state);
		}
	} catch (err: unknown) {
		// Network/API-level failure — all widgets get the same error
		const message = err instanceof Error ? err.message : 'Failed to load dashboard data';
		for (const widgetId of ALL_WIDGETS) {
			setWidgetError(widgetId, message);
		}
	} finally {
		setAllWidgetsLoading(false);
	}
}

/**
 * Apply a WebSocket MonitorPatch to incrementally update dashboard state.
 *
 * Called by the page component via patchBus subscription when a monitor_status
 * message arrives. Updates statusDistribution counts, activeIncidents list,
 * recentEvents feed, healthScore, and lastUpdated timestamp.
 *
 * Requirements: 1.4, 2.5, 3.4, 3.5, 7.3, 9.1
 */
function applyPatch(patch: MonitorPatch): void {
	const oldState = monitorStateCache.get(patch.monitor_id);

	// If we don't know this monitor, silently discard (matches monitorStore behavior)
	if (oldState === undefined) {
		return;
	}

	const newState = patch.state;

	// --- 1. Update statusDistribution counts ---
	if (statusDistribution && oldState !== newState) {
		const next: StatusDistribution = { ...statusDistribution };
		// Decrement old state count (floor at 0 for safety)
		next[oldState] = Math.max(0, next[oldState] - 1);
		// Increment new state count
		next[newState] = next[newState] + 1;
		statusDistribution = next;
	}

	// --- 2. Update activeIncidents list ---
	if (oldState !== newState) {
		// If monitor transitions TO down, add to activeIncidents
		if (newState === 'down') {
			// Look up monitor name from monitorStore (already patched at this point)
			const monitor = monitorStore.getById(patch.monitor_id);
			const monitorName = monitor?.name ?? patch.monitor_id;

			const newIncident: ActiveIncident = {
				monitor_id: patch.monitor_id,
				monitor_name: monitorName,
				started_at: patch.checked_at,
				cause: patch.error ?? null,
				state: 'down'
			};

			activeIncidents = [...activeIncidents, newIncident];
		}

		// If monitor transitions FROM down, remove from activeIncidents
		if (oldState === 'down') {
			activeIncidents = activeIncidents.filter(
				(incident) => incident.monitor_id !== patch.monitor_id
			);
		}
	}

	// --- 3. Prepend new event to recentEvents (if state changed) ---
	if (oldState !== newState) {
		const monitor = monitorStore.getById(patch.monitor_id);
		const monitorName = monitor?.name ?? patch.monitor_id;

		const newEvent: RecentEvent = {
			monitor_id: patch.monitor_id,
			monitor_name: monitorName,
			from_state: oldState,
			to_state: newState,
			occurred_at: patch.checked_at
		};

		// Prepend and cap at 10 entries
		const updatedEvents = [newEvent, ...recentEvents];
		recentEvents = updatedEvents.slice(0, 10);
	}

	// --- 4. Recalculate healthScore based on updated distribution ---
	if (statusDistribution && healthScore) {
		const dist = statusDistribution;
		const total = dist.total;
		if (total > 0) {
			// Health score = percentage of monitors that are "up"
			const uptimePercent = Math.round((dist.up / total) * 10000) / 100;
			healthScore = {
				...healthScore,
				uptime_percent: uptimePercent
			};
		}
	}

	// --- 5. Update monitorStateCache and lastUpdated ---
	monitorStateCache.set(patch.monitor_id, newState);
	lastUpdated = new Date();
}

/**
 * Mark the dashboard data as stale.
 * Called by the page component after 60s of WebSocket silence (Requirement 9.4).
 */
function markStale(): void {
	stale = true;
}

/**
 * Clear the stale indicator and update the lastUpdated timestamp.
 * Called by the page component when a new WebSocket message is received (Requirement 9.5).
 */
function clearStale(): void {
	stale = false;
	lastUpdated = new Date();
}

/**
 * Reset the store to initial empty state.
 */
function reset(): void {
	healthScore = null;
	statusDistribution = null;
	activeIncidents = [];
	topLatencyMonitors = [];
	sslExpiry = [];
	heatmap = [];
	recentEvents = [];
	lastUpdated = null;
	stale = false;
	widgetErrors = new Map();
	widgetLoading = new Map();
	monitorStateCache = new Map();
}

// --- Exported singleton store ---

export const dashboardStore = {
	// Reactive getters
	get healthScore(): HealthScoreData | null {
		return healthScore;
	},
	get statusDistribution(): StatusDistribution | null {
		return statusDistribution;
	},
	get activeIncidents(): ActiveIncident[] {
		return activeIncidents;
	},
	get topLatencyMonitors(): TopLatencyMonitor[] {
		return topLatencyMonitors;
	},
	get sslExpiry(): SSLExpiryEntry[] {
		return sslExpiry;
	},
	get heatmap(): HeatmapHour[] {
		return heatmap;
	},
	get recentEvents(): RecentEvent[] {
		return recentEvents;
	},
	get lastUpdated(): Date | null {
		return lastUpdated;
	},
	get stale(): boolean {
		return stale;
	},
	get widgetErrors(): Map<WidgetId, string> {
		return widgetErrors;
	},
	get widgetLoading(): Map<WidgetId, boolean> {
		return widgetLoading;
	},
	get isLoading(): boolean {
		return isLoading;
	},
	get hasErrors(): boolean {
		return hasErrors;
	},

	// Actions
	load,
	applyPatch,
	reset,
	markStale,
	clearStale,

	// Per-widget error/loading management (exposed for retry patterns)
	setWidgetLoading,
	setWidgetError
};
