<script lang="ts">
  import type { Heartbeat } from '$lib/types';
  import { formatDate } from '$lib/format';
  import { t } from '$lib/i18n';

  interface Props {
    heartbeat: Heartbeat;
  }

  let { heartbeat }: Props = $props();

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    paused: 'bg-slate-400',
    new: 'bg-amber-400'
  };

  const stateTranslationKeys: Record<string, string> = {
    up: 'heartbeats.state.up',
    down: 'heartbeats.state.down',
    paused: 'heartbeats.state.paused',
    new: 'heartbeats.state.new'
  };

  // Expected interval formatted as a human-readable duration (seconds → m/h/d).
  function formatInterval(seconds: number): string {
    if (seconds % 86400 === 0) return `${seconds / 86400}d`;
    if (seconds % 3600 === 0) return `${seconds / 3600}h`;
    if (seconds % 60 === 0) return `${seconds / 60}m`;
    return `${seconds}s`;
  }

  const isPaused = $derived(heartbeat.state === 'paused');
  const stateColor = $derived(stateColors[heartbeat.state] ?? 'bg-slate-400');
  const stateLabel = $derived(t(stateTranslationKeys[heartbeat.state] ?? 'heartbeats.state.new'));
</script>

<a
  href="/heartbeats/{heartbeat.id}"
  class="flex h-16 items-center gap-4 border-b border-[var(--color-border)] bg-surface px-4 transition hover:bg-[var(--color-bg-surface-hover)] {isPaused ? 'opacity-60' : ''}"
  data-testid="heartbeat-row"
>
  <!-- State indicator dot -->
  <span
    class="h-3 w-3 flex-shrink-0 rounded-full {stateColor}"
    title={stateLabel}
    data-testid="state-indicator"
    data-state={heartbeat.state}
  ></span>

  <!-- Name -->
  <span class="min-w-0 flex-1 truncate text-sm font-medium text-primary" data-testid="heartbeat-name">
    {heartbeat.name}
  </span>

  <!-- Tags (compact chips) -->
  {#if heartbeat.tags && heartbeat.tags.length > 0}
    <div class="hidden flex-shrink-0 items-center gap-1 md:flex" data-testid="heartbeat-tags">
      {#each heartbeat.tags.slice(0, 3) as tag}
        <span class="inline-flex items-center rounded-full bg-indigo-50 px-1.5 py-0.5 text-[10px] leading-none">
          <span class="font-medium text-indigo-700">{tag.key}</span><span class="text-indigo-400">:</span><span class="text-indigo-600">{tag.value}</span>
        </span>
      {/each}
      {#if heartbeat.tags.length > 3}
        <span class="text-[10px] text-secondary">+{heartbeat.tags.length - 3}</span>
      {/if}
    </div>
  {/if}

  <!-- State badge -->
  <span
    class="flex-shrink-0 rounded-full bg-[var(--color-bg-surface-hover)] px-2 py-0.5 text-xs font-medium text-primary"
    data-testid="heartbeat-state"
  >
    {stateLabel}
  </span>

  <!-- Expected interval -->
  <span class="hidden flex-shrink-0 text-sm text-secondary sm:inline" data-testid="heartbeat-interval">
    {t('heartbeats.fields.expectedInterval')}: {formatInterval(heartbeat.expected_interval_seconds)}
  </span>

  <!-- Last success -->
  <span class="flex-shrink-0 text-xs text-[var(--color-text-muted)]" data-testid="heartbeat-last-success">
    {formatDate(heartbeat.last_success_at, t('common.never'))}
  </span>
</a>
