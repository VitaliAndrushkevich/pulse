<script lang="ts">
  import { goto } from '$app/navigation';
  import type { NotificationChannel } from '$lib/types';
  import NotificationChannelForm from '../../../components/NotificationChannelForm.svelte';
  import { t } from '$lib/i18n';

  function handleSubmit(channel: NotificationChannel) {
    goto('/notifications');
  }

  function handleCancel() {
    goto('/notifications');
  }
</script>

<section class="space-y-6">
  <h1 class="text-2xl font-bold tracking-tight text-primary">{t('notifications.channels.create')}</h1>

  <NotificationChannelForm
    mode="create"
    onSubmit={handleSubmit}
    onCancel={handleCancel}
  />
</section>
