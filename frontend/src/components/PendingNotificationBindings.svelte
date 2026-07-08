<script lang="ts">
  import { onMount } from 'svelte';
  import { listNotificationChannels } from '$lib/api';
  import type { NotificationChannel, TriggerCondition, TriggerType } from '$lib/types';
  import { t } from '$lib/i18n';

  export interface PendingBinding {
    channel_id: string;
    channel_name: string;
    channel_type: string;
    triggers: TriggerCondition[];
    reminder_interval_minutes: number | null;
  }

  interface Props {
    bindings: PendingBinding[];
    onchange: (bindings: PendingBinding[]) => void;
  }

  let { bindings, onchange }: Props = $props();

  let channels = $state<NotificationChannel[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let noChannelsExist = $state(false);

  // Add binding form state
  let showAddForm = $state(false);
  let selectedChannelId = $state('');
  let validationError = $state<string | null>(null);
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

  const reminderOptions: { value: number | null; label: string }[] = [
    { value: null, label: t('notifications.reminders.disabled') },
    { value: 30, label: t('notifications.reminders.minutes', { count: 30 }) },
    { value: 60, label: t('notifications.reminders.hours', { count: 1 }) },
    { value: 120, label: t('notifications.reminders.hours', { count: 2 }) },
    { value: 240, label: t('notifications.reminders.hours', { count: 4 }) },
    { value: 480, label: t('notifications.reminders.hours', { count: 8 }) },
    { value: 720, label: t('notifications.reminders.hours', { count: 12 }) },
    { value: 1440, label: t('notifications.reminders.hours', { count: 24 }) },
  ];

  function availableChannels(): NotificationChannel[] {
    const boundChannelIds = bindings.map((b) => b.channel_id);
    return channels.filter((c) => !boundChannelIds.includes(c.id));
  }

  function buildTriggers(): TriggerCondition[] {
    const result: TriggerCondition[] = [];
    if (addTriggers.monitor_down) result.push({ type: 'monitor_down' });
    if (addTriggers.monitor_up) result.push({ type: 'monitor_up' });
    if (addTriggers.degraded) result.push({ type: 'degraded', threshold_ms: addDegradedThreshold });
    if (addTriggers.ssl_expiring) result.push({ type: 'ssl_expiring', days_before: addSslDays });
    if (addTriggers.n_failures_in_row) result.push({ type: 'n_failures_in_row', count: addFailureCount });
    return result;
  }

  function hasAnyTrigger(): boolean {
    return Object.values(addTriggers).some(Boolean);
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

  function handleAdd() {
    validationError = null;
    if (!selectedChannelId) return;
    if (!hasAnyTrigger()) {
      validationError = t('notifications.triggers.noneSelected');
      return;
    }

    const channel = channels.find((c) => c.id === selectedChannelId);
    if (!channel) return;

    const newBinding: PendingBinding = {
      channel_id: selectedChannelId,
      channel_name: channel.name,
      channel_type: channel.type,
      triggers: buildTriggers(),
      reminder_interval_minutes: addReminderInterval,
    };

    onchange([...bindings, newBinding]);
    showAddForm = false;
    resetAddForm();
  }

  function handleRemove(index: number) {
    const updated = bindings.filter((_, i) => i !== index);
    onchange(updated);
  }

  async function fetchChannels() {
    loading = true;
    error = null;
    try {
      const result = await listNotificationChannels(1, 100);
      channels = result.data;
      noChannelsExist = channels.length === 0;
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to load notification channels';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    fetchChannels();
  });
</script>

<section class="space-y-3" data-testid="pending-notification-bindings">
  <div class="flex items-center justify-between">
    <div>
      <h3 class="text-sm font-medium text-primary">{t('notifications.bindings.title')}</h3>
      <p class="mt-0.5 text-xs text-secondary">{t('notifications.bindings.createDescription')}</p>
    </div>
    {#if !loading && !noChannelsExist && !showAddForm}
      <button
        type="button"
        onclick={() => { showAddForm = true; resetAddForm(); }}
        disabled={availableChannels().length === 0}
        class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
        data-testid="pending-add-binding-button"
      >
        {t('notifications.bindings.add')}
      </button>
    {/if}
  </div>

  {#if loading}
    <div class="flex items-center gap-2 py-3 text-secondary text-sm">
      <svg class="h-4 w-4 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
      <span>{t('common.loading')}</span>
    </div>

  {:else if noChannelsExist}
    <div class="rounded-md border border-[var(--color-border)] bg-page px-4 py-3 text-center" data-testid="no-channels-state">
      <p class="text-sm text-secondary">{t('notifications.bindings.noChannels.title')}</p>
      <p class="mt-1 text-xs text-[var(--color-text-muted)]">{t('notifications.bindings.noChannels.description')}</p>
      <a
        href="/notifications"
        class="mt-2 inline-block text-xs font-medium text-indigo-600 hover:text-indigo-800"
      >
        {t('notifications.bindings.noChannels.action')}
      </a>
    </div>

  {:else}
    {#if error}
      <div class="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700" data-testid="pending-binding-error">
        {error}
      </div>
    {/if}

    <!-- Pending bindings list -->
    {#if bindings.length > 0}
      <div class="space-y-2">
        {#each bindings as binding, index (index)}
          <div class="flex items-center justify-between rounded-lg border border-[var(--color-border)] bg-page px-4 py-3" data-testid="pending-binding-card">
            <div>
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-primary">{binding.channel_name}</span>
                <span class="rounded-full px-2 py-0.5 text-[10px] font-medium bg-[var(--color-bg-surface-hover)] text-secondary">
                  {binding.channel_type}
                </span>
              </div>
              <div class="mt-1.5 flex flex-wrap gap-1">
                {#each binding.triggers as trigger}
                  <span class="inline-flex items-center rounded-md bg-indigo-50 px-2 py-0.5 text-[11px] font-medium text-indigo-700">
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
                  <span class="inline-flex items-center rounded-md bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700">
                    {t('notifications.reminders.minutes', { count: binding.reminder_interval_minutes })}
                  </span>
                {/if}
              </div>
            </div>
            <button
              type="button"
              onclick={() => handleRemove(index)}
              class="rounded p-1 text-secondary hover:text-rose-600 hover:bg-rose-50 transition"
              aria-label={t('notifications.bindings.remove')}
              data-testid="pending-remove-binding-{index}"
            >
              <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
              </svg>
            </button>
          </div>
        {/each}
      </div>
    {:else if !showAddForm}
      <p class="py-2 text-xs text-secondary">{t('notifications.bindings.empty.createHint')}</p>
    {/if}

    <!-- Add binding form -->
    {#if showAddForm}
      <div class="rounded-lg border border-indigo-200 bg-indigo-50/30 p-4" data-testid="pending-add-binding-form">
        <!-- Channel selector -->
        <div class="mb-3">
          <label class="text-xs font-medium text-secondary" for="pending-channel-select">{t('notifications.bindings.channel')}</label>
          <select
            id="pending-channel-select"
            bind:value={selectedChannelId}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
            data-testid="pending-channel-select"
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
              />
              <span class="text-xs text-secondary ml-1">{t('notifications.triggers.nFailuresCount')}</span>
            </div>
          {/if}
        </div>

        <!-- Reminder policy -->
        <div class="mb-3">
          <label class="text-xs font-medium text-secondary" for="pending-reminder-select">{t('notifications.reminders.interval')}</label>
          <select
            id="pending-reminder-select"
            bind:value={addReminderInterval}
            class="mt-1 block w-48 rounded-md border border-[var(--color-border)] bg-page px-2 py-1.5 text-sm text-primary"
            data-testid="pending-reminder-select"
          >
            {#each reminderOptions as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </div>

        <!-- Validation error -->
        {#if validationError}
          <p class="text-xs text-rose-600 mb-3" data-testid="pending-validation-error">{validationError}</p>
        {/if}

        <!-- Form actions -->
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={handleAdd}
            disabled={!selectedChannelId}
            class="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
            data-testid="pending-confirm-add-binding"
          >
            {t('notifications.bindings.add')}
          </button>
          <button
            type="button"
            onclick={() => { showAddForm = false; resetAddForm(); }}
            class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
          >
            {t('common.cancel')}
          </button>
        </div>
      </div>
    {/if}
  {/if}
</section>
