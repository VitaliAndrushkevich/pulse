<script lang="ts">
  import { untrack } from 'svelte';
  import { notificationStore } from '$lib/stores/notifications.svelte';
  import { toastStore } from '$lib/stores/toast.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import { t } from '$lib/i18n';

  let deleteConfirmId = $state<string | null>(null);
  let deleteConfirmName = $state<string>('');
  let testingChannelId = $state<string | null>(null);

  function handlePageChange(newPage: number) {
    notificationStore.fetchChannels(newPage);
  }

  async function handleDelete(id: string, name: string) {
    deleteConfirmId = id;
    deleteConfirmName = name;
  }

  async function confirmDelete() {
    if (!deleteConfirmId) return;
    try {
      await notificationStore.remove(deleteConfirmId);
      toastStore.addToast({
        type: 'success',
        message: t('notifications.toast.channelDeleted', { name: deleteConfirmName }),
        persistent: false
      });
    } catch (err: unknown) {
      // Error toast handled by API client
    } finally {
      deleteConfirmId = null;
      deleteConfirmName = '';
    }
  }

  function cancelDelete() {
    deleteConfirmId = null;
    deleteConfirmName = '';
  }

  async function handleTest(id: string) {
    testingChannelId = id;
    try {
      const result = await notificationStore.test(id);
      if (result.success) {
        toastStore.addToast({
          type: 'success',
          message: t('notifications.toast.testSuccess'),
          persistent: false
        });
      } else {
        toastStore.addToast({
          type: 'error',
          message: t('notifications.toast.testFailure', { error: result.error || 'Unknown error' }),
          persistent: false
        });
      }
    } catch (err: unknown) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error';
      toastStore.addToast({
        type: 'error',
        message: t('notifications.toast.testFailure', { error: errorMsg }),
        persistent: false
      });
    } finally {
      testingChannelId = null;
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    });
  }

  // Fetch channels on mount
  $effect(() => {
    untrack(() => notificationStore.fetchChannels());
  });
</script>

