<script lang="ts">
  /**
   * InsightsPanel — tabbed widget combining Response Time, SSL Warnings, and Recent Events.
   *
   * Consolidates three separate dashboard sections into one compact card
   * with tab navigation for cleaner layout.
   */
  import type { TopLatencyMonitor, SSLExpiryEntry, RecentEvent } from '$lib/types';
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';
  import { truncateMonitorName, formatLatency, selectTopMonitors } from './ResponseSparklines.svelte';
  import { filterSSLEntries, sortSSLEntries, getUrgencyTier } from './SSLWarnings.svelte';
  import { formatRelativeTime, getEventColor } from './EventsFeed.svelte';

  type TabId = 'latency' | 'ssl' | 'events';

  interface Props {
    monitors: TopLatencyMonitor[];
    sslEntries: SSLExpiryEntry[];
    events: RecentEvent[];
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { monitors, sslEntries, events, loading, error, onRetry }: Props = $props();

  let activeTab = $state<TabId>('events');

  // Derived data for each tab
  let topMonitors = $derived(selectTopMonitors(monitors));
  let sslFiltered = $derived(sortSSLEntries(filterSSLEntries(sslEntries)));
  let sslBadgeCount = $derived(sslFiltered.length);

  // Tick for relative time refresh
  let tick = $state(0);
  $effect(() => {
    const interval = setInterval(() => { tick++; }, 30_000);
    return () => clearInterval(interval);
  });

  let now = $derived.by(() => { void tick; return Date.now(); });

  const tabs: { id: TabId; labelKey: string }[] = [
    { id: 'events', labelKey: 'dashboard.insights.tabEvents' },
    { id: 'latency', labelKey: 'dashboard.insights.tabLatency' },
    { id: 'ssl', labelKey: 'dashboard.insights.tabSSL' },
  ];

  function getUrgencyColor(tier: 'expired' | 'critical' | 'warning'): string {
    if (tier === 'warning') return 'var(--color-warning)';
    return 'var(--color-error)';
  }

  function formatDaysRemaining(days: number): string {
    if (days <= 0) return t('dashboard.ssl.expired');
    return t('dashboard.ssl.daysRemaining', { count: String(days) });
  }

  function buildSparklinePath(latencyMs: number): string {
    const points: number[] = [];
    const numPoints = 12;
    const height = 24;
    const width = 80;

    for (let i = 0; i < numPoints; i++) {
      const variance = Math.sin(i * 0.8 + latencyMs * 0.01) * 0.3 + 0.5;
      points.push(variance * height);
    }

    return points.map((y, i) => `${(i / (numPoints - 1)) * width},${y}`).join(' ');
  }
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-3 p-4" data-testid="insights-panel">
    <!-- Tab bar -->
    <div class="flex gap-1 rounded-lg p-0.5" style="background-color: var(--color-bg-secondary)" role="tablist">
      {#each tabs as tab (tab.id)}
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === tab.id}
          class="relative flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          style={activeTab === tab.id
            ? 'background-color: var(--color-bg-primary); color: var(--color-text-primary); box-shadow: 0 1px 2px rgba(0,0,0,.1)'
            : 'color: var(--color-text-secondary)'}
          onclick={() => (activeTab = tab.id)}
          data-testid="insights-tab-{tab.id}"
        >
          {t(tab.labelKey)}
          {#if tab.id === 'ssl' && sslBadgeCount > 0}
            <span
              class="ml-1 inline-flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold"
              style="background-color: var(--color-warning); color: var(--color-bg-primary)"
            >
              {sslBadgeCount}
            </span>
          {/if}
        </button>
      {/each}
    </div>

    <!-- Tab content -->
    <div data-testid="insights-content">
      {#if activeTab === 'latency'}
        <!-- Response Time tab -->
        {#if topMonitors.length === 0}
          <p class="py-4 text-center text-sm" style="color: var(--color-text-secondary)" data-testid="insights-latency-empty">
            {t('dashboard.sparklines.empty')}
          </p>
        {:else}
          <ul class="flex flex-col gap-1" role="list" data-testid="insights-latency-list">
            {#each topMonitors as monitor (monitor.monitor_id)}
              <li
                class="flex items-center gap-3 rounded-md px-3 py-2"
                style="background-color: var(--color-bg-secondary)"
              >
                <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)" title={monitor.monitor_name}>
                    {truncateMonitorName(monitor.monitor_name)}
                  </span>
                  <span class="text-xs tabular-nums" style="color: var(--color-text-secondary)">
                    {formatLatency(monitor.avg_latency_ms)}
                  </span>
                </div>
                <svg class="shrink-0" width="80" height="24" viewBox="0 0 80 24" fill="none" aria-hidden="true">
                  <polyline
                    points={buildSparklinePath(monitor.avg_latency_ms)}
                    stroke="var(--color-brand-primary)"
                    stroke-width="1.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    fill="none"
                  />
                </svg>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if activeTab === 'ssl'}
        <!-- SSL Warnings tab -->
        {#if sslFiltered.length === 0}
          <p class="py-4 text-center text-sm" style="color: var(--color-text-secondary)" data-testid="insights-ssl-empty">
            {t('dashboard.ssl.allClear')}
          </p>
        {:else}
          <ul class="flex flex-col gap-1" role="list" data-testid="insights-ssl-list">
            {#each sslFiltered as entry (entry.monitor_id)}
              {@const tier = getUrgencyTier(entry.days_remaining)}
              {@const color = getUrgencyColor(tier)}
              <li
                class="flex items-center justify-between gap-2 rounded-md px-3 py-2"
                style="background-color: var(--color-bg-secondary)"
              >
                <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)">
                  {entry.monitor_name}
                </span>
                <span class="shrink-0 text-xs font-medium tabular-nums" style="color: {color}" data-urgency={tier}>
                  {formatDaysRemaining(entry.days_remaining)}
                </span>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if activeTab === 'events'}
        <!-- Recent Events tab -->
        {#if events.length === 0}
          <p class="py-4 text-center text-sm" style="color: var(--color-text-secondary)" data-testid="insights-events-empty">
            {t('dashboard.events.empty')}
          </p>
        {:else}
          <ul class="flex flex-col gap-1" role="list" data-testid="insights-events-list">
            {#each events as event, idx (event.monitor_id + event.occurred_at + '-' + idx)}
              {@const colorStyle = getEventColor(event.to_state)}
              {@const relative = formatRelativeTime(event.occurred_at, now)}
              <li
                class="flex items-center justify-between gap-2 rounded-md px-3 py-2"
                style="background-color: var(--color-bg-secondary)"
              >
                <div class="flex min-w-0 flex-col gap-0.5">
                  <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)">
                    {event.monitor_name}
                  </span>
                  <span class="text-xs font-medium" style="color: {colorStyle}">
                    {t('dashboard.events.transition', { from: event.from_state, to: event.to_state })}
                  </span>
                </div>
                <span class="shrink-0 text-xs tabular-nums" style="color: var(--color-text-secondary)">
                  {t(relative.key, relative.params)}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </div>
  </div>
</WidgetShell>
