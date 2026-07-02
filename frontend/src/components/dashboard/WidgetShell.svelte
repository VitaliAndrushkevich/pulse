<script lang="ts">
  /**
   * WidgetShell — shared wrapper for dashboard widgets.
   *
   * Provides consistent loading, error, and ready states for each widget.
   * Priority: loading > error > content (slot).
   *
   * Requirements 8.4: Per-widget error isolation with retry action.
   * Requirements 8.6: Animated loading skeleton placeholder.
   */
  import type { Snippet } from 'svelte';
  import { t } from '$lib/i18n';

  interface Props {
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
    children: Snippet;
  }

  let { loading, error, onRetry, children }: Props = $props();
</script>

{#if loading}
  <div class="space-y-3 p-4" data-testid="widget-skeleton" aria-busy="true" aria-label={t('common.loading')}>
    <div class="h-4 w-3/4 rounded bg-[var(--color-border)] animate-pulse"></div>
    <div class="h-4 w-1/2 rounded bg-[var(--color-border)] animate-pulse"></div>
    <div class="h-20 w-full rounded bg-[var(--color-border)] animate-pulse"></div>
  </div>
{:else if error}
  <div class="flex flex-col items-center justify-center gap-3 p-4 text-center" data-testid="widget-error" role="alert">
    <p class="text-sm" style="color: var(--color-error)">{error}</p>
    {#if onRetry}
      <button
        onclick={onRetry}
        class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
        style="background-color: var(--color-brand-primary)"
        data-testid="widget-retry"
      >
        {t('common.retry')}
      </button>
    {/if}
  </div>
{:else}
  {@render children()}
{/if}
