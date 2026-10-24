<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 6, 7, 8).
   */
  import type { ActiveIncident, PeriodIncident } from '$lib/types';

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
  export function formatDuration(startedAt: string, resolvedAt?: string | null, now = Date.now()): string {
    const start = new Date(startedAt).getTime();
    const end = resolvedAt ? new Date(resolvedAt).getTime() : now;
    const diffMs = Math.max(0, end - start);
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
  export function sortIncidents<T extends ActiveIncident | PeriodIncident>(incidents: T[]): T[] {
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
  import { resolve } from '$app/paths';
  /**
   * IncidentsPanel — displays active incidents (down monitors) with live durations.
   *
   * Requirements 3.1: Display name, elapsed duration, truncated cause
   * Requirements 3.2: Empty state when no incidents
   * Requirements 3.3: Order by duration descending, name tiebreaker
   * Requirements 3.6: Cap at 10 entries with overflow indicator
   * Requirements 3.7: Auto-refresh durations every 60 seconds
   */
  import { t, getLocale } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    incidents: (ActiveIncident | PeriodIncident)[];
    historical?: boolean;
    periodFrom?: string;
    periodTo?: string;
    totalCount?: number;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  const MAX_VISIBLE = 10;

  let { incidents, totalCount = incidents.length, historical = false, periodFrom, periodTo, loading, error, onRetry }: Props = $props();

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
    return historical ? [...incidents].sort((a, b) => Number(!!resolvedAt(a)) - Number(!!resolvedAt(b)) || Date.parse(b.started_at) - Date.parse(a.started_at)) : sortIncidents(incidents);
  });

  let visible = $derived(sorted.slice(0, MAX_VISIBLE));
  let overflowCount = $derived(Math.max(0, Math.max(totalCount, sorted.length) - MAX_VISIBLE));
  let isEmpty = $derived(incidents.length === 0);
  const now = $derived.by(() => { void tick; return Date.now(); });
  function resolvedAt(incident: ActiveIncident | PeriodIncident): string | null {
    return 'resolved_at' in incident ? incident.resolved_at : null;
  }
  function dateText(value: string): string {
    return new Intl.DateTimeFormat(getLocale(), {dateStyle: 'medium', timeStyle: 'short'}).format(new Date(value));
  }
  function elapsedText(incident: ActiveIncident | PeriodIncident): string {
    const end = resolvedAt(incident);
    const seconds = Math.max(0, Math.floor(((end ? Date.parse(end) : now) - Date.parse(incident.started_at)) / 1000));
    if (seconds < 60) return t('dashboard.incidents.durationSeconds', {count: seconds});
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return t('dashboard.incidents.durationMinutes', {count: minutes});
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return t('dashboard.incidents.durationHours', {hours, minutes: minutes % 60});
    return t('dashboard.incidents.durationDays', {days: Math.floor(hours / 24), hours: hours % 24});
  }
  const timeZone = new Intl.DateTimeFormat().resolvedOptions().timeZone;
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-3 p-5" data-testid="incidents-panel">
    <h3 class="pulse-eyebrow">
      {t(historical ? 'dashboard.incidents.periodTitle' : 'dashboard.incidents.title')}
    </h3>

    {#if historical && periodFrom && periodTo}
      <p class="text-xs" style="color: var(--color-text-secondary)">
        {t('dashboard.incidents.range', {from: dateText(periodFrom), to: dateText(periodTo), zone: timeZone})}
      </p>
    {/if}
    {#if isEmpty}
      <p
        class="py-4 text-center text-sm"
        style="color: var(--color-text-secondary)"
        data-testid="incidents-empty"
      >
        {t(historical ? 'dashboard.incidents.periodEmpty' : 'dashboard.incidents.allOperational')}
      </p>
    {:else}
      <ul class="flex flex-col gap-1" role="list" data-testid="incidents-list">
        {#each visible as incident ('id' in incident ? incident.id : incident.monitor_id + '-' + incident.started_at)}
          <li
            class="flex flex-col gap-0.5 rounded-md px-3 py-2"
            style="background-color: var(--color-bg-secondary)"
          >
            <div class="flex items-center justify-between gap-2">
              <a class="incident-link truncate text-sm font-medium" href={resolve('/monitors/[id]', { id: incident.monitor_id })} title={incident.monitor_name}>
                {incident.monitor_name}
              </a>
              <span
                class="shrink-0 text-xs font-medium tabular-nums"
                style:color={resolvedAt(incident) ? 'var(--color-text-secondary)' : 'var(--color-error)'}
                data-testid="incident-duration"
                title={t('dashboard.incidents.duration')}
              >
                {elapsedText(incident)}
              </span>
            </div>
            {#if historical}
              <span class="text-xs font-medium" style:color={resolvedAt(incident) ? 'var(--color-success)' : 'var(--color-error)'}>
                {t(resolvedAt(incident) ? 'dashboard.incidents.resolved' : 'dashboard.incidents.ongoing')}
              </span>
              <dl class="incident-dates">
                <div><dt>{t('dashboard.incidents.started')}</dt><dd><time datetime={incident.started_at}>{dateText(incident.started_at)}</time></dd></div>
                <div><dt>{t('dashboard.incidents.ended')}</dt><dd>
                  {#if resolvedAt(incident)}<time datetime={resolvedAt(incident)!}>{dateText(resolvedAt(incident)!)}</time>
                  {:else}{t('dashboard.incidents.ongoing')}{/if}
                </dd></div>
              </dl>
            {/if}
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

<style>
 .incident-link { color: var(--color-text-primary); text-decoration: none; }
 .incident-link:hover { color: var(--color-brand-primary); text-decoration: underline; }
 .incident-link:focus-visible { outline: 2px solid var(--color-brand-primary); outline-offset: 2px; }
 .incident-dates { display: flex; flex-wrap: wrap; gap: 0.35rem 1.25rem; font-size: 0.75rem; color: var(--color-text-secondary); }
 .incident-dates > div { display: flex; flex-wrap: wrap; gap: 0.35rem; }
 .incident-dates dd { font-variant-numeric: tabular-nums; color: var(--color-text-primary); }
</style>
