<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import { getMonitor, getMonitorHistory, getMonitorIncidents, getMonitorStats, deleteMonitor, updateMonitor, ApiRequestError } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { patchBus } from '$lib/stores/patchBus.svelte';
  import { formatDate, formatLatency } from '$lib/format';
  import HistoryChart from '../../../components/HistoryChart.svelte';
  import StatusTimeline from '../../../components/StatusTimeline.svelte';
  import HistoryExplorer from '../../../components/HistoryExplorer.svelte';
  import MonitorNotificationBindings from '../../../components/MonitorNotificationBindings.svelte';
  import MonitorDeliveryLogs from '../../../components/MonitorDeliveryLogs.svelte';

  import type { Monitor, HistoryPoint, Incident, MonitorPatch, MonitorStats } from '$lib/types';
  import { t } from '$lib/i18n';

  type Tab = 'overview' | 'history' | 'notifications';
  let activeTab = $state<Tab>('overview');

  let history = $state<HistoryPoint[]>([]);
  let incidents = $state<Incident[]>([]);
  let stats = $state<MonitorStats | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);
  let deleting = $state(false);
  let toggling = $state(false);
  let showPauseConfirm = $state(false);

  let monitorId = $derived($page.params.id);

  let monitor = $derived<Monitor | null>(monitorStore.getById(monitorId) ?? null);

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    unknown: 'bg-slate-400'
  };

  const stateLabels: Record<string, string> = {
    up: t('monitors.status.up'),
    down: t('monitors.status.down'),
    unknown: t('monitors.status.unknown')
  };

  const statusLabels: Record<string, string> = {
    active: t('monitors.status.active'),
    paused: t('monitors.status.paused')
  };

  const typeBadgeColors: Record<string, string> = {
    http: 'bg-blue-100 text-blue-700',
    http3: 'bg-cyan-100 text-cyan-700',
    tcp: 'bg-purple-100 text-purple-700',
    udp: 'bg-amber-100 text-amber-700',
    websocket: 'bg-pink-100 text-pink-700',
    grpc: 'bg-indigo-100 text-indigo-700',
    dns: 'bg-teal-100 text-teal-700',
    icmp: 'bg-orange-100 text-orange-700',
    smtp: 'bg-rose-100 text-rose-700'
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
        error = err instanceof Error ? err.message : t('monitors.errors.detailFailed');
      }
    } finally {
      loading = false;
    }
  }

  async function handleDelete() {
    if (!monitor) return;
    const confirmed = confirm(t('monitors.deleteConfirm', { name: monitor.name }));
    if (!confirmed) return;

    deleting = true;
    try {
      await deleteMonitor(monitor.id);
      goto('/monitors');
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('monitors.errors.deleteFailed');
      deleting = false;
    }
  }

  async function handleTogglePause() {
    if (!monitor) return;
    const newStatus = monitor.status === 'active' ? 'paused' : 'active';

    toggling = true;
    try {
      const updated = await updateMonitor(monitor.id, {
        name: monitor.name,
        type: monitor.type,
        target: monitor.target,
        interval_seconds: monitor.interval_seconds,
        timeout_seconds: monitor.timeout_seconds,
        status: newStatus,
        settings: monitor.settings,
        tags: monitor.tags,
        history_retention_days: monitor.history_retention_days,
      });
      monitorStore.updateMonitor(updated);
      showPauseConfirm = false;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : `Failed to ${newStatus === 'paused' ? 'pause' : 'resume'} monitor.`;
    } finally {
      toggling = false;
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
    if (days > 30) return 'border-emerald-500/30';
    if (days > 14) return 'border-amber-500/30';
    return 'border-rose-500/30';
  }

  function sslBgColor(days: number): string {
    if (days > 30) return 'bg-emerald-500/10';
    if (days > 14) return 'bg-amber-500/10';
    return 'bg-rose-500/10';
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
  <div class="flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface p-12" data-testid="loading-state">
    <div class="flex items-center gap-3 text-secondary">
      <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
      <span>{t('monitors.loadingDetails')}</span>
    </div>
  </div>

<!-- 404 Not Found -->
{:else if notFound}
  <div class="rounded-xl border border-[var(--color-border)] bg-surface p-12 text-center" data-testid="not-found">
    <h2 class="text-lg font-semibold text-primary">{t('monitors.notFound.title')}</h2>
    <p class="mt-2 text-sm text-secondary">{t('monitors.notFound.description')}</p>
    <a
      href="/monitors"
      class="mt-4 inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
    >
      {t('monitors.notFound.action')}
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
      {t('common.retry')}
    </button>
  </div>

<!-- Monitor detail -->
{:else if monitor}
  <section class="space-y-6">
    <!-- Header with actions -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <a href="/monitors" class="text-[var(--color-text-muted)] hover:text-secondary transition" aria-label={t('monitors.detail.backToMonitors')}>
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"></path>
          </svg>
        </a>
        <h1 class="text-2xl font-bold tracking-tight text-primary" data-testid="monitor-name">{monitor.name}</h1>
        <span
          class="rounded-full px-2.5 py-0.5 text-xs font-medium {typeBadgeColors[monitor.type] ?? 'bg-[var(--color-bg-surface-hover)] text-primary'}"
          data-testid="monitor-type"
        >
          {monitor.type === 'http' ? 'HTTP(S)' : monitor.type.toUpperCase()}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          onclick={() => { showPauseConfirm = true; }}
          class="inline-flex items-center gap-1.5 rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
          data-testid="pause-button"
        >
          {#if monitor.status === 'active'}
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
            </svg>
            {t('monitors.detail.pause.button')}
          {:else}
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"></path>
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
            </svg>
            {t('monitors.detail.resume.button')}
          {/if}
        </button>
        <a
          href="/monitors/{monitor.id}/edit"
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
          data-testid="edit-button"
        >
          {t('monitors.detail.edit')}
        </a>
        <button
          type="button"
          onclick={handleDelete}
          disabled={deleting}
          class="rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
          data-testid="delete-button"
        >
          {deleting ? t('common.deleting') : t('monitors.detail.delete')}
        </button>
      </div>
    </div>

    <!-- Tab bar -->
    <div class="border-b border-[var(--color-border)]" data-testid="tab-bar">
      <nav class="-mb-px flex gap-6" aria-label="Monitor tabs">
        <button
          type="button"
          onclick={() => activeTab = 'overview'}
          class="whitespace-nowrap border-b-2 px-1 py-3 text-sm font-medium transition-colors {activeTab === 'overview' ? 'border-[var(--color-brand-primary)] text-[var(--color-brand-primary)]' : 'border-transparent text-secondary hover:border-[var(--color-border)] hover:text-primary'}"
          aria-selected={activeTab === 'overview'}
          role="tab"
          data-testid="tab-overview"
        >
          {t('monitors.detail.tabs.overview')}
        </button>
        <button
          type="button"
          onclick={() => activeTab = 'history'}
          class="whitespace-nowrap border-b-2 px-1 py-3 text-sm font-medium transition-colors {activeTab === 'history' ? 'border-[var(--color-brand-primary)] text-[var(--color-brand-primary)]' : 'border-transparent text-secondary hover:border-[var(--color-border)] hover:text-primary'}"
          aria-selected={activeTab === 'history'}
          role="tab"
          data-testid="tab-history"
        >
          {t('monitors.detail.tabs.history')}
        </button>
        <button
          type="button"
          onclick={() => activeTab = 'notifications'}
          class="whitespace-nowrap border-b-2 px-1 py-3 text-sm font-medium transition-colors {activeTab === 'notifications' ? 'border-[var(--color-brand-primary)] text-[var(--color-brand-primary)]' : 'border-transparent text-secondary hover:border-[var(--color-border)] hover:text-primary'}"
          aria-selected={activeTab === 'notifications'}
          role="tab"
          data-testid="tab-notifications"
        >
          {t('monitors.detail.tabs.notifications')}
        </button>
      </nav>
    </div>

    <!-- Tab content: Overview -->
    <div class="space-y-6" hidden={activeTab !== 'overview'} role="tabpanel" data-testid="tab-panel-overview">

    <!-- Status bar with current state + error highlight -->
    <div class="rounded-lg border border-[var(--color-border)] bg-surface" data-testid="status-bar">
      <div class="flex items-center gap-4 px-5 py-3">
        <span
          class="h-3 w-3 rounded-full {stateColors[monitor.state] ?? 'bg-slate-400'}"
          data-testid="state-indicator"
        ></span>
        <span class="text-sm font-semibold {monitor.state === 'up' ? 'text-emerald-700' : monitor.state === 'down' ? 'text-rose-700' : 'text-primary'}" data-testid="monitor-state">
          {stateLabels[monitor.state] ?? t('monitors.status.unknown')}
        </span>
        <span class="text-sm text-[var(--color-text-muted)]">·</span>
        <span class="text-sm text-secondary" data-testid="monitor-status">
          {statusLabels[monitor.status] ?? monitor.status}
        </span>
        <span class="text-sm text-[var(--color-text-muted)]">·</span>
        <span class="text-xs text-[var(--color-text-muted)]">
          {t('monitors.detail.checkInterval', { seconds: monitor.interval_seconds })}
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
              <p class="text-sm font-medium text-rose-800">{t('monitors.status.down')}</p>
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
      <div class="rounded-lg border border-[var(--color-border)] bg-surface p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.response')}</dt>
        <dd class="mt-1 text-lg font-semibold text-primary" data-testid="stat-response">
          {#if history.length > 0 && history[history.length - 1].latency_ms != null}
            {formatLatency(history[history.length - 1].latency_ms!)}
          {:else}
            {t('common.na')}
          {/if}
        </dd>
        <span class="text-[10px] text-[var(--color-text-muted)]">{t('monitors.detail.stats.current')}</span>
      </div>

      <!-- Avg response time 24h -->
      <div class="rounded-lg border border-[var(--color-border)] bg-surface p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.avgResponse')}</dt>
        <dd class="mt-1 text-lg font-semibold text-primary" data-testid="stat-avg-response">
          {#if stats && stats.uptime_24h.avg_latency_ms > 0}
            {formatLatency(stats.uptime_24h.avg_latency_ms)}
          {:else}
            {t('common.na')}
          {/if}
        </dd>
        <span class="text-[10px] text-[var(--color-text-muted)]">{t('monitors.detail.stats.period24h')}</span>
      </div>

      <!-- Uptime 24h -->
      <div class="rounded-lg border border-[var(--color-border)] bg-surface p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.uptime')}</dt>
        <dd class="mt-1 text-lg font-semibold {stats ? uptimeColor(stats.uptime_24h.uptime_percent) : 'text-primary'}" data-testid="stat-uptime-24h">
          {#if stats && stats.uptime_24h.total_checks > 0}
            {formatUptime(stats.uptime_24h.uptime_percent)}
          {:else}
            {t('common.na')}
          {/if}
        </dd>
        <span class="text-[10px] text-[var(--color-text-muted)]">{t('monitors.detail.stats.period24h')}</span>
      </div>

      <!-- Uptime 30d -->
      <div class="rounded-lg border border-[var(--color-border)] bg-surface p-4 text-center">
        <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.uptime')}</dt>
        <dd class="mt-1 text-lg font-semibold {stats ? uptimeColor(stats.uptime_30d.uptime_percent) : 'text-primary'}" data-testid="stat-uptime-30d">
          {#if stats && stats.uptime_30d.total_checks > 0}
            {formatUptime(stats.uptime_30d.uptime_percent)}
          {:else}
            {t('common.na')}
          {/if}
        </dd>
        <span class="text-[10px] text-[var(--color-text-muted)]">{t('monitors.detail.stats.period30d')}</span>
      </div>

      <!-- SSL Certificate (only for HTTP monitors) -->
      {#if monitor.type === 'http'}
        {#if stats?.ssl}
          <div class="rounded-lg border {sslBorderColor(stats.ssl.days_remaining)} {sslBgColor(stats.ssl.days_remaining)} p-4 text-center" data-testid="stat-ssl">
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.certExpiry')}</dt>
            <dd class="mt-1 text-lg font-semibold {sslColor(stats.ssl.days_remaining)}">
              {t('monitors.detail.stats.days', { count: stats.ssl.days_remaining })}
            </dd>
            <span class="text-[10px] text-secondary">({stats.ssl.expires_at})</span>
          </div>
        {:else}
          <div class="rounded-lg border border-[var(--color-border)] bg-surface p-4 text-center" data-testid="stat-ssl">
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.stats.certExpiry')}</dt>
            <dd class="mt-1 text-lg font-semibold text-[var(--color-text-muted)]">{t('common.na')}</dd>
            <span class="text-[10px] text-[var(--color-text-muted)]">{t('monitors.detail.stats.noData')}</span>
          </div>
        {/if}
      {/if}
    </div>

    <!-- Status timeline -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-5" data-testid="status-timeline-section">
      <h2 class="mb-4 text-sm font-semibold text-primary">{t('monitors.detail.timeline.title')}</h2>
      <StatusTimeline data={history} />
    </div>

    <!-- History chart -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-5" data-testid="history-section">
      <h2 class="mb-4 text-sm font-semibold text-primary">{t('monitors.detail.responseTime.title')}</h2>
      <HistoryChart data={history} />
    </div>

    <!-- Monitor details grid -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface" data-testid="monitor-details">
      <div class="border-b border-[var(--color-border)] px-5 py-3">
        <h2 class="text-sm font-semibold text-primary">{t('monitors.detail.configuration.title')}</h2>
      </div>
      <div class="grid grid-cols-1 divide-y divide-slate-100 sm:grid-cols-2 sm:divide-y-0 sm:divide-x">
        <div class="space-y-4 p-5">
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.target')}</dt>
            <dd class="mt-1 break-all text-sm text-primary" data-testid="monitor-target">{monitor.target}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.interval')}</dt>
            <dd class="mt-1 text-sm text-primary" data-testid="monitor-interval">{monitor.interval_seconds}s</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.timeout')}</dt>
            <dd class="mt-1 text-sm text-primary" data-testid="monitor-timeout">{monitor.timeout_seconds}s</dd>
          </div>
        </div>
        <div class="space-y-4 p-5">
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.lastChecked')}</dt>
            <dd class="mt-1 text-sm text-primary" data-testid="monitor-last-checked">{formatDate(monitor.last_checked_at)}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.nextCheck')}</dt>
            <dd class="mt-1 text-sm text-primary" data-testid="monitor-next-check">{formatDate(monitor.next_check_at, t('monitors.detail.configuration.notScheduled'))}</dd>
          </div>
          <div>
            <dt class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)]">{t('monitors.detail.configuration.created')}</dt>
            <dd class="mt-1 text-sm text-primary" data-testid="monitor-created">{formatDate(monitor.created_at)}</dd>
          </div>
        </div>
      </div>
    </div>

    <!-- Incident timeline -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-5" data-testid="incidents-section">
      <h2 class="mb-4 text-sm font-semibold text-primary">{t('monitors.detail.incidents.title')}</h2>
      {#if incidents.length === 0}
        <p class="text-sm text-[var(--color-text-muted)]">{t('monitors.detail.incidents.empty')}</p>
      {:else}
        <div class="space-y-3">
          {#each incidents as incident (incident.id)}
            <div class="flex items-start gap-3 rounded-lg border border-[var(--color-border)] bg-page p-3" data-testid="incident-item">
              <div class="mt-0.5 h-2.5 w-2.5 flex-shrink-0 rounded-full {incident.resolved_at ? 'bg-emerald-500' : 'bg-rose-500'}"></div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2 text-sm">
                  <span class="font-medium text-primary" data-testid="incident-started">
                    {formatDate(incident.started_at)}
                  </span>
                  <span class="text-[var(--color-text-muted)]">→</span>
                  <span class="text-secondary" data-testid="incident-resolved">
                    {#if incident.resolved_at}
                      {formatDate(incident.resolved_at)}
                    {:else}
                      <span class="rounded bg-rose-100 px-1.5 py-0.5 text-xs font-medium text-rose-700">{t('monitors.detail.incidents.ongoing')}</span>
                    {/if}
                  </span>
                </div>
                {#if incident.cause}
                  <p class="mt-1 text-xs text-secondary">{incident.cause}</p>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    </div><!-- /tab-panel-overview -->

    <!-- Tab content: History -->
    <div hidden={activeTab !== 'history'} role="tabpanel" data-testid="tab-panel-history">
      <HistoryExplorer monitorId={monitor.id} retentionDays={monitor.history_retention_days ?? 30} />
    </div>

    <!-- Tab content: Notifications -->
    <div hidden={activeTab !== 'notifications'} role="tabpanel" data-testid="tab-panel-notifications" class="space-y-6">
      <MonitorNotificationBindings monitorId={monitor.id} />
      <div>
        <h3 class="text-base font-semibold text-primary mb-3">{t('notifications.deliveryLogs.title')}</h3>
        <MonitorDeliveryLogs monitorId={monitor.id} />
      </div>
    </div>

  </section>

  <!-- Pause/Resume confirmation modal -->
  {#if showPauseConfirm}
    <div
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="pause-confirm-title"
      data-testid="pause-confirm-modal"
    >
      <div class="mx-4 w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
        <h3 id="pause-confirm-title" class="text-lg font-semibold text-primary">
          {monitor.status === 'active' ? t('monitors.detail.pause.title') : t('monitors.detail.resume.title')}
        </h3>
        <p class="mt-2 text-sm text-secondary">
          {#if monitor.status === 'active'}
            {t('monitors.detail.pause.description', { name: monitor.name })}
          {:else}
            {t('monitors.detail.resume.description', { name: monitor.name })}
          {/if}
        </p>
        <div class="mt-5 flex items-center justify-end gap-3">
          <button
            type="button"
            onclick={() => { showPauseConfirm = false; }}
            disabled={toggling}
            class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50"
            data-testid="pause-cancel-button"
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onclick={handleTogglePause}
            disabled={toggling}
            class="rounded-md {monitor.status === 'active' ? 'bg-amber-600 hover:bg-amber-700 focus:ring-amber-500' : 'bg-emerald-600 hover:bg-emerald-700 focus:ring-emerald-500'} px-4 py-2 text-sm font-medium text-white transition focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
            data-testid="pause-confirm-button"
          >
            {#if toggling}
              {monitor.status === 'active' ? t('monitors.detail.pause.pausing') : t('monitors.detail.resume.resuming')}
            {:else}
              {monitor.status === 'active' ? t('monitors.detail.pause.button') : t('monitors.detail.resume.button')}
            {/if}
          </button>
        </div>
      </div>
    </div>
  {/if}
{/if}
