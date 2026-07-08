<script lang="ts">
  import { onMount } from 'svelte';
  import { listMonitorDeliveryLogs, type DeliveryLogEntry } from '$lib/api';
  import { t } from '$lib/i18n';
  import { formatDate } from '$lib/format';
  import Pagination from './Pagination.svelte';

  interface Props {
    monitorId: string;
  }

  let { monitorId }: Props = $props();

  let logs = $state<DeliveryLogEntry[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let page = $state(1);
  let totalPages = $state(0);
  let total = $state(0);
  const limit = 20;

  async function fetchLogs() {
    loading = true;
    error = null;
    try {
      const result = await listMonitorDeliveryLogs(monitorId, page, limit);
      logs = result.data;
      totalPages = result.total_pages;
      total = result.total;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load delivery logs';
    } finally {
      loading = false;
    }
  }

  function handlePageChange(newPage: number) {
    page = newPage;
    fetchLogs();
  }

  onMount(() => {
    fetchLogs();
  });
</script>

<div class="space-y-4">
  {#if loading}
    <div class="flex items-center justify-center p-8">
      <div class="flex items-center gap-3 text-secondary">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>{t('common.loading')}</span>
      </div>
    </div>
  {:else if error}
    <div class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-center">
      <p class="text-sm text-rose-700">{error}</p>
      <button
        type="button"
        onclick={() => fetchLogs()}
        class="mt-2 rounded-md bg-rose-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-rose-700"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else if logs.length === 0}
    <div class="rounded-lg border border-[var(--color-border)] bg-surface p-8 text-center">
      <p class="text-sm text-secondary">{t('notifications.deliveryLogs.empty')}</p>
    </div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-[var(--color-border)]">
      <table class="w-full text-sm">
        <thead class="border-b border-[var(--color-border)] bg-[var(--color-bg-surface-hover)]">
          <tr>
            <th class="px-4 py-2.5 text-left font-medium text-secondary">{t('notifications.deliveryLogs.status')}</th>
            <th class="px-4 py-2.5 text-left font-medium text-secondary">{t('notifications.deliveryLogs.trigger')}</th>
            <th class="px-4 py-2.5 text-left font-medium text-secondary">{t('notifications.deliveryLogs.attempt')}</th>
            <th class="px-4 py-2.5 text-left font-medium text-secondary">{t('notifications.deliveryLogs.error')}</th>
            <th class="px-4 py-2.5 text-left font-medium text-secondary">{t('notifications.deliveryLogs.time')}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[var(--color-border)]">
          {#each logs as log}
            <tr class="hover:bg-[var(--color-bg-surface-hover)] transition-colors">
              <td class="px-4 py-2.5">
                {#if log.status === 'success'}
                  <span class="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700">
                    <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>
                    {t('notifications.deliveryLogs.statusSuccess')}
                  </span>
                {:else}
                  <span class="inline-flex items-center gap-1 rounded-full bg-rose-100 px-2 py-0.5 text-xs font-medium text-rose-700">
                    <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"></path></svg>
                    {t('notifications.deliveryLogs.statusFailure')}
                  </span>
                {/if}
              </td>
              <td class="px-4 py-2.5 text-primary">
                <span class="rounded bg-[var(--color-bg-surface-hover)] px-1.5 py-0.5 text-xs font-mono">
                  {log.trigger_type}
                </span>
              </td>
              <td class="px-4 py-2.5 text-secondary">{log.attempt}</td>
              <td class="px-4 py-2.5 text-secondary max-w-xs truncate" title={log.error_detail ?? ''}>
                {log.error_detail ?? '—'}
              </td>
              <td class="px-4 py-2.5 text-secondary whitespace-nowrap">{formatDate(log.created_at)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if totalPages > 1}
      <Pagination
        {page}
        {totalPages}
        onPageChange={handlePageChange}
      />
    {/if}

    <p class="text-xs text-secondary">
      {t('notifications.deliveryLogs.total', { count: total })}
    </p>
  {/if}
</div>
