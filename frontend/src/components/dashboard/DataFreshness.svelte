<script lang="ts" module>
  /**
   * Formats a Date as a human-readable relative time string.
   *
   * Exported for property-based testing (Property 18).
   *
   * Validates: Requirements 9.3
   */
  export function formatRelativeTime(date: Date, now: Date = new Date()): string {
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.max(0, Math.floor(diffMs / 1000));

    if (diffSec < 60) return `${diffSec}s ago`;

    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;

    const diffHour = Math.floor(diffMin / 60);
    if (diffHour < 24) return `${diffHour}h ago`;

    const diffDay = Math.floor(diffHour / 24);
    return `${diffDay}d ago`;
  }
</script>

<script lang="ts">
  /**
   * DataFreshness — displays "Last updated: X ago" with stale indicator.
   *
   * Requirements 9.3: Global "last updated" timestamp in relative format, refreshed every 5s
   * Requirements 9.4: Stale-data indicator after 60s without WebSocket messages
   * Requirements 9.5: Remove stale indicator on new WebSocket message
   */
  import { t } from '$lib/i18n';

  interface Props {
    lastUpdated: Date | null;
    stale: boolean;
  }

  let { lastUpdated, stale }: Props = $props();

  let relativeTime = $state<string>('');

  function updateRelativeTime(): void {
    if (lastUpdated) {
      relativeTime = formatRelativeTime(lastUpdated);
    }
  }

  $effect(() => {
    // Access lastUpdated to register reactivity
    const _lu = lastUpdated;
    updateRelativeTime();

    const interval = setInterval(updateRelativeTime, 5000);
    return () => clearInterval(interval);
  });
</script>

<div
  class="flex items-center gap-2 text-xs"
  style="color: var(--color-text-secondary)"
  data-testid="data-freshness"
>
  <span>
    {t('dashboard.freshness.label')}
    {#if lastUpdated}
      {relativeTime}
    {:else}
      {t('common.never')}
    {/if}
  </span>

  {#if stale}
    <span
      class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
      style="background-color: var(--color-warning); color: var(--color-bg-primary)"
      role="status"
      data-testid="stale-indicator"
    >
      {t('dashboard.freshness.stale')}
    </span>
  {/if}
</div>
