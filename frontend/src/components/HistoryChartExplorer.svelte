<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { HistoryPoint } from '$lib/types';
  import type { AggregatedHistoryPoint } from '$lib/api';
  import { formatLatency } from '$lib/format';
  import { t } from '$lib/i18n';

  // ---------------------------------------------------------------------------
  // Props (Svelte 5 runes)
  // ---------------------------------------------------------------------------

  interface Props {
    points?: HistoryPoint[];
    aggregatedPoints?: AggregatedHistoryPoint[];
    loading: boolean;
    from?: string;
    to?: string;
    onzoom?: (from: string, to: string) => void;
  }

  let { points = [], aggregatedPoints = [], loading = false, from, to, onzoom }: Props = $props();

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
  let avgLatency: number | null = $state(null);

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

  /**
   * Compute the gap threshold: if the interval between two consecutive points
   * exceeds 3x the median interval, we treat it as a gap (monitor paused, etc.)
   * Zero-intervals (duplicate timestamps) are excluded from the calculation.
   */
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

  function buildRawData(pts: HistoryPoint[]): uPlot.AlignedData {
    const sorted = [...pts].sort(
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
    // State encoded: 1=up, 0=down, 0.5=unknown
    const states: (number | null)[] = [];

    for (let i = 0; i < deduped.length; i++) {
      const p = deduped[i];

      // Insert a null gap-breaker point if there's a large time gap
      if (i > 0) {
        const gap = rawTimestamps[i] - rawTimestamps[i - 1];
        if (gap > gapThreshold) {
          // Insert a synthetic null point 1s after the previous point to break the line
          timestamps.push(rawTimestamps[i - 1] + 1);
          latencies.push(null);
          states.push(null);
        }
      }

      timestamps.push(rawTimestamps[i]);
      // Don't show latency for failed checks — it's just the timeout duration
      latencies.push(p.state === 'up' ? p.latency_ms : null);
      states.push(p.state === 'up' ? 1 : p.state === 'down' ? 0 : 0.5);
    }

    return [timestamps, latencies, states];
  }

  function buildAggregatedData(pts: AggregatedHistoryPoint[]): uPlot.AlignedData {
    const sorted = [...pts].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    );

    // First pass: collect raw timestamps for gap detection
    const rawTimestamps = sorted.map((p) => Math.floor(new Date(p.timestamp).getTime() / 1000));
    const gapThreshold = computeGapThreshold(rawTimestamps);

    const timestamps: number[] = [];
    const avgLatencies: (number | null)[] = [];
    const minLatencies: (number | null)[] = [];
    const maxLatencies: (number | null)[] = [];
    // State from uptime_ratio: >= 0.5 = up, < 0.5 = down
    const states: (number | null)[] = [];

    for (let i = 0; i < sorted.length; i++) {
      const p = sorted[i];

      // Insert a null gap-breaker point if there's a large time gap
      if (i > 0) {
        const gap = rawTimestamps[i] - rawTimestamps[i - 1];
        if (gap > gapThreshold) {
          timestamps.push(rawTimestamps[i - 1] + 1);
          minLatencies.push(null);
          maxLatencies.push(null);
          avgLatencies.push(null);
          states.push(null);
        }
      }

      timestamps.push(rawTimestamps[i]);
      minLatencies.push(p.min_latency_ms);
      maxLatencies.push(p.max_latency_ms);
      avgLatencies.push(p.avg_latency_ms);
      states.push(p.uptime_ratio >= 0.5 ? 1 : 0);
    }

    return [timestamps, minLatencies, maxLatencies, avgLatencies, states];
  }

  // ---------------------------------------------------------------------------
  // Compute average latency from data
  // ---------------------------------------------------------------------------

  function computeAvgLatency(): number | null {
    if (isAggregated && aggregatedPoints && aggregatedPoints.length > 0) {
      let sum = 0;
      let count = 0;
      for (const p of aggregatedPoints) {
        if (p.avg_latency_ms != null) {
          sum += p.avg_latency_ms;
          count++;
        }
      }
      return count > 0 ? sum / count : null;
    }
    if (points && points.length > 0) {
      let sum = 0;
      let count = 0;
      for (const p of points) {
        if (p.state === 'up' && p.latency_ms != null) {
          sum += p.latency_ms;
          count++;
        }
      }
      return count > 0 ? sum / count : null;
    }
    return null;
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
              const val = data[i];

              // Skip gap-breaker null points — don't paint the band during gaps
              if (val === null || val === undefined) continue;

              const nextVal = data[i + 1];
              // If the next point is a gap-breaker, don't extend the band into the gap
              if (nextVal === null || nextVal === undefined) continue;

              const x0 = u.valToPos(xData[i], 'x', true);
              const x1 = u.valToPos(xData[i + 1], 'x', true);

              if (val >= 0.9) {
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

            let content = `<div class="text-sm font-mono">${timeStr}</div>`;

            if (isAggregated) {
              const min = u.data[1]?.[idx];
              const max = u.data[2]?.[idx];
              const avg = u.data[3]?.[idx];
              const stateVal = u.data[4]?.[idx];
              content += `<div class="text-sm">Avg: ${avg != null ? formatLatency(avg) : 'N/A'}</div>`;
              content += `<div class="text-sm">Min: ${min != null ? formatLatency(min) : 'N/A'} / Max: ${max != null ? formatLatency(max) : 'N/A'}</div>`;
              content += `<div class="text-sm">State: ${stateLabel(stateVal)}</div>`;
            } else {
              const latency = u.data[1]?.[idx];
              const stateVal = u.data[2]?.[idx];
              content += `<div class="text-sm">Latency: ${latency != null ? formatLatency(latency) : 'N/A'}</div>`;
              content += `<div class="text-sm">State: ${stateLabel(stateVal)}</div>`;
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

    // Compute average latency for the legend
    avgLatency = computeAvgLatency();

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
          spanGaps: false,
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'Max',
          stroke: 'rgba(59, 130, 246, 0.3)',
          width: 1,
          fill: 'rgba(59, 130, 246, 0.05)',
          spanGaps: false,
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'Avg',
          stroke: '#3b82f6',
          width: 2,
          spanGaps: false,
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
          spanGaps: false,
          points: { size: 6, fill: '#3b82f6' },
          value: (_u: uPlot, val: number | null) => val == null ? '--' : formatLatency(val)
        },
        {
          label: 'State',
          show: false // hidden from chart, used by plugin
        }
      ];
    }

    if (chartData[0].length === 0) return;

    // Determine X-axis bounds: use the requested time range (from/to props)
    // so that different presets show different scale even with same data.
    const dataXMin = chartData[0][0];
    const dataXMax = chartData[0][chartData[0].length - 1];
    const rangeXMin = from ? Math.floor(new Date(from).getTime() / 1000) : dataXMin;
    const rangeXMax = to ? Math.floor(new Date(to).getTime() / 1000) : dataXMax;

    fullXMin = rangeXMin;
    fullXMax = rangeXMax;

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
      legend: { show: false },
      axes: [
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke + '40' },
          font: '12px system-ui, -apple-system, sans-serif',
          space: 80,
          values: (_u: uPlot, splits: number[]) => {
            const rangeSec = rangeXMax - rangeXMin;
            return splits.map((ts) => {
              const d = new Date(ts * 1000);
              const pad = (n: number) => String(n).padStart(2, '0');
              const hhmm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
              if (rangeSec <= 86400) {
                // Up to 24h: show HH:MM
                return hhmm;
              }
              // More than 24h: show MM/DD HH:MM
              return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${hhmm}`;
            });
          }
        },
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke + '40' },
          font: '12px system-ui, -apple-system, sans-serif',
          gap: 8,
          size: 64,
          values: (_u: uPlot, splits: number[]) => splits.map((v) => v == null ? '' : formatLatency(v))
        }
      ],
      scales: {
        x: {
          time: true,
          min: rangeXMin,
          max: rangeXMax
        },
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
    from;
    to;

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

    <!-- Custom legend below the chart -->
    <div class="mt-3 flex items-center justify-center gap-6 text-sm text-[var(--color-text-secondary)]">
      {#if tooltipVisible}
        <span class="font-mono">{@html tooltipContent.replace(/<div[^>]*>/g, '').replace(/<\/div>/g, ' · ').replace(/ · $/, '')}</span>
      {:else}
        <span class="flex items-center gap-1.5">
          <span class="inline-block h-2.5 w-2.5 rounded-full bg-[#3b82f6]"></span>
          {t('history.chart.avgLatency')}: {avgLatency != null ? formatLatency(avgLatency) : '--'}
        </span>
        <span class="flex items-center gap-1.5">
          <span class="inline-block h-2.5 w-2.5 rounded-full bg-[#10b981]"></span>
          {t('history.chart.checksUp', { count: points ? points.filter(p => p.state === 'up').length : 0 })}
        </span>
      {/if}
    </div>
  {/if}
</div>
