<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { untrack } from 'svelte';
  import type { NotificationChannel } from '$lib/types';
  import { notificationStore } from '$lib/stores/notifications.svelte';
  import NotificationChannelForm from '../../../../components/NotificationChannelForm.svelte';
  import { t } from '$lib/i18n';

  let channel = $state<NotificationChannel | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    const id = page.params.id;
    untrack(() => loadChannel(id));
  });

  async function loadChannel(id: string) {
    loading = true;
    error = null;
    try {
      channel = await notificationStore.getById(id);
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load channel';
    } finally {
      loading = false;
    }
  }

  function handleSubmit(_channel: NotificationChannel) {
    goto('/notifications');
  }

  function handleCancel() {
    goto('/notifications');
  }
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-primary">{t('notifications.channels.edit')}</h1>

  {#if loading}
    <div class="flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface p-12">
      <div class="flex items-center gap-3 text-secondary">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>{t('common.loading')}</span>
      </div>
    </div>
  {:else if error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center">
      <p class="text-sm text-rose-700">{error}</p>
      <a
        href="/notifications"
        class="mt-3 inline-block rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
      >
        {t('common.back')}
      </a>
    </div>
  {:else if channel}
    <NotificationChannelForm
      mode="edit"
      initialData={channel}
      onSubmit={handleSubmit}
      onCancel={handleCancel}
    />
  {/if}
</section>
