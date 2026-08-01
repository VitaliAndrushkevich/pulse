<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { HistoryPoint } from '$lib/types';
  import { formatLatency } from '$lib/format';

  export let data: HistoryPoint[] = [];

  let chartContainer: HTMLElement;
  let chart: uPlot | null = null;

  function computeGapThreshold(timestamps: number[]): number {
    if (timestamps.length < 2) return Infinity;
    const intervals: number[] = [];
    for (let i = 1; i < timestamps.length; i++) {
      const diff = timestamps[i] - timestamps[i - 1];
      if (diff > 0) intervals.push(diff);
    }
    if (intervals.length === 0) return Infinity;
    intervals.sort((a, b) => a - b);
    const median = intervals[Math.floor(intervals.length / 2)];
    return median * 3;
  }

  function buildChartData(points: HistoryPoint[]): uPlot.AlignedData {
    // Sort by time ascending
    const sorted = [...points].sort(
      (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
    );

    // Deduplicate points with the same second-precision timestamp (keep last)
    const deduped: HistoryPoint[] = [];
    for (let i = 0; i < sorted.length; i++) {
      const ts = Math.floor(new Date(sorted[i].checked_at).getTime() / 1000);
      const nextTs = i + 1 < sorted.length
        ? Math.floor(new Date(sorted[i + 1].checked_at).getTime() / 1000)
        : -1;
      if (ts !== nextTs) {
        deduped.push(sorted[i]);
      }
    }

    // First pass: collect raw timestamps for gap detection
    const rawTimestamps = deduped.map((p) => Math.floor(new Date(p.checked_at).getTime() / 1000));
    const gapThreshold = computeGapThreshold(rawTimestamps);

    const timestamps: number[] = [];
    const latencies: (number | null)[] = [];

    for (let i = 0; i < deduped.length; i++) {
      const p = deduped[i];

      // Insert a null gap-breaker if there's a large time gap
      if (i > 0) {
        const gap = rawTimestamps[i] - rawTimestamps[i - 1];
        if (gap > gapThreshold) {
          timestamps.push(rawTimestamps[i - 1] + 1);
          latencies.push(null);
        }
      }

      timestamps.push(rawTimestamps[i]);
      // Don't show latency for failed checks — it's just the timeout value
      latencies.push(p.state === 'up' ? p.latency_ms : null);
    }

    return [timestamps, latencies];
  }

  function createChart() {
    if (!chartContainer || data.length === 0) return;

    const chartData = buildChartData(data);

    // Don't render if there are no data points at all
    if (chartData[0].length === 0) return;

    // If all latency values are null (all checks failed), still render
    // the chart with axes — it provides temporal context

    const width = chartContainer.clientWidth || 600;

    // Read theme colors from CSS custom properties
    const styles = getComputedStyle(document.documentElement);
    const axisStroke = styles.getPropertyValue('--color-text-muted').trim() || '#64748b';
    const gridStroke = styles.getPropertyValue('--color-border').trim() || '#e2e8f0';

    const opts: uPlot.Options = {
      width,
      height: 250,
      series: [
        {},
        {
          label: 'Latency',
          stroke: '#3b82f6',
          width: 2,
          fill: 'rgba(59, 130, 246, 0.1)',
          spanGaps: false,
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        }
      ],
      axes: [
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke + '40' }
        },
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke + '40' },
          label: 'Latency',
          values: (_u: uPlot, splits: number[]) => splits.map((v) => v == null ? '' : formatLatency(v))
        }
      ],
      scales: {
        x: { time: true },
        y: { auto: true }
      }
    };

    chart = new uPlot(opts, chartData, chartContainer);
  }

  function destroyChart() {
    if (chart) {
      chart.destroy();
      chart = null;
    }
  }

  onMount(() => {
    createChart();
  });

  onDestroy(() => {
    destroyChart();
  });
</script>

<div class="w-full">
  {#if data.length === 0}
    <div class="flex items-center justify-center rounded-lg border border-[var(--color-border)] bg-page py-12 text-sm text-secondary">
      No check data available for the selected time range
    </div>
  {:else}
    <div bind:this={chartContainer} class="w-full"></div>
  {/if}
</div>
