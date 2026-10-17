<script lang="ts">
  /**
   * ConnectionBadge — displays live/paused connection status.
   *
   * Shows a visible indicator when the WebSocket connection is lost,
   * satisfying Requirements 1.7 and 5.8:
   * - Stale-data indicator within 2s of disconnection
   * - Removed when connection is re-established
   * - Visible without scrolling (placed in navigation footer)
   */
  import { connectionStore } from '$lib/stores/connection.svelte';
  import { t } from '$lib/i18n';

  let { compact = false }: { compact?: boolean } = $props();

  let status = $derived(connectionStore.status);
  let visible = $derived(status === 'disconnected' || status === 'connecting');
</script>

{#if visible}
  <div
    class="connection-badge inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium text-[var(--color-warning)]"
    class:compact
    title={t(visible ? 'connection.statusPaused' : 'connection.statusLive')}
    role="status"
    aria-live="polite"
    aria-label={t('connection.statusPaused')}
  >
    <span class="relative flex h-2 w-2">
      <span
        class="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-warning)] opacity-75"
      ></span>
      <span
        class="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-warning)]"
      ></span>
    </span>
    <span class="connection-label">
      {#if status === 'connecting'}
        {t('connection.reconnecting')}
      {:else}
        {t('connection.paused')}
      {/if}
    </span>
  </div>
{:else if status === 'connected'}
  <div
    class="connection-badge inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium text-[var(--color-success)]"
    class:compact
    title={t(visible ? 'connection.statusPaused' : 'connection.statusLive')}
    role="status"
    aria-live="polite"
    aria-label={t('connection.statusLive')}
  >
    <span class="relative flex h-2 w-2">
      <span class="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]"></span>
    </span>
    <span class="connection-label">{t('connection.live')}</span>
  </div>
{/if}

<style>
  @media (min-width: 1024px) {
    .compact .connection-label { display: none; }
  }
</style>
