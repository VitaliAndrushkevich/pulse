<script lang="ts">
  import { isAdmin } from '$lib/stores/auth.svelte';
  import { untrack } from 'svelte';
  import { getHeartbeats } from '$lib/api';
  import type { Heartbeat, PaginatedList } from '$lib/types';
  import { heartbeatStore } from '$lib/stores/heartbeats.svelte';
  import HeartbeatRow from '../../components/HeartbeatRow.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import VirtualList from '../../components/VirtualList.svelte';
  import { t } from '$lib/i18n';

  let page = $state(1);
  let total = $state(0);
  let totalPages = $state(1);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const LIMIT = 20;
  const ROW_HEIGHT = 64; // matches HeartbeatRow h-16

  // The reactive store is the source of truth so WS patches re-render the list.
  let heartbeats = $derived<Heartbeat[]>(heartbeatStore.list);

  async function fetchHeartbeats() {
    loading = true;
    error = null;
    try {
      const result: PaginatedList<Heartbeat> = await getHeartbeats(page, LIMIT);
      heartbeatStore.setHeartbeats(result.data);
      total = result.total;
      totalPages = result.total_pages;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('heartbeats.loadFailed');
    } finally {
      loading = false;
    }
  }

  function handlePageChange(newPage: number) {
    page = newPage;
    fetchHeartbeats();
  }

  // Cap the virtual list viewport height so large collections stay virtualized.
  let listHeight = $derived(Math.min(heartbeats.length * ROW_HEIGHT, 640));

  $effect(() => {
    untrack(() => fetchHeartbeats());
  });
</script>

<section class="space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-bold tracking-tight text-primary">{t('heartbeats.title')}</h1>
      <p class="mt-1 text-sm text-secondary">{t('heartbeats.subtitle')}</p>
    </div>
    {#if isAdmin()}<a
      href="/heartbeats/create"
      class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
    >
      {t('heartbeats.create')}
    </a>{/if}
  </div>

  {#if loading}
    <div class="flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface p-12" data-testid="loading-state">
      <span class="text-sm text-secondary">{t('heartbeats.loading')}</span>
    </div>
  {:else if error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="error-state">
      <p class="text-sm text-rose-700">{error}</p>
      <button
        type="button"
        onclick={() => fetchHeartbeats()}
        class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else if total === 0}
    <div class="rounded-xl border border-dashed border-[var(--color-border)] bg-surface p-12 text-center" data-testid="empty-state">
      <p class="text-secondary">{t('heartbeats.empty')}</p>
      {#if isAdmin()}<a
        href="/heartbeats/create"
        class="mt-3 inline-block text-sm font-medium text-indigo-600 hover:text-indigo-700"
      >
        {t('heartbeats.create')}
      </a>{/if}
    </div>
  {:else}
    <div class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-surface" data-testid="heartbeat-list">
      <VirtualList items={heartbeats} itemHeight={ROW_HEIGHT} containerHeight={listHeight}>
        {#snippet row(heartbeat: Heartbeat)}
          <HeartbeatRow {heartbeat} />
        {/snippet}
      </VirtualList>
    </div>

    {#if totalPages > 1}
      <Pagination {page} {totalPages} onPageChange={handlePageChange} />
    {/if}
  {/if}
</section>
