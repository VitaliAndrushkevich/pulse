<script lang="ts">
  import TimeRangePicker from './TimeRangePicker.svelte';
  import HistoryChartExplorer from './HistoryChartExplorer.svelte';
  import { getMonitorHistoryExtended, type AggregatedHistoryPoint } from '$lib/api';
  import type { HistoryPoint } from '$lib/types';

  interface Props {
    monitorId: string;
    retentionDays: number;
  }

  let { monitorId, retentionDays }: Props = $props();

  let loading = $state(true);
  let error = $state<string | null>(null);
  let points = $state<HistoryPoint[]>([]);
  let aggregatedPoints = $state<AggregatedHistoryPoint[]>([]);
  let step = $state<number | undefined>(undefined);
  let truncated = $state(false);

  // Default to 24h preset
  let selectedRange = $state<{ from: string; to: string }>({
    from: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    to: new Date().toISOString()
  });

  async function fetchHistory() {
    loading = true;
    error = null;

    try {
      const response = await getMonitorHistoryExtended(
        monitorId,
        selectedRange.from,
        selectedRange.to,
        step
      );

      points = response.points ?? [];
      aggregatedPoints = response.aggregated_points ?? [];
      step = response.step;
      truncated = response.truncated ?? false;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load history data.';
    } finally {
      loading = false;
    }
  }

  function handleRangeChange(range: { from: string; to: string }) {
    selectedRange = range;

    // Auto-compute step for ranges > 24h
    const rangeMs = new Date(range.to).getTime() - new Date(range.from).getTime();
    const rangeSec = rangeMs / 1000;
    if (rangeSec > 86400) {
      step = Math.ceil(rangeSec / 1000);
    } else {
      step = undefined;
    }

    fetchHistory();
  }

  function handleRetry() {
    fetchHistory();
  }

  // Initial fetch
  $effect(() => {
    monitorId;
    fetchHistory();
  });
</script>

<div class="space-y-4" data-testid="history-explorer">
  <TimeRangePicker
    selected="24h"
    {retentionDays}
    onchange={handleRangeChange}
  />

  {#if truncated}
    <div class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700" data-testid="truncated-notice">
      Data has been truncated to the monitor's retention period ({retentionDays} days).
    </div>
  {/if}

  {#if loading}
    <div class="h-[250px] animate-pulse rounded-lg bg-[var(--color-bg-surface-hover)]" data-testid="history-loading-skeleton"></div>
  {:else if error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="history-error">
      <p class="text-sm text-rose-700">{error}</p>
      <button
        type="button"
        onclick={handleRetry}
        class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
      >
        Retry
      </button>
    </div>
  {:else if points.length === 0 && aggregatedPoints.length === 0}
    <div class="flex h-[250px] items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface" data-testid="history-empty">
      <p class="text-sm text-[var(--color-text-muted)]">No data available for the selected period.</p>
    </div>
  {:else}
    <HistoryChartExplorer
      {points}
      {aggregatedPoints}
      loading={false}
    />
  {/if}
</div>
