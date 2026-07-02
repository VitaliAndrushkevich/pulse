<script lang="ts">
  import type { Monitor } from '$lib/types';
  import { formatDate } from '$lib/format';
  import { t } from '$lib/i18n';

  interface Props {
    monitor: Monitor;
  }

  let { monitor }: Props = $props();

  const isPaused = $derived(monitor.status === 'paused');

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    unknown: 'bg-slate-400'
  };

  const stateTranslationKeys: Record<string, string> = {
    up: 'monitors.status.up',
    down: 'monitors.status.down',
    unknown: 'monitors.status.unknown'
  };

  const typeBadgeColors: Record<string, string> = {
    http: 'bg-blue-100 text-blue-700',
    http3: 'bg-cyan-100 text-cyan-700',
    tcp: 'bg-purple-100 text-purple-700',
    udp: 'bg-amber-100 text-amber-700',
    websocket: 'bg-pink-100 text-pink-700',
    grpc: 'bg-indigo-100 text-indigo-700'
  };
</script>

<a
  href="/monitors/{monitor.id}"
  class="flex h-16 items-center gap-4 border-b border-[var(--color-border)] bg-surface px-4 transition hover:bg-[var(--color-bg-surface-hover)] {isPaused ? 'opacity-60' : ''}"
  data-testid="monitor-row"
>
  <!-- State indicator dot -->
  <span
    class="h-3 w-3 flex-shrink-0 rounded-full {isPaused ? 'bg-slate-400' : (stateColors[monitor.state] ?? 'bg-slate-400')}"
    title={isPaused ? t('monitors.status.paused') : t(stateTranslationKeys[monitor.state] ?? 'monitors.status.unknown')}
    data-testid="state-indicator"
    data-state={isPaused ? 'paused' : monitor.state}
  ></span>

  <!-- Name -->
  <span class="min-w-0 flex-1 truncate text-sm font-medium text-primary" data-testid="monitor-name">
    {monitor.name}
  </span>

  <!-- Paused badge -->
  {#if isPaused}
    <span
      class="flex-shrink-0 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600"
      data-testid="paused-badge"
    >
      {t('monitors.status.paused')}
    </span>
  {/if}

  <!-- Type badge -->
  <span
    class="flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {typeBadgeColors[monitor.type] ?? 'bg-[var(--color-bg-surface-hover)] text-primary'}"
    data-testid="monitor-type"
  >
    {monitor.type === 'http' ? 'HTTP(S)' : monitor.type === 'http3' ? 'HTTP/3' : monitor.type === 'grpc' ? 'gRPC' : monitor.type.toUpperCase()}
  </span>

  <!-- Target -->
  <span class="hidden min-w-0 max-w-48 flex-shrink truncate text-sm text-secondary sm:inline" data-testid="monitor-target">
    {monitor.target}
  </span>

  <!-- Last checked -->
  <span class="flex-shrink-0 text-xs text-[var(--color-text-muted)]" data-testid="monitor-last-checked">
    {isPaused ? t('monitors.status.paused') : formatDate(monitor.last_checked_at)}
  </span>
</a>
