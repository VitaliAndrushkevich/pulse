<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import { getMonitor, getMonitorHistory, getMonitorIncidents, deleteMonitor, ApiRequestError } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { formatDate } from '$lib/format';
  import HistoryChart from '../../../components/HistoryChart.svelte';
  import StatusTimeline from '../../../components/StatusTimeline.svelte';
  import type { Monitor, HistoryPoint, Incident } from '$lib/types';

  let history = $state<HistoryPoint[]>([]);
  let incidents = $state<Incident[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);
  let deleting = $state(false);

  let monitorId = $derived($page.params.id);

  // Derive the displayed monitor from the store.
  // When a WS patch arrives for this monitor_id, the store updates and
  // this $derived re-evaluates — giving us real-time updates without a full reload.
  let monitor = $derived<Monitor | null>(monitorStore.getById(monitorId) ?? null);

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    unknown: 'bg-slate-400'
  };

  const stateLabels: Record<string, string> = {
    up: 'Up',
    down: 'Down',
    unknown: 'Unknown'
  };

  const statusLabels: Record<string, string> = {
    active: 'Active',
    paused: 'Paused'
  };

  const typeBadgeColors: Record<string, string> = {
    http: 'bg-blue-100 text-blue-700',
    https: 'bg-green-100 text-green-700',
    tcp: 'bg-purple-100 text-purple-700',
    udp: 'bg-amber-100 text-amber-700',
    websocket: 'bg-pink-100 text-pink-700'
  };

  async function fetchData() {
    loading = true;
    error = null;
    notFound = false;

    try {
      const id = monitorId;

      // Compute 24h time window
      const to = new Date().toISOString();
      const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();

      // Fetch all data in parallel
      const [monitorData, historyData, incidentsData] = await Promise.all([
        getMonitor(id),
        getMonitorHistory(id, from, to),
        getMonitorIncidents(id, 1, 20)
      ]);

      // Put the fetched monitor into the store so it's reactive to WS patches.
      // The $derived `monitor` will pick this up immediately.
      monitorStore.updateMonitor(monitorData);
      history = historyData;
      incidents = incidentsData.data;
    } catch (err: unknown) {
      if (err instanceof ApiRequestError && err.statusCode === 404) {
        notFound = true;
      } else {
        error = err instanceof Error ? err.message : 'Failed to load monitor details. Please try again.';
      }
    } finally {
      loading = false;
    }
  }

  async function handleDelete() {
    if (!monitor) return;
    const confirmed = confirm(`Are you sure you want to delete "${monitor.name}"? This action cannot be undone.`);
    if (!confirmed) return;

    deleting = true;
    try {
      await deleteMonitor(monitor.id);
      goto('/monitors');
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to delete monitor. Please try again.';
      deleting = false;
    }
  }

  // Initial fetch on mount
  $effect(() => {
    // Track monitorId so this re-runs if the route param changes
    monitorId;
    untrack(() => fetchData());
  });
</script>

