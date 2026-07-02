<script lang="ts" module>
  /**
   * Maps a health score percentage to its corresponding CSS custom property color.
   *
   * Exported for property-based testing (Property 2).
   *
   * Validates: Requirements 1.2
   */
  export function getHealthScoreColor(percent: number): string {
    if (percent >= 99) return 'var(--color-success)';
    if (percent >= 95) return 'var(--color-warning)';
    return 'var(--color-error)';
  }
</script>

<script lang="ts">
  /**
   * HealthScore — displays the global uptime percentage with stepped color.
   *
   * Requirements 1.1: Display aggregated uptime percentage (2 decimal places)
   * Requirements 1.2: Stepped color (green >= 99%, amber >= 95%, red < 95%)
   * Requirements 1.3: Empty state "—" when no active monitors
   * Requirements 1.5: Partial data warning indicator
   */
  import type { HealthScoreData } from '$lib/types';
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    data: HealthScoreData | null;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { data, loading, error, onRetry }: Props = $props();

  let isEmpty = $derived(data === null || data.active_monitor_count === 0);
  let color = $derived(isEmpty ? 'var(--color-text-secondary)' : getHealthScoreColor(data!.uptime_percent));
  let formattedValue = $derived(isEmpty ? '—' : `${data!.uptime_percent.toFixed(2)}%`);
  let showPartialWarning = $derived(!isEmpty && data!.partial_data === true);
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col items-center justify-center gap-2 p-4" data-testid="health-score">
    <span class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.healthScore.title')}
    </span>
    <span
      class="text-4xl font-bold tabular-nums"
      style="color: {color}"
      aria-label={isEmpty ? t('dashboard.healthScore.noData') : t('dashboard.healthScore.value', { value: formattedValue })}
    >
      {formattedValue}
    </span>
    {#if showPartialWarning}
      <span
        class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
        style="background-color: var(--color-warning); color: var(--color-bg-primary)"
        role="status"
        data-testid="health-score-partial-warning"
      >
        {t('dashboard.healthScore.partialData')}
      </span>
    {/if}
  </div>
</WidgetShell>
