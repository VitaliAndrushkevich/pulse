<script lang="ts">
  import { untrack } from 'svelte';
  import { getMonitors } from '$lib/api';
  import type { Monitor, MonitorType, PaginatedList, Tag } from '$lib/types';
  import MonitorRow from '../../components/MonitorRow.svelte';
  import Pagination from '../../components/Pagination.svelte';
  import FilterBar from '../../components/FilterBar.svelte';

  let page = $state(1);
  let monitors = $state<Monitor[]>([]);
  let total = $state(0);
  let totalPages = $state(1);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Filter state
  let activeFilters = $state<{ types: MonitorType[]; tags: Tag[] }>({ types: [], tags: [] });
  const availableTypes: MonitorType[] = ['http', 'tcp', 'udp', 'websocket'];

  const LIMIT = 20;

  async function fetchMonitors() {
    loading = true;
    error = null;
    try {
      // Build filter options for the API call
      const filterOptions: { type?: string; tags?: string[] } = {};

      if (activeFilters.types.length === 1) {
        filterOptions.type = activeFilters.types[0];
      }

      if (activeFilters.tags.length > 0) {
        filterOptions.tags = activeFilters.tags.map((t) => `${t.key}:${t.value}`);
      }

      const result: PaginatedList<Monitor> = await getMonitors(page, LIMIT, filterOptions);
      monitors = result.data;
      total = result.total;
      totalPages = result.total_pages;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load monitors. Please try again.';
    } finally {
      loading = false;
    }
  }

  function handleFilterChange(filters: { types: MonitorType[]; tags: Tag[] }) {
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
    <h1 class="text-2xl font-bold tracking-tight text-primary">Monitors</h1>
    <a
      href="/monitors/create"
      class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
    >
      Create Monitor
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
        <span>Loading monitors...</span>
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
        Retry
      </button>
    </div>

  <!-- Empty state -->
  {:else if total === 0}
    <div class="rounded-xl border border-dashed border-[var(--color-border)] bg-surface p-12 text-center" data-testid="empty-state">
      <p class="text-secondary">No monitors found.</p>
      <a
        href="/monitors/create"
        class="mt-3 inline-block text-sm font-medium text-indigo-600 hover:text-indigo-700"
      >
        Create your first monitor
      </a>
    </div>

  <!-- Monitor list -->
  {:else}
    <div class="overflow-hidden rounded-xl border border-[var(--color-border)] bg-surface" data-testid="monitor-list">
      {#each monitors as monitor (monitor.id)}
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
