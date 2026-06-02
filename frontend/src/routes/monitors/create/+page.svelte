<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { createMonitor, getSecrets } from '$lib/api';
  import MonitorForm from '../../../components/MonitorForm.svelte';

  let secrets = $state<Array<{ id: string; name: string }>>([]);
  let loadingSecrets = $state(true);

  async function fetchSecrets() {
    try {
      const result = await getSecrets();
      secrets = result.map((s) => ({ id: s.id, name: s.name }));
    } catch {
      // Secrets are optional — if fetch fails, form shows without secret dropdown
    } finally {
      loadingSecrets = false;
    }
  }

  async function handleSubmit(values: Parameters<typeof createMonitor>[0]) {
    const created = await createMonitor(values);
    await goto(`/monitors/${created.id}`);
  }

  function handleCancel() {
    goto('/monitors');
  }

  // Fetch secrets on mount
  $effect(() => {
    untrack(() => fetchSecrets());
  });
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-slate-900">Create Monitor</h1>

  {#if loadingSecrets}
    <div class="flex items-center justify-center p-12" data-testid="loading-state">
      <div class="flex items-center gap-3 text-slate-500">
        <svg class="h-5 w-5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        <span>Loading...</span>
      </div>
    </div>
  {:else}
    <MonitorForm
      mode="create"
      {secrets}
      onSubmit={handleSubmit}
      onCancel={handleCancel}
    />
  {/if}
</section>
