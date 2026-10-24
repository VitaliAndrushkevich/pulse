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
  import { resolve } from '$app/paths';
  import { t, getLocale } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';
  import VirtualList from '../VirtualList.svelte';

  let { entries, checkedMonitorCount = entries.length, loading, error, onRetry }: {
    entries: SSLExpiryEntry[];
    checkedMonitorCount?: number;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  } = $props();

  const sorted = $derived(sortSSLEntries(filterSSLEntries(entries)));
  function expiryDate(value: string): string {
    return new Intl.DateTimeFormat(getLocale(), {year: 'numeric', month: 'short', day: 'numeric'}).format(new Date(value));
  }
</script>

{#snippet certificate(entry: SSLExpiryEntry)}
  {@const tier = getUrgencyTier(entry.days_remaining)}
            <a class="certificate-row" href={resolve('/monitors/[id]', { id: entry.monitor_id })}>
              <div class="row-heading">
                <span class="monitor-name" title={entry.monitor_name}>{entry.monitor_name}</span>
                <span class="days-remaining" style:color={tier === 'warning' ? 'var(--color-warning)' : 'var(--color-error)'} data-urgency={tier}>
                  {entry.days_remaining <= 0 ? t('dashboard.ssl.expired') : t('dashboard.ssl.daysRemaining', { count: String(entry.days_remaining) })}
                </span>
              </div>
              <span class="expiry-date">{t('dashboard.certificates.expires', {date: expiryDate(entry.expires_at)})}</span>
            </a>
{/snippet}

<section class="certificates-panel" aria-labelledby="certificates-title" data-testid="ssl-warnings">
  <header class="panel-heading">
    <div class="heading-row">
      <h3 id="certificates-title" class="pulse-eyebrow">{t('dashboard.certificates.title')}</h3>
      {#if !loading && !error && sorted.length > 0}<span class="pulse-tag">{sorted.length}</span>{/if}
    </div>
    <p>{t('dashboard.certificates.description')}</p>
  </header>
  <WidgetShell {loading} {error} {onRetry}>
    {#if checkedMonitorCount === 0 && sorted.length === 0}
      <p class="empty-message" data-testid="ssl-no-data">{t('dashboard.certificates.noData')}</p>
    {:else if sorted.length === 0}
      <p class="empty-message" data-testid="ssl-empty">{t('dashboard.certificates.allClear')}</p>
    {:else}
      {#if sorted.length > 6}
        <VirtualList items={sorted} itemHeight={72} containerHeight={360}>
          {#snippet row(entry)}{@render certificate(entry)}{/snippet}
        </VirtualList>
      {:else}
        <ul class="certificate-list">
          {#each sorted as entry (entry.monitor_id)}<li>{@render certificate(entry)}</li>{/each}
        </ul>
      {/if}
    {/if}
  </WidgetShell>
</section>

<style>
  .certificates-panel { padding: 1.25rem; }
  .panel-heading { display: grid; gap: 0.5rem; margin-block-end: 0.75rem; }
  .heading-row { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
  .panel-heading p, .empty-message, .expiry-date { color: var(--color-text-secondary); font-size: 0.75rem; }
  .empty-message { padding-block: 0.5rem; }
  .certificate-list { display: grid; gap: 0.25rem;  }
  .certificate-row { block-size: 72px; box-sizing: border-box; display: grid; gap: 0.35rem; padding: 0.75rem; border-radius: 0.25rem; text-decoration: none; }
  .certificate-row:hover { background: var(--color-bg-secondary); }
  .certificate-row:focus-visible { outline: 2px solid var(--color-brand-primary); outline-offset: -2px; }
  .expiry-date { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .row-heading { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
  .monitor-name { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--color-text-primary); font-size: 0.875rem; font-weight: 500; }
  .days-remaining { flex-shrink: 0; font-size: 0.75rem; font-weight: 600; font-variant-numeric: tabular-nums; font-family: var(--font-data); }
</style>