<section class="space-y-6">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-bold tracking-tight text-primary">{t('notifications.title')}</h1>
      <p class="mt-1 text-sm text-secondary">{t('notifications.description')}</p>
    </div>
    <a
      href="/notifications/create"
      class="rounded-md bg-[var(--color-brand-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--color-brand-primary)] focus:ring-offset-2"
    >
      {t('notifications.channels.create')}
    </a>
  </div>

  <!-- Loading state -->
  {#if notificationStore.loading}
    <div class="flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface p-12" data-testid="loading-state">
      <div class="flex items-center gap-3 text-secondary">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>{t('notifications.channels.loading')}</span>
      </div>
    </div>

  <!-- Error state -->
  {:else if notificationStore.error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="error-state">
      <p class="text-sm text-rose-700">{notificationStore.error}</p>
      <button
        type="button"
        onclick={() => notificationStore.fetchChannels()}
        class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
      >
        {t('common.retry')}
      </button>
    </div>

  <!-- Empty state -->
  {:else if notificationStore.isEmpty}
    <div class="rounded-xl border border-dashed border-[var(--color-border)] bg-surface p-12 text-center" data-testid="empty-state">
      <svg class="mx-auto h-12 w-12 text-secondary opacity-50" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 0 0 5.454-1.31A8.967 8.967 0 0 1 18 9.75V9A6 6 0 0 0 6 9v.75a8.967 8.967 0 0 1-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 0 1-5.714 0m5.714 0a3 3 0 1 1-5.714 0" />
      </svg>
      <p class="mt-4 text-sm font-medium text-primary">{t('notifications.channels.empty.title')}</p>
      <p class="mt-1 text-sm text-secondary">{t('notifications.channels.empty.description')}</p>
      <a
        href="/notifications/create"
        class="mt-4 inline-block rounded-md bg-[var(--color-brand-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--color-brand-primary)] focus:ring-offset-2"
      >
        {t('notifications.channels.empty.action')}
      </a>
    </div>

  <!-- Channel list -->
  {:else}
    <div class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-surface" data-testid="channel-list">
      <!-- Table header -->
      <div class="grid grid-cols-[1fr_auto_auto_auto] gap-4 border-b border-[var(--color-border)] px-6 py-3 text-xs font-medium uppercase tracking-wider text-secondary">
        <span>{t('notifications.channels.list.name')}</span>
        <span>{t('notifications.channels.list.type')}</span>
        <span>{t('notifications.channels.list.created')}</span>
        <span>{t('notifications.channels.list.actions')}</span>
      </div>

      <!-- Channel rows -->
      {#each notificationStore.channels as channel (channel.id)}
        <div class="grid grid-cols-[1fr_auto_auto_auto] items-center gap-4 border-b border-[var(--color-border)] px-6 py-4 last:border-b-0 transition hover:bg-[var(--color-bg-surface-hover)]">
          <!-- Name -->
          <span class="text-sm font-medium text-primary truncate">{channel.name}</span>

          <!-- Type badge -->
          <span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium {channel.type === 'email' ? 'bg-blue-100 text-blue-800' : 'bg-purple-100 text-purple-800'}">
            {t(`notifications.channels.types.${channel.type}`)}
          </span>

          <!-- Created date -->
          <span class="text-sm text-secondary whitespace-nowrap">{formatDate(channel.created_at)}</span>

          <!-- Actions -->
          <div class="flex items-center gap-2">
            <!-- Edit -->
            <a
              href="/notifications/{channel.id}/edit"
              class="rounded-md px-2 py-1 text-xs font-medium text-[var(--color-brand-primary)] transition hover:bg-[var(--color-bg-surface-hover)]"
              aria-label="{t('notifications.channels.edit')} {channel.name}"
            >
              {t('notifications.channels.edit')}
            </a>

            <!-- Test -->
            <button
              type="button"
              onclick={() => handleTest(channel.id)}
              disabled={testingChannelId === channel.id}
              class="rounded-md px-2 py-1 text-xs font-medium text-secondary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
              aria-label="{t('notifications.test.button')} {channel.name}"
            >
              {#if testingChannelId === channel.id}
                {t('notifications.test.sending')}
              {:else}
                {t('notifications.test.button')}
              {/if}
            </button>

            <!-- Delete -->
            <button
              type="button"
              onclick={() => handleDelete(channel.id, channel.name)}
              class="rounded-md px-2 py-1 text-xs font-medium text-red-600 transition hover:bg-red-50"
              aria-label="{t('common.delete')} {channel.name}"
            >
              {t('common.delete')}
            </button>
          </div>
        </div>
      {/each}
    </div>

    <!-- Pagination -->
    {#if notificationStore.totalPages > 1}
      <Pagination
        page={notificationStore.page}
        totalPages={notificationStore.totalPages}
        onPageChange={handlePageChange}
      />
    {/if}
  {/if}
</section>

<!-- Delete confirmation dialog -->
{#if deleteConfirmId}
  <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true" aria-labelledby="delete-dialog-title">
    <!-- Backdrop -->
    <button type="button" class="fixed inset-0 bg-black/50 transition-opacity cursor-default" onclick={cancelDelete} aria-label="Close dialog"></button>

    <!-- Dialog -->
    <div class="relative z-10 w-full max-w-md rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
      <h2 id="delete-dialog-title" class="text-lg font-semibold text-primary">
        {t('notifications.channels.deleteConfirm.title')}
      </h2>
      <p class="mt-2 text-sm text-secondary">
        {t('notifications.channels.deleteConfirm.description', { name: deleteConfirmName })}
      </p>
      <div class="mt-6 flex justify-end gap-3">
        <button
          type="button"
          onclick={cancelDelete}
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={confirmDelete}
          class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
        >
          {t('notifications.channels.deleteConfirm.confirm')}
        </button>
      </div>
    </div>
  </div>
{/if}
