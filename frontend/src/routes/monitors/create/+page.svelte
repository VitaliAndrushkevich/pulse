<script lang="ts">
  import { goto } from '$app/navigation';
  import { createMonitor } from '$lib/api';
  import MonitorForm from '../../../components/MonitorForm.svelte';

  async function handleSubmit(values: Parameters<typeof createMonitor>[0]) {
    const created = await createMonitor(values);
    await goto(`/monitors/${created.id}`);
  }

  function handleCancel() {
    goto('/monitors');
  }
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-slate-900">Create Monitor</h1>

  <MonitorForm
    mode="create"
    onSubmit={handleSubmit}
    onCancel={handleCancel}
  />
</section>
