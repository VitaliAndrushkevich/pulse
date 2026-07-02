<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { HistoryPoint } from '$lib/types';
  import type { AggregatedHistoryPoint } from '$lib/api';
  import { formatLatency } from '$lib/format';

  // ---------------------------------------------------------------------------
  // Props (Svelte 5 runes)
  // ---------------------------------------------------------------------------

  interface Props {
    points?: HistoryPoint[];
    aggregatedPoints?: AggregatedHistoryPoint[];
    loading: boolean;
    onzoom?: (from: string, to: string) => void;
  }

  let { points = [], aggregatedPoints = [], loading = false, onzoom }: Props = $props();

  // ---------------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------------

  let chartContainer: HTMLElement | undefined = $state(undefined);
  let chart: uPlot | null = $state(null);
  let isZoomed: boolean = $state(false);
  let fullXMin: number = $state(0);
  let fullXMax: number = $state(0);
  let tooltipVisible: boolean = $state(false);
  let tooltipX: number = $state(0);
  let tooltipY: number = $state(0);
  let tooltipContent: string = $state('');

  // ---------------------------------------------------------------------------
  // Derived
  // ---------------------------------------------------------------------------

  let isAggregated = $derived(
    aggregatedPoints !== undefined && aggregatedPoints.length > 0
  );

  let hasData = $derived(
    isAggregated
      ? aggregatedPoints!.length > 0
      : (points?.length ?? 0) > 0
  );

  // ---------------------------------------------------------------------------
  // Chart data builders
  // ---------------------------------------------------------------------------

  function buildRawData(pts: HistoryPoint[]): uPlot.AlignedData {
    const sorted = [...pts].sort(
      (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
    );

    const timestamps: number[] = [];
    const latencies: (number | null)[] = [];
    // State encoded: 1=up, 0=down, 0.5=unknown
    const states: (number | null)[] = [];

    for (const p of sorted) {
      timestamps.push(Math.floor(new Date(p.checked_at).getTime() / 1000));
      latencies.push(p.latency_ms);
      states.push(p.state === 'up' ? 1 : p.state === 'down' ? 0 : 0.5);
    }

    return [timestamps, latencies, states];
  }

  function buildAggregatedData(pts: AggregatedHistoryPoint[]): uPlot.AlignedData {
    const sorted = [...pts].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    );

    const timestamps: number[] = [];
    const avgLatencies: (number | null)[] = [];
    const minLatencies: (number | null)[] = [];
    const maxLatencies: (number | null)[] = [];
    // State from uptime_ratio: >= 0.5 = up, < 0.5 = down
    const states: (number | null)[] = [];

    for (const p of sorted) {
      timestamps.push(Math.floor(new Date(p.timestamp).getTime() / 1000));
      avgLatencies.push(p.avg_latency_ms);
      minLatencies.push(p.min_latency_ms);
      maxLatencies.push(p.max_latency_ms);
      states.push(p.uptime_ratio >= 0.5 ? 1 : 0);
    }

    return [timestamps, minLatencies, maxLatencies, avgLatencies, states];
  }

  // ---------------------------------------------------------------------------
  // Tooltip formatter
  // ---------------------------------------------------------------------------

  function formatTimestamp(unix: number): string {
    const d = new Date(unix * 1000);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  }

  function stateLabel(val: number | null | undefined): string {
    if (val === null || val === undefined) return 'unknown';
    if (val >= 0.9) return 'up';
    if (val <= 0.1) return 'down';
    return 'unknown';
  }

  // ---------------------------------------------------------------------------
  // Uptime band plugin
  // ---------------------------------------------------------------------------

  function uptimeBandPlugin(stateSeriesIdx: number): uPlot.Plugin {
    return {
      hooks: {
        draw: [
          (u: uPlot) => {
            const ctx = u.ctx;
            const { left, top, width, height } = u.bbox;
            const bandHeight = 12 * devicePixelRatio;
            const bandTop = top + height - bandHeight;

            const data = u.data[stateSeriesIdx];
            const xData = u.data[0];
            if (!data || !xData || xData.length < 2) return;

            ctx.save();

            for (let i = 0; i < xData.length - 1; i++) {
              const x0 = u.valToPos(xData[i], 'x', true);
              const x1 = u.valToPos(xData[i + 1], 'x', true);
              const val = data[i];

              if (val === null || val === undefined) {
                ctx.fillStyle = '#9ca3af'; // gray
              } else if (val >= 0.9) {
                ctx.fillStyle = '#10b981'; // green (up)
              } else if (val <= 0.1) {
                ctx.fillStyle = '#ef4444'; // red (down)
              } else {
                ctx.fillStyle = '#9ca3af'; // gray (unknown)
              }

              ctx.fillRect(x0, bandTop, x1 - x0, bandHeight);
            }

            ctx.restore();
          }
        ]
      }
    };
  }

  // ---------------------------------------------------------------------------
  // Tooltip plugin
  // ---------------------------------------------------------------------------

  function tooltipPlugin(): uPlot.Plugin {
    return {
      hooks: {
        setCursor: [
          (u: uPlot) => {
            const idx = u.cursor.idx;
            if (idx === null || idx === undefined || !u.data[0] || idx >= u.data[0].length) {
              tooltipVisible = false;
              return;
            }

            const ts = u.data[0][idx];
            const timeStr = formatTimestamp(ts);

            let content = `<div class="text-xs font-mono">${timeStr}</div>`;

            if (isAggregated) {
              const min = u.data[1]?.[idx];
              const max = u.data[2]?.[idx];
              const avg = u.data[3]?.[idx];
              const stateVal = u.data[4]?.[idx];
              content += `<div class="text-xs">Avg: ${avg != null ? formatLatency(avg) : 'N/A'}</div>`;
              content += `<div class="text-xs">Min: ${min != null ? formatLatency(min) : 'N/A'} / Max: ${max != null ? formatLatency(max) : 'N/A'}</div>`;
              content += `<div class="text-xs">State: ${stateLabel(stateVal)}</div>`;
            } else {
              const latency = u.data[1]?.[idx];
              const stateVal = u.data[2]?.[idx];
              content += `<div class="text-xs">Latency: ${latency != null ? formatLatency(latency) : 'N/A'}</div>`;
              content += `<div class="text-xs">State: ${stateLabel(stateVal)}</div>`;
            }

            tooltipContent = content;

            const cx = u.cursor.left ?? 0;
            const cy = u.cursor.top ?? 0;
            tooltipX = cx;
            tooltipY = cy;
            tooltipVisible = true;
          }
        ]
      }
    };
  }

  // ---------------------------------------------------------------------------
  // Chart creation
  // ---------------------------------------------------------------------------

  function createChart() {
    if (!chartContainer || !hasData) return;

    destroyChart();

    const styles = getComputedStyle(document.documentElement);
    const axisStroke = styles.getPropertyValue('--color-text-muted').trim() || '#64748b';
    const gridStroke = styles.getPropertyValue('--color-border').trim() || '#e2e8f0';

    let chartData: uPlot.AlignedData;
    let series: uPlot.Series[];
    let stateSeriesIdx: number;

    if (isAggregated) {
      chartData = buildAggregatedData(aggregatedPoints!);
      stateSeriesIdx = 4;
      series = [
        {}, // x-axis (timestamps)
        {
          label: 'Min',
          stroke: 'rgba(59, 130, 246, 0.3)',
          width: 1,
          fill: 'rgba(59, 130, 246, 0.05)',
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'Max',
          stroke: 'rgba(59, 130, 246, 0.3)',
          width: 1,
          fill: 'rgba(59, 130, 246, 0.05)',
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'Avg',
          stroke: '#3b82f6',
          width: 2,
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'State',
          show: false // hidden from chart, used by plugin
        }
      ];
    } else {
      chartData = buildRawData(points!);
      stateSeriesIdx = 2;
      series = [
        {}, // x-axis (timestamps)
        {
          label: 'Latency',
          stroke: '#3b82f6',
          width: 2,
          fill: 'rgba(59, 130, 246, 0.1)',
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'State',
          show: false // hidden from chart, used by plugin
        }
      ];
    }

    if (chartData[0].length === 0) return;

    fullXMin = chartData[0][0];
    fullXMax = chartData[0][chartData[0].length - 1];

    const width = chartContainer.clientWidth || 600;
    const MIN_ZOOM_SECONDS = 60;

    const opts: uPlot.Options = {
      width,
      height: 250,
      padding: [16, 8, 0, 8],
      series,
      bands: isAggregated
        ? [{ series: [1, 2], fill: 'rgba(59, 130, 246, 0.08)' }]
        : undefined,
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
      },
      cursor: {
        drag: { x: true, y: false }
      },
      select: {
        height: 250,
        width: 0,
        top: 0,
        left: 0
      },
      hooks: {
        setSelect: [
          (u: uPlot) => {
            const sel = u.select;
            if (sel.width < 2) return;

            const xMin = u.posToVal(sel.left, 'x');
            const xMax = u.posToVal(sel.left + sel.width, 'x');

            // Enforce minimum 60s zoom window
            if (xMax - xMin < MIN_ZOOM_SECONDS) {
              // Reset selection visuals
              u.setSelect({ left: 0, width: 0, top: 0, height: 0 }, false);
              return;
            }

            u.setScale('x', { min: xMin, max: xMax });
            u.setSelect({ left: 0, width: 0, top: 0, height: 0 }, false);

            isZoomed = true;

            if (onzoom) {
              onzoom(new Date(xMin * 1000).toISOString(), new Date(xMax * 1000).toISOString());
            }
          }
        ]
      },
      plugins: [uptimeBandPlugin(stateSeriesIdx), tooltipPlugin()]
    };

    chart = new uPlot(opts, chartData, chartContainer);
  }

  function destroyChart() {
    if (chart) {
      chart.destroy();
      chart = null;
    }
    tooltipVisible = false;
  }

  function resetZoom() {
    if (chart) {
      chart.setScale('x', { min: fullXMin, max: fullXMax });
      isZoomed = false;
    }
  }

  // ---------------------------------------------------------------------------
  // Resize observer
  // ---------------------------------------------------------------------------

  let resizeObserver: ResizeObserver | null = null;

  function setupResizeObserver() {
    if (!chartContainer) return;
    resizeObserver = new ResizeObserver(() => {
      if (chart && chartContainer) {
        chart.setSize({ width: chartContainer.clientWidth, height: 250 });
      }
    });
    resizeObserver.observe(chartContainer);
  }

  // ---------------------------------------------------------------------------
  // Lifecycle
  // ---------------------------------------------------------------------------

  onMount(() => {
    createChart();
    setupResizeObserver();
  });

  onDestroy(() => {
    destroyChart();
    if (resizeObserver) {
      resizeObserver.disconnect();
      resizeObserver = null;
    }
  });

  // Re-create chart when data changes
  $effect(() => {
    // Access reactive deps
    points;
    aggregatedPoints;
    loading;

    if (!loading && chartContainer) {
      // Use microtask to avoid re-entrancy
      queueMicrotask(() => {
        createChart();
      });
    }
  });
