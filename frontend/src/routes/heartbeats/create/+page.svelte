<script lang="ts">
  import EditorPage from '../../../components/EditorPage.svelte';
  import { goto } from '$app/navigation';
  import { createHeartbeat } from '$lib/api';
  import { heartbeatStore } from '$lib/stores/heartbeats.svelte';
  import HeartbeatForm, { type HeartbeatFormValues } from '../../../components/HeartbeatForm.svelte';
  import ShowOnceModal from '../../../components/ShowOnceModal.svelte';
  import { t } from '$lib/i18n';

  // After create, the raw ping URL is shown once; dismissing navigates to detail.
  let createdId = $state<string | null>(null);
  let showOnceSecret = $state<string | null>(null);

  async function handleSubmit(values: HeartbeatFormValues) {
    const created = await createHeartbeat({
      name: values.name,
      expected_interval_seconds: values.expected_interval_seconds,
      grace_seconds: values.grace_seconds,
      status: values.status,
      tags: values.tags,
    });

    // Store the heartbeat (without the raw token) in the reactive store.
    const { ping_token, ping_url, ping_fail_url, ...heartbeat } = created;
    void ping_token;
    void ping_fail_url;
    heartbeatStore.updateHeartbeat(heartbeat);

    createdId = created.id;
    // Prefer the full ping URL; fall back to the raw token when PULSE_BASE_URL is unset.
    showOnceSecret = ping_url ?? created.ping_token;
  }

  function handleCancel() {
    goto('/heartbeats');
  }

  async function handleDismiss() {
    showOnceSecret = null;
    if (createdId) {
      await goto(`/heartbeats/${createdId}`);
    }
  }
</script>

<EditorPage eyebrow={t('nav.heartbeats')} title={t('heartbeats.create')}>

  <HeartbeatForm mode="create" onSubmit={handleSubmit} onCancel={handleCancel} />
</EditorPage>

{#if showOnceSecret}
  <ShowOnceModal secret={showOnceSecret} onDismiss={handleDismiss} />
{/if}
