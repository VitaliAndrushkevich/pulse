<script lang="ts">
  import type { MonitorType } from '$lib/types';
  import { validateName, validateTarget, validateInterval, validateTimeout, validateType } from '$lib/validation';
  import { formatSecretReference } from '$lib/format';
  import AuthSection from './AuthSection.svelte';

  interface MonitorFormValues {
    name: string;
    type: MonitorType;
    target: string;
    interval_seconds: number;
    timeout_seconds: number;
    status: 'active' | 'paused';
    settings: Record<string, unknown>;
  }

  interface Props {
    mode: 'create' | 'edit';
    initialValues?: Partial<MonitorFormValues>;
    secrets?: Array<{ id: string; name: string }>;
    monitorId?: string;
    onSubmit: (values: MonitorFormValues) => Promise<void>;
    onCancel: () => void;
  }

  let { mode, initialValues, secrets = [], monitorId, onSubmit, onCancel }: Props = $props();

  // Form field state
  let name = $state(initialValues?.name ?? '');
  let type: MonitorType = $state(initialValues?.type ?? 'http');
  let target = $state(initialValues?.target ?? '');

  // Whether the auth section should be visible (HTTP and WebSocket only)
  let showAuthSection = $derived(type === 'http' || type === 'websocket');
  let interval_seconds = $state(initialValues?.interval_seconds ?? 60);
  let timeout_seconds = $state(initialValues?.timeout_seconds ?? 10);
  let status: 'active' | 'paused' = $state(initialValues?.status ?? 'active');

  // Type-specific settings
  let expectedStatusCodes = $state(
    Array.isArray(initialValues?.settings?.expected_status_codes)
      ? (initialValues.settings.expected_status_codes as number[]).join(', ')
      : ''
  );
  let payload = $state((initialValues?.settings?.payload as string) ?? '');
  let handshakeMessage = $state((initialValues?.settings?.handshake_message as string) ?? '');
  let selectedSecretId = $state((initialValues?.settings?.secret_id as string) ?? '');

  // UI state
  let submitting = $state(false);
  let apiError = $state<string | null>(null);

  // Field validation results
  let nameValidation = $derived(validateName(name));
  let typeValidation = $derived(validateType(type));
  let targetValidation = $derived(validateTarget(target));
  let intervalValidation = $derived(validateInterval(interval_seconds));
  let timeoutValidation = $derived(validateTimeout(timeout_seconds));

  // Track touched fields for showing errors only after interaction
  let touched = $state<Record<string, boolean>>({});

  function markTouched(field: string) {
    touched[field] = true;
  }

  // Overall form validity
  let isFormValid = $derived(
    nameValidation.valid &&
    typeValidation.valid &&
    targetValidation.valid &&
    intervalValidation.valid &&
    timeoutValidation.valid
  );

  // Build settings object based on type
  function buildSettings(): Record<string, unknown> {
    const settings: Record<string, unknown> = {};

    if (type === 'http' && expectedStatusCodes.trim()) {
      const codes = expectedStatusCodes
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n));
      if (codes.length > 0) {
        settings.expected_status_codes = codes;
      }
    }

    if (type === 'udp' && payload.trim()) {
      settings.payload = payload;
    }

    if (type === 'websocket' && handshakeMessage.trim()) {
      settings.handshake_message = handshakeMessage;
    }

    if (selectedSecretId) {
      settings.secret_id = selectedSecretId;
    }

    return settings;
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!isFormValid || submitting) return;

    submitting = true;
    apiError = null;

    const values: MonitorFormValues = {
      name: name.trim(),
      type,
      target: target.trim(),
      interval_seconds,
      timeout_seconds,
      status,
      settings: buildSettings()
    };

    try {
      await onSubmit(values);
    } catch (err: unknown) {
      if (err instanceof Error) {
        apiError = err.message;
      } else {
        apiError = 'An unexpected error occurred';
      }
    } finally {
      submitting = false;
    }
  }

  const monitorTypes: MonitorType[] = ['http', 'tcp', 'udp', 'websocket'];
</script>

