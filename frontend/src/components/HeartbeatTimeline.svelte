<script lang="ts">
  import type { HeartbeatEvent, HeartbeatEventType, HeartbeatHistorySummary } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    events: HeartbeatEvent[];
    summary: HeartbeatHistorySummary;
    loading: boolean;
    error: string | null;
    from: string;
    to: string;
    /** Bucket events into fixed-count markers once the event count exceeds this. */
    maxMarkers?: number;
  }

  let { events, summary, loading, error, from, to, maxMarkers = 120 }: Props = $props();

  // A bucket is a contiguous slice of the timeline carrying one or more events.
  // Its state is the WORST event inside it so problems are never hidden.
  interface Bucket {
    state: HeartbeatEventType;
    events: HeartbeatEvent[];
    startTime: number;
    endTime: number;
  }

  // success is best, missed worst. Used both for bucket colour and ordering.
  const SEVERITY: Record<HeartbeatEventType, number> = {
    success: 0,
    fail: 1,
    missed: 2,
  };

  let activePopoverIndex = $state<number | null>(null);

  function isProblem(state: HeartbeatEventType): boolean {
    return state === 'fail' || state === 'missed';
  }

  function handleMarkerClick(bucket: Bucket, index: number, event: MouseEvent): void {
    event.stopPropagation();
    if (!isProblem(bucket.state)) {
      activePopoverIndex = null;
      return;
    }
    activePopoverIndex = activePopoverIndex === index ? null : index;
  }

  function handleMarkerKeydown(bucket: Bucket, index: number, event: KeyboardEvent): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      if (!isProblem(bucket.state)) return;
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
    if (!target.closest('[data-testid="hb-event-popover"]') && !target.closest('[data-hb-marker]')) {
      closePopover();
    }
  }

  function worstState(list: HeartbeatEvent[]): HeartbeatEventType {
    let worst: HeartbeatEventType = 'success';
    for (const ev of list) {
      if (SEVERITY[ev.event_type] > SEVERITY[worst]) {
        worst = ev.event_type;
      }
    }
    return worst;
  }

  // Events are already ascending by occurred_at from the API, but sort defensively.
  let sortedEvents = $derived(
    [...events].sort((a, b) => new Date(a.occurred_at).getTime() - new Date(b.occurred_at).getTime())
  );

  let buckets = $derived.by((): Bucket[] => {
    const list = sortedEvents;
    if (list.length === 0) return [];

    // Below the threshold: one marker per event.
    if (list.length <= maxMarkers) {
      return list.map((ev) => {
        const ts = new Date(ev.occurred_at).getTime();
        return { state: ev.event_type, events: [ev], startTime: ts, endTime: ts };
      });
    }

    // Above the threshold: bucket by equal time slices over [from, to].
    const windowStart = new Date(from).getTime();
    const windowEnd = new Date(to).getTime();
    const start = Number.isFinite(windowStart) ? windowStart : new Date(list[0].occurred_at).getTime();
    const end = Number.isFinite(windowEnd)
      ? windowEnd
      : new Date(list[list.length - 1].occurred_at).getTime();
    const span = Math.max(end - start, 1);
    const sliceDuration = span / maxMarkers;

    const slots: HeartbeatEvent[][] = Array.from({ length: maxMarkers }, () => []);
    for (const ev of list) {
      const ts = new Date(ev.occurred_at).getTime();
      const idx = Math.min(Math.max(Math.floor((ts - start) / sliceDuration), 0), maxMarkers - 1);
      slots[idx].push(ev);
    }

    const result: Bucket[] = [];
    for (let i = 0; i < maxMarkers; i++) {
      const slot = slots[i];
      if (slot.length === 0) continue; // skip empty slices — no event, no marker
      result.push({
        state: worstState(slot),
        events: slot,
        startTime: start + i * sliceDuration,
        endTime: start + (i + 1) * sliceDuration,
      });
    }
    return result;
  });

  function stateClass(state: HeartbeatEventType): string {
    // Brand/semantic utilities backed by CSS custom properties in app.css.
    if (state === 'success') return 'bg-success';
    if (state === 'missed') return 'bg-warning';
    return 'bg-error';
  }

  function uptimeClass(percent: number): string {
    if (percent >= 99) return 'text-success';
    if (percent >= 95) return 'text-warning';
    return 'text-error';
  }

  function formatTime(ts: number): string {
    if (!Number.isFinite(ts)) return '';
    return new Date(ts).toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  function eventLabel(ev: HeartbeatEvent): string {
    const time = formatTime(new Date(ev.occurred_at).getTime());
    if (ev.event_type === 'success') return t('heartbeats.timeline.eventSuccessAt', { time });
    if (ev.event_type === 'missed') return t('heartbeats.timeline.eventMissedAt', { time });
    return t('heartbeats.timeline.eventFailAt', { time });
  }

  // Only problem events are listed in a popover.
  function popoverEvents(bucket: Bucket): HeartbeatEvent[] {
    return bucket.events.filter((ev) => isProblem(ev.event_type));
  }

  let rangeStartLabel = $derived(formatTime(new Date(from).getTime()));
  let rangeEndLabel = $derived(formatTime(new Date(to).getTime()));
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="w-full" data-testid="heartbeat-timeline" onkeydown={handleKeydown} onclick={handleClickOutside}>
  {#if loading}
    <div
      class="flex items-center justify-center rounded-lg border border-border bg-page py-8 text-sm text-secondary"
      data-testid="hb-timeline-loading"
    >
      {t('heartbeats.timeline.loading')}
    </div>
  {:else if error}
    <div
      class="rounded-lg border border-error bg-page p-4 text-center text-sm text-error"
      data-testid="hb-timeline-error"
    >
      {error}
    </div>
  {:else if summary.total_events === 0}
    <div
      class="flex flex-col items-center justify-center gap-1 rounded-lg border border-border bg-page py-8 text-center"
      data-testid="hb-timeline-empty"
    >
      <p class="text-sm font-medium text-primary">{t('heartbeats.timeline.empty')}</p>
      <p class="text-xs text-secondary">{t('heartbeats.timeline.emptyHint')}</p>
    </div>
  {:else}
    <!-- Summary + legend -->
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <div class="flex flex-wrap items-center gap-4">
        <span class="text-sm font-medium text-primary" title={t('heartbeats.timeline.uptimeTooltip')}>
          {t('heartbeats.timeline.uptime')}:
          <span class="font-semibold {summary.uptime_percent === null ? 'text-secondary' : uptimeClass(summary.uptime_percent)}">
            {summary.uptime_percent === null ? '—' : `${summary.uptime_percent.toFixed(1)}%`}
          </span>
        </span>
        <span class="text-sm text-secondary">
          {t('heartbeats.timeline.missed')}:
          <span class="font-semibold text-primary">{summary.missed_count}</span>
        </span>
        <div class="flex items-center gap-3 text-xs text-secondary">
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-success"></span>
            {t('heartbeats.timeline.legendSuccess')}
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-warning"></span>
            {t('heartbeats.timeline.legendMissed')}
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm bg-error"></span>
            {t('heartbeats.timeline.legendFail')}
          </span>
        </div>
      </div>
    </div>

    <!-- Timeline bar -->
    <div class="relative" data-testid="hb-timeline-bar">
      <div class="flex items-end gap-[2px]">
        {#each buckets as bucket, i (i)}
          <button
            type="button"
            class="h-8 min-w-[3px] flex-1 rounded-sm transition-all {stateClass(bucket.state)} {isProblem(bucket.state) ? 'cursor-pointer' : 'cursor-default'} {activePopoverIndex === i ? 'ring-2 ring-brand-400 ring-offset-1' : ''}"
            title={eventLabel(bucket.events[0])}
            aria-label={eventLabel(bucket.events[0])}
            data-hb-marker
            data-state={bucket.state}
            onclick={(e) => handleMarkerClick(bucket, i, e)}
            onkeydown={(e) => handleMarkerKeydown(bucket, i, e)}
          ></button>
        {/each}
      </div>

      {#each buckets as bucket, i (i)}
        {#if activePopoverIndex === i && isProblem(bucket.state)}
          {@const leftPercent = ((i + 0.5) / buckets.length) * 100}
          {@const problems = popoverEvents(bucket)}
          <div
            class="absolute bottom-full z-50 mb-2 w-72 -translate-x-1/2 rounded-lg border border-border bg-surface p-3 shadow-lg"
            style="left: clamp(8rem, {leftPercent}%, calc(100% - 8rem))"
            data-testid="hb-event-popover"
            onclick={(e) => e.stopPropagation()}
            onkeydown={(e) => e.stopPropagation()}
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-semibold {bucket.state === 'missed' ? 'text-warning' : 'text-error'}">
                {bucket.state === 'missed'
                  ? t('heartbeats.timeline.legendMissed')
                  : t('heartbeats.timeline.legendFail')}
              </span>
              <button
                type="button"
                class="text-secondary transition hover:text-primary"
                onclick={closePopover}
                aria-label={t('common.close')}
              >
                ✕
              </button>
            </div>
            <div class="max-h-40 space-y-2 overflow-y-auto">
              {#each problems.slice(0, 5) as ev}
                <div class="rounded border border-border bg-page px-2 py-1.5">
                  <div class="text-xs text-primary">{eventLabel(ev)}</div>
                </div>
              {/each}
              {#if problems.length > 5}
                <p class="text-[10px] text-secondary">+ {problems.length - 5}</p>
              {/if}
            </div>
          </div>
        {/if}
      {/each}
    </div>

    <!-- Range labels -->
    <div class="mt-1.5 flex items-center justify-between text-xs text-secondary">
      <span>{rangeStartLabel}</span>
      <span>{rangeEndLabel}</span>
    </div>
  {/if}
</div>
