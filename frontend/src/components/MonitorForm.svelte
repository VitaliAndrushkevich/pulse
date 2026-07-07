<script lang="ts">
  import type { MonitorType, GrpcSettings, DnsSettings, IcmpSettings, SmtpSettings, Tag } from '$lib/types';
  import type { CreateCredentialRequest } from '$lib/api';
  import { validateName, validateTarget, validateInterval, validateTimeout, validateType } from '$lib/validation';
  import { formatSecretReference } from '$lib/format';
  import { t } from '$lib/i18n';
  import AuthSection from './AuthSection.svelte';
  import CredentialForm from './CredentialForm.svelte';
  import GrpcSettingsForm from './GrpcSettingsForm.svelte';
  import DnsSettingsForm from './DnsSettingsForm.svelte';
  import IcmpSettingsForm from './IcmpSettingsForm.svelte';
  import SmtpSettingsForm from './SmtpSettingsForm.svelte';
  import TagEditor from './TagEditor.svelte';

  interface MonitorFormValues {
    name: string;
    type: MonitorType;
    target: string;
    interval_seconds: number;
    timeout_seconds: number;
    status: 'active' | 'paused';
    settings: Record<string, unknown>;
    tags?: Tag[];
    history_retention_days?: number;
  }

  export interface MonitorFormSubmitData {
    values: MonitorFormValues;
    pendingCredential?: CreateCredentialRequest;
  }

  interface Props {
    mode: 'create' | 'edit';
    initialValues?: Partial<MonitorFormValues>;
    initialTags?: Tag[];
    secrets?: Array<{ id: string; name: string }>;
    monitorId?: string;
    onSubmit: (values: MonitorFormValues, pendingCredential?: CreateCredentialRequest) => Promise<void>;
    onCancel: () => void;
    extraSections?: import('svelte').Snippet;
  }

  let { mode, initialValues, initialTags = [], secrets = [], monitorId, onSubmit, onCancel, extraSections }: Props = $props();

  // Form field state
  let name = $state(initialValues?.name ?? '');
  let type: MonitorType = $state(initialValues?.type ?? 'http');
  let target = $state(initialValues?.target ?? '');

  // Whether the auth section should be visible (HTTP-family and WebSocket)
  let showAuthSection = $derived(type === 'http' || type === 'http3' || type === 'websocket' || type === 'quic');
  let interval_seconds = $state(initialValues?.interval_seconds ?? 60);
  let timeout_seconds = $state(initialValues?.timeout_seconds ?? 10);
  let status: 'active' | 'paused' = $state(initialValues?.status ?? 'active');

  // History retention days (1-365, default 30)
  let history_retention_days = $state<number>(
    initialValues?.history_retention_days ?? 30
  );

  // Tags (key-value pairs)
  let formTags = $state<Tag[]>(initialTags);

  // Type-specific settings
  let expectedStatusCodes = $state(
    Array.isArray(initialValues?.settings?.expected_statuses)
      ? (initialValues.settings.expected_statuses as number[]).join(', ')
      : ''
  );
  let skipTLSVerify = $state((initialValues?.settings?.skip_tls_verify as boolean) ?? false);

  // Custom headers (up to 5 non-secret key-value pairs for HTTP-family monitors)
  type HeaderEntry = { key: string; value: string };
  let customHeaders = $state<HeaderEntry[]>(
    (() => {
      const h = initialValues?.settings?.headers;
      if (h && typeof h === 'object' && !Array.isArray(h)) {
        return Object.entries(h as Record<string, string>)
          .slice(0, 5)
          .map(([key, value]) => ({ key, value }));
      }
      return [];
    })()
  );

  let payload = $state((initialValues?.settings?.payload as string) ?? '');
  let handshakeMessage = $state((initialValues?.settings?.handshake_message as string) ?? '');
  let selectedSecretId = $state((initialValues?.settings?.secret_id as string) ?? '');

  // gRPC-specific settings
  let grpcSettings: GrpcSettings = $state(
    initialValues?.type === 'grpc' && initialValues?.settings
      ? {
          service_method: (initialValues.settings.service_method as string) ?? 'grpc.health.v1.Health/Check',
          tls_mode: (initialValues.settings.tls_mode as GrpcSettings['tls_mode']) ?? 'tls',
          ssl_expiry_threshold: initialValues.settings.ssl_expiry_threshold as number | undefined,
          metadata: initialValues.settings.metadata as Record<string, string> | undefined,
          expected_statuses: (initialValues.settings.expected_statuses as number[]) ?? [0],
          request_payload: initialValues.settings.request_payload as string | undefined,
        }
      : {
          service_method: 'grpc.health.v1.Health/Check',
          tls_mode: 'tls',
          expected_statuses: [0],
        }
  );

  // DNS-specific settings
  let dnsSettings: DnsSettings = $state(
    initialValues?.type === 'dns' && initialValues?.settings
      ? {
          record_type: (initialValues.settings.record_type as DnsSettings['record_type']) ?? 'A',
          expected_value: initialValues.settings.expected_value as string | undefined,
          dns_server: initialValues.settings.dns_server as string | undefined,
        }
      : { record_type: 'A' }
  );

  // ICMP-specific settings
  let icmpSettings: IcmpSettings = $state(
    initialValues?.type === 'icmp' && initialValues?.settings
      ? {
          packet_count: (initialValues.settings.packet_count as number) ?? 3,
          loss_threshold_percent: (initialValues.settings.loss_threshold_percent as number) ?? 100,
          use_ipv6: (initialValues.settings.use_ipv6 as boolean) ?? false,
        }
      : { packet_count: 3, loss_threshold_percent: 100, use_ipv6: false }
  );

  // SMTP-specific settings
  let smtpSettings: SmtpSettings = $state(
    initialValues?.type === 'smtp' && initialValues?.settings
      ? {
          port: (initialValues.settings.port as number) ?? 25,
          starttls: (initialValues.settings.starttls as boolean) ?? true,
          implicit_tls: (initialValues.settings.implicit_tls as boolean) ?? false,
          ehlo_domain: initialValues.settings.ehlo_domain as string | undefined,
          ssl_expiry_threshold: initialValues.settings.ssl_expiry_threshold as number | undefined,
        }
      : { port: 25, starttls: true, implicit_tls: false }
  );

  // Pending credential for create mode (saved locally, sent after monitor creation)
  let pendingCredential = $state<CreateCredentialRequest | null>(null);

  function handlePendingCredential(req: CreateCredentialRequest) {
    pendingCredential = req;
  }

  function clearPendingCredential() {
    pendingCredential = null;
  }

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

  // Dynamic placeholder based on monitor type
  let targetPlaceholder = $derived(
    type === 'tcp' ? 'example.com:443' :
    type === 'udp' ? 'example.com:53' :
    type === 'websocket' ? 'wss://example.com/ws' :
    type === 'grpc' ? 'example.com:443' :
    type === 'dns' ? 'example.com' :
    type === 'icmp' ? '8.8.8.8' :
    type === 'smtp' ? 'mail.example.com' :
    type === 'quic' ? 'https://example.com:4433' :
    'https://example.com'
  );

  // Helper text for target field based on monitor type
  let targetHelp = $derived(
    type === 'tcp' ? t('monitors.form.targetHelp.tcp') :
    type === 'udp' ? t('monitors.form.targetHelp.udp') :
    type === 'websocket' ? t('monitors.form.targetHelp.websocket') :
    type === 'grpc' ? t('monitors.form.targetHelp.grpc') :
    type === 'dns' ? t('monitors.form.targetHelp.dns') :
    type === 'icmp' ? t('monitors.form.targetHelp.icmp') :
    type === 'smtp' ? t('monitors.form.targetHelp.smtp') :
    type === 'quic' ? t('monitors.form.targetHelp.quic') :
    t('monitors.form.targetHelp.http')
  );

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
    if (type === 'grpc') {
      return grpcSettings as unknown as Record<string, unknown>;
    }

    if (type === 'dns') {
      return dnsSettings as unknown as Record<string, unknown>;
    }

    if (type === 'icmp') {
      return icmpSettings as unknown as Record<string, unknown>;
    }

    if (type === 'smtp') {
      return smtpSettings as unknown as Record<string, unknown>;
    }

    const settings: Record<string, unknown> = {};

    if ((type === 'http' || type === 'http3' || type === 'quic') && expectedStatusCodes.trim()) {
      const codes = expectedStatusCodes
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n));
      if (codes.length > 0) {
        settings.expected_statuses = codes;
      }
    }

    // include skip_tls_verify for HTTP-family monitors (disable certificate verification)
    if ((type === 'http' || type === 'http3' || type === 'quic') && skipTLSVerify) {
      settings.skip_tls_verify = true;
    }

    // include custom headers for HTTP-family monitors (non-secret key-value pairs)
    if (type === 'http' || type === 'http3' || type === 'quic') {
      const validHeaders = customHeaders.filter(h => h.key.trim() && h.value.trim());
      if (validHeaders.length > 0) {
        const headersMap: Record<string, string> = {};
        for (const h of validHeaders) {
          headersMap[h.key.trim()] = h.value.trim();
        }
        settings.headers = headersMap;
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
      settings: buildSettings(),
      tags: formTags.length > 0 ? formTags : undefined,
      history_retention_days
    };

    try {
      await onSubmit(values, pendingCredential ?? undefined);
    } catch (err: unknown) {
      if (err instanceof Error) {
        apiError = err.message;
      } else {
        apiError = t('monitors.form.unexpectedError');
      }
    } finally {
      submitting = false;
    }
  }

  const monitorTypes: MonitorType[] = ['http', 'http3', 'tcp', 'udp', 'websocket', 'grpc', 'dns', 'icmp', 'smtp', 'quic'];
