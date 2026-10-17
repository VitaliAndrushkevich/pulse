<script lang="ts">
  import { onMount } from 'svelte';
  import {
    listNotificationChannels,
    listHeartbeatBindings,
    createHeartbeatBinding,
    updateHeartbeatBinding,
    deleteHeartbeatBinding,
    type CreateHeartbeatBindingRequest,
    type UpdateHeartbeatBindingRequest,
  } from '$lib/api';
  import type {
    NotificationChannel,
    HeartbeatBinding,
    HeartbeatTriggerCondition,
    HeartbeatTriggerType,
  } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    heartbeatId: string;
  }

  let { heartbeatId }: Props = $props();

  let channels = $state<NotificationChannel[]>([]);
  let bindings = $state<HeartbeatBinding[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let validationError = $state<string | null>(null);
  let noChannelsExist = $state(false);

  // Add-binding form state
  let showAddForm = $state(false);
  let selectedChannelId = $state('');
  let addTriggers = $state<Record<HeartbeatTriggerType, boolean>>({
    heartbeat_missed: true,
    heartbeat_recovered: true,
  });

  // Edit state
  let editingBindingId = $state<string | null>(null);
  let editTriggers = $state<Record<HeartbeatTriggerType, boolean>>({
    heartbeat_missed: false,
    heartbeat_recovered: false,
  });

  // Delete confirmation
  let deletingBindingId = $state<string | null>(null);
  let showDeleteConfirm = $state(false);

  function getChannelName(channelId: string): string {
    const channel = channels.find((c) => c.id === channelId);
    return channel ? channel.name : channelId;
  }

  function getChannelType(channelId: string): string {
    const channel = channels.find((c) => c.id === channelId);
    return channel ? channel.type : '';
  }

  function availableChannels(): NotificationChannel[] {
    const boundChannelIds = bindings.map((b) => b.channel_id);
    return channels.filter((c) => !boundChannelIds.includes(c.id));
  }

  function buildTriggers(triggers: Record<HeartbeatTriggerType, boolean>): HeartbeatTriggerCondition[] {
    const result: HeartbeatTriggerCondition[] = [];
    if (triggers.heartbeat_missed) result.push({ type: 'heartbeat_missed' });
    if (triggers.heartbeat_recovered) result.push({ type: 'heartbeat_recovered' });
    return result;
  }

  function hasAnyTrigger(triggers: Record<HeartbeatTriggerType, boolean>): boolean {
    return Object.values(triggers).some(Boolean);
  }

  async function fetchData() {
    loading = true;
    error = null;
    try {
      const [channelsResult, bindingsResult] = await Promise.all([
        listNotificationChannels(1, 100),
        listHeartbeatBindings(heartbeatId),
      ]);
      channels = channelsResult.data;
      bindings = bindingsResult;
      noChannelsExist = channels.length === 0;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      loading = false;
    }
  }

  function resetAddForm() {
    selectedChannelId = '';
    addTriggers = { heartbeat_missed: true, heartbeat_recovered: true };
    validationError = null;
  }

  async function handleAdd() {
    validationError = null;
    if (!selectedChannelId) return;
    if (!hasAnyTrigger(addTriggers)) {
      validationError = t('errors.unexpected');
      return;
    }

    saving = true;
    try {
      const data: CreateHeartbeatBindingRequest = {
        channel_id: selectedChannelId,
        triggers: buildTriggers(addTriggers),
      };
      const created = await createHeartbeatBinding(heartbeatId, data);
      bindings = [...bindings, created];
      showAddForm = false;
      resetAddForm();
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      saving = false;
    }
  }

  function startEdit(binding: HeartbeatBinding) {
    editingBindingId = binding.id;
    editTriggers = {
      heartbeat_missed: binding.triggers.some((tr) => tr.type === 'heartbeat_missed'),
      heartbeat_recovered: binding.triggers.some((tr) => tr.type === 'heartbeat_recovered'),
    };
    validationError = null;
  }

  function cancelEdit() {
    editingBindingId = null;
    validationError = null;
  }

  async function handleUpdate(bindingId: string) {
    validationError = null;
    if (!hasAnyTrigger(editTriggers)) {
      validationError = t('errors.unexpected');
      return;
    }

    saving = true;
    try {
      const data: UpdateHeartbeatBindingRequest = {
        triggers: buildTriggers(editTriggers),
      };
      const updated = await updateHeartbeatBinding(heartbeatId, bindingId, data);
      bindings = bindings.map((b) => (b.id === bindingId ? updated : b));
      editingBindingId = null;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      saving = false;
    }
  }

  function confirmDelete(bindingId: string) {
    deletingBindingId = bindingId;
    showDeleteConfirm = true;
  }

  async function handleDelete() {
    if (!deletingBindingId) return;
    saving = true;
    try {
      await deleteHeartbeatBinding(heartbeatId, deletingBindingId);
      bindings = bindings.filter((b) => b.id !== deletingBindingId);
      showDeleteConfirm = false;
      deletingBindingId = null;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      saving = false;
    }
  }

  function triggerLabel(type: HeartbeatTriggerType): string {
    return type === 'heartbeat_missed'
      ? t('heartbeats.bindings.triggerMissed')
      : t('heartbeats.bindings.triggerRecovered');
  }

  onMount(() => {
    fetchData();
  });
</script>

<div class="rounded-xl border border-[var(--color-border)] bg-surface" data-testid="heartbeat-bindings-section">
  <div class="border-b border-[var(--color-border)] px-5 py-3 flex items-center justify-between">
    <h2 class="text-sm font-semibold text-primary">{t('heartbeats.bindings.title')}</h2>
    {#if !noChannelsExist && !showAddForm}
      <button
        type="button"
        onclick={() => { showAddForm = true; resetAddForm(); }}
        disabled={availableChannels().length === 0}
        class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
        data-testid="add-hb-binding-button"
      >
        {t('heartbeats.bindings.add')}
      </button>
    {/if}
  </div>

  <div class="p-5">
    {#if loading}
      <div class="flex items-center justify-center py-6">
        <span class="text-sm text-secondary">{t('common.loading')}</span>
      </div>
    {:else if noChannelsExist}
      <div class="text-center py-6" data-testid="no-channels-state">
        <p class="text-sm text-secondary">{t('heartbeats.bindings.empty')}</p>
        <a
          href="/notifications"
          class="mt-3 inline-block rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
        >
          {t('nav.notifications')}
        </a>
      </div>
    {:else}
      {#if error}
        <div class="mb-4 rounded-md border border-rose-200 bg-rose-50 px-4 py-2 text-sm text-rose-700" data-testid="hb-binding-error">
          {error}
        </div>
      {/if}

      {#if bindings.length === 0 && !showAddForm}
        <div class="text-center py-6" data-testid="empty-hb-bindings">
          <p class="text-sm text-secondary">{t('heartbeats.bindings.empty')}</p>
        </div>
      {/if}

      {#each bindings as binding (binding.id)}
        <div class="rounded-lg border border-[var(--color-border)] bg-page p-4 mb-3" data-testid="hb-binding-card">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-primary">{getChannelName(binding.channel_id)}</span>
              <span class="rounded-full px-2 py-0.5 text-[10px] font-medium bg-[var(--color-bg-surface-hover)] text-secondary">
                {getChannelType(binding.channel_id)}
              </span>
            </div>
            {#if editingBindingId !== binding.id}
              <div class="flex items-center gap-1">
                <button
                  type="button"
                  onclick={() => startEdit(binding)}
                  class="rounded px-2 py-1 text-xs text-secondary hover:text-primary hover:bg-[var(--color-bg-surface-hover)] transition"
                  data-testid="edit-hb-binding-button"
                >
                  {t('heartbeats.edit')}
                </button>
                <button
                  type="button"
                  onclick={() => confirmDelete(binding.id)}
                  class="rounded px-2 py-1 text-xs text-secondary hover:text-rose-600 hover:bg-rose-50 transition"
                  data-testid="remove-hb-binding-button"
                >
                  {t('common.remove')}
                </button>
              </div>
            {/if}
          </div>

          {#if editingBindingId !== binding.id}
            <div class="flex flex-wrap gap-1.5">
              {#each binding.triggers as trigger}
                <span class="inline-flex items-center gap-1 rounded-md bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700">
                  {triggerLabel(trigger.type)}
                </span>
              {/each}
            </div>
          {:else}
            <div class="space-y-3 mt-3 pt-3 border-t border-[var(--color-border)]">
              <label class="flex items-center gap-2">
                <input type="checkbox" bind:checked={editTriggers.heartbeat_missed} class="rounded border-[var(--color-border)]" />
                <span class="text-sm text-primary">{t('heartbeats.bindings.triggerMissed')}</span>
              </label>
              <label class="flex items-center gap-2">
                <input type="checkbox" bind:checked={editTriggers.heartbeat_recovered} class="rounded border-[var(--color-border)]" />
                <span class="text-sm text-primary">{t('heartbeats.bindings.triggerRecovered')}</span>
              </label>

              {#if validationError}
                <p class="text-xs text-rose-600" data-testid="hb-validation-error">{validationError}</p>
              {/if}

              <div class="flex items-center gap-2 pt-2">
                <button
                  type="button"
                  onclick={() => handleUpdate(binding.id)}
                  disabled={saving}
                  class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
                  data-testid="save-hb-binding-button"
                >
                  {t('common.save')}
                </button>
                <button
                  type="button"
                  onclick={cancelEdit}
                  disabled={saving}
                  class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
                >
                  {t('common.cancel')}
                </button>
              </div>
            </div>
          {/if}
        </div>
      {/each}

      {#if showAddForm}
        <div class="rounded-lg border border-indigo-200 bg-indigo-50/30 p-4 mt-3" data-testid="add-hb-binding-form">
          <h3 class="text-sm font-medium text-primary mb-3">{t('heartbeats.bindings.add')}</h3>

          <div class="mb-3">
            <label class="text-xs font-medium text-secondary" for="add-hb-channel-select">{t('heartbeats.bindings.title')}</label>
            <select
              id="add-hb-channel-select"
              bind:value={selectedChannelId}
              class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
              data-testid="hb-channel-select"
            >
              <option value="">{t('common.none')}</option>
              {#each availableChannels() as channel}
                <option value={channel.id}>{channel.name} ({channel.type})</option>
              {/each}
            </select>
          </div>

          <div class="mb-3 space-y-2">
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.heartbeat_missed} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('heartbeats.bindings.triggerMissed')}</span>
            </label>
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.heartbeat_recovered} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('heartbeats.bindings.triggerRecovered')}</span>
            </label>
          </div>

          {#if validationError}
            <p class="text-xs text-rose-600 mb-3" data-testid="hb-validation-error">{validationError}</p>
          {/if}

          <div class="flex items-center gap-2">
            <button
              type="button"
              onclick={handleAdd}
              disabled={saving || !selectedChannelId}
              class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
              data-testid="confirm-add-hb-binding"
            >
              {saving ? t('common.saving') : t('heartbeats.bindings.add')}
            </button>
            <button
              type="button"
              onclick={() => { showAddForm = false; resetAddForm(); }}
              disabled={saving}
              class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
          </div>
        </div>
      {/if}
    {/if}
  </div>
</div>

{#if showDeleteConfirm}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
    role="dialog"
    aria-modal="true"
    aria-labelledby="delete-hb-binding-title"
    data-testid="delete-hb-binding-modal"
  >
    <div class="mx-4 w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
      <h3 id="delete-hb-binding-title" class="text-lg font-semibold text-primary">
        {t('common.delete')}
      </h3>
      <div class="mt-5 flex items-center justify-end gap-3">
        <button
          type="button"
          onclick={() => { showDeleteConfirm = false; deletingBindingId = null; }}
          disabled={saving}
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
          data-testid="cancel-delete-hb-binding"
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={handleDelete}
          disabled={saving}
          class="rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2 disabled:opacity-50"
          data-testid="confirm-delete-hb-binding"
        >
          {t('common.delete')}
        </button>
      </div>
    </div>
  </div>
{/if}
