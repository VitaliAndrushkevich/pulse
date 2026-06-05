<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import { getMonitor, getMonitorHistory, getMonitorIncidents, getMonitorStats, deleteMonitor, ApiRequestError } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { patchBus } from '$lib/stores/patchBus.svelte';
  import { formatDate } from '$lib/format';
  import HistoryChart from '../../../components/HistoryChart.svelte';
  import StatusTimeline from '../../../components/StatusTimeline.svelte';
  import type { Monitor, HistoryPoint, Incident, MonitorPatch, MonitorStats } from '$lib/types';

  let history = $state<HistoryPoint[]>([]);
  let incidents = $state<Incident[]>([]);
  let stats = $state<MonitorStats | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);
  let deleting = $state(false);

  let monitorId = $derived($page.params.id);

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

      const to = new Date().toISOString();
      const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();

      const [monitorData, historyData, incidentsData, statsData] = await Promise.all([
        getMonitor(id),
        getMonitorHistory(id, from, to),
        getMonitorIncidents(id, 1, 20),
        getMonitorStats(id).catch(() => null)
      ]);

      monitorStore.updateMonitor(monitorData);
      history = historyData;
      incidents = incidentsData.data;
      stats = statsData;
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

  function formatUptime(percent: number): string {
    return percent.toFixed(2) + '%';
  }

  function uptimeColor(percent: number): string {
    if (percent >= 99) return 'text-emerald-600';
    if (percent >= 95) return 'text-amber-600';
    return 'text-rose-600';
  }

  function sslColor(days: number): string {
    if (days > 30) return 'text-emerald-600';
    if (days > 14) return 'text-amber-600';
    return 'text-rose-600';
  }

  function sslBorderColor(days: number): string {
    if (days > 30) return 'border-emerald-200';
    if (days > 14) return 'border-amber-200';
    return 'border-rose-200';
  }

  function sslBgColor(days: number): string {
    if (days > 30) return 'bg-emerald-50';
    if (days > 14) return 'bg-amber-50';
    return 'bg-rose-50';
  }

  // Initial fetch on mount
  $effect(() => {
    monitorId;
    untrack(() => fetchData());
  });

  // Subscribe to WS monitor_status patches for the current monitor.
  $effect(() => {
    const currentId = monitorId;
    const unsubscribe = patchBus.subscribe((patch: MonitorPatch) => {
      if (patch.monitor_id !== currentId) return;

      const state: 'up' | 'down' = patch.state === 'up' ? 'up' : 'down';

      const newPoint: HistoryPoint = {
        state,
        latency_ms: patch.latency_ms,
        status_code: patch.status_code ?? null,
        error: patch.error ?? null,
        ssl_days_remaining: patch.ssl_days_remaining ?? null,
        checked_at: patch.checked_at
      };

      const currentHistory = untrack(() => history);
      const updated = [...currentHistory, newPoint];

      const cutoff = Date.now() - 24 * 60 * 60 * 1000;
      history = updated.filter(p => new Date(p.checked_at).getTime() >= cutoff);

      // Update SSL info in stats from patch if available
      if (patch.ssl_days_remaining != null && stats) {
        const now = new Date();
        const expiresAt = new Date(now.getTime() + patch.ssl_days_remaining * 24 * 60 * 60 * 1000);
        stats = {
          ...stats,
          ssl: {
            days_remaining: patch.ssl_days_remaining,
            expires_at: expiresAt.toISOString().split('T')[0]
          }
        };
      }

      // Update last error from patch
      if (patch.state !== 'up' && patch.error && stats) {
        stats = {
          ...stats,
          last_error: {
            error: patch.error,
            checked_at: patch.checked_at
          }
        };
      } else if (patch.state === 'up' && stats?.last_error) {
        stats = { ...stats, last_error: undefined };
      }
    });

    return () => {
      unsubscribe();
    };
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
        <a href="/monitors" class="text-slate-400 hover:text-slate-600 transition" aria-label="Back to monitors">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
          </svg>
        </a>
        <h1 class="text-2xl font-bold tracking-tight text-slate-900" data-testid="monitor-name">{monitor.name}</h1>
        <span
          class="rounded-full px-2.5 py-0.5 text-xs font-medium {typeBadgeColors[monitor.type] ?? 'bg-slate-100 text-slate-700'}"
          data-testid="monitor-type"
        >
          {monitor.type === 'http' ? 'HTTP(S)' : monitor.type.toUpperCase()}
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

    <!-- Status bar with current state + error highlight -->
    <div class="rounded-lg border border-slate-200 bg-white" data-testid="status-bar">
      <div class="flex items-center gap-4 px-5 py-3">
        <span
          class="h-3 w-3 rounded-full {stateColors[monitor.state] ?? 'bg-slate-400'}"
          data-testid="state-indicator"
        ></span>
        <span class="text-sm font-semibold {monitor.state === 'up' ? 'text-emerald-700' : monitor.state === 'down' ? 'text-rose-700' : 'text-slate-700'}" data-testid="monitor-state">
          {stateLabels[monitor.state] ?? 'Unknown'}
        </span>
        <span class="text-sm text-slate-400">·</span>
        <span class="text-sm text-slate-500" data-testid="monitor-status">
          {statusLabels[monitor.status] ?? monitor.status}
        </span>
        <span class="text-sm text-slate-400">·</span>
        <span class="text-xs text-slate-400">
          Checking every {monitor.interval_seconds}s
        </span>
      </div>

      <!-- Error banner when monitor is down -->
      {#if monitor.state === 'down' && stats?.last_error}
        <div class="border-t border-rose-200 bg-rose-50 px-5 py-3" data-testid="error-banner">
          <div class="flex items-start gap-3">
            <div class="mt-0.5 flex-shrink-0">
              <svg class="h-4 w-4 text-rose-500" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-sm font-medium text-rose-800">Down</p>
              <p class="mt-0.5 text-sm text-rose-700" data-testid="error-message">{stats.last_error.error}</p>
              <p class="mt-1 text-xs text-rose-500">{formatDate(stats.last_error.checked_at)}</p>
            </div>
          </div>
        </div>
      {/if}
    </div>

    <!-- Stats cards (Kuma-style) -->
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 {monitor.type === 'http' ? 'lg:grid-cols-5' : 'lg:grid-cols-4'}" data-testid="stats-cards">
      <!-- Response time (current) -->
      <div class="rounded-lg border border-slate-200 bg-white p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Response</dt>
        <dd class="mt-1 text-lg font-semibold text-slate-900" data-testid="stat-response">
          {#if history.length > 0 && history[history.length - 1].latency_ms != null}
            {history[history.length - 1].latency_ms}ms
          {:else}
            N/A
          {/if}
        </dd>
        <span class="text-[10px] text-slate-400">(Current)</span>
      </div>

      <!-- Avg response time 24h -->
      <div class="rounded-lg border border-slate-200 bg-white p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Avg. Response</dt>
        <dd class="mt-1 text-lg font-semibold text-slate-900" data-testid="stat-avg-response">
          {#if stats && stats.uptime_24h.avg_latency_ms > 0}
            {stats.uptime_24h.avg_latency_ms}ms
          {:else}
            N/A
          {/if}
        </dd>
        <span class="text-[10px] text-slate-400">(24 hours)</span>
      </div>

      <!-- Uptime 24h -->
      <div class="rounded-lg border border-slate-200 bg-white p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Uptime</dt>
        <dd class="mt-1 text-lg font-semibold {stats ? uptimeColor(stats.uptime_24h.uptime_percent) : 'text-slate-900'}" data-testid="stat-uptime-24h">
          {#if stats && stats.uptime_24h.total_checks > 0}
            {formatUptime(stats.uptime_24h.uptime_percent)}
          {:else}
            N/A
          {/if}
        </dd>
        <span class="text-[10px] text-slate-400">(24 hours)</span>
      </div>

      <!-- Uptime 30d -->
      <div class="rounded-lg border border-slate-200 bg-white p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Uptime</dt>
        <dd class="mt-1 text-lg font-semibold {stats ? uptimeColor(stats.uptime_30d.uptime_percent) : 'text-slate-900'}" data-testid="stat-uptime-30d">
          {#if stats && stats.uptime_30d.total_checks > 0}
            {formatUptime(stats.uptime_30d.uptime_percent)}
          {:else}
            N/A
          {/if}
        </dd>
        <span class="text-[10px] text-slate-400">(30 days)</span>
      </div>

      <!-- SSL Certificate (only for HTTP monitors) -->
      {#if monitor.type === 'http'}
        {#if stats?.ssl}
          <div class="rounded-lg border {sslBorderColor(stats.ssl.days_remaining)} {sslBgColor(stats.ssl.days_remaining)} p-4 text-center" data-testid="stat-ssl">
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Cert. Expiry</dt>
            <dd class="mt-1 text-lg font-semibold {sslColor(stats.ssl.days_remaining)}">
              {stats.ssl.days_remaining} days
            </dd>
            <span class="text-[10px] text-slate-500">({stats.ssl.expires_at})</span>
          </div>
        {:else}
          <div class="rounded-lg border border-slate-200 bg-white p-4 text-center" data-testid="stat-ssl">
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Cert. Expiry</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-400">N/A</dd>
            <span class="text-[10px] text-slate-400">(no data yet)</span>
          </div>
        {/if}
      {/if}
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

    <!-- Monitor details grid -->
    <div class="rounded-xl border border-slate-200 bg-white" data-testid="monitor-details">
      <div class="border-b border-slate-100 px-5 py-3">
        <h2 class="text-sm font-semibold text-slate-700">Configuration</h2>
      </div>
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
        </div>
        <div class="space-y-4 p-5">
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Last Checked</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-last-checked">{formatDate(monitor.last_checked_at)}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Next Check</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-next-check">{formatDate(monitor.next_check_at, 'Not scheduled')}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">Created</dt>
            <dd class="mt-1 text-sm text-slate-900" data-testid="monitor-created">{formatDate(monitor.created_at)}</dd>
          </div>
        </div>
      </div>
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
