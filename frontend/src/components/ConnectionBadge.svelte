<script lang="ts">
  /**
   * ConnectionBadge — displays live/paused connection status.
   *
   * Shows a visible indicator when the WebSocket connection is lost,
   * satisfying Requirements 1.7 and 5.8:
   * - Stale-data indicator within 2s of disconnection
   * - Removed when connection is re-established
   * - Visible without scrolling (placed in app header)
   */
  import { connectionStore } from '$lib/stores/connection.svelte';
  import { t } from '$lib/i18n';

  let status = $derived(connectionStore.status);
  let visible = $derived(status === 'disconnected' || status === 'connecting');
</script>

{#if visible}
  <div
    class="inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-800 border border-amber-300"
    role="status"
    aria-live="polite"
    aria-label={t('connection.statusPaused')}
  >
    <span class="relative flex h-2 w-2">
      <span
        class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
        class:bg-amber-400={status === 'connecting'}
        class:bg-amber-500={status === 'disconnected'}
      ></span>
      <span
        class="relative inline-flex h-2 w-2 rounded-full"
        class:bg-amber-400={status === 'connecting'}
        class:bg-amber-500={status === 'disconnected'}
      ></span>
    </span>
    <span>
      {#if status === 'connecting'}
        {t('connection.reconnecting')}
      {:else}
        {t('connection.paused')}
      {/if}
    </span>
  </div>
{:else if status === 'connected'}
  <div
    class="inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium text-green-700"
    role="status"
    aria-live="polite"
    aria-label={t('connection.statusLive')}
  >
    <span class="relative flex h-2 w-2">
      <span class="relative inline-flex h-2 w-2 rounded-full bg-green-500"></span>
    </span>
    <span>{t('connection.live')}</span>
  </div>
{/if}
