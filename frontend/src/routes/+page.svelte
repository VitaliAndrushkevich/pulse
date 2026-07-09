<script lang="ts">
  /**
   * Dashboard Page — operational health overview.
   *
   * Composes all dashboard widgets in a responsive grid layout.
   * Subscribes to patchBus for real-time updates and manages staleness.
   *
   * Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 9.1, 9.2
   */
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte';
  import { patchBus } from '$lib/stores/patchBus.svelte';
  import { connectionStore } from '$lib/stores/connection.svelte';
  import { t } from '$lib/i18n';
  import type { MonitorPatch } from '$lib/types';

  import FleetHealth from '../components/dashboard/FleetHealth.svelte';
  import IncidentsPanel from '../components/dashboard/IncidentsPanel.svelte';
  import InsightsPanel from '../components/dashboard/InsightsPanel.svelte';
  import UptimeHeatmap from '../components/dashboard/UptimeHeatmap.svelte';
  import DataFreshness from '../components/dashboard/DataFreshness.svelte';

  // --- Staleness timer state ---
  let stalenessTimer: ReturnType<typeof setTimeout> | null = null;
  const STALENESS_TIMEOUT_MS = 60_000;

  // --- Track previous connection status for reconnect detection ---
  let previousConnectionStatus = $state(connectionStore.status);
  let initialLoadDone = $state(false);

  function resetStalenessTimer(): void {
    if (stalenessTimer !== null) {
      clearTimeout(stalenessTimer);
    }
    stalenessTimer = setTimeout(() => {
      dashboardStore.markStale();
    }, STALENESS_TIMEOUT_MS);
  }

  function handlePatch(patch: MonitorPatch): void {
    // Apply patch to dashboard store (Req 9.1)
    dashboardStore.applyPatch(patch);
    // Clear stale indicator on new WS message (Req 9.5)
    dashboardStore.clearStale();
    // Reset staleness timer (Req 9.4)
    resetStalenessTimer();
  }

  // --- Watch connection status for reconnection (Req 9.2) ---
  $effect(() => {
    const currentStatus = connectionStore.status;
    if (initialLoadDone && previousConnectionStatus !== 'connected' && currentStatus === 'connected') {
      // WS reconnected — re-fetch all data
      dashboardStore.load();
      dashboardStore.clearStale();
      resetStalenessTimer();
    }
    previousConnectionStatus = currentStatus;
  });

  onMount(() => {
    // Load dashboard data on mount (Req 8.3)
    dashboardStore.load();
    initialLoadDone = true;

    // Subscribe to patchBus for monitor_status messages (Req 9.1)
    const unsubscribe = patchBus.subscribe(handlePatch);

    // Start staleness timer
    resetStalenessTimer();

    return () => {
      unsubscribe();
      if (stalenessTimer !== null) {
        clearTimeout(stalenessTimer);
      }
    };
  });

  // --- Per-widget retry handlers ---
  function retryWidget(widgetId: 'health-score' | 'status-ring' | 'incidents' | 'sparklines' | 'ssl-expiry' | 'heatmap' | 'events-feed'): () => void {
    return () => {
      dashboardStore.setWidgetError(widgetId, null);
      dashboardStore.load();
    };
  }
</script>

<section class="space-y-6 overflow-x-hidden">
  <div class="flex items-center justify-between">
    <h1 class="text-3xl font-bold tracking-tight text-primary">{t('dashboard.title')}</h1>
    <DataFreshness lastUpdated={dashboardStore.lastUpdated} stale={dashboardStore.stale} />
  </div>

  <!-- Responsive grid: 3 columns at >= 768px, single-column below (Req 8.1, 8.7) -->
  <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
    <!-- UptimeHeatmap — full width at top for at-a-glance history -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface shadow-sm md:col-span-3">
      <UptimeHeatmap
        data={dashboardStore.heatmap}
        loading={dashboardStore.isWidgetLoading('heatmap')}
        error={dashboardStore.getWidgetError('heatmap')}
        onRetry={retryWidget('heatmap')}
      />
    </div>

    <!-- FleetHealth — combined health score + status distribution -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface shadow-sm">
      <FleetHealth
        healthData={dashboardStore.healthScore}
        distributionData={dashboardStore.statusDistribution}
        loading={dashboardStore.isWidgetLoading('health-score')}
        error={dashboardStore.getWidgetError('health-score')}
        onRetry={retryWidget('health-score')}
      />
    </div>

    <!-- IncidentsPanel -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface shadow-sm md:col-span-2">
      <IncidentsPanel
        incidents={dashboardStore.activeIncidents}
        loading={dashboardStore.isWidgetLoading('incidents')}
        error={dashboardStore.getWidgetError('incidents')}
        onRetry={retryWidget('incidents')}
      />
    </div>

    <!-- InsightsPanel — tabbed: Response Time, SSL, Events -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface shadow-sm md:col-span-3">
      <InsightsPanel
        monitors={dashboardStore.topLatencyMonitors}
        sslEntries={dashboardStore.sslExpiry}
        events={dashboardStore.recentEvents}
        loading={dashboardStore.isWidgetLoading('sparklines')}
        error={dashboardStore.getWidgetError('sparklines')}
        onRetry={retryWidget('sparklines')}
      />
    </div>
  </div>
</section>
