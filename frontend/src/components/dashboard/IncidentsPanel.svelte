<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 6, 7, 8).
   */
  import type { ActiveIncident } from '$lib/types';

  /**
   * Truncates a cause string to the given limit, appending "…" if exceeded.
   * Returns empty string for null/undefined causes.
   *
   * Validates: Requirements 3.1
   */
  export function truncateCause(cause: string | null, limit: number = 120): string {
    if (cause === null || cause === undefined) return '';
    if (cause.length <= limit) return cause;
    return cause.slice(0, limit) + '…';
  }

  /**
   * Formats duration from a started_at ISO timestamp to a human-readable string.
   * Examples: "2h 15m", "3d 4h", "45m", "30s"
   *
   * Validates: Requirements 3.1, 3.7
   */
  export function formatDuration(startedAt: string): string {
    const start = new Date(startedAt).getTime();
    const now = Date.now();
    const diffMs = Math.max(0, now - start);
    const totalSeconds = Math.floor(diffMs / 1000);

    if (totalSeconds < 60) {
      return `${totalSeconds}s`;
    }

    const totalMinutes = Math.floor(totalSeconds / 60);
    if (totalMinutes < 60) {
      return `${totalMinutes}m`;
    }

    const hours = Math.floor(totalMinutes / 60);
    const minutes = totalMinutes % 60;
    if (hours < 24) {
      return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
    }

    const days = Math.floor(hours / 24);
    const remainingHours = hours % 24;
    return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
  }

  /**
   * Sorts incidents by elapsed duration descending (longest first),
   * with alphabetical monitor name as tiebreaker.
   *
   * Validates: Requirements 3.3
   */
  export function sortIncidents(incidents: ActiveIncident[]): ActiveIncident[] {
    const now = Date.now();
    return [...incidents].sort((a, b) => {
      const durationA = now - new Date(a.started_at).getTime();
      const durationB = now - new Date(b.started_at).getTime();
      if (durationA !== durationB) return durationB - durationA;
      return a.monitor_name.localeCompare(b.monitor_name);
    });
  }
</script>

<script lang="ts">
  /**
   * IncidentsPanel — displays active incidents (down monitors) with live durations.
   *
   * Requirements 3.1: Display name, elapsed duration, truncated cause
   * Requirements 3.2: Empty state when no incidents
   * Requirements 3.3: Order by duration descending, name tiebreaker
   * Requirements 3.6: Cap at 10 entries with overflow indicator
   * Requirements 3.7: Auto-refresh durations every 60 seconds
   */
  import type { ActiveIncident } from '$lib/types';
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    incidents: ActiveIncident[];
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  const MAX_VISIBLE = 10;

  let { incidents, loading, error, onRetry }: Props = $props();

  // Tick counter to force duration recalculation every 60s
  let tick = $state(0);

  $effect(() => {
    const interval = setInterval(() => {
      tick++;
    }, 60_000);
    return () => clearInterval(interval);
  });

  // Sort and cap entries reactively (tick dependency forces refresh)
  let sorted = $derived.by(() => {
    // Reference tick to create reactive dependency
    void tick;
    return sortIncidents(incidents);
  });

  let visible = $derived(sorted.slice(0, MAX_VISIBLE));
  let overflowCount = $derived(Math.max(0, sorted.length - MAX_VISIBLE));
  let isEmpty = $derived(incidents.length === 0);
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-2 p-4" data-testid="incidents-panel">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.incidents.title')}
    </h3>

    {#if isEmpty}
      <p
        class="py-4 text-center text-sm"
        style="color: var(--color-text-secondary)"
        data-testid="incidents-empty"
      >
        {t('dashboard.incidents.allOperational')}
      </p>
    {:else}
      <ul class="flex flex-col gap-1" role="list" data-testid="incidents-list">
        {#each visible as incident, idx (incident.monitor_id + '-' + incident.started_at)}
          <li
            class="flex flex-col gap-0.5 rounded-md px-3 py-2"
            style="background-color: var(--color-bg-secondary)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)">
                {incident.monitor_name}
              </span>
              <span
                class="shrink-0 text-xs font-medium tabular-nums"
                style="color: var(--color-error)"
                data-testid="incident-duration"
              >
                {formatDuration(incident.started_at)}
              </span>
            </div>
            {#if incident.cause}
              <span class="text-xs" style="color: var(--color-text-secondary)">
                {truncateCause(incident.cause)}
              </span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if overflowCount > 0}
        <p
          class="text-center text-xs font-medium"
          style="color: var(--color-text-secondary)"
          data-testid="incidents-overflow"
        >
          {t('dashboard.incidents.overflow', { count: String(overflowCount) })}
        </p>
      {/if}
    {/if}
  </div>
</WidgetShell>