<form onsubmit={handleSubmit} class="mx-auto max-w-2xl space-y-6" data-testid="monitor-form">
  <!-- API Error Summary -->
  {#if apiError}
    <div
      class="rounded-md border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700"
      role="alert"
      data-testid="api-error"
    >
      <p class="font-medium">Error</p>
      <p>{apiError}</p>
    </div>
  {/if}

  <!-- Name -->
  <div>
    <label for="monitor-name" class="block text-sm font-medium text-slate-700">Name</label>
    <input
      id="monitor-name"
      type="text"
      bind:value={name}
      onblur={() => markTouched('name')}
      placeholder="My Monitor"
      class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-name"
    />
    {#if touched.name && !nameValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-name">{nameValidation.error}</p>
    {/if}
  </div>

  <!-- Type -->
  <div>
    <label for="monitor-type" class="block text-sm font-medium text-slate-700">Type</label>
    <select
      id="monitor-type"
      bind:value={type}
      onblur={() => markTouched('type')}
      class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-type"
    >
      {#each monitorTypes as t}
        <option value={t}>{t === 'http' ? 'HTTP(S)' : t.toUpperCase()}</option>
      {/each}
    </select>
    {#if touched.type && !typeValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-type">{typeValidation.error}</p>
    {/if}
  </div>

  <!-- Target -->
  <div>
    <label for="monitor-target" class="block text-sm font-medium text-slate-700">Target</label>
    <input
      id="monitor-target"
      type="text"
      bind:value={target}
      onblur={() => markTouched('target')}
      placeholder="https://example.com"
      class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-target"
    />
    {#if touched.target && !targetValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-target">{targetValidation.error}</p>
    {/if}
  </div>

  <!-- Interval and Timeout -->
  <div class="grid grid-cols-2 gap-4">
    <div>
      <label for="monitor-interval" class="block text-sm font-medium text-slate-700">Interval (seconds)</label>
      <input
        id="monitor-interval"
        type="number"
        bind:value={interval_seconds}
        onblur={() => markTouched('interval')}
        min="10"
        max="86400"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-interval"
      />
      {#if touched.interval && !intervalValidation.valid}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-interval">{intervalValidation.error}</p>
      {/if}
    </div>

    <div>
      <label for="monitor-timeout" class="block text-sm font-medium text-slate-700">Timeout (seconds)</label>
      <input
        id="monitor-timeout"
        type="number"
        bind:value={timeout_seconds}
        onblur={() => markTouched('timeout')}
        min="1"
        max="300"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-timeout"
      />
      {#if touched.timeout && !timeoutValidation.valid}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-timeout">{timeoutValidation.error}</p>
      {/if}
    </div>
  </div>

  <!-- Status Toggle -->
  <fieldset>
    <legend class="block text-sm font-medium text-slate-700">Status</legend>
    <div class="mt-1 flex items-center gap-4">
      <label class="flex items-center gap-2 text-sm text-slate-600">
        <input
          type="radio"
          bind:group={status}
          value="active"
          class="text-blue-600 focus:ring-blue-500"
          data-testid="input-status-active"
        />
        Active
      </label>
      <label class="flex items-center gap-2 text-sm text-slate-600">
        <input
          type="radio"
          bind:group={status}
          value="paused"
          class="text-blue-600 focus:ring-blue-500"
          data-testid="input-status-paused"
        />
        Paused
      </label>
    </div>
  </fieldset>

  <!-- Type-specific settings -->
  {#if type === 'http'}
    <div>
      <label for="monitor-status-codes" class="block text-sm font-medium text-slate-700">
        Expected Status Codes
      </label>
      <input
        id="monitor-status-codes"
        type="text"
        bind:value={expectedStatusCodes}
        placeholder="200, 201, 204"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-expected-status-codes"
      />
      <p class="mt-1 text-xs text-slate-500">Comma-separated HTTP status codes considered successful</p>
    </div>
  {/if}

  {#if type === 'udp'}
    <div>
      <label for="monitor-payload" class="block text-sm font-medium text-slate-700">Payload</label>
      <input
        id="monitor-payload"
        type="text"
        bind:value={payload}
        placeholder="ping"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-payload"
      />
      <p class="mt-1 text-xs text-slate-500">Data to send to the UDP target</p>
    </div>
  {/if}

  {#if type === 'websocket'}
    <div>
      <label for="monitor-handshake" class="block text-sm font-medium text-slate-700">Handshake Message</label>
      <input
        id="monitor-handshake"
        type="text"
        bind:value={handshakeMessage}
        placeholder="ping"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-handshake-message"
      />
      <p class="mt-1 text-xs text-slate-500">Message to send after WebSocket connection is established</p>
    </div>
  {/if}

  <!-- Authentication Section (HTTP and WebSocket only) -->
  {#if showAuthSection}
    {#if mode === 'edit' && monitorId}
      <AuthSection {monitorId} />
    {:else}
      <section class="space-y-2" data-testid="auth-section-placeholder">
        <div>
          <h3 class="text-sm font-medium text-slate-900">Authentication</h3>
          <p class="mt-1 text-xs text-slate-500">
            Save the monitor first to configure authentication credentials.
          </p>
        </div>
      </section>
    {/if}
  {/if}

  <!-- Secret Reference Dropdown -->
  {#if secrets && secrets.length > 0}
    <div>
      <label for="monitor-secret" class="block text-sm font-medium text-slate-700">Secret Reference</label>
      <select
        id="monitor-secret"
        bind:value={selectedSecretId}
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-secret"
      >
        <option value="">None</option>
        {#each secrets as secret}
          <option value={secret.id}>{formatSecretReference(secret.name, secret.id)}</option>
        {/each}
      </select>
      <p class="mt-1 text-xs text-slate-500">Optional secret to attach to this monitor</p>
    </div>
  {/if}

  <!-- Form Actions -->
  <div class="flex items-center justify-end gap-3 border-t border-slate-200 pt-4">
    <button
      type="button"
      onclick={onCancel}
      class="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50"
      data-testid="btn-cancel"
    >
      Cancel
    </button>
    <button
      type="submit"
      disabled={!isFormValid || submitting}
      class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
      data-testid="btn-submit"
    >
      {#if submitting}
        Saving…
      {:else if mode === 'create'}
        Create Monitor
      {:else}
        Update Monitor
      {/if}
    </button>
  </div>
</form>
