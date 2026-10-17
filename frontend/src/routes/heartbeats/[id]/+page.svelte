<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import {
    getHeartbeat,
    deleteHeartbeat,
    rotateHeartbeatToken,
    ApiRequestError,
  } from '$lib/api';
  import { heartbeatStore } from '$lib/stores/heartbeats.svelte';
  import HeartbeatBindings from '../../../components/HeartbeatBindings.svelte';
  import PingExamples from '../../../components/PingExamples.svelte';
  import ShowOnceModal from '../../../components/ShowOnceModal.svelte';
  import type { Heartbeat } from '$lib/types';
  import { formatDate } from '$lib/format';
  import { t } from '$lib/i18n';

  let heartbeat = $state<Heartbeat | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);

  let busy = $state(false);
  let showRotateConfirm = $state(false);
  let showDeleteConfirm = $state(false);
  let showOnceSecret = $state<string | null>(null);

  let heartbeatId = $derived($page.params.id);

  const stateColors: Record<string, string> = {
    up: 'bg-emerald-500',
    down: 'bg-rose-500',
    paused: 'bg-slate-400',
    new: 'bg-amber-400',
  };
  const stateTranslationKeys: Record<string, string> = {
    up: 'heartbeats.state.up',
    down: 'heartbeats.state.down',
    paused: 'heartbeats.state.paused',
    new: 'heartbeats.state.new',
  };

  // The full ping token is write-only; it is never returned on GET. Detail-page
  // usage examples therefore use a placeholder token the operator substitutes
  // (the real URL is surfaced once on create and on rotate via ShowOnceModal).
  const PLACEHOLDER_TOKEN = '<TOKEN>';
  let origin = $derived(typeof window !== 'undefined' ? window.location.origin : '');
  let pingUrl = $derived(`${origin}/ping/${PLACEHOLDER_TOKEN}`);
  let pingFailUrl = $derived(`${origin}/ping/${PLACEHOLDER_TOKEN}/fail`);

  async function fetchData() {
    loading = true;
    error = null;
    notFound = false;
    try {
      heartbeat = await getHeartbeat(heartbeatId);
      heartbeatStore.updateHeartbeat(heartbeat);
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

  async function handleRotate() {
    busy = true;
    try {
      const result = await rotateHeartbeatToken(heartbeatId);
      showRotateConfirm = false;
      // Show the working URL once; fall back to the raw token when base URL is unset.
      showOnceSecret = result.ping_url ?? result.ping_token;
      await fetchData();
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      busy = false;
    }
  }

  async function handleDelete() {
    busy = true;
    try {
      await deleteHeartbeat(heartbeatId);
      heartbeatStore.removeHeartbeat(heartbeatId);
      showDeleteConfirm = false;
      await goto('/heartbeats');
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      busy = false;
    }
  }

  $effect(() => {
    heartbeatId;
    untrack(() => fetchData());
  });
</script>

<section class="space-y-6">
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
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3 min-w-0">
        <span
          class="h-3 w-3 flex-shrink-0 rounded-full {stateColors[heartbeat.state] ?? 'bg-slate-400'}"
          data-testid="hb-state-indicator"
          data-state={heartbeat.state}
        ></span>
        <h1 class="truncate text-2xl font-bold tracking-tight text-primary">{heartbeat.name}</h1>
      </div>
      <div class="flex items-center gap-2">
        <a
          href="/heartbeats/{heartbeat.id}/edit"
          class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
        >
          {t('heartbeats.edit')}
        </a>
        <button
          type="button"
          onclick={() => (showDeleteConfirm = true)}
          class="rounded-md border border-rose-200 bg-surface px-3 py-2 text-sm font-medium text-rose-600 transition hover:bg-rose-50"
          data-testid="hb-delete-button"
        >
          {t('heartbeats.delete')}
        </button>
      </div>
    </div>

    <!-- Overview fields -->
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-6">
      <dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.status')}</dt>
          <dd class="mt-1 text-sm text-primary">{t(stateTranslationKeys[heartbeat.state] ?? 'heartbeats.state.new')}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.expectedInterval')}</dt>
          <dd class="mt-1 text-sm text-primary">{heartbeat.expected_interval_seconds}s</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.grace')}</dt>
          <dd class="mt-1 text-sm text-primary">{heartbeat.grace_seconds}s</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.lastSuccess')}</dt>
          <dd class="mt-1 text-sm text-primary">{formatDate(heartbeat.last_success_at, t('common.never'))}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.nextDeadline')}</dt>
          <dd class="mt-1 text-sm text-primary">{formatDate(heartbeat.next_expected_deadline, t('common.na'))}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.token.prefix')}</dt>
          <dd class="mt-1 font-mono text-sm text-primary">{heartbeat.token_prefix}</dd>
        </div>
      </dl>

      <!-- Tags -->
      {#if heartbeat.tags && heartbeat.tags.length > 0}
        <div class="mt-4">
          <dt class="text-xs font-medium text-secondary">{t('heartbeats.fields.tags')}</dt>
          <div class="mt-2 flex flex-wrap gap-1.5">
            {#each heartbeat.tags as tag}
              <span class="inline-flex items-center rounded-full bg-indigo-50 px-2 py-0.5 text-xs">
                <span class="font-medium text-indigo-700">{tag.key}</span><span class="text-indigo-400">:</span><span class="text-indigo-600">{tag.value}</span>
              </span>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Ping URL / rotate -->
      <div class="mt-6 border-t border-[var(--color-border)] pt-4">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-primary">{t('heartbeats.token.title')}</h2>
          <button
            type="button"
            onclick={() => (showRotateConfirm = true)}
            class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
            data-testid="hb-rotate-button"
          >
            {t('heartbeats.token.rotate')}
          </button>
        </div>
      </div>
    </div>

    <!-- Usage examples -->
    <PingExamples {pingUrl} {pingFailUrl} />

    <!-- Notification bindings -->
    <HeartbeatBindings heartbeatId={heartbeat.id} />
  {/if}
</section>

<!-- Rotate confirmation -->
{#if showRotateConfirm}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true" aria-labelledby="hb-rotate-title">
    <div class="mx-4 w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
      <h3 id="hb-rotate-title" class="text-lg font-semibold text-primary">{t('heartbeats.token.rotate')}</h3>
      <p class="mt-2 text-sm text-secondary">{t('heartbeats.token.rotateConfirm')}</p>
      <div class="mt-5 flex items-center justify-end gap-3">
        <button
          type="button"
          onclick={() => (showRotateConfirm = false)}
          disabled={busy}
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={handleRotate}
          disabled={busy}
          class="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
          data-testid="confirm-hb-rotate"
        >
          {t('heartbeats.token.rotate')}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Delete confirmation -->
{#if showDeleteConfirm}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true" aria-labelledby="hb-delete-title">
    <div class="mx-4 w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
      <h3 id="hb-delete-title" class="text-lg font-semibold text-primary">{t('heartbeats.delete')}</h3>
      <p class="mt-2 text-sm text-secondary">{t('heartbeats.deleteConfirm')}</p>
      <div class="mt-5 flex items-center justify-end gap-3">
        <button
          type="button"
          onclick={() => (showDeleteConfirm = false)}
          disabled={busy}
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={handleDelete}
          disabled={busy}
          class="rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 disabled:opacity-50"
          data-testid="confirm-hb-delete"
        >
          {t('heartbeats.delete')}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Raw ping URL shown once after rotate -->
{#if showOnceSecret}
  <ShowOnceModal secret={showOnceSecret} onDismiss={() => (showOnceSecret = null)} />
{/if}
