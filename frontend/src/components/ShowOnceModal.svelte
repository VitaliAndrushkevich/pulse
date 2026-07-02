<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '$lib/i18n';

  interface Props {
    secret: string;
    onDismiss: () => void;
  }

  let { secret, onDismiss }: Props = $props();

  // Internal copy of secret that gets cleared on dismiss
  let displaySecret = $state(secret);
  let copied = $state(false);
  let copyTimeout: ReturnType<typeof setTimeout> | null = null;

  // Reference to the dialog element for focus management
  let dialogRef: HTMLDialogElement | undefined = $state(undefined);

  onMount(() => {
    // Focus the dialog when it mounts
    dialogRef?.focus();

    // Prevent navigation away while modal is open
    function handleBeforeUnload(e: BeforeUnloadEvent) {
      e.preventDefault();
    }

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
      if (copyTimeout) clearTimeout(copyTimeout);
    };
  });

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(displaySecret);
      copied = true;
      if (copyTimeout) clearTimeout(copyTimeout);
      copyTimeout = setTimeout(() => {
        copied = false;
      }, 2000);
    } catch {
      // Fallback: select the text in the input for manual copy
    }
  }

  function handleDismiss() {
    // Clear secret from component state
    displaySecret = '';
    onDismiss();
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
  role="presentation"
  onkeydown={(e) => e.key === 'Escape' && e.preventDefault()}
>
  <dialog
    bind:this={dialogRef}
    open
    aria-label="Secret value — shown once only"
    aria-modal="true"
    class="relative m-0 w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-2xl"
    tabindex="-1"
    onkeydown={(e) => {
      if (e.key === 'Escape') e.preventDefault();
    }}
  >
    <div class="flex flex-col gap-5">
      <!-- Header -->
      <div class="flex items-center gap-3">
        <span class="flex h-10 w-10 items-center justify-center rounded-full bg-amber-100">
          <svg class="h-5 w-5 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
          </svg>
        </span>
        <h2 class="text-lg font-semibold text-primary">{t('modal.secretTitle')}</h2>
      </div>

      <!-- Warning -->
      <div class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
        <p class="text-sm font-medium text-amber-800">
          {t('modal.secretWarning')}
        </p>
      </div>

      <!-- Secret display -->
      <div class="flex flex-col gap-2">
        <label for="secret-value" class="text-sm font-medium text-primary">{t('modal.secretLabel')}</label>
        <div class="flex gap-2">
          <input
            id="secret-value"
            type="text"
            readonly
            value={displaySecret}
            class="flex-1 rounded-md border border-[var(--color-border)] bg-page px-3 py-2 font-mono text-sm text-primary select-all focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20"
            aria-label="Secret value"
          />
          <button
            type="button"
            onclick={handleCopy}
            class="inline-flex items-center gap-1.5 rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] focus:outline-none focus:ring-2 focus:ring-blue-500/20"
            aria-label={copied ? 'Copied to clipboard' : 'Copy secret to clipboard'}
          >
            {#if copied}
              <svg class="h-4 w-4 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              <span class="text-green-600">{t('common.copied')}</span>
            {:else}
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
              <span>{t('common.copy')}</span>
            {/if}
          </button>
        </div>
      </div>

      <!-- Dismiss button -->
      <button
        type="button"
        onclick={handleDismiss}
        class="w-full rounded-md bg-blue-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
      >
        {t('modal.secretDismiss')}
      </button>
    </div>
  </dialog>
</div>
