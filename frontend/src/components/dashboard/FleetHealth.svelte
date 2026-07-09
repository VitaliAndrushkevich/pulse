<script lang="ts" module>
  export function getHealthScoreColor(percent: number): string {
    if (percent >= 99) return 'var(--color-success)';
    if (percent >= 95) return 'var(--color-warning)';
    return 'var(--color-error)';
  }
</script>

<script lang="ts">
  /**
   * FleetHealth — compact health widget.
   *
   * Big uptime percentage + "{down}/{total} down" summary underneath.
   * No donut, no legend — just the essentials.
   */
  import type { HealthScoreData, StatusDistribution } from '$lib/types';
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    healthData: HealthScoreData | null;
    distributionData: StatusDistribution | null;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { healthData, distributionData, loading, error, onRetry }: Props = $props();

  let isEmpty = $derived(healthData === null || healthData.active_monitor_count === 0);
  let color = $derived(
    isEmpty ? 'var(--color-text-secondary)' : getHealthScoreColor(healthData!.uptime_percent)
  );
  let formattedValue = $derived(isEmpty ? '—' : `${healthData!.uptime_percent.toFixed(2)}%`);
  let showPartialWarning = $derived(!isEmpty && healthData!.partial_data === true);

  let summaryText = $derived.by(() => {
    if (!distributionData || distributionData.total === 0) return null;
    if (distributionData.down === 0) return null;
    return `${distributionData.down}/${distributionData.total}`;
  });
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex h-full flex-col items-center justify-center gap-1 px-4 py-8" data-testid="fleet-health">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.fleetHealth.title')}
    </h3>

    <span
      class="text-5xl font-bold tabular-nums"
      style="color: {color}"
      aria-label={isEmpty
        ? t('dashboard.healthScore.noData')
        : t('dashboard.healthScore.value', { value: formattedValue })}
      data-testid="health-score-value"
    >
      {formattedValue}
    </span>

    {#if summaryText}
      <span
        class="text-sm font-medium tabular-nums"
        style="color: var(--color-error)"
        data-testid="fleet-health-down-count"
      >
        {t('dashboard.fleetHealth.downSummary', { down: String(distributionData!.down), total: String(distributionData!.total) })}
      </span>
    {:else if !isEmpty}
      <span class="text-sm" style="color: var(--color-success)" data-testid="fleet-health-all-up">
        {t('dashboard.fleetHealth.allUp')}
      </span>
    {/if}

    {#if showPartialWarning}
      <span
        class="mt-1 inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
        style="background-color: var(--color-warning); color: var(--color-bg-primary)"
        role="status"
        data-testid="health-score-partial-warning"
      >
        {t('dashboard.healthScore.partialData')}
      </span>
    {/if}
  </div>
</WidgetShell>
