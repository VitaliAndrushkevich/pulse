<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 9, 10).
   */
  import type { TopLatencyMonitor } from '$lib/types';

  /**
   * Truncates a monitor name to the given character limit, appending "…" if exceeded.
   *
   * Validates: Requirements 4.2
   */
  export function truncateMonitorName(name: string, limit: number = 40): string {
    if (name.length <= limit) return name;
    return name.slice(0, limit) + '…';
  }

  /**
   * Formats an avg_latency_ms value as a rounded integer followed by " ms".
   *
   * Validates: Requirements 4.2
   */
  export function formatLatency(avgLatencyMs: number): string {
    return `${Math.round(avgLatencyMs)} ms`;
  }

  /**
   * Selects up to `limit` monitors with the highest non-null avg_latency_ms,
   * ordered by latency descending.
   *
   * Monitors with null/undefined/NaN latency are excluded.
   * Returns at most `limit` entries (default 5).
   *
   * Validates: Requirements 4.1, 4.4, 4.5, 4.6
   */
  export function selectTopMonitors(
    monitors: TopLatencyMonitor[],
    limit: number = 5
  ): TopLatencyMonitor[] {
    return [...monitors]
      .filter((m) => m.avg_latency_ms != null && !Number.isNaN(m.avg_latency_ms))
      .sort((a, b) => b.avg_latency_ms - a.avg_latency_ms)
      .slice(0, limit);
  }
</script>

<script lang="ts">
  import { resolve } from '$app/paths';
  import { t, getLocale } from '$lib/i18n';
  import type { FleetPeriod } from '$lib/types';
  import WidgetShell from './WidgetShell.svelte';

  let { monitors, period = '24h', loading, error, onRetry }: {
    monitors: TopLatencyMonitor[];
    period?: FleetPeriod;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  } = $props();

  const topMonitors = $derived(selectTopMonitors(monitors));
  const maximum = $derived(Math.max(0, ...topMonitors.map(monitor => monitor.avg_latency_ms)));
  const periodLabel = $derived(t({ '1h': 'dashboard.fleetHealth.hour', '24h': 'dashboard.fleetHealth.day', '7d': 'dashboard.fleetHealth.week', '30d': 'dashboard.fleetHealth.month' }[period]));
</script>

<section class="response-panel" aria-labelledby="response-times-title" data-testid="response-times">
  <header class="panel-heading">
    <h3 id="response-times-title" class="pulse-eyebrow">{t('dashboard.responseTimes.title')}</h3>
    <p>{t('dashboard.responseTimes.description', { period: periodLabel })}</p>
  </header>
  <WidgetShell {loading} {error} {onRetry}>
    {#if topMonitors.length === 0}
      <p class="empty-message" data-testid="response-times-empty">{t('dashboard.responseTimes.empty')}</p>
    {:else}
      <ol class="response-list">
        {#each topMonitors as monitor (monitor.monitor_id)}
          <li>
            <a class="monitor-row" href={resolve('/monitors/[id]', { id: monitor.monitor_id })}>
              <div class="row-heading">
                <span class="monitor-name" title={monitor.monitor_name}>{monitor.monitor_name}</span>
                <span class="latency">{t('dashboard.responseTimes.milliseconds', { value: new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 0 }).format(monitor.avg_latency_ms) })}</span>
              </div>
              <div class="latency-track" aria-hidden="true">
                <span style:inline-size="{maximum > 0 ? Math.max(0, monitor.avg_latency_ms / maximum * 100) : 0}%"></span>
              </div>
            </a>
          </li>
        {/each}
      </ol>
    {/if}
  </WidgetShell>
</section>

<style>
  .response-panel { padding: 1.25rem; }
  .panel-heading { display: grid; gap: 0.5rem; margin-block-end: 1rem; }
  .panel-heading p, .empty-message { color: var(--color-text-secondary); font-size: 0.75rem; }
  .empty-message { padding-block: 0.5rem; }
  .response-list { display: grid; gap: 0.25rem; }
  .monitor-row { display: grid; gap: 0.6rem; padding: 0.75rem; border-radius: 0.25rem; text-decoration: none; }
  .monitor-row:hover { background: var(--color-bg-secondary); }
  .monitor-row:focus-visible { outline: 2px solid var(--color-brand-primary); outline-offset: 2px; }
  .row-heading { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
  .monitor-name { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--color-text-primary); font-size: 0.875rem; font-weight: 500; }
  .latency { flex-shrink: 0; color: var(--color-text-primary); font-size: 0.875rem; font-variant-numeric: tabular-nums; font-family: var(--font-data); }
  .latency-track { block-size: 0.25rem; background: var(--color-border); border-radius: 0.125rem; overflow: hidden; }
  .latency-track span { display: block; block-size: 100%; background: var(--color-brand-primary); border-radius: inherit; }
</style>
