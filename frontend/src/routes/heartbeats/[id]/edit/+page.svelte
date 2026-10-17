<script lang="ts">
  import EditorPage from '../../../../components/EditorPage.svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import { getHeartbeat, updateHeartbeat, ApiRequestError } from '$lib/api';
  import { heartbeatStore } from '$lib/stores/heartbeats.svelte';
  import HeartbeatForm, { type HeartbeatFormValues } from '../../../../components/HeartbeatForm.svelte';
  import HeartbeatBindings from '../../../../components/HeartbeatBindings.svelte';
  import type { Heartbeat } from '$lib/types';
  import { t } from '$lib/i18n';

  let heartbeat = $state<Heartbeat | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);

  let heartbeatId = $derived($page.params.id);

  async function fetchData() {
    loading = true;
    error = null;
    notFound = false;
    try {
      heartbeat = await getHeartbeat(heartbeatId);
    } catch (err: unknown) {
      if (err instanceof ApiRequestError && err.statusCode === 404) {
        notFound = true;
      } else {
        error = err instanceof Error ? err.message : t('heartbeats.loadFailed');
      }
    } finally {
      loading = false;
    }
  }

  async function handleSubmit(values: HeartbeatFormValues) {
    const result = await updateHeartbeat(heartbeatId, {
      name: values.name,
      expected_interval_seconds: values.expected_interval_seconds,
      grace_seconds: values.grace_seconds,
      status: values.status,
      tags: values.tags,
    });
    heartbeatStore.updateHeartbeat(result);
    await goto(`/heartbeats/${heartbeatId}`);
  }

  function handleCancel() {
    goto(`/heartbeats/${heartbeatId}`);
  }

  $effect(() => {
    heartbeatId;
    untrack(() => fetchData());
  });
</script>

{#snippet notificationBindingsSnippet()}
  <HeartbeatBindings heartbeatId={heartbeatId} />
{/snippet}

<EditorPage eyebrow={t('nav.heartbeats')} title={t('heartbeats.edit')}>

  {#if loading}
    <div class="flex items-center justify-center p-12" data-testid="loading-state">
      <span class="text-sm text-secondary">{t('heartbeats.loading')}</span>
    </div>
  {:else if notFound}
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-12 text-center" data-testid="not-found-state">
      <p class="text-lg font-medium text-primary">{t('heartbeats.notFoundTitle')}</p>
      <p class="mt-2 text-sm text-secondary">{t('heartbeats.notFoundDescription')}</p>
      <a
        href="/heartbeats"
        class="mt-4 inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700"
      >
        {t('heartbeats.notFoundAction')}
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
        {t('common.retry')}
      </button>
    </div>
  {:else if heartbeat}
    <HeartbeatForm
      mode="edit"
      initialValues={{
        name: heartbeat.name,
        expected_interval_seconds: heartbeat.expected_interval_seconds,
        grace_seconds: heartbeat.grace_seconds,
        status: heartbeat.status,
      }}
      initialTags={heartbeat.tags}
      onSubmit={handleSubmit}
      onCancel={handleCancel}
      extraSections={notificationBindingsSnippet}
    />

  {/if}
</EditorPage>
