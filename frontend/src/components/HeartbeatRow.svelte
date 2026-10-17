<script lang="ts">
  import type { Heartbeat } from '$lib/types';
  import { formatDate } from '$lib/format';
  import { t } from '$lib/i18n';
  import { resolve } from '$app/paths';

  interface Props {
    heartbeat: Heartbeat;
  }

  let { heartbeat }: Props = $props();

  const stateColors: Record<string, string> = {
    up: 'bg-[var(--color-success)]',
    down: 'bg-[var(--color-error)]',
    paused: 'bg-[var(--color-text-muted)]',
    new: 'bg-[var(--color-warning)]'
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
  const stateColor = $derived(stateColors[heartbeat.state] ?? 'bg-[var(--color-text-muted)]');
  const stateLabel = $derived(t(stateTranslationKeys[heartbeat.state] ?? 'heartbeats.state.new'));
</script>

<a
  href={resolve('/heartbeats/[id]', { id: heartbeat.id })}
  class="flex h-16 items-center gap-3 sm:gap-4 border-b border-[var(--color-border)] bg-surface px-4 transition hover:bg-[var(--color-bg-surface-hover)] {isPaused ? 'opacity-60' : ''}"
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
      {#each heartbeat.tags.slice(0, 3) as tag (tag.key)}
        <span class="pulse-tag">
          <span class="font-medium">{tag.key}</span><span class="opacity-50">:</span><span>{tag.value}</span>
        </span>
      {/each}
      {#if heartbeat.tags.length > 3}
        <span class="text-[10px] text-secondary">+{heartbeat.tags.length - 3}</span>
      {/if}
    </div>
  {/if}

  <!-- State badge -->
  <span
    class="pulse-protocol"
    data-testid="heartbeat-state"
  >
    {stateLabel}
  </span>

  <!-- Expected interval -->
  <span class="hidden flex-shrink-0 font-mono text-xs text-secondary lg:inline" data-testid="heartbeat-interval">
    {t('heartbeats.fields.expectedInterval')}: {formatInterval(heartbeat.expected_interval_seconds)}
  </span>

  <!-- Last success -->
  <span class="hidden flex-shrink-0 font-mono text-[10px] text-[var(--color-text-muted)] sm:inline" data-testid="heartbeat-last-success">
    {formatDate(heartbeat.last_success_at, t('common.never'))}
  </span>
</a>
