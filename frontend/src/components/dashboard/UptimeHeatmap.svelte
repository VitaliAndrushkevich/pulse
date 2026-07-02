<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 14, 15).
   */
  import type { HeatmapHour } from '$lib/types';

  /**
   * Ensures exactly 24 hourly blocks exist.
   * Pads missing hours with zero-count (grey) blocks.
   *
   * Validates: Requirements 6.1
   */
  export function normalizeHeatmapData(data: HeatmapHour[]): HeatmapHour[] {
    if (data.length === 0) {
      // Generate 24 empty blocks starting from 24h ago
      const now = new Date();
      const startHour = new Date(now);
      startHour.setMinutes(0, 0, 0);
      startHour.setHours(startHour.getHours() - 23);

      return Array.from({ length: 24 }, (_, i) => {
        const hour = new Date(startHour);
        hour.setHours(hour.getHours() + i);
        return {
          hour_start: hour.toISOString(),
          up_count: 0,
          down_count: 0,
          unknown_count: 0,
        };
      });
    }

    if (data.length >= 24) {
      return data.slice(0, 24);
    }

    // Pad with empty blocks to reach 24
    const result = [...data];
    const lastHour = new Date(result[result.length - 1].hour_start);
    while (result.length < 24) {
      const nextHour = new Date(lastHour);
      nextHour.setHours(nextHour.getHours() + (result.length - data.length + 1));
      result.push({
        hour_start: nextHour.toISOString(),
        up_count: 0,
        down_count: 0,
        unknown_count: 0,
      });
    }

    return result;
  }

  /**
   * Determines the block color based on worst-state priority:
   * Red: down_count > 0
   * Amber: unknown_count > 0 and down_count === 0
   * Green: up_count > 0 and down_count === 0 and unknown_count === 0
   * Grey: all counts are 0 (no data)
   *
   * Validates: Requirements 6.2
   */
  export function getBlockColor(hour: HeatmapHour): string {
    if (hour.down_count > 0) return 'var(--color-error)';
    if (hour.unknown_count > 0) return 'var(--color-warning)';
    if (hour.up_count > 0) return 'var(--color-success)';
    return 'var(--color-border)';
  }
</script>

<script lang="ts">
  /**
   * UptimeHeatmap — 24-block hourly heatmap of aggregated monitor state.
   *
   * Requirements 6.1: 24 hourly blocks
   * Requirements 6.2: Worst-state coloring (red > amber > green > grey)
   * Requirements 6.3: Tooltip with time range and per-state counts
   * Requirements 6.4: Time axis labels every 3 hours in local timezone
   * Requirements 6.5: Error state with retry control
   * Requirements 6.6: Real-time update of current hour block
   */
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    data: HeatmapHour[];
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { data, loading, error, onRetry }: Props = $props();

  let hoveredIndex = $state<number | null>(null);

  let normalizedData = $derived(normalizeHeatmapData(data));

  /**
   * Formats hour in user's local timezone (e.g., "14:00").
   */
  function formatHour(isoString: string): string {
    const date = new Date(isoString);
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  /**
   * Generates time axis labels every 3 hours (8 labels total).
   */
  let axisLabels = $derived.by(() => {
    const labels: { index: number; text: string }[] = [];
    for (let i = 0; i < 24; i += 3) {
      if (normalizedData[i]) {
        labels.push({ index: i, text: formatHour(normalizedData[i].hour_start) });
      }
    }
    return labels;
  });

  /**
   * Tooltip content for hovered block.
   */
  let tooltipContent = $derived.by(() => {
    if (hoveredIndex === null) return null;
    const hour = normalizedData[hoveredIndex];
    if (!hour) return null;

    const startTime = formatHour(hour.hour_start);
    const endDate = new Date(hour.hour_start);
    endDate.setHours(endDate.getHours() + 1);
    const endTime = endDate.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false });

    return {
      timeRange: `${startTime} – ${endTime}`,
      up: hour.up_count,
      down: hour.down_count,
      unknown: hour.unknown_count,
    };
  });
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-2 p-4" data-testid="uptime-heatmap">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.heatmap.title')}
    </h3>

    <div class="relative">
      <!-- Heatmap blocks -->
      <div class="flex gap-0.5" role="img" aria-label={t('dashboard.heatmap.ariaLabel')}>
        {#each normalizedData as hour, i (i)}
          <button
            type="button"
            class="h-8 flex-1 rounded-sm transition-opacity hover:opacity-80"
            style="background-color: {getBlockColor(hour)}"
            data-testid="heatmap-block"
            aria-label={t('dashboard.heatmap.blockAria', {
              time: formatHour(hour.hour_start),
              up: String(hour.up_count),
              down: String(hour.down_count),
              unknown: String(hour.unknown_count),
            })}
            onmouseenter={() => (hoveredIndex = i)}
            onmouseleave={() => (hoveredIndex = null)}
            onfocus={() => (hoveredIndex = i)}
            onblur={() => (hoveredIndex = null)}
          ></button>
        {/each}
      </div>

      <!-- Time axis labels -->
      <div class="relative mt-1 flex" aria-hidden="true">
        {#each axisLabels as label (label.index)}
          <span
            class="absolute text-[10px]"
            style="left: {(label.index / 24) * 100}%; color: var(--color-text-secondary)"
          >
            {label.text}
          </span>
        {/each}
      </div>

      <!-- Tooltip -->
      {#if tooltipContent}
        <div
          class="absolute -top-12 left-1/2 -translate-x-1/2 rounded-md px-2 py-1 text-xs shadow-md whitespace-nowrap"
          style="background-color: var(--color-bg-secondary); color: var(--color-text-primary); border: 1px solid var(--color-border)"
          role="tooltip"
          data-testid="heatmap-tooltip"
        >
          <span class="font-medium">{tooltipContent.timeRange}</span>
          <span class="ml-2">
            {t('dashboard.heatmap.tooltipCounts', {
              up: String(tooltipContent.up),
              down: String(tooltipContent.down),
              unknown: String(tooltipContent.unknown),
            })}
          </span>
        </div>
      {/if}
    </div>
  </div>
</WidgetShell>
