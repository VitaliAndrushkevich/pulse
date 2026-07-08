<script lang="ts">
  import { goto } from '$app/navigation';
  import { createMonitor, createCredential, createNotificationBinding, type CreateCredentialRequest, type CreateBindingRequest } from '$lib/api';
  import MonitorForm from '../../../components/MonitorForm.svelte';
  import type { PendingBinding } from '../../../components/PendingNotificationBindings.svelte';
  import { t } from '$lib/i18n';

  async function handleSubmit(
    values: Parameters<typeof createMonitor>[0],
    pendingCredential?: CreateCredentialRequest,
    pendingBindings?: PendingBinding[]
  ) {
    const created = await createMonitor(values);

    // If user added auth credentials, create them now that we have the monitor ID
    if (pendingCredential) {
      await createCredential(created.id, pendingCredential);
    }

    // Create notification bindings if any were configured
    if (pendingBindings && pendingBindings.length > 0) {
      await Promise.all(
        pendingBindings.map((binding) => {
          const req: CreateBindingRequest = {
            channel_id: binding.channel_id,
            triggers: binding.triggers,
            reminder_interval_minutes: binding.reminder_interval_minutes,
          };
          return createNotificationBinding(created.id, req);
        })
      );
    }

    await goto(`/monitors/${created.id}`);
  }

  function handleCancel() {
    goto('/monitors');
  }
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-primary">{t('monitors.create')}</h1>

  <MonitorForm
    mode="create"
    onSubmit={handleSubmit}
    onCancel={handleCancel}
  />
</section>
