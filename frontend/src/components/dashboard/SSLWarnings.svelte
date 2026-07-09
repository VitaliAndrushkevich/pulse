<script lang="ts" module>
  /**
   * Utility functions exported for property-based testing (Properties 11, 12, 13).
   */
  import type { SSLExpiryEntry } from '$lib/types';

  /**
   * Filters SSL entries to only include those with days_remaining <= 30.
   * Belt-and-suspenders filter — backend should already filter, but ensures correctness.
   *
   * Validates: Requirements 5.1, 5.7
   */
  export function filterSSLEntries(entries: SSLExpiryEntry[]): SSLExpiryEntry[] {
    return entries.filter((entry) => entry.days_remaining <= 30);
  }

  /**
   * Sorts SSL entries by days_remaining ascending (most urgent first),
   * with alphabetical monitor name as tiebreaker.
   *
   * Validates: Requirements 5.3
   */
  export function sortSSLEntries(entries: SSLExpiryEntry[]): SSLExpiryEntry[] {
    return [...entries].sort((a, b) => {
      if (a.days_remaining !== b.days_remaining) return a.days_remaining - b.days_remaining;
      return a.monitor_name.localeCompare(b.monitor_name);
    });
  }

  /**
   * Determines the urgency tier for a given days_remaining value.
   * - expired: days_remaining <= 0
   * - critical: days_remaining 1–7
   * - warning: days_remaining 8–30
   *
   * Validates: Requirements 5.5
   */
  export function getUrgencyTier(daysRemaining: number): 'expired' | 'critical' | 'warning' {
    if (daysRemaining <= 0) return 'expired';
    if (daysRemaining <= 7) return 'critical';
    return 'warning';
  }
</script>

<script lang="ts">
  /**
   * SSLWarnings — displays monitors with SSL certificates expiring within 30 days.
   *
   * Requirements 5.1: Display monitors with SSL expiring within 30 days or already expired
   * Requirements 5.2: Show name, days remaining, expiry date (locale-formatted)
   * Requirements 5.3: Order by days remaining ascending, name tiebreaker
   * Requirements 5.4: Hide section entirely when no entries
   * Requirements 5.5: Urgency styling (expired/critical red, warning amber)
   */
  import type { SSLExpiryEntry } from '$lib/types';
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    entries: SSLExpiryEntry[];
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { entries, loading, error, onRetry }: Props = $props();

  // Locale-aware date formatter using browser's Intl.DateTimeFormat
  const dateFormatter = new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  });

  // Filter and sort entries reactively
  let filtered = $derived(filterSSLEntries(entries));
  let sorted = $derived(sortSSLEntries(filtered));
  let isEmpty = $derived(sorted.length === 0);

  /**
   * Returns the CSS color variable for a given urgency tier.
   */
  function getUrgencyColor(tier: 'expired' | 'critical' | 'warning'): string {
    if (tier === 'warning') return 'var(--color-warning)';
    return 'var(--color-error)';
  }

  /**
   * Formats the days remaining display text.
   */
  function formatDaysRemaining(days: number): string {
    if (days <= 0) return t('dashboard.ssl.expired');
    return t('dashboard.ssl.daysRemaining', { count: String(days) });
  }

  /**
   * Formats the expiry date using locale-aware formatting.
   */
  function formatExpiryDate(expiresAt: string): string {
    const date = new Date(expiresAt);
    return dateFormatter.format(date);
  }
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col gap-2 p-4" data-testid="ssl-warnings">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.ssl.title')}
    </h3>

    {#if isEmpty}
      <p
        class="py-4 text-center text-sm"
        style="color: var(--color-text-secondary)"
        data-testid="ssl-empty"
      >
        {t('dashboard.ssl.allClear')}
      </p>
    {:else}
      <ul class="flex flex-col gap-1" role="list" data-testid="ssl-list">
        {#each sorted as entry (entry.monitor_id)}
          {@const tier = getUrgencyTier(entry.days_remaining)}
          {@const color = getUrgencyColor(tier)}
          <li
            class="flex items-center justify-between gap-2 rounded-md px-3 py-2"
            style="background-color: var(--color-bg-secondary)"
          >
            <span class="truncate text-sm font-medium" style="color: var(--color-text-primary)">
              {entry.monitor_name}
            </span>
            <div class="flex shrink-0 items-center gap-2">
              <span
                class="text-xs font-medium tabular-nums"
                style="color: {color}"
                data-testid="ssl-days-remaining"
                data-urgency={tier}
              >
                {formatDaysRemaining(entry.days_remaining)}
              </span>
              <span class="text-xs" style="color: var(--color-text-secondary)">
                {formatExpiryDate(entry.expires_at)}
              </span>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</WidgetShell>
