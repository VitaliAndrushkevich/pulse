<script lang="ts">
  import { onMount } from 'svelte';
  import {
    listNotificationChannels,
    listNotificationBindings,
    createNotificationBinding,
    updateNotificationBinding,
    deleteNotificationBinding,
    type CreateBindingRequest,
    type UpdateBindingRequest,
  } from '$lib/api';
  import type { NotificationChannel, ChannelBinding, TriggerCondition, TriggerType } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    monitorId: string;
  }

  let { monitorId }: Props = $props();

  let channels = $state<NotificationChannel[]>([]);
  let bindings = $state<ChannelBinding[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let validationError = $state<string | null>(null);
  let noChannelsExist = $state(false);

  // Add binding form state
  let showAddForm = $state(false);
  let selectedChannelId = $state('');
  let addTriggers = $state<Record<TriggerType, boolean>>({
    monitor_down: true,
    monitor_up: true,
    degraded: false,
    ssl_expiring: false,
    n_failures_in_row: false,
  });
  let addDegradedThreshold = $state(5000);
  let addSslDays = $state(14);
  let addFailureCount = $state(5);
  let addReminderInterval = $state<number | null>(null);

  // Delete confirmation
  let deletingBindingId = $state<string | null>(null);
  let showDeleteConfirm = $state(false);

  // Edit state per binding
  let editingBindingId = $state<string | null>(null);
  let editTriggers = $state<Record<TriggerType, boolean>>({
    monitor_down: false,
    monitor_up: false,
    degraded: false,
    ssl_expiring: false,
    n_failures_in_row: false,
  });
  let editDegradedThreshold = $state(5000);
  let editSslDays = $state(14);
  let editFailureCount = $state(5);
  let editReminderInterval = $state<number | null>(null);

  const reminderOptions: { value: number | null; label: string }[] = [
    { value: null, label: t('notifications.reminders.disabled') },
    { value: 5, label: t('notifications.reminders.minutes', { count: 5 }) },
    { value: 10, label: t('notifications.reminders.minutes', { count: 10 }) },
    { value: 15, label: t('notifications.reminders.minutes', { count: 15 }) },
    { value: 30, label: t('notifications.reminders.minutes', { count: 30 }) },
    { value: 60, label: t('notifications.reminders.minutes', { count: 60 }) },
  ];

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

  function buildTriggers(
    triggers: Record<TriggerType, boolean>,
    degradedMs: number,
    sslDays: number,
    failureCount: number
  ): TriggerCondition[] {
    const result: TriggerCondition[] = [];
    if (triggers.monitor_down) result.push({ type: 'monitor_down' });
    if (triggers.monitor_up) result.push({ type: 'monitor_up' });
    if (triggers.degraded) result.push({ type: 'degraded', threshold_ms: degradedMs });
    if (triggers.ssl_expiring) result.push({ type: 'ssl_expiring', days_before: sslDays });
    if (triggers.n_failures_in_row) result.push({ type: 'n_failures_in_row', count: failureCount });
    return result;
  }

  function hasAnyTrigger(triggers: Record<TriggerType, boolean>): boolean {
    return Object.values(triggers).some(Boolean);
  }

  async function fetchData() {
    loading = true;
    error = null;
    try {
      const [channelsResult, bindingsResult] = await Promise.all([
        listNotificationChannels(1, 100),
        listNotificationBindings(monitorId),
      ]);
      channels = channelsResult.data;
      bindings = bindingsResult;
      noChannelsExist = channels.length === 0;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load notification bindings';
    } finally {
      loading = false;
    }
  }

  function resetAddForm() {
    selectedChannelId = '';
    addTriggers = {
      monitor_down: true,
      monitor_up: true,
      degraded: false,
      ssl_expiring: false,
      n_failures_in_row: false,
    };
    addDegradedThreshold = 5000;
    addSslDays = 14;
    addFailureCount = 5;
    addReminderInterval = null;
    validationError = null;
  }

  async function handleAdd() {
    validationError = null;
    if (!selectedChannelId) return;
    if (!hasAnyTrigger(addTriggers)) {
      validationError = t('notifications.triggers.noneSelected');
      return;
    }

    saving = true;
    try {
      const data: CreateBindingRequest = {
        channel_id: selectedChannelId,
        triggers: buildTriggers(addTriggers, addDegradedThreshold, addSslDays, addFailureCount),
        reminder_interval_minutes: addReminderInterval,
      };
      const created = await createNotificationBinding(monitorId, data);
      bindings = [...bindings, created];
      showAddForm = false;
      resetAddForm();
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to create binding';
    } finally {
      saving = false;
    }
  }

  function startEdit(binding: ChannelBinding) {
    editingBindingId = binding.id;
    editTriggers = {
      monitor_down: binding.triggers.some((t) => t.type === 'monitor_down'),
      monitor_up: binding.triggers.some((t) => t.type === 'monitor_up'),
      degraded: binding.triggers.some((t) => t.type === 'degraded'),
      ssl_expiring: binding.triggers.some((t) => t.type === 'ssl_expiring'),
      n_failures_in_row: binding.triggers.some((t) => t.type === 'n_failures_in_row'),
    };
    const degradedTrigger = binding.triggers.find((t) => t.type === 'degraded');
    editDegradedThreshold = degradedTrigger?.threshold_ms ?? 5000;
    const sslTrigger = binding.triggers.find((t) => t.type === 'ssl_expiring');
    editSslDays = sslTrigger?.days_before ?? 14;
    const failureTrigger = binding.triggers.find((t) => t.type === 'n_failures_in_row');
    editFailureCount = failureTrigger?.count ?? 5;
    editReminderInterval = binding.reminder_interval_minutes;
    validationError = null;
  }

  function cancelEdit() {
    editingBindingId = null;
    validationError = null;
  }

  async function handleUpdate(bindingId: string) {
    validationError = null;
    if (!hasAnyTrigger(editTriggers)) {
      validationError = t('notifications.triggers.noneSelected');
      return;
    }

    saving = true;
    try {
      const data: UpdateBindingRequest = {
        triggers: buildTriggers(editTriggers, editDegradedThreshold, editSslDays, editFailureCount),
        reminder_interval_minutes: editReminderInterval,
      };
      const updated = await updateNotificationBinding(monitorId, bindingId, data);
      bindings = bindings.map((b) => (b.id === bindingId ? updated : b));
      editingBindingId = null;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to update binding';
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
      await deleteNotificationBinding(monitorId, deletingBindingId);
      bindings = bindings.filter((b) => b.id !== deletingBindingId);
      showDeleteConfirm = false;
      deletingBindingId = null;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to delete binding';
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    fetchData();
  });
</script>

<!-- Notifications section -->
<div class="rounded-xl border border-[var(--color-border)] bg-surface" data-testid="notification-bindings-section">
  <div class="border-b border-[var(--color-border)] px-5 py-3 flex items-center justify-between">
    <div>
      <h2 class="text-sm font-semibold text-primary">{t('notifications.bindings.title')}</h2>
      <p class="text-xs text-secondary mt-0.5">{t('notifications.bindings.description')}</p>
    </div>
    {#if !noChannelsExist && !showAddForm}
      <button
        type="button"
        onclick={() => { showAddForm = true; resetAddForm(); }}
        disabled={availableChannels().length === 0}
        class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
        data-testid="add-binding-button"
      >
        {t('notifications.bindings.add')}
      </button>
    {/if}
  </div>

  <div class="p-5">
    {#if loading}
      <div class="flex items-center justify-center py-6">
        <div class="flex items-center gap-2 text-secondary text-sm">
          <svg class="h-4 w-4 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
          </svg>
          <span>{t('common.loading')}</span>
        </div>
      </div>

    <!-- No channels exist state -->
    {:else if noChannelsExist}
      <div class="text-center py-6" data-testid="no-channels-state">
        <p class="text-sm font-medium text-primary">{t('notifications.bindings.noChannels.title')}</p>
        <p class="mt-1 text-xs text-secondary">{t('notifications.bindings.noChannels.description')}</p>
        <a
          href="/notifications"
          class="mt-3 inline-block rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
        >
          {t('notifications.bindings.noChannels.action')}
        </a>
      </div>

    {:else}
      <!-- Error display -->
      {#if error}
        <div class="mb-4 rounded-md border border-rose-200 bg-rose-50 px-4 py-2 text-sm text-rose-700" data-testid="binding-error">
          {error}
        </div>
      {/if}

      <!-- Existing bindings list -->
      {#if bindings.length === 0 && !showAddForm}
        <div class="text-center py-6" data-testid="empty-bindings">
          <p class="text-sm text-secondary">{t('notifications.bindings.empty.title')}</p>
          <p class="text-xs text-[var(--color-text-muted)] mt-1">{t('notifications.bindings.empty.description')}</p>
        </div>
      {/if}

      <!-- Binding cards -->
      {#each bindings as binding (binding.id)}
        <div class="rounded-lg border border-[var(--color-border)] bg-page p-4 mb-3" data-testid="binding-card">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-primary">{getChannelName(binding.channel_id)}</span>
              <span class="rounded-full px-2 py-0.5 text-[10px] font-medium bg-[var(--color-bg-surface-hover)] text-secondary">
                {getChannelType(binding.channel_id)}
              </span>
            </div>
            <div class="flex items-center gap-1">
              {#if editingBindingId !== binding.id}
                <button
                  type="button"
                  onclick={() => startEdit(binding)}
                  class="rounded p-1 text-secondary hover:text-primary hover:bg-[var(--color-bg-surface-hover)] transition"
                  aria-label="Edit binding"
                  data-testid="edit-binding-button"
                >
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
                  </svg>
                </button>
                <button
                  type="button"
                  onclick={() => confirmDelete(binding.id)}
                  class="rounded p-1 text-secondary hover:text-rose-600 hover:bg-rose-50 transition"
                  aria-label={t('notifications.bindings.remove')}
                  data-testid="remove-binding-button"
                >
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                  </svg>
                </button>
              {/if}
            </div>
          </div>

          <!-- Display mode -->
          {#if editingBindingId !== binding.id}
            <div class="flex flex-wrap gap-1.5">
              {#each binding.triggers as trigger}
                <span class="inline-flex items-center gap-1 rounded-md bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700">
                  {#if trigger.type === 'monitor_down'}
                    {t('notifications.triggers.monitorDown')}
                  {:else if trigger.type === 'monitor_up'}
                    {t('notifications.triggers.monitorUp')}
                  {:else if trigger.type === 'degraded'}
                    {t('notifications.triggers.degraded')} ({trigger.threshold_ms}ms)
                  {:else if trigger.type === 'ssl_expiring'}
                    {t('notifications.triggers.sslExpiring')} ({trigger.days_before}d)
                  {:else if trigger.type === 'n_failures_in_row'}
                    {t('notifications.triggers.nFailuresInRow')} (×{trigger.count})
                  {/if}
                </span>
              {/each}
              {#if binding.reminder_interval_minutes}
                <span class="inline-flex items-center gap-1 rounded-md bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                  {t('notifications.reminders.minutes', { count: binding.reminder_interval_minutes })}
                </span>
              {/if}
            </div>

          <!-- Edit mode -->
          {:else}
            <div class="space-y-3 mt-3 pt-3 border-t border-[var(--color-border)]">
              <!-- Trigger checkboxes -->
              <div class="space-y-2">
                <label class="flex items-center gap-2">
                  <input type="checkbox" bind:checked={editTriggers.monitor_down} class="rounded border-[var(--color-border)]" />
                  <span class="text-sm text-primary">{t('notifications.triggers.monitorDown')}</span>
                </label>
                <label class="flex items-center gap-2">
                  <input type="checkbox" bind:checked={editTriggers.monitor_up} class="rounded border-[var(--color-border)]" />
                  <span class="text-sm text-primary">{t('notifications.triggers.monitorUp')}</span>
                </label>
                <label class="flex items-center gap-2">
                  <input type="checkbox" bind:checked={editTriggers.degraded} class="rounded border-[var(--color-border)]" />
                  <span class="text-sm text-primary">{t('notifications.triggers.degraded')}</span>
                </label>
                {#if editTriggers.degraded}
                  <div class="ml-6">
                    <input
                      type="number"
                      min="1"
                      max="60000"
                      bind:value={editDegradedThreshold}
                      class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                      placeholder={t('notifications.triggers.degradedThresholdPlaceholder')}
                    />
                    <span class="text-xs text-secondary ml-1">ms</span>
                  </div>
                {/if}
                <label class="flex items-center gap-2">
                  <input type="checkbox" bind:checked={editTriggers.ssl_expiring} class="rounded border-[var(--color-border)]" />
                  <span class="text-sm text-primary">{t('notifications.triggers.sslExpiring')}</span>
                </label>
                {#if editTriggers.ssl_expiring}
                  <div class="ml-6">
                    <input
                      type="number"
                      min="1"
                      max="365"
                      bind:value={editSslDays}
                      class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                      placeholder={t('notifications.triggers.sslExpiringDaysPlaceholder')}
                    />
                    <span class="text-xs text-secondary ml-1">{t('notifications.triggers.sslExpiringDays')}</span>
                  </div>
                {/if}
                <label class="flex items-center gap-2">
                  <input type="checkbox" bind:checked={editTriggers.n_failures_in_row} class="rounded border-[var(--color-border)]" />
                  <span class="text-sm text-primary">{t('notifications.triggers.nFailuresInRow')}</span>
                </label>
                {#if editTriggers.n_failures_in_row}
                  <div class="ml-6">
                    <input
                      type="number"
                      min="1"
                      max="100"
                      bind:value={editFailureCount}
                      class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                      placeholder={t('notifications.triggers.nFailuresCountPlaceholder')}
                    />
                    <span class="text-xs text-secondary ml-1">{t('notifications.triggers.nFailuresCount')}</span>
                  </div>
                {/if}
              </div>

              <!-- Reminder policy -->
              <div>
                <label class="text-xs font-medium text-secondary" for="edit-reminder-{binding.id}">{t('notifications.reminders.interval')}</label>
                <select
                  id="edit-reminder-{binding.id}"
                  bind:value={editReminderInterval}
                  class="mt-1 block w-48 rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
                >
                  {#each reminderOptions as opt}
                    <option value={opt.value}>{opt.label}</option>
                  {/each}
                </select>
              </div>

              <!-- Validation error -->
              {#if validationError}
                <p class="text-xs text-rose-600" data-testid="validation-error">{validationError}</p>
              {/if}

              <!-- Edit actions -->
              <div class="flex items-center gap-2 pt-2">
                <button
                  type="button"
                  onclick={() => handleUpdate(binding.id)}
                  disabled={saving}
                  class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
                  data-testid="save-binding-button"
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

      <!-- Add binding form -->
      {#if showAddForm}
        <div class="rounded-lg border border-indigo-200 bg-indigo-50/30 p-4 mt-3" data-testid="add-binding-form">
          <h3 class="text-sm font-medium text-primary mb-3">{t('notifications.bindings.add')}</h3>

          <!-- Channel selector -->
          <div class="mb-3">
            <label class="text-xs font-medium text-secondary" for="add-channel-select">{t('notifications.bindings.channel')}</label>
            <select
              id="add-channel-select"
              bind:value={selectedChannelId}
              class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
              data-testid="channel-select"
            >
              <option value="">{t('notifications.bindings.channelPlaceholder')}</option>
              {#each availableChannels() as channel}
                <option value={channel.id}>{channel.name} ({channel.type})</option>
              {/each}
            </select>
          </div>

          <!-- Trigger conditions -->
          <div class="mb-3 space-y-2">
            <p class="text-xs font-medium text-secondary">{t('notifications.triggers.title')}</p>
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.monitor_down} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('notifications.triggers.monitorDown')}</span>
            </label>
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.monitor_up} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('notifications.triggers.monitorUp')}</span>
            </label>
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.degraded} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('notifications.triggers.degraded')}</span>
            </label>
            {#if addTriggers.degraded}
              <div class="ml-6">
                <input
                  type="number"
                  min="1"
                  max="60000"
                  bind:value={addDegradedThreshold}
                  class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                  placeholder={t('notifications.triggers.degradedThresholdPlaceholder')}
                />
                <span class="text-xs text-secondary ml-1">ms</span>
              </div>
            {/if}
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.ssl_expiring} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('notifications.triggers.sslExpiring')}</span>
            </label>
            {#if addTriggers.ssl_expiring}
              <div class="ml-6">
                <input
                  type="number"
                  min="1"
                  max="365"
                  bind:value={addSslDays}
                  class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                  placeholder={t('notifications.triggers.sslExpiringDaysPlaceholder')}
                />
                <span class="text-xs text-secondary ml-1">{t('notifications.triggers.sslExpiringDays')}</span>
              </div>
            {/if}
            <label class="flex items-center gap-2">
              <input type="checkbox" bind:checked={addTriggers.n_failures_in_row} class="rounded border-[var(--color-border)]" />
              <span class="text-sm text-primary">{t('notifications.triggers.nFailuresInRow')}</span>
            </label>
            {#if addTriggers.n_failures_in_row}
              <div class="ml-6">
                <input
                  type="number"
                  min="1"
                  max="100"
                  bind:value={addFailureCount}
                  class="w-28 rounded-md border border-[var(--color-border)] bg-page px-2 py-1 text-sm text-primary"
                  placeholder={t('notifications.triggers.nFailuresCountPlaceholder')}
                />
                <span class="text-xs text-secondary ml-1">{t('notifications.triggers.nFailuresCount')}</span>
              </div>
            {/if}
          </div>

          <!-- Reminder policy -->
          <div class="mb-3">
            <label class="text-xs font-medium text-secondary" for="add-reminder-select">{t('notifications.reminders.interval')}</label>
            <select
              id="add-reminder-select"
              bind:value={addReminderInterval}
              class="mt-1 block w-48 rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
              data-testid="reminder-select"
            >
              {#each reminderOptions as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>

          <!-- Validation error -->
          {#if validationError}
            <p class="text-xs text-rose-600 mb-3" data-testid="validation-error">{validationError}</p>
          {/if}

          <!-- Form actions -->
          <div class="flex items-center gap-2">
            <button
              type="button"
              onclick={handleAdd}
              disabled={saving || !selectedChannelId}
              class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
              data-testid="confirm-add-binding"
            >
              {saving ? t('common.saving') : t('notifications.bindings.add')}
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

<!-- Delete confirmation modal -->
{#if showDeleteConfirm}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
    role="dialog"
    aria-modal="true"
    aria-labelledby="delete-binding-title"
    data-testid="delete-binding-modal"
  >
    <div class="mx-4 w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-surface p-6 shadow-xl">
      <h3 id="delete-binding-title" class="text-lg font-semibold text-primary">
        {t('notifications.bindings.deleteConfirm.title')}
      </h3>
      <p class="mt-2 text-sm text-secondary">
        {t('notifications.bindings.deleteConfirm.description')}
      </p>
      <div class="mt-5 flex items-center justify-end gap-3">
        <button
          type="button"
          onclick={() => { showDeleteConfirm = false; deletingBindingId = null; }}
          disabled={saving}
          class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
          data-testid="cancel-delete-binding"
        >
          {t('common.cancel')}
        </button>
        <button
          type="button"
          onclick={handleDelete}
          disabled={saving}
          class="rounded-md bg-rose-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
          data-testid="confirm-delete-binding"
        >
          {t('notifications.bindings.deleteConfirm.confirm')}
        </button>
      </div>
    </div>
  </div>
{/if}
