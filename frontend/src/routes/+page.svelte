<script lang="ts">
  /**
   * Dashboard Page — operational health overview.
   *
   * Composes all dashboard widgets in a responsive grid layout.
   * Subscribes to patchBus for real-time updates and manages staleness.
   *
   * Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 9.1, 9.2
   */
  import { onMount, untrack } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte';
  import { patchBus } from '$lib/stores/patchBus.svelte';
  import { connectionStore } from '$lib/stores/connection.svelte';
  import { t } from '$lib/i18n';
  import type { MonitorPatch } from '$lib/types';

  import PageHeader from '../components/PageHeader.svelte';
  import FleetHealth from '../components/dashboard/FleetHealth.svelte';
  import IncidentsPanel from '../components/dashboard/IncidentsPanel.svelte';
  import ResponseTimes from '../components/dashboard/ResponseSparklines.svelte';
  import SSLWarnings from '../components/dashboard/SSLWarnings.svelte';
  import UptimeHeatmap from '../components/dashboard/UptimeHeatmap.svelte';
  import DataFreshness from '../components/dashboard/DataFreshness.svelte';

  import SelectField from '../components/SelectField.svelte';
  import type { FleetPeriod } from '$lib/types';
  let period = $state<FleetPeriod>('24h');
  let mounted = false;
  let refreshTimer: ReturnType<typeof setTimeout> | undefined;
  let lastRefresh = 0;

  function refresh(): void {
    lastRefresh = Date.now();
    void dashboardStore.load(period);
  }

  function handlePatch(_patch: MonitorPatch): void {
    // Coalesce high-volume WS traffic into authoritative snapshots. This also refreshes
    // history, latency, SSL and incidents without inventing aggregate values locally.
    if (!refreshTimer) {
      refreshTimer = setTimeout(() => {
        refreshTimer = undefined;
        refresh();
      }, Math.max(0, 15_000 - (Date.now() - lastRefresh)));
    }
  }

  $effect(() => {
    if (connectionStore.status === 'connected' && mounted) untrack(refresh);
  });

  onMount(() => {
    mounted = true;
    try {
      const saved = localStorage.getItem('pulse-fleet-period');
      if (saved === '1h' || saved === '24h' || saved === '7d' || saved === '30d') period = saved;
    } catch { /* Storage can be unavailable in private browsing. */ }
    refresh();
    const unsubscribe = patchBus.subscribe(handlePatch);
    // Poll even without WS traffic: retention, config changes and rolling windows change.
    const interval = setInterval(refresh, 30_000);
    return () => {
      mounted = false;
      unsubscribe();
      clearInterval(interval);
      clearTimeout(refreshTimer);
    };
  });

  function changePeriod(): void {
    try { localStorage.setItem('pulse-fleet-period', period); } catch { /* Optional preference. */ }
    refresh();
  }

  // --- Per-widget retry handlers ---
  function retryWidget(): () => void { return refresh; }
</script>

<section class="dashboard space-y-6">
  <PageHeader eyebrow={t('shell.overview')} title={t('dashboard.title')}>
    {#snippet actions()}
      <div class="overview-controls">
        <div class="period-control">
          <label for="fleet-period" class="pulse-eyebrow">{t('dashboard.period.label')}</label>
          <div class="period-select">
            <SelectField id="fleet-period" bind:value={period} onchange={changePeriod}>
              <option value="1h">{t('dashboard.fleetHealth.hour')}</option>
              <option value="24h">{t('dashboard.fleetHealth.day')}</option>
              <option value="7d">{t('dashboard.fleetHealth.week')}</option>
              <option value="30d">{t('dashboard.fleetHealth.month')}</option>
            </SelectField>
          </div>
        </div>
        <span class="period-scope">{t('dashboard.period.scope')}</span>
        <DataFreshness lastUpdated={dashboardStore.lastUpdated} stale={dashboardStore.stale} />
      </div>
    {/snippet}
  </PageHeader>

  <!-- Responsive grid: 3 columns at >= 768px, single-column below (Req 8.1, 8.7) -->
  <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
    <!-- UptimeHeatmap — full width at top for at-a-glance history -->
    <div class="pulse-panel md:col-span-3">
      <UptimeHeatmap
        data={dashboardStore.heatmap}
        loading={dashboardStore.isWidgetLoading('heatmap')}
        error={dashboardStore.getWidgetError('heatmap')}
        onRetry={retryWidget()}
      />
    </div>

    <!-- FleetHealth — historical uptime and separate current status -->
    <div class="pulse-panel fleet-panel">
      <FleetHealth
        healthData={dashboardStore.healthScore}
        distributionData={dashboardStore.statusDistribution}
        loading={dashboardStore.isWidgetLoading('health-score')}
        error={dashboardStore.getWidgetError('health-score')}
        onRetry={retryWidget()}
      />
    </div>

    <!-- IncidentsPanel -->
    <div class="pulse-panel md:col-span-2">
      <IncidentsPanel
        incidents={dashboardStore.incidents}
        totalCount={dashboardStore.incidentCount}
        periodFrom={dashboardStore.periodFrom}
        periodTo={dashboardStore.periodTo}
        historical
        loading={dashboardStore.isWidgetLoading('incidents')}
        error={dashboardStore.getWidgetError('incidents')}
        onRetry={retryWidget()}
      />
    </div>

    <div class="pulse-panel md:col-span-2">
      <ResponseTimes
        monitors={dashboardStore.topLatencyMonitors}
        period={period}
        loading={dashboardStore.isWidgetLoading('sparklines')}
        error={dashboardStore.getWidgetError('sparklines')}
        onRetry={retryWidget()}
      />
    </div>
    <div class="pulse-panel self-start">
      <SSLWarnings
        entries={dashboardStore.sslExpiry}
        checkedMonitorCount={dashboardStore.sslCheckedMonitorCount}
        loading={dashboardStore.isWidgetLoading('ssl-expiry')}
        error={dashboardStore.getWidgetError('ssl-expiry')}
        onRetry={retryWidget()}
      />
    </div>
  </div>
</section>

<style>
  .overview-controls { display: flex; flex-direction: column; align-items: flex-end; gap: 0.5rem; }
  .period-control { display: flex; align-items: center; flex-wrap: wrap; gap: 0.75rem; }
  .period-select { inline-size: 12rem; max-inline-size: 100%; }
  .period-scope { font-size: 0.75rem; color: var(--color-text-secondary); }
  @media (max-width: 639px) {
    .overview-controls { align-items: flex-start; max-inline-size: 100%; }
    .period-control { gap: 0.5rem; }
  }
  .fleet-panel { border-block-start: 3px solid var(--color-brand-primary); }
</style>