<!-- Loading state -->
{#if loading}
  <div class="flex items-center justify-center rounded-xl border border-slate-200 bg-white p-12" data-testid="loading-state">
    <div class="flex items-center gap-3 text-slate-500">
      <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
      <span>Loading monitor details...</span>
    </div>
  </div>

<!-- 404 Not Found -->
{:else if notFound}
  <div class="rounded-xl border border-slate-200 bg-white p-12 text-center" data-testid="not-found">
    <h2 class="text-lg font-semibold text-slate-900">Monitor not found</h2>
    <p class="mt-2 text-sm text-slate-500">The monitor you're looking for doesn't exist or has been deleted.</p>
    <a
      href="/monitors"
      class="mt-4 inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
    >
      Back to Monitors
    </a>
  </div>

<!-- Error state -->
{:else if error}
  <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="error-state">
    <p class="text-sm text-rose-700">{error}</p>
    <button
      type="button"
      onclick={() => fetchData()}
      class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
    >
      Retry
    </button>
  </div>

<!-- Monitor detail -->
{:else if monitor}
  <section class="space-y-6">
    <!-- Header with actions -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <a href="/monitors" class="text-slate-400 hover:text-slate-600 transition">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
          </svg>
        </a>
        <h1 class="text-2xl font-bold tracking-tight text-slate-900" data-testid="monitor-name">{monitor.name}</h1>
        <span
          class="rounded-full px-2.5 py-0.5 text-xs font-medium {typeBadgeColors[monitor.type] ?? 'bg-slate-100 text-slate-700'}"
          data-testid="monitor-type"
        >
          {monitor.type.toUpperCase()}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <a
          href="/monitors/{monitor.id}/edit"
          class="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
          data-testid="edit-button"
        >
          Edit
        </a>
        <button
          type="button"
          onclick={handleDelete}
          disabled={deleting}
          class="rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
          data-testid="delete-button"
        >
          {deleting ? 'Deleting...' : 'Delete'}
        </button>
      </div>
    </div>

    <!-- Status bar -->
    <div class="flex items-center gap-4 rounded-lg border border-slate-200 bg-white px-5 py-3">
      <span
        class="h-3 w-3 rounded-full {stateColors[monitor.state] ?? 'bg-slate-400'}"
        data-testid="state-indicator"
      ></span>
      <span class="text-sm font-medium text-slate-700" data-testid="monitor-state">
        {stateLabels[monitor.state] ?? 'Unknown'}
      </span>
      <span class="text-sm text-slate-400">·</span>
      <span class="text-sm text-slate-500" data-testid="monitor-status">
        {statusLabels[monitor.status] ?? monitor.status}
      </span>
    </div>

    <!-- Monitor fields grid -->
    <div class="rounded-xl border border-slate-200 bg-white" data-testid="monitor-details">
      <div class="grid grid-cols-1 divide-y divide-slate-100 sm:grid-cols-2 sm:divide-y-0 sm:divide-x">
        <div class="space-y-4 p-5">
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Target</dt>
            <dd class="mt-1 break-all text-sm text-slate-900" data-testid="monitor-target">{monitor.target}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Interval</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-interval">{monitor.interval_seconds}s</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Timeout</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-timeout">{monitor.timeout_seconds}s</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Last Checked</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-last-checked">{formatDate(monitor.last_checked_at)}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Next Check</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-next-check">{formatDate(monitor.next_check_at, 'Not scheduled')}</dd>
          </div>
        </div>
        <div class="space-y-4 p-5">
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Settings</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-settings">
              {#if Object.keys(monitor.settings).length === 0}
                <span class="text-slate-400">None</span>
              {:else}
                <pre class="rounded bg-slate-50 p-2 text-xs">{JSON.stringify(monitor.settings, null, 2)}</pre>
              {/if}
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Created</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-created">{formatDate(monitor.created_at)}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Updated</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-updated">{formatDate(monitor.updated_at)}</dd>
          </div>
        </div>
      </div>
    </div>

    <!-- Status timeline -->
    <div class="rounded-xl border border-slate-200 bg-white p-5" data-testid="status-timeline-section">
      <h2 class="mb-4 text-sm font-semibold text-slate-700">Status Timeline (24h)</h2>
      <StatusTimeline data={history} />
    </div>

    <!-- History chart -->
    <div class="rounded-xl border border-slate-200 bg-white p-5" data-testid="history-section">
      <h2 class="mb-4 text-sm font-semibold text-slate-700">Response Time (24h)</h2>
      <HistoryChart data={history} />
    </div>

    <!-- Incident timeline -->
    <div class="rounded-xl border border-slate-200 bg-white p-5" data-testid="incidents-section">
      <h2 class="mb-4 text-sm font-semibold text-slate-700">Recent Incidents</h2>
      {#if incidents.length === 0}
        <p class="text-sm text-slate-400">No incidents recorded.</p>
      {:else}
        <div class="space-y-3">
          {#each incidents as incident (incident.id)}
            <div class="flex items-start gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3" data-testid="incident-item">
              <div class="mt-0.5 h-2.5 w-2.5 flex-shrink-0 rounded-full {incident.resolved_at ? 'bg-emerald-500' : 'bg-rose-500'}"></div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2 text-sm">
                  <span class="font-medium text-slate-700" data-testid="incident-started">
                    {formatDate(incident.started_at)}
                  </span>
                  <span class="text-slate-400">→</span>
                  <span class="text-slate-600" data-testid="incident-resolved">
                    {#if incident.resolved_at}
                      {formatDate(incident.resolved_at)}
                    {:else}
                      <span class="rounded bg-rose-100 px-1.5 py-0.5 text-xs font-medium text-rose-700">Ongoing</span>
                    {/if}
                  </span>
                </div>
                {#if incident.cause}
                  <p class="mt-1 text-xs text-slate-500">{incident.cause}</p>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>
{/if}