</script>

<form onsubmit={handleSubmit} class="mx-auto max-w-2xl space-y-6" data-testid="monitor-form">
  <!-- API Error Summary -->
  {#if apiError}
    <div
      class="rounded-md border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700"
      role="alert"
      data-testid="api-error"
    >
      <p class="font-medium">{t('monitors.form.error')}</p>
      <p>{apiError}</p>
    </div>
  {/if}

  <!-- Name -->
  <div>
    <label for="monitor-name" class="block text-sm font-medium text-primary">{t('monitors.form.name')}</label>
    <input
      id="monitor-name"
      type="text"
      bind:value={name}
      onblur={() => markTouched('name')}
      placeholder={t('monitors.form.namePlaceholder')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-name"
    />
    {#if touched.name && !nameValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-name">{nameValidation.error}</p>
    {/if}
  </div>

  <!-- Type -->
  <div>
    <label for="monitor-type" class="block text-sm font-medium text-primary">{t('monitors.form.type')}</label>
    <select
      id="monitor-type"
      bind:value={type}
      onblur={() => markTouched('type')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-type"
    >
      {#each monitorTypes as t}
        <option value={t}>{t === 'http' ? 'HTTP(S)' : t === 'http3' ? 'HTTP/3' : t === 'quic' ? 'QUIC' : t === 'grpc' ? 'gRPC' : t === 'dns' ? 'DNS' : t === 'icmp' ? 'ICMP' : t === 'smtp' ? 'SMTP' : t.toUpperCase()}</option>
      {/each}
    </select>
    {#if touched.type && !typeValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-type">{typeValidation.error}</p>
    {/if}
  </div>

  <!-- Target -->
  <div>
    <label for="monitor-target" class="block text-sm font-medium text-primary">{t('monitors.form.target')}</label>
    <input
      id="monitor-target"
      type="text"
      bind:value={target}
      onblur={() => markTouched('target')}
      placeholder={targetPlaceholder}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-target"
    />
    {#if touched.target && !targetValidation.valid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-target">{targetValidation.error}</p>
    {/if}
    <p class="mt-1 text-xs text-secondary">{targetHelp}</p>
  </div>

  <!-- Interval and Timeout -->
  <div class="grid grid-cols-2 gap-4">
    <div>
      <label for="monitor-interval" class="block text-sm font-medium text-primary">{t('monitors.form.interval')}</label>
      <input
        id="monitor-interval"
        type="number"
        bind:value={interval_seconds}
        onblur={() => markTouched('interval')}
        min="10"
        max="86400"
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-interval"
      />
      {#if touched.interval && !intervalValidation.valid}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-interval">{intervalValidation.error}</p>
      {/if}
    </div>

    <div>
      <label for="monitor-timeout" class="block text-sm font-medium text-primary">{t('monitors.form.timeout')}</label>
      <input
        id="monitor-timeout"
        type="number"
        bind:value={timeout_seconds}
        onblur={() => markTouched('timeout')}
        min="1"
        max="300"
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-timeout"
      />
      {#if touched.timeout && !timeoutValidation.valid}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-timeout">{timeoutValidation.error}</p>
      {/if}
    </div>
  </div>

  <!-- Status Toggle -->
  <fieldset>
    <legend class="block text-sm font-medium text-primary">{t('monitors.form.status')}</legend>
    <div class="mt-1 flex items-center gap-4">
      <label class="flex items-center gap-2 text-sm text-secondary">
        <input
          type="radio"
          bind:group={status}
          value="active"
          class="text-blue-600 focus:ring-blue-500"
          data-testid="input-status-active"
        />
        {t('monitors.form.statusActive')}
      </label>
      <label class="flex items-center gap-2 text-sm text-secondary">
        <input
          type="radio"
          bind:group={status}
          value="paused"
          class="text-blue-600 focus:ring-blue-500"
          data-testid="input-status-paused"
        />
        {t('monitors.form.statusPaused')}
      </label>
    </div>
  </fieldset>

  <!-- History Retention Days -->
  <div>
    <label for="monitor-retention" class="block text-sm font-medium text-primary">{t('monitors.form.retention')}</label>
    <input
      id="monitor-retention"
      type="number"
      bind:value={history_retention_days}
      min="1"
      max="365"
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-retention-days"
    />
    <p class="mt-1 text-xs text-secondary">{t('monitors.form.retentionHelp')}</p>
  </div>

  <!-- Type-specific settings -->
  {#if type === 'http' || type === 'http3' || type === 'quic'}
    <div>
      <label for="monitor-status-codes" class="block text-sm font-medium text-primary">
        {t('monitors.form.expectedStatusCodes')}
      </label>
      <input
        id="monitor-status-codes"
        type="text"
        bind:value={expectedStatusCodes}
        placeholder={t('monitors.form.expectedStatusCodesPlaceholder')}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-expected-status-codes"
      />
      <p class="mt-1 text-xs text-secondary">{t('monitors.form.expectedStatusCodesHelp')}</p>
    </div>
    <div class="mt-2">
      <label class="inline-flex items-center gap-2 text-sm text-slate-700">
        <input id="skip-tls-verify" type="checkbox" bind:checked={skipTLSVerify} class="h-4 w-4" data-testid="input-skip-tls-verify" />
        <span>{t('monitors.form.skipTlsVerify')}</span>
      </label>
      <p class="mt-1 text-xs text-slate-500">{t('monitors.form.skipTlsVerifyHelp')}</p>
    </div>

    <!-- Custom Headers (up to 5, non-secret) -->
    <div class="mt-4">
      <div class="flex items-center justify-between">
        <span class="block text-sm font-medium text-primary">{t('monitors.form.customHeaders')}</span>
        {#if customHeaders.length < 5}
          <button
            type="button"
            onclick={() => { customHeaders.push({ key: '', value: '' }); }}
            class="text-xs font-medium text-blue-600 hover:text-blue-800"
            data-testid="btn-add-header"
          >
            {t('monitors.form.addHeader')}
          </button>
        {/if}
      </div>
      <p class="mt-1 text-xs text-secondary">
        {t('monitors.form.customHeadersHelp')}
      </p>
      {#if customHeaders.length > 0}
        <div class="mt-2 space-y-2">
          {#each customHeaders as header, i (i)}
            <div class="flex items-center gap-2">
              <input
                type="text"
                bind:value={header.key}
                placeholder={t('monitors.form.headerNamePlaceholder')}
                class="block w-1/3 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                data-testid="input-header-key-{i}"
              />
              <input
                type="text"
                bind:value={header.value}
                placeholder={t('monitors.form.headerValuePlaceholder')}
                class="block flex-1 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                data-testid="input-header-value-{i}"
              />
              <button
                type="button"
                onclick={() => { customHeaders.splice(i, 1); }}
                class="rounded p-1 text-slate-400 hover:text-rose-600"
                aria-label={t('monitors.form.removeHeader')}
                data-testid="btn-remove-header-{i}"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                </svg>
              </button>
            </div>
          {/each}
        </div>
      {/if}
    </div>

  {/if}

  {#if type === 'udp'}
    <div>
      <label for="monitor-payload" class="block text-sm font-medium text-primary">{t('monitors.form.payload')}</label>
      <input
        id="monitor-payload"
        type="text"
        bind:value={payload}
        placeholder={t('monitors.form.payloadPlaceholder')}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-payload"
      />
      <p class="mt-1 text-xs text-secondary">{t('monitors.form.payloadHelp')}</p>
    </div>
  {/if}

  {#if type === 'websocket'}
    <div>
      <label for="monitor-handshake" class="block text-sm font-medium text-primary">{t('monitors.form.handshakeMessage')}</label>
      <input
        id="monitor-handshake"
        type="text"
        bind:value={handshakeMessage}
        placeholder={t('monitors.form.handshakeMessagePlaceholder')}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-handshake-message"
      />
      <p class="mt-1 text-xs text-secondary">{t('monitors.form.handshakeMessageHelp')}</p>
    </div>
  {/if}

  {#if type === 'grpc'}
    <GrpcSettingsForm bind:settings={grpcSettings} {monitorId} {target} />
  {/if}

  {#if type === 'dns'}
    <DnsSettingsForm bind:settings={dnsSettings} />
  {/if}

  {#if type === 'icmp'}
    <IcmpSettingsForm bind:settings={icmpSettings} />
  {/if}

  {#if type === 'smtp'}
    <SmtpSettingsForm bind:settings={smtpSettings} />
  {/if}

  <!-- Authentication Section (HTTP and WebSocket only) -->
  {#if showAuthSection}
    {#if mode === 'edit' && monitorId}
      <AuthSection {monitorId} />
    {:else}
      <section class="space-y-4" data-testid="auth-section-create">
        <div>
          <h3 class="text-sm font-medium text-primary">{t('auth.title')}</h3>
          <p class="mt-1 text-xs text-secondary">
            {t('auth.createDescription')}
          </p>
        </div>

        {#if pendingCredential}
          <div class="flex items-center justify-between rounded-md border border-green-200 bg-green-50 px-3 py-2">
            <p class="text-sm text-green-800">
              {t('auth.pendingCredential', { authType: pendingCredential.auth_type })}
            </p>
            <button
              type="button"
              onclick={clearPendingCredential}
              class="text-xs font-medium text-green-700 hover:text-green-900"
              data-testid="btn-remove-pending-credential"
            >
              {t('common.remove')}
            </button>
          </div>
        {:else}
          <CredentialForm onSubmit={handlePendingCredential} inline />
        {/if}
      </section>
    {/if}
  {/if}

  <!-- Secret Reference Dropdown -->
  {#if secrets && secrets.length > 0}
    <div>
      <label for="monitor-secret" class="block text-sm font-medium text-primary">{t('monitors.form.secretReference')}</label>
      <select
        id="monitor-secret"
        bind:value={selectedSecretId}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-secret"
      >
        <option value="">{t('common.none')}</option>
        {#each secrets as secret}
          <option value={secret.id}>{formatSecretReference(secret.name, secret.id)}</option>
        {/each}
      </select>
      <p class="mt-1 text-xs text-secondary">{t('monitors.form.secretReferenceHelp')}</p>
    </div>
  {/if}

  <!-- Tags -->
  <TagEditor tags={formTags} onchange={(updated) => { formTags = updated; }} />

  <!-- Extra sections (e.g. notification bindings in edit mode) -->
  {#if extraSections}
    {@render extraSections()}
  {/if}

  <!-- Form Actions -->
  <div class="flex items-center justify-end gap-3 border-t border-[var(--color-border)] pt-4">
    <button
      type="button"
      onclick={onCancel}
      class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
      data-testid="btn-cancel"
    >
      {t('common.cancel')}
    </button>
    <button
      type="submit"
      disabled={!isFormValid || submitting}
      class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
      data-testid="btn-submit"
    >
      {#if submitting}
        {t('common.saving')}
      {:else if mode === 'create'}
        {t('monitors.form.submitCreate')}
      {:else}
        {t('monitors.form.submitUpdate')}
      {/if}
    </button>
  </div>
</form>
