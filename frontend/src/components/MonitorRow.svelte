<script lang="ts">
  import type { Monitor } from '$lib/types';
  import { formatDate } from '$lib/format';

  interface Props {
    monitor: Monitor;
  }

  let { monitor }: Props = $props();

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    unknown: 'bg-slate-400'
  };

  const stateLabels: Record<string, string> = {
    up: 'Up',
    down: 'Down',
    unknown: 'Unknown'
  };

  const typeBadgeColors: Record<string, string> = {
    http: 'bg-blue-100 text-blue-700',
    tcp: 'bg-purple-100 text-purple-700',
    udp: 'bg-amber-100 text-amber-700',
    websocket: 'bg-pink-100 text-pink-700'
  };
</script>

<a
  href="/monitors/{monitor.id}"
  class="flex h-16 items-center gap-4 border-b border-slate-200 bg-white px-4 transition hover:bg-slate-50"
  data-testid="monitor-row"
>
  <!-- State indicator dot -->
  <span
    class="h-3 w-3 flex-shrink-0 rounded-full {stateColors[monitor.state] ?? 'bg-slate-400'}"
    title={stateLabels[monitor.state] ?? 'Unknown'}
    data-testid="state-indicator"
    data-state={monitor.state}
  ></span>

  <!-- Name -->
  <span class="min-w-0 flex-1 truncate text-sm font-medium text-slate-900" data-testid="monitor-name">
    {monitor.name}
  </span>

  <!-- Type badge -->
  <span
    class="flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {typeBadgeColors[monitor.type] ?? 'bg-slate-100 text-slate-700'}"
    data-testid="monitor-type"
  >
    {monitor.type === 'http' ? 'HTTP(S)' : monitor.type.toUpperCase()}
  </span>

  <!-- Target -->
  <span class="hidden min-w-0 max-w-48 flex-shrink truncate text-sm text-slate-500 sm:inline" data-testid="monitor-target">
    {monitor.target}
  </span>

  <!-- Last checked -->
  <span class="flex-shrink-0 text-xs text-slate-400" data-testid="monitor-last-checked">
    {formatDate(monitor.last_checked_at)}
  </span>
</a>