</script>

<div class="relative w-full">
  {#if loading}
    <!-- 250px skeleton loader -->
    <div
      class="flex h-[250px] w-full animate-pulse items-center justify-center rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)]"
      aria-label="Loading chart data"
    >
      <div class="flex flex-col items-center gap-2">
        <div class="h-3 w-48 rounded bg-[var(--color-border)]"></div>
        <div class="h-3 w-32 rounded bg-[var(--color-border)]"></div>
      </div>
    </div>
  {:else if !hasData}
    <!-- Empty state -->
    <div
      class="flex h-[250px] w-full items-center justify-center rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)]"
    >
      <p class="text-sm text-[var(--color-text-secondary)]">No data available</p>
    </div>
  {:else}
    <!-- Chart container -->
    <div class="relative">
      {#if isZoomed}
        <button
          type="button"
          onclick={resetZoom}
          class="absolute right-2 top-2 z-10 rounded border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-2 py-1 text-xs text-[var(--color-text-secondary)] shadow-sm transition-colors hover:bg-[var(--color-bg-surface-hover)]"
        >
          Reset zoom
        </button>
      {/if}

      <div bind:this={chartContainer} class="w-full"></div>

      <!-- Tooltip -->
      {#if tooltipVisible}
        <div
          class="pointer-events-none absolute z-20 rounded border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-2 py-1 shadow-md"
          style="left: {tooltipX + 12}px; top: {tooltipY - 10}px;"
        >
          {@html tooltipContent}
        </div>
      {/if}
    </div>
  {/if}
</div>
