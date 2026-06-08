<script lang="ts">
  import { goto } from '$app/navigation';
  import { createMonitor, createCredential, type CreateCredentialRequest } from '$lib/api';
  import MonitorForm from '../../../components/MonitorForm.svelte';

  async function handleSubmit(
    values: Parameters<typeof createMonitor>[0],
    pendingCredential?: CreateCredentialRequest
  ) {
    const created = await createMonitor(values);

    // If user added auth credentials, create them now that we have the monitor ID
    if (pendingCredential) {
      await createCredential(created.id, pendingCredential);
    }

    await goto(`/monitors/${created.id}`);
  }

  function handleCancel() {
    goto('/monitors');
  }
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-primary">Create Monitor</h1>

  <MonitorForm
    mode="create"
    onSubmit={handleSubmit}
    onCancel={handleCancel}
  />
</section>
