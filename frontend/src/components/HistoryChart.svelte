<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { HistoryPoint } from '$lib/types';

  export let data: HistoryPoint[] = [];

  let chartContainer: HTMLElement;
  let chart: uPlot | null = null;

  function buildChartData(points: HistoryPoint[]): uPlot.AlignedData {
    // Filter out points with null latency and sort by time ascending
    const valid = points
      .filter((p): p is HistoryPoint & { latency_ms: number } => p.latency_ms !== null)
      .sort((a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime());

    const timestamps = valid.map((p) => Math.floor(new Date(p.checked_at).getTime() / 1000));
    const latencies = valid.map((p) => p.latency_ms);

    return [timestamps, latencies];
  }

  function createChart() {
    if (!chartContainer || data.length === 0) return;

    const chartData = buildChartData(data);

    // Don't render if all points had null latency
    if (chartData[0].length === 0) return;

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
          fill: 'rgba(59, 130, 246, 0.1)'
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
          label: 'Latency (ms)'
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
