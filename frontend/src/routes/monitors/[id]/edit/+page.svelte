<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import { getMonitor, updateMonitor, ApiRequestError } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import MonitorForm from '../../../../components/MonitorForm.svelte';
  import type { Monitor } from '$lib/types';

  let monitor = $state<Monitor | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);

  let monitorId = $derived($page.params.id);

  async function fetchData() {
    loading = true;
    error = null;
    notFound = false;

    try {
      monitor = await getMonitor(monitorId);
    } catch (err: unknown) {
      if (err instanceof ApiRequestError && err.statusCode === 404) {
        notFound = true;
      } else {
        error = err instanceof Error ? err.message : 'Failed to load monitor. Please try again.';
      }
    } finally {
      loading = false;
    }
  }

  async function handleSubmit(values: Parameters<typeof updateMonitor>[1]) {
    const result = await updateMonitor(monitorId, values);
    monitorStore.updateMonitor(result);
    await goto(`/monitors/${monitorId}`);
  }

  function handleCancel() {
    goto(`/monitors/${monitorId}`);
  }

  // Fetch data on mount
  $effect(() => {
    monitorId;
    untrack(() => fetchData());
  });
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-primary">Edit Monitor</h1>

  {#if loading}
    <div class="flex items-center justify-center p-12" data-testid="loading-state">
      <div class="flex items-center gap-3 text-secondary">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>Loading monitor...</span>
      </div>
    </div>

  {:else if notFound}
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-12 text-center" data-testid="not-found-state">
      <p class="text-lg font-medium text-primary">Monitor not found</p>
      <p class="mt-2 text-sm text-secondary">The monitor you're looking for doesn't exist or has been deleted.</p>
      <a
        href="/monitors"
        class="mt-4 inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700"
      >
        Back to Monitors
      </a>
    </div>

  {:else if error}
    <div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center" data-testid="error-state">
      <p class="text-sm text-rose-700">{error}</p>
      <button
        type="button"
        onclick={() => fetchData()}
        class="mt-3 rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2"
      >
        Retry
      </button>
    </div>

  {:else if monitor}
    <MonitorForm
      mode="edit"
      monitorId={monitorId}
      initialValues={{
        name: monitor.name,
        type: monitor.type,
        target: monitor.target,
        interval_seconds: monitor.interval_seconds,
        timeout_seconds: monitor.timeout_seconds,
        status: monitor.status,
        settings: monitor.settings
      }}
      onSubmit={handleSubmit}
      onCancel={handleCancel}
    />
  {/if}
</section>
