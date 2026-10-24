/**
 * DashboardStore — reactive dashboard state with per-widget error isolation.
 *
 * Uses Svelte 5 runes ($state, $derived) for fine-grained reactivity.
 * Fetches aggregated data from GET /api/v1/dashboard/summary and populates
 * widget-level state. Each widget tracks its own loading/error independently
 * via Maps keyed by WidgetId.
 *
 * Authoritative server snapshots include per-widget query errors.
 * Concurrent requests are deduplicated and older period responses are ignored.
 *
 * Requirements: 8.3, 8.4, 8.6, 9.2
 */

import { getDashboardSummary } from '$lib/api';
import { t } from '$lib/i18n';
import type { FleetPeriod } from '$lib/types';
import { monitorStore } from '$lib/stores/monitors.svelte';
import type {
	HealthScoreData,
 PeriodIncident,
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
let incidents = $state<PeriodIncident[]>([]);
let incidentCount = $state(0);
let periodFrom = $state<string | undefined>();
let periodTo = $state<string | undefined>();
let activeIncidents = $state<ActiveIncident[]>([]);
let topLatencyMonitors = $state<TopLatencyMonitor[]>([]);
let sslCheckedMonitorCount = $state(0);
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

// --- Internal loading guard ---
let loadInFlight: Promise<void> | null = null;
let requestedPeriod: FleetPeriod = '24h';
let revision = 0;

// --- Actions ---

/** Fetch a snapshot; keep previous data during background refreshes. */
function load(period: FleetPeriod = requestedPeriod): Promise<void> {
 if (loadInFlight && period === requestedPeriod) return loadInFlight;
 requestedPeriod = period;
 const requestRevision = ++revision;
 // Background refreshes keep the last snapshot visible. Period changes hide old values.
 if (!healthScore || healthScore.period !== period) setAllWidgetsLoading(true);
 const request = (async () => {
 try {
 const summary = await getDashboardSummary(period);
 if (requestRevision !== revision) return;
 clearAllErrors();
 healthScore = { ...summary.health_score, period };
 statusDistribution = summary.status_distribution;
 activeIncidents = summary.active_incidents;
 incidents = summary.incidents ?? [];
 incidentCount = summary.incident_count ?? incidents.length;
 periodFrom = summary.period_from;
 periodTo = summary.period_to;
 topLatencyMonitors = summary.top_latency_monitors;
 sslExpiry = summary.ssl_expiry;
 sslCheckedMonitorCount = summary.ssl_checked_monitor_count ?? sslExpiry.length;
 heatmap = summary.heatmap;
 recentEvents = summary.recent_events;
 for (const widget of ALL_WIDGETS) {
 if (summary.widget_errors?.[widget]) setWidgetError(widget, t('dashboard.loadError'));
 }
 lastUpdated = new Date(summary.generated_at);
 stale = summary.partial_data === true;
 monitorStateCache = new Map(monitorStore.list.filter(m => m.status === 'active').map(m => [m.id, m.state]));
 } catch (err: unknown) {
 if (requestRevision !== revision) return;
 stale = true;
 for (const widget of ALL_WIDGETS) setWidgetError(widget, t('dashboard.loadError'));
 } finally {
 if (requestRevision === revision) {
 setAllWidgetsLoading(false);
 loadInFlight = null;
 }
 }
 })();
 loadInFlight = request;
 return request;
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

	// Historical uptime is refreshed from the server; a live state is not an uptime sample.

	// --- 5. Update heatmap current hour block ---
	if (heatmap.length > 0) {
		const checkedAt = new Date(patch.checked_at);
		checkedAt.setMinutes(0, 0, 0);
		checkedAt.setMilliseconds(0);
		const checkedHourMs = checkedAt.getTime();

		const idx = heatmap.findIndex((h) => {
			const hourDate = new Date(h.hour_start);
			hourDate.setMinutes(0, 0, 0);
			hourDate.setMilliseconds(0);
			return hourDate.getTime() === checkedHourMs;
		});

		if (idx !== -1) {
			const updated = { ...heatmap[idx] };
			if (newState === 'up') updated.up_count += 1;
			else if (newState === 'down') updated.down_count += 1;
			else updated.unknown_count += 1;
			heatmap = [...heatmap.slice(0, idx), updated, ...heatmap.slice(idx + 1)];
		}
	}

	// --- 6. Update monitorStateCache and lastUpdated ---
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
 revision++;
 loadInFlight = null;
 requestedPeriod = '24h';
	healthScore = null;
	statusDistribution = null;
	activeIncidents = [];
 incidents = [];
 incidentCount = 0;
 periodFrom = undefined;
 periodTo = undefined;
	topLatencyMonitors = [];
	sslExpiry = [];
 sslCheckedMonitorCount = 0;
	heatmap = [];
	recentEvents = [];
	lastUpdated = null;
	stale = false;
	widgetErrors = new Map();
	widgetLoading = new Map();
	monitorStateCache = new Map();
}

// --- Per-widget accessor helpers (reactivity-safe) ---

/**
 * Returns true if the given widget is currently loading.
 * Using a function call ensures Svelte 5 tracks the $state read properly,
 * avoiding stale Map.get() results in template expressions.
 */
function isWidgetLoading(widgetId: WidgetId): boolean {
	return widgetLoading.get(widgetId) ?? false;
}

/**
 * Returns the error string for the given widget, or null if none.
 */
function getWidgetError(widgetId: WidgetId): string | null {
	return widgetErrors.get(widgetId) ?? null;
}

// --- Exported singleton store ---

export const dashboardStore = {
	// Reactive getters
 get incidents() { return incidents; },
 get incidentCount() { return incidentCount; },
 get periodFrom() { return periodFrom; },
 get periodTo() { return periodTo; },
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
	get sslCheckedMonitorCount() { return sslCheckedMonitorCount; },
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
	setWidgetError,

	// Per-widget accessors (reactivity-safe alternatives to Map.get() in templates)
	isWidgetLoading,
	getWidgetError
};
