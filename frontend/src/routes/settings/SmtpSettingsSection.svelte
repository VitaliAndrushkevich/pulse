<script lang="ts">
  import { onMount } from 'svelte';
  import {
    getSMTPSettings,
    updateSMTPSettings,
    deleteSMTPSettings,
    testSMTPSettings,
    ApiRequestError,
  } from '$lib/api';
  import type { SMTPSettings, SMTPSettingsRequest } from '$lib/types';
  import { toastStore } from '$lib/stores/toast.svelte';
  import { t } from '$lib/i18n/locale.svelte';

  // Current settings from API
  let settings = $state<SMTPSettings | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);

  // Form state
  let host = $state('');
  let port = $state(587);
  let username = $state('');
  let password = $state('');
  let fromAddress = $state('');
  let tlsEnabled = $state(true);

  // Action states
  let saving = $state(false);
  let testing = $state(false);
  let deleting = $state(false);
  let showDeleteConfirm = $state(false);

  // Validation
  let errors = $state<Record<string, string>>({});

  function populateForm(s: SMTPSettings | null) {
    if (s) {
      host = s.host;
      port = s.port;
      username = s.username ?? '';
      password = '';
      fromAddress = s.from_address;
      tlsEnabled = s.tls_enabled;
    } else {
      host = '';
      port = 587;
      username = '';
      password = '';
      fromAddress = '';
      tlsEnabled = true;
    }
  }

  async function fetchSettings() {
    loading = true;
    loadError = null;
    try {
      settings = await getSMTPSettings();
      populateForm(settings);
    } catch (err: unknown) {
      loadError = t('common.error');
    } finally {
      loading = false;
    }
  }

  function validate(): boolean {
    const newErrors: Record<string, string> = {};

    if (!host.trim()) {
      newErrors.host = t('notifications.validation.smtpHostRequired');
    }

    if (port < 1 || port > 65535 || !Number.isInteger(port)) {
      newErrors.port = t('notifications.validation.smtpPortRange');
    }

    if (!fromAddress.trim()) {
      newErrors.fromAddress = t('notifications.validation.smtpFromRequired');
    } else {
      // Basic RFC 5322 email validation
      const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
      if (!emailRegex.test(fromAddress.trim())) {
        newErrors.fromAddress = t('notifications.validation.smtpFromInvalid');
      }
    }

    errors = newErrors;
    return Object.keys(newErrors).length === 0;
  }

  async function handleSave(event: Event) {
    event.preventDefault();
    if (saving || !validate()) return;

    saving = true;
    try {
      const data: SMTPSettingsRequest = {
        host: host.trim(),
        port,
        from_address: fromAddress.trim(),
        tls_enabled: tlsEnabled,
      };

      if (username.trim()) {
        data.username = username.trim();
      }

      if (password) {
        data.password = password;
      }

      settings = await updateSMTPSettings(data);
      populateForm(settings);
      toastStore.addToast({
        type: 'success',
        message: t('notifications.smtpSettings.saved'),
        persistent: false,
      });
    } catch (err: unknown) {
      // Error toast is handled by the API client
    } finally {
      saving = false;
    }
  }

  async function handleTest() {
    if (testing) return;

    // Validate minimum fields before sending.
    if (!host.trim() || port < 1) {
      toastStore.addToast({
        type: 'error',
        message: t('notifications.smtpSettings.testFailure', { error: 'Host and port are required' }),
        persistent: false,
      });
      return;
    }

    testing = true;
    try {
      // Send current form values so the test works without prior save.
      const testData = {
        host: host.trim(),
        port,
        username: username.trim() || undefined,
        password: password || undefined,
        from_address: fromAddress.trim() || 'test@localhost',
        tls_enabled: tlsEnabled,
      };
      const result = await testSMTPSettings(testData);
      if (result.success) {
        toastStore.addToast({
          type: 'success',
          message: t('notifications.smtpSettings.testSuccess'),
          persistent: false,
        });
      } else {
        toastStore.addToast({
          type: 'error',
          message: t('notifications.smtpSettings.testFailure', { error: result.error ?? '' }),
          persistent: false,
        });
      }
    } catch (err: unknown) {
      // Error toast is handled by the API client for network/unexpected errors
    } finally {
      testing = false;
    }
  }

  async function handleDelete() {
    if (deleting) return;

    deleting = true;
    try {
      await deleteSMTPSettings();
      settings = null;
      populateForm(null);
      showDeleteConfirm = false;
      toastStore.addToast({
        type: 'success',
        message: t('notifications.smtpSettings.deleted'),
        persistent: false,
      });
    } catch (err: unknown) {
      // Error toast is handled by the API client
    } finally {
      deleting = false;
    }
  }

  onMount(() => {
    fetchSettings();
  });
</script>

