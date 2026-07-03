<script lang="ts">
  import { untrack } from 'svelte';
  import { getMonitors } from '$lib/api';
  import type { Monitor, MonitorType, PaginatedList, Tag } from '$lib/types';
  import MonitorRow from '../../components/MonitorRow.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import FilterBar from '../../components/FilterBar.svelte';
  import { t } from '$lib/i18n';

  let page = $state(1);
  let monitors = $state<Monitor[]>([]);
  let total = $state(0);
  let totalPages = $state(1);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Filter state
  let activeFilters = $state<{ types: MonitorType[]; tags: Tag[]; showPaused: boolean }>({ types: [], tags: [], showPaused: false });
  const availableTypes: MonitorType[] = ['http', 'http3', 'tcp', 'udp', 'websocket', 'grpc', 'dns', 'icmp', 'smtp'];

  const LIMIT = 20;

  // Wildcard tag matching: converts "service:*payment*" to a regex pattern
  function matchesTagFilter(monitorTags: Tag[], filterTag: Tag): boolean {
    const keyPattern = filterTag.key.includes('*')
      ? new RegExp('^' + filterTag.key.replace(/\*/g, '.*') + '$', 'i')
      : null;
    const valuePattern = filterTag.value.includes('*')
      ? new RegExp('^' + filterTag.value.replace(/\*/g, '.*') + '$', 'i')
      : null;

    return monitorTags.some((mt) => {
      const keyMatch = keyPattern ? keyPattern.test(mt.key) : mt.key === filterTag.key;
      const valueMatch = valuePattern ? valuePattern.test(mt.value) : mt.value === filterTag.value;
      return keyMatch && valueMatch;
    });
  }

  // Separate wildcard tags (client-side) from exact tags (server-side)
  function splitTagFilters(tags: Tag[]): { exact: Tag[]; wildcard: Tag[] } {
    const exact: Tag[] = [];
    const wildcard: Tag[] = [];
    for (const tag of tags) {
      if (tag.key.includes('*') || tag.value.includes('*')) {
        wildcard.push(tag);
      } else {
        exact.push(tag);
      }
    }
    return { exact, wildcard };
  }

  async function fetchMonitors() {
    loading = true;
    error = null;
    try {
      // Build filter options for the API call
      const filterOptions: { type?: string; tags?: string[] } = {};

      if (activeFilters.types.length === 1) {
        filterOptions.type = activeFilters.types[0];
      }

      // Only send exact tags to the backend; wildcards are applied client-side
      const { exact } = splitTagFilters(activeFilters.tags);
      if (exact.length > 0) {
        filterOptions.tags = exact.map((t) => `${t.key}:${t.value}`);
      }

      const result: PaginatedList<Monitor> = await getMonitors(page, LIMIT, filterOptions);
      monitors = result.data;
      total = result.total;
      totalPages = result.total_pages;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('monitors.errors.loadFailed');
    } finally {
      loading = false;
    }
  }

  // Client-side filtering (wildcards + paused)
  let filteredMonitors = $derived.by(() => {
    let result = monitors;

    // Filter by paused status
    if (activeFilters.showPaused) {
      result = result.filter((m) => m.status === 'paused');
    }

    // Apply wildcard tag filters client-side
    const { wildcard } = splitTagFilters(activeFilters.tags);
    if (wildcard.length > 0) {
      result = result.filter((m) =>
        wildcard.every((wt) => matchesTagFilter(m.tags, wt))
      );
    }

    return result;
  });

  function handleFilterChange(filters: { types: MonitorType[]; tags: Tag[]; showPaused: boolean }) {
    activeFilters = filters;
    page = 1;
    fetchMonitors();
  }

  function handlePageChange(newPage: number) {
    page = newPage;
    fetchMonitors();
  }

  // Initial fetch on mount
  $effect(() => {
    untrack(() => fetchMonitors());
  });
</script>

<section class="space-y-6">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold tracking-tight text-primary">{t('monitors.title')}</h1>
    <a
      href="/monitors/create"
      class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
    >
      {t('monitors.create')}
    </a>
  </div>

  <!-- Filter Bar -->
  <FilterBar
    {availableTypes}
    {activeFilters}
    onFilterChange={handleFilterChange}
  />

  <!-- Loading state -->
  {#if loading}
    <div class="flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-surface p-12" data-testid="loading-state">
      <div class="flex items-center gap-3 text-secondary">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>{t('monitors.loading')}</span>
      </div>
    </div>

  <!-- Error state -->
  {:else if error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="error-state">
      <p class="text-sm text-rose-700">{error}</p>
      <button
        type="button"
        onclick={() => fetchMonitors()}
        class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
      >
        {t('common.retry')}
      </button>
    </div>

  <!-- Empty state -->
  {:else if total === 0}
    <div class="rounded-xl border border-dashed border-[var(--color-border)] bg-surface p-12 text-center" data-testid="empty-state">
      <p class="text-secondary">{t('monitors.empty.title')}</p>
      <a
        href="/monitors/create"
        class="mt-3 inline-block text-sm font-medium text-indigo-600 hover:text-indigo-700"
      >
        {t('monitors.empty.action')}
      </a>
    </div>

  <!-- Monitor list -->
  {:else}
    <div class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-surface" data-testid="monitor-list">
      {#each filteredMonitors as monitor (monitor.id)}
        <MonitorRow {monitor} />
      {/each}
    </div>

    <!-- Pagination -->
    {#if totalPages > 1}
      <Pagination
        {page}
        {totalPages}
        onPageChange={handlePageChange}
      />
    {/if}
  {/if}
</section>
