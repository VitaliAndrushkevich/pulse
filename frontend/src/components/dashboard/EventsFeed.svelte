<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 16, 17, 18).
   */
  import type { RecentEvent } from '$lib/types';

  /**
   * Formats a timestamp as a relative human-readable duration.
   * Examples: "just now", "45s ago", "3m ago", "2h ago", "1d ago"
   *
   * Validates: Requirements 7.2, 9.3
   */
  export function formatRelativeTime(
    occurredAt: string,
    now: number = Date.now()
  ): { key: string; params?: Record<string, string> } {
    const occurred = new Date(occurredAt).getTime();
    const diffMs = Math.max(0, now - occurred);
    const totalSeconds = Math.floor(diffMs / 1000);

    if (totalSeconds < 10) {
      return { key: 'dashboard.events.justNow' };
    }

    if (totalSeconds < 60) {
      return { key: 'dashboard.events.secondsAgo', params: { count: String(totalSeconds) } };
    }

    const totalMinutes = Math.floor(totalSeconds / 60);
    if (totalMinutes < 60) {
      return { key: 'dashboard.events.minutesAgo', params: { count: String(totalMinutes) } };
    }

    const totalHours = Math.floor(totalMinutes / 60);
    if (totalHours < 24) {
      return { key: 'dashboard.events.hoursAgo', params: { count: String(totalHours) } };
    }

    const totalDays = Math.floor(totalHours / 24);
    return { key: 'dashboard.events.daysAgo', params: { count: String(totalDays) } };
  }

  /**
   * Filters events to those within the last 24 hours from the given reference time,
   * sorts in reverse chronological order, and caps at 10 entries.
   *
   * Validates: Requirements 7.1, 7.4
   */
  export function filterInitialEvents(
    events: RecentEvent[],
    now: number = Date.now()
  ): RecentEvent[] {
    const twentyFourHoursMs = 24 * 60 * 60 * 1000;
    const cutoff = now - twentyFourHoursMs;

    return events
      .filter((e) => new Date(e.occurred_at).getTime() >= cutoff)
      .sort((a, b) => new Date(b.occurred_at).getTime() - new Date(a.occurred_at).getTime())
      .slice(0, 10);
  }

  /**
   * Prepends a new event and removes the oldest if the list exceeds maxSize.
   * Returns a new array (immutable).
   *
   * Validates: Requirements 7.3
   */
  export function prependEvent(
    events: RecentEvent[],
    newEvent: RecentEvent,
    maxSize: number = 10
  ): RecentEvent[] {
    const updated = [newEvent, ...events];
    return updated.slice(0, maxSize);
  }

  /**
   * Determines the color token for an event based on the transition.
   * Recovery (to "up") → --color-success
   * Failure (to "down") → --color-error
   * Other transitions → --color-text-secondary
   *
   * Validates: Requirements 7.6
   */
  export function getEventColor(toState: 'up' | 'down' | 'unknown'): string {
    switch (toState) {
      case 'up':
        return 'var(--color-success)';
      case 'down':
        return 'var(--color-error)';
      default:
        return 'var(--color-text-secondary)';
    }
  }
</script>

<script lang="ts">
  /**
   * EventsFeed — displays the 10 most recent state transitions.
   *
   * Requirements 7.1: Display 10 most recent transitions in reverse chronological order
   * Requirements 7.2: Show monitor name, transition (from → to), relative timestamp
   * Requirements 7.3: Prepend new events from WS, remove oldest if > 10
   * Requirements 7.4: Initial population from incident data (24h window)
   * Requirements 7.5: Empty state when no transitions exist
   * Requirements 7.6: Visually distinguish recovery vs failure with color tokens
   * Requirements 7.7: Session-scoped, cleared on refresh
   */
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    events: RecentEvent[];
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { events, loading, error, onRetry }: Props = $props();

  // Tick counter to force relative timestamp recalculation every 30s
  let tick = $state(0);

  $effect(() => {
    const interval = setInterval(() => {
      tick++;
    }, 30_000);
    return () => clearInterval(interval);
  });

  let isEmpty = $derived(events.length === 0);

  // Force reactivity on tick so timestamps re-render
  let now = $derived.by(() => {
    void tick;
    return Date.now();
  });
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-2 p-4" data-testid="events-feed">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.events.title')}
    </h3>

    {#if isEmpty}
      <p
        class="py-4 text-center text-sm"
        style="color: var(--color-text-secondary)"
        data-testid="events-empty"
      >
        {t('dashboard.events.empty')}
      </p>
    {:else}
      <ul class="flex flex-col gap-1" role="list" data-testid="events-list">
        {#each events as event (event.monitor_id + event.occurred_at)}
          {@const colorStyle = getEventColor(event.to_state)}
          {@const relative = formatRelativeTime(event.occurred_at, now)}
          <li
            class="flex items-center justify-between gap-2 rounded-md px-3 py-2"
            style="background-color: var(--color-bg-secondary)"
          >
            <div class="flex min-w-0 flex-col gap-0.5">
              <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)">
                {event.monitor_name}
              </span>
              <span class="text-xs font-medium" style="color: {colorStyle}">
                {t('dashboard.events.transition', { from: event.from_state, to: event.to_state })}
              </span>
            </div>
            <span
              class="shrink-0 text-xs tabular-nums"
              style="color: var(--color-text-secondary)"
              data-testid="event-timestamp"
            >
              {t(relative.key, relative.params)}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</WidgetShell>
