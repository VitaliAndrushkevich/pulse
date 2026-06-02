<script lang="ts">
  import type { HistoryPoint } from '$lib/types';

  interface Props {
    data: HistoryPoint[];
  }

  let { data }: Props = $props();

  interface Segment {
    state: 'up' | 'down';
    startTime: number;
    endTime: number;
    widthPercent: number;
    points: number;
  }

  let segments = $derived.by(() => {
    if (data.length === 0) return [] as Segment[];

    // Sort by time ascending
    const sorted = [...data].sort(
      (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
    );

    const totalStart = new Date(sorted[0].checked_at).getTime();
    const totalEnd = new Date(sorted[sorted.length - 1].checked_at).getTime();
    const totalDuration = totalEnd - totalStart;

    if (totalDuration === 0) {
      // Single point or all at the same time
      return [
        {
          state: sorted[0].state,
          startTime: totalStart,
          endTime: totalEnd,
          widthPercent: 100,
          points: sorted.length
        }
      ] as Segment[];
    }

    // Build contiguous segments of same state
    const result: Segment[] = [];
    let currentState = sorted[0].state;
    let segStart = totalStart;
    let pointCount = 1;

    for (let i = 1; i < sorted.length; i++) {
      const pt = sorted[i];
      const ptState = pt.state as 'up' | 'down';

      if (ptState !== currentState) {
        // End of current segment — boundary is midpoint between last and current
        const prevTime = new Date(sorted[i - 1].checked_at).getTime();
        const currTime = new Date(pt.checked_at).getTime();
        const boundary = prevTime + (currTime - prevTime) / 2;

        result.push({
          state: currentState,
          startTime: segStart,
          endTime: boundary,
          widthPercent: ((boundary - segStart) / totalDuration) * 100,
          points: pointCount
        });

        currentState = ptState;
        segStart = boundary;
        pointCount = 1;
      } else {
        pointCount++;
      }
    }

    // Final segment
    result.push({
      state: currentState,
      startTime: segStart,
      endTime: totalEnd,
      widthPercent: ((totalEnd - segStart) / totalDuration) * 100,
      points: pointCount
    });

    return result;
  });

  let timeRange = $derived.by(() => {
    if (data.length === 0) return { start: '', end: '' };
    const sorted = [...data].sort(
      (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
    );
    return {
      start: formatTime(new Date(sorted[0].checked_at)),
      end: formatTime(new Date(sorted[sorted.length - 1].checked_at))
    };
  });

  let uptimePercent = $derived.by(() => {
    if (data.length === 0) return null;
    const upCount = data.filter((p) => p.state === 'up').length;
    return ((upCount / data.length) * 100).toFixed(1);
  });

  function formatTime(date: Date): string {
    return date.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  function formatDateTime(ts: number): string {
    const d = new Date(ts);
    return d.toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<div class="w-full" data-testid="status-timeline">
  {#if data.length === 0}
    <div class="flex items-center justify-center rounded-lg border border-slate-200 bg-slate-50 py-8 text-sm text-slate-500">
      No check data available for the selected time range
    </div>
  {:else}
    <!-- Uptime summary -->
    <div class="mb-3 flex items-center justify-between">
      <div class="flex items-center gap-4">
        {#if uptimePercent !== null}
          <span class="text-sm font-medium text-slate-700">
            Uptime: <span class="font-semibold {Number(uptimePercent) >= 99 ? 'text-emerald-600' : Number(uptimePercent) >= 95 ? 'text-amber-600' : 'text-rose-600'}">{uptimePercent}%</span>
          </span>
        {/if}
        <div class="flex items-center gap-3 text-xs text-slate-500">
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-emerald-500"></span> Healthy
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-rose-500"></span> Unhealthy
          </span>
        </div>
      </div>
      <span class="text-xs text-slate-400">{data.length} checks</span>
    </div>

    <!-- Timeline bar -->
    <div class="relative h-8 w-full overflow-hidden rounded-md border border-slate-200 bg-slate-100" data-testid="timeline-bar">
      <div class="absolute inset-0 flex">
        {#each segments as segment, i (i)}
          <div
            class="h-full transition-all {segment.state === 'up' ? 'bg-emerald-500 hover:bg-emerald-400' : 'bg-rose-500 hover:bg-rose-400'}"
            style="width: {segment.widthPercent}%"
            title="{segment.state === 'up' ? 'Healthy' : 'Unhealthy'} — {formatDateTime(segment.startTime)} → {formatDateTime(segment.endTime)} ({segment.points} check{segment.points > 1 ? 's' : ''})"
            role="img"
            aria-label="{segment.state === 'up' ? 'Healthy' : 'Unhealthy'} from {formatDateTime(segment.startTime)} to {formatDateTime(segment.endTime)}"
          ></div>
        {/each}
      </div>
    </div>

    <!-- Time labels -->
    <div class="mt-1.5 flex items-center justify-between text-xs text-slate-400">
      <span>{timeRange.start}</span>
      <span>{timeRange.end}</span>
    </div>
  {/if}
</div>
