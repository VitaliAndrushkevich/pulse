<script lang="ts">
  import type { HistoryPoint } from '$lib/types';

  interface Props {
    data: HistoryPoint[];
    /** Maximum number of bars to display. Older points are bucketed if data exceeds this. */
    maxBars?: number;
  }

  let { data, maxBars = 90 }: Props = $props();

  interface Beat {
    state: 'up' | 'down' | 'mixed';
    points: HistoryPoint[];
    startTime: number;
    endTime: number;
  }

  // Popover state
  let activePopoverIndex = $state<number | null>(null);
  let popoverElement = $state<HTMLElement | null>(null);

  function handleBeatClick(beat: Beat, index: number, event: MouseEvent): void {
    event.stopPropagation();
    if (beat.state === 'up') {
      activePopoverIndex = null;
      return;
    }
    activePopoverIndex = activePopoverIndex === index ? null : index;
  }

  function handleBeatKeydown(beat: Beat, index: number, event: KeyboardEvent): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      if (beat.state === 'up') return;
      activePopoverIndex = activePopoverIndex === index ? null : index;
    }
  }

  function closePopover(): void {
    activePopoverIndex = null;
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape' && activePopoverIndex !== null) {
      closePopover();
    }
  }

  function handleClickOutside(event: MouseEvent): void {
    if (activePopoverIndex === null) return;
    const target = event.target as HTMLElement;
    if (!target.closest('[data-testid="beat-popover"]') && !target.closest('[data-beat]')) {
      closePopover();
    }
  }

  /**
   * Bucket data into fixed-count beats.
   * Each beat represents a time slice and aggregates points within it.
   */
  let beats = $derived.by((): Beat[] => {
    if (data.length === 0) return [];

    const sorted = [...data].sort(
      (a, b) => new Date(a.checked_at).getTime() - new Date(b.checked_at).getTime()
    );

    // If data fits within maxBars, each point becomes its own beat
    if (sorted.length <= maxBars) {
      return sorted.map((pt) => ({
        state: pt.state,
        points: [pt],
        startTime: new Date(pt.checked_at).getTime(),
        endTime: new Date(pt.checked_at).getTime()
      }));
    }

    // Bucket points into maxBars time slices
    const totalStart = new Date(sorted[0].checked_at).getTime();
    const totalEnd = new Date(sorted[sorted.length - 1].checked_at).getTime();
    const sliceDuration = (totalEnd - totalStart) / maxBars;

    const result: Beat[] = [];
    let bucketIndex = 0;
    let currentBucket: HistoryPoint[] = [];
    let bucketStart = totalStart;

    for (const pt of sorted) {
      const ptTime = new Date(pt.checked_at).getTime();
      const targetBucket = Math.min(Math.floor((ptTime - totalStart) / sliceDuration), maxBars - 1);

      while (bucketIndex < targetBucket) {
        // Close current bucket
        if (currentBucket.length > 0) {
          result.push(makeBeat(currentBucket, bucketStart, bucketStart + sliceDuration));
        } else {
          // Empty bucket — no data for this slice, skip it
        }
        bucketIndex++;
        bucketStart = totalStart + bucketIndex * sliceDuration;
        currentBucket = [];
      }

      currentBucket.push(pt);
    }

    // Final bucket
    if (currentBucket.length > 0) {
      result.push(makeBeat(currentBucket, bucketStart, totalEnd));
    }

    return result;
  });

  function makeBeat(points: HistoryPoint[], startTime: number, endTime: number): Beat {
    const hasDown = points.some((p) => p.state === 'down');
    const hasUp = points.some((p) => p.state === 'up');

    let state: 'up' | 'down' | 'mixed';
    if (hasDown && hasUp) {
      state = 'mixed';
    } else if (hasDown) {
      state = 'down';
    } else {
      state = 'up';
    }

    return { state, points, startTime, endTime };
  }

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
      minute: '2-digit',
      second: '2-digit'
    });
  }

  function beatColor(state: 'up' | 'down' | 'mixed'): string {
    if (state === 'up') return 'bg-emerald-500 hover:bg-emerald-400';
    if (state === 'down') return 'bg-rose-500 hover:bg-rose-400';
    return 'bg-amber-500 hover:bg-amber-400';
  }

  function beatTooltip(beat: Beat): string {
    if (beat.points.length === 1) {
      const pt = beat.points[0];
      const time = formatDateTime(new Date(pt.checked_at).getTime());
      if (pt.state === 'up') {
        return `${time} — OK${pt.latency_ms != null ? ` (${pt.latency_ms}ms)` : ''}`;
      }
      const parts: string[] = [];
      if (pt.status_code != null) {
        parts.push(`HTTP ${pt.status_code}`);
      }
      if (pt.error) {
        parts.push(pt.error);
      }
      const err = parts.length > 0 ? parts.join(' — ') : 'Unknown error';
      return `${time} — ${err}`;
    }

    // Multiple points in bucket
    const downCount = beat.points.filter((p) => p.state === 'down').length;
    const total = beat.points.length;
    if (beat.state === 'up') {
      return `${total} checks — all healthy`;
    }
    return `${downCount}/${total} checks failed`;
  }

  function getPopoverFailures(beat: Beat): { error: string; status_code: number | null; checked_at: string }[] {
    return beat.points
      .filter((p) => p.state === 'down')
      .map((p) => ({
        error: p.error ?? (p.status_code != null ? `HTTP ${p.status_code}` : 'Unknown error'),
        status_code: p.status_code,
        checked_at: p.checked_at
      }));
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="w-full" data-testid="status-timeline" onkeydown={handleKeydown} onclick={handleClickOutside}>
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
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-rose-500"></span> Down
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-amber-500"></span> Mixed
          </span>
        </div>
      </div>
      <span class="text-xs text-slate-400">{data.length} checks</span>
    </div>

    <!-- Heartbeat bar -->
    <div class="relative" data-testid="timeline-bar">
      <div class="flex items-end gap-[2px]">
        {#each beats as beat, i (i)}
          <button
            type="button"
            class="flex-1 min-w-[3px] h-8 rounded-sm transition-all {beatColor(beat.state)} {beat.state !== 'up' ? 'cursor-pointer' : 'cursor-default'} {activePopoverIndex === i ? 'ring-2 ring-slate-400 ring-offset-1' : ''}"
            title={beatTooltip(beat)}
            aria-label={beatTooltip(beat)}
            data-beat
            onclick={(e) => handleBeatClick(beat, i, e)}
            onkeydown={(e) => handleBeatKeydown(beat, i, e)}
          ></button>
        {/each}
      </div>

      <!-- Popover for clicked beat -->
      {#each beats as beat, i (i)}
        {#if activePopoverIndex === i && beat.state !== 'up'}
          {@const leftPercent = ((i + 0.5) / beats.length) * 100}
          {@const failures = getPopoverFailures(beat)}
          <div
            class="absolute bottom-full z-50 mb-2 w-72 -translate-x-1/2 rounded-lg border border-slate-200 bg-white p-3 shadow-lg"
            style="left: clamp(8rem, {leftPercent}%, calc(100% - 8rem))"
            data-testid="beat-popover"
            bind:this={popoverElement}
            onclick={(e) => e.stopPropagation()}
            onkeydown={(e) => e.stopPropagation()}
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-semibold {beat.state === 'down' ? 'text-rose-600' : 'text-amber-600'}">
                {beat.state === 'down' ? 'Down' : 'Degraded'}
              </span>
              <button
                type="button"
                class="text-slate-400 hover:text-slate-600"
                onclick={closePopover}
                aria-label="Close popover"
              >
                ✕
              </button>
            </div>

            {#if beat.points.length === 1}
              <!-- Single check — show full details -->
              {@const pt = beat.points[0]}
              <div class="space-y-1.5">
                <div class="text-xs text-slate-500">
                  {formatDateTime(new Date(pt.checked_at).getTime())}
                </div>
                {#if pt.error}
                  <p class="text-xs font-medium text-slate-800">{pt.error}</p>
                {/if}
                {#if pt.status_code != null}
                  <p class="text-xs text-slate-600">HTTP {pt.status_code}</p>
                {/if}
                {#if pt.latency_ms != null}
                  <p class="text-xs text-slate-500">Response: {pt.latency_ms}ms</p>
                {/if}
              </div>
            {:else}
              <!-- Multiple checks in bucket — show failures list -->
              <div class="mb-2 text-xs text-slate-500">
                {beat.points.filter(p => p.state === 'down').length} of {beat.points.length} checks failed
              </div>
              {#if failures.length > 0}
                <div class="max-h-40 space-y-2 overflow-y-auto">
                  {#each failures.slice(0, 5) as failure}
                    <div class="rounded border border-slate-100 bg-slate-50 px-2 py-1.5">
                      <div class="text-[10px] text-slate-400">{formatDateTime(new Date(failure.checked_at).getTime())}</div>
                      <div class="text-xs text-slate-700">{failure.error}</div>
                    </div>
                  {/each}
                  {#if failures.length > 5}
                    <p class="text-[10px] text-slate-400">+ {failures.length - 5} more</p>
                  {/if}
                </div>
              {/if}
            {/if}
          </div>
        {/if}
      {/each}
    </div>

    <!-- Time labels -->
    <div class="mt-1.5 flex items-center justify-between text-xs text-slate-400">
      <span>{timeRange.start}</span>
      <span>{timeRange.end}</span>
    </div>
  {/if}
</div>