<section class="space-y-6">
  <!-- Section header -->
  <div>
    <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{t('notifications.smtpSettings.title')}</h2>
    <p class="mt-1 text-sm text-[var(--color-text-secondary)]">
      {t('notifications.smtpSettings.description')}
    </p>
  </div>

  {#if loading}
    <div class="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
      <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
      {t('common.loading')}
    </div>
  {:else if loadError}
    <div class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
      <p>{loadError}</p>
      <button
        onclick={fetchSettings}
        class="mt-2 text-sm font-medium text-red-800 underline hover:text-red-900"
      >
        {t('common.retry')}
      </button>
    </div>
  {:else}
    <!-- Not configured banner -->
    {#if !settings}
      <div class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-4 py-3 text-sm text-[var(--color-text-secondary)]">
        {t('notifications.smtpSettings.notConfigured')}
      </div>
    {/if}

    <!-- SMTP Form -->
    <form onsubmit={handleSave} class="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] p-6 shadow-sm">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <!-- Host -->
        <div class="sm:col-span-2">
          <label for="smtp-host" class="block text-sm font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.host')}
          </label>
          <input
            id="smtp-host"
            type="text"
            bind:value={host}
            placeholder={t('notifications.smtpSettings.hostPlaceholder')}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            class:border-red-500={errors.host}
          />
          {#if errors.host}
            <p class="mt-1 text-xs text-red-600">{errors.host}</p>
          {/if}
        </div>

        <!-- Port -->
        <div>
          <label for="smtp-port" class="block text-sm font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.port')}
          </label>
          <input
            id="smtp-port"
            type="number"
            min="1"
            max="65535"
            bind:value={port}
            placeholder={t('notifications.smtpSettings.portPlaceholder')}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            class:border-red-500={errors.port}
          />
          {#if errors.port}
            <p class="mt-1 text-xs text-red-600">{errors.port}</p>
          {/if}
        </div>

        <!-- Username -->
        <div>
          <label for="smtp-username" class="block text-sm font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.username')}
          </label>
          <input
            id="smtp-username"
            type="text"
            bind:value={username}
            placeholder={t('notifications.smtpSettings.usernamePlaceholder')}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          />
        </div>

        <!-- Password -->
        <div class="sm:col-span-2">
          <label for="smtp-password" class="block text-sm font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.password')}
          </label>
          <input
            id="smtp-password"
            type="password"
            bind:value={password}
            placeholder={t('notifications.smtpSettings.passwordPlaceholder')}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          />
          <!-- Password set indicator -->
          {#if settings?.password_set}
            <p class="mt-1 flex items-center gap-1 text-xs text-green-600">
              <svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
              </svg>
              {t('notifications.smtpSettings.passwordSet')}
            </p>
          {:else}
            <p class="mt-1 text-xs text-[var(--color-text-muted)]">
              {t('notifications.smtpSettings.passwordNotSet')}
            </p>
          {/if}
        </div>

        <!-- From Address -->
        <div class="sm:col-span-2">
          <label for="smtp-from" class="block text-sm font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.fromAddress')}
          </label>
          <input
            id="smtp-from"
            type="email"
            bind:value={fromAddress}
            placeholder={t('notifications.smtpSettings.fromAddressPlaceholder')}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            class:border-red-500={errors.fromAddress}
          />
          <p class="mt-1 text-xs text-[var(--color-text-muted)]">{t('notifications.smtpSettings.fromAddressHelp')}</p>
          {#if errors.fromAddress}
            <p class="mt-1 text-xs text-red-600">{errors.fromAddress}</p>
          {/if}
        </div>

        <!-- TLS Toggle -->
        <div class="sm:col-span-2">
          <label class="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              bind:checked={tlsEnabled}
              class="h-4 w-4 rounded border-[var(--color-border)] text-brand-600 focus:ring-brand-500"
            />
            <span class="text-sm font-medium text-[var(--color-text-primary)]">
              {t('notifications.smtpSettings.tlsEnabled')}
            </span>
          </label>
          <p class="mt-1 ml-7 text-xs text-[var(--color-text-muted)]">
            {t('notifications.smtpSettings.tlsEnabledHelp')}
          </p>
        </div>
      </div>

      <!-- Actions -->
      <div class="mt-6 flex flex-wrap items-center gap-3">
        <!-- Save -->
        <button
          type="submit"
          disabled={saving}
          class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {#if saving}
            {t('common.saving')}
          {:else}
            {t('common.save')}
          {/if}
        </button>

        <!-- Test Connection -->
        {#if settings}
          <button
            type="button"
            onclick={handleTest}
            disabled={testing}
            class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text-primary)] shadow-sm transition hover:bg-[var(--color-bg-elevated)] focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {#if testing}
              {t('notifications.smtpSettings.testingConnection')}
            {:else}
              {t('notifications.smtpSettings.testConnection')}
            {/if}
          </button>
        {/if}

        <!-- Delete -->
        {#if settings}
          <button
            type="button"
            onclick={() => showDeleteConfirm = true}
            class="rounded-md border border-red-200 bg-red-50 px-4 py-2 text-sm font-medium text-red-700 shadow-sm transition hover:bg-red-100 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
          >
            {t('common.delete')}
          </button>
        {/if}
      </div>
    </form>

    <!-- Delete Confirmation Dialog -->
    {#if showDeleteConfirm}
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        role="dialog"
        aria-modal="true"
        aria-labelledby="smtp-delete-title"
      >
        <div class="mx-4 w-full max-w-md rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] p-6 shadow-xl">
          <h3 id="smtp-delete-title" class="text-lg font-medium text-[var(--color-text-primary)]">
            {t('notifications.smtpSettings.deleteConfirm.title')}
          </h3>
          <p class="mt-2 text-sm text-[var(--color-text-secondary)]">
            {t('notifications.smtpSettings.deleteConfirm.description')}
          </p>
          <div class="mt-4 flex justify-end gap-3">
            <button
              type="button"
              onclick={() => showDeleteConfirm = false}
              class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text-primary)] transition hover:bg-[var(--color-bg-elevated)] focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onclick={handleDelete}
              disabled={deleting}
              class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {#if deleting}
                {t('common.deleting')}
              {:else}
                {t('notifications.smtpSettings.deleteConfirm.confirm')}
              {/if}
            </button>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</section>
