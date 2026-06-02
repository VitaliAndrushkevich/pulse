<script lang="ts">
  import { onMount } from 'svelte';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { getMonitors } from '$lib/api';
  import VirtualList from '../components/VirtualList.svelte';
  import MonitorRow from '../components/MonitorRow.svelte';

  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(async () => {
    try {
      const result = await getMonitors(1, 500);
      monitorStore.setMonitors(result.data);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load monitors';
    } finally {
      loading = false;
    }
  });
</script>

<section class="space-y-6">
  <h1 class="text-3xl font-bold tracking-tight text-slate-900">Uptime Dashboard</h1>

  <!-- Stats Bar -->
  <div class="grid gap-4 sm:grid-cols-3">
    <article class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <h2 class="text-sm font-medium text-slate-500">Total Monitors</h2>
      <p class="mt-2 text-2xl font-semibold text-slate-900" data-testid="stat-total">
        {monitorStore.totalCount}
      </p>
    </article>
    <article class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <h2 class="text-sm font-medium text-slate-500">Healthy</h2>
      <p class="mt-2 text-2xl font-semibold text-emerald-600" data-testid="stat-healthy">
        {monitorStore.healthyCount}
      </p>
    </article>
    <article class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <h2 class="text-sm font-medium text-slate-500">Unhealthy</h2>
      <p class="mt-2 text-2xl font-semibold text-rose-600" data-testid="stat-unhealthy">
        {monitorStore.unhealthyCount}
      </p>
    </article>
  </div>

  <!-- Monitor List -->
  {#if loading}
    <div class="flex items-center justify-center py-12" data-testid="loading-state">
      <div class="flex items-center gap-3 text-slate-500">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>Loading monitors…</span>
      </div>
    </div>
  {:else if error}
    <div class="rounded-lg border border-red-200 bg-red-50 p-6 text-center" data-testid="error-state">
      <p class="text-sm text-red-700">{error}</p>
    </div>
  {:else if monitorStore.list.length === 0}
    <div class="rounded-lg border border-slate-200 bg-white p-12 text-center" data-testid="empty-state">
      <p class="text-lg font-medium text-slate-700">No monitors yet</p>
      <p class="mt-2 text-sm text-slate-500">Create your first monitor to start tracking uptime.</p>
      <a
        href="/monitors/create"
        class="mt-4 inline-block rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-700"
      >
        Create Monitor
      </a>
    </div>
  {:else}
    <div class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm" data-testid="monitor-list">
      {#snippet row(monitor: import('$lib/types').Monitor, _index: number)}
        <MonitorRow {monitor} />
      {/snippet}

      <VirtualList items={monitorStore.list} itemHeight={64} {row} />
    </div>
  {/if}
</section>
