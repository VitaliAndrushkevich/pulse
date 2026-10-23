<script lang="ts">
  import { untrack } from 'svelte';
  import SelectField from './SelectField.svelte';
  import type {
    NotificationChannelType,
    NotificationChannel,
    WebhookMethod,
    NotificationChannelConfig,
    SlackChannelConfig,
    TelegramChannelConfig,
    MatrixChannelConfig,
    EmailChannelConfig,
    WebhookChannelConfig,
    TemplateVariableGroup,
  } from '$lib/types';
  import {
    createNotificationChannel,
    updateNotificationChannel,
    getTemplateVariables,
    testNotificationChannelDraft,
    type CreateNotificationChannelRequest,
    type UpdateNotificationChannelRequest,
  } from '$lib/api';
  import { t } from '$lib/i18n';

  // ---------------------------------------------------------------------------
  // Props
  // ---------------------------------------------------------------------------

  interface Props {
    mode: 'create' | 'edit';
    initialData?: NotificationChannel;
    onSubmit: (channel: NotificationChannel) => void;
    onCancel: () => void;
  }

  let { mode, initialData, onSubmit, onCancel }: Props = $props();

  // ---------------------------------------------------------------------------
  // Form State
  // ---------------------------------------------------------------------------

  // Editable state is intentionally initialized once when this form mounts.
  const seed = untrack(() => initialData);
  let name = $state(seed?.name ?? '');
  let channelType: NotificationChannelType = $state(seed?.type ?? 'email');

  // Email fields
  let recipients = $state<string[]>(
    seed?.type === 'email'
      ? (seed.config as EmailChannelConfig).recipients ?? []
      : []
  );
  let newRecipient = $state('');

  // Webhook fields
  let webhookUrl = $state(
    seed?.type === 'webhook'
      ? (seed.config as WebhookChannelConfig).url ?? ''
      : ''
  );
  let webhookMethod: WebhookMethod = $state(
    seed?.type === 'webhook'
      ? (seed.config as WebhookChannelConfig).method ?? 'POST'
      : 'POST'
  );
  interface HeaderRow { id: number; name: string; value: string; originalName: string; saved: boolean; replace: boolean }
  let nextHeaderId = 0;
  let webhookHeaders = $state<HeaderRow[]>(
    seed?.type === 'webhook'
      ? ((seed.config as WebhookChannelConfig).headers ?? []).map(header => ({
          id: nextHeaderId++, name: header.name, value: '', originalName: header.name,
          saved: header.value_configured === true || header.value === '[REDACTED]', replace: false,
        })) : []
  );
  let bodyTemplate = $state(
    seed?.type === 'webhook'
      ? (seed.config as WebhookChannelConfig).body_template ?? ''
      : '{ "text": "{{.Monitor.Name}} is {{.Status}}" }'
  );

  const slackSeed = seed?.type === 'slack' ? seed.config as SlackChannelConfig : undefined;
  const telegramSeed = seed?.type === 'telegram' ? seed.config as TelegramChannelConfig : undefined;
  const matrixSeed = seed?.type === 'matrix' ? seed.config as MatrixChannelConfig : undefined;
  let slackWebhookUrl = $state('');
  let telegramBotToken = $state('');
  let telegramChatId = $state(telegramSeed?.chat_id ?? '');
  let telegramThreadId = $state<number | undefined>(telegramSeed?.message_thread_id);
  let matrixHomeserverUrl = $state(matrixSeed?.homeserver_url ?? '');
  let matrixRoomId = $state(matrixSeed?.room_id ?? '');
  let matrixAccessToken = $state('');

  // Template variables reference
  let templateVariables = $state<TemplateVariableGroup[]>([]);
  let templateVarsLoading = $state(false);
  let templateVarsError = $state<string | null>(null);

  // UI state
  let submitting = $state(false);
  let apiError = $state<string | null>(null);

  // Track touched fields
  let touched = $state<Record<string, boolean>>({});

  function markTouched(field: string) {
    touched[field] = true;
  }

  // ---------------------------------------------------------------------------
  // Validation
  // ---------------------------------------------------------------------------

  const WEBHOOK_METHODS: WebhookMethod[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];

  let nameError = $derived(
    name.trim().length === 0
      ? t('notifications.validation.nameRequired')
      : name.length > 100
        ? t('notifications.validation.nameTooLong')
        : null
  );

  let recipientsError = $derived(
    channelType === 'email' && recipients.length === 0
      ? t('notifications.validation.recipientsRequired')
      : channelType === 'email' && recipients.length > 50
        ? t('notifications.validation.recipientsTooMany')
        : null
  );

  let urlError = $derived(
    channelType === 'webhook' && webhookUrl.trim().length === 0
      ? t('notifications.validation.urlRequired')
      : channelType === 'webhook' && !/^https?:\/\//i.test(webhookUrl.trim())
        ? t('notifications.validation.urlInvalid')
        : channelType === 'webhook' && webhookUrl.length > 2048
          ? t('notifications.validation.urlTooLong')
          : null
  );

  let methodError = $derived(
    channelType === 'webhook' && !WEBHOOK_METHODS.includes(webhookMethod)
      ? t('notifications.validation.methodInvalid')
      : null
  );

  let bodyTemplateError = $derived(
    channelType === 'webhook' && bodyTemplate.trim().length === 0
      ? t('notifications.validation.bodyTemplateRequired')
      : null
  );

  let headersError = $derived(
    channelType === 'webhook' && webhookHeaders.length > 20
      ? t('notifications.validation.headersTooMany')
      : null
  );



  function validHttpUrl(value: string): boolean {
    try { const url = new URL(value); return ['http:', 'https:'].includes(url.protocol) && !!url.hostname && !url.username && !url.password; }
    catch { return false; }
  }

  let nativeError = $derived.by(() => {
    if (channelType === 'slack' && ((!slackWebhookUrl && !slackSeed?.webhook_url_configured) || (slackWebhookUrl && (!validHttpUrl(slackWebhookUrl) || !slackWebhookUrl.startsWith('https://'))))) return t('notifications.validation.slackWebhookRequired');
    if (channelType === 'telegram' && ((!telegramBotToken && !telegramSeed?.bot_token_configured) || !telegramChatId.trim() || (telegramThreadId !== undefined && (!Number.isSafeInteger(telegramThreadId) || telegramThreadId < 1)))) return t('notifications.validation.telegramRequired');
    if (channelType === 'matrix' && (!validHttpUrl(matrixHomeserverUrl) || !matrixRoomId.trim().startsWith('!') || (!matrixAccessToken && !matrixSeed?.access_token_configured))) return t('notifications.validation.matrixRequired');
    return null;
  });

  let isFormValid = $derived(
    !nameError &&
    (channelType === 'email' ? !recipientsError : true) &&
    (channelType === 'webhook' ? !urlError && !methodError && !bodyTemplateError && !headersError && webhookHeaders.every(h => !getHeaderError(h)) : true) && !nativeError
  );

  // ---------------------------------------------------------------------------
  // Email recipient management
  // ---------------------------------------------------------------------------

  function validateEmailAddress(email: string): boolean {
    if (email.length > 254) return false;
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
  }

  let newRecipientError = $state<string | null>(null);

  function addRecipient() {
    const email = newRecipient.trim();
    if (!email) return;

    if (!validateEmailAddress(email)) {
      newRecipientError = t('notifications.validation.recipientInvalid', { email });
      return;
    }

    if (recipients.length >= 50) {
      newRecipientError = t('notifications.validation.recipientsTooMany');
      return;
    }

    if (recipients.includes(email)) {
      newRecipientError = t('notifications.validation.recipientDuplicate', { email });
      return;
    }

    recipients = [...recipients, email];
    newRecipient = '';
    newRecipientError = null;
  }

  function removeRecipient(index: number) {
    recipients = recipients.filter((_, i) => i !== index);
  }

  function handleRecipientKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      addRecipient();
    }
  }

  // ---------------------------------------------------------------------------
  // Webhook header management
  // ---------------------------------------------------------------------------

  function addHeader() {
    if (webhookHeaders.length >= 20) return;
    webhookHeaders = [...webhookHeaders, { id: nextHeaderId++, name: '', value: '', originalName: '', saved: false, replace: true }];
  }

  function removeHeader(index: number) {
    webhookHeaders = webhookHeaders.filter((_, i) => i !== index);
  }

  function getHeaderError(header: HeaderRow): string | null {
    if (header.saved && !header.replace && header.name.trim().toLowerCase() !== header.originalName.trim().toLowerCase()) return t('notifications.validation.headerRenameRequiresValue');
    if (header.value === '[REDACTED]') return t('notifications.validation.secretPlaceholder');
    if (header.name.length > 128) return t('notifications.validation.headerNameTooLong');
    if (header.value.length > 8192) return t('notifications.validation.headerValueTooLong');
    return null;
  }

  // ---------------------------------------------------------------------------
  // Template variables loading
  // ---------------------------------------------------------------------------

  async function loadTemplateVariables() {
    if (templateVariables.length > 0 || templateVarsLoading) return;
    templateVarsLoading = true;
    templateVarsError = null;
    try {
      templateVariables = await getTemplateVariables();
    } catch (err) {
      templateVarsError = err instanceof Error ? err.message : t('notifications.templateVariables.loadError');
    } finally {
      templateVarsLoading = false;
    }
  }

  // Load template variables when webhook is selected
  let templateVarsLoaded = $state(false);

  $effect(() => {
    if (channelType === 'webhook' && !templateVarsLoaded) {
      templateVarsLoaded = true;
      loadTemplateVariables();
    }
  });

  // ---------------------------------------------------------------------------
  // Form submission
  // ---------------------------------------------------------------------------

  function buildConfig(): NotificationChannelConfig {
    switch (channelType) {
      case 'email': return { recipients: [...recipients] };
      case 'webhook': return {
        url: webhookUrl.trim(), method: webhookMethod, body_template: bodyTemplate,
        headers: webhookHeaders.filter(h => h.name.trim()).map(h => ({ name: h.name.trim(), ...(!h.saved || h.replace ? { value: h.value } : {}) })),
      };
      case 'slack': return { ...(slackWebhookUrl ? { webhook_url: slackWebhookUrl.trim() } : {}) };
      case 'telegram': return { chat_id: telegramChatId.trim(), ...(telegramBotToken ? { bot_token: telegramBotToken.trim() } : {}), ...(telegramThreadId !== undefined ? { message_thread_id: telegramThreadId } : {}) };
      case 'matrix': return { homeserver_url: matrixHomeserverUrl.trim(), room_id: matrixRoomId.trim(), ...(matrixAccessToken ? { access_token: matrixAccessToken } : {}) };
    }
  }

  let testing = $state(false);
  let testResult = $state<{ success: boolean; error?: string } | null>(null);
  let testedDraft = $state('');
  let draftFingerprint = $derived(JSON.stringify({ name, type: channelType, config: buildConfig() }));
  let currentTestResult = $derived(testedDraft === draftFingerprint ? testResult : null);

  async function handleTest() {
    if (!isFormValid || testing || submitting) return;
    testing = true;
    testResult = null;
    testedDraft = draftFingerprint;
    try {
      testResult = await testNotificationChannelDraft({
        name: name.trim(), type: channelType, config: buildConfig(),
        ...(mode === 'edit' && seed ? { existing_channel_id: seed.id } : {}),
      });
    } catch (err) {
      testResult = { success: false, error: err instanceof Error ? err.message : t('common.error') };
    } finally { testing = false; }
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!isFormValid || submitting || testing) return;

    submitting = true;
    apiError = null;


    try {
      let result: NotificationChannel;

      if (mode === 'create') {
        const req: CreateNotificationChannelRequest = {
          name: name.trim(),
          type: channelType,
          config: buildConfig(),
        };
        result = await createNotificationChannel(req);
      } else {
        const req: UpdateNotificationChannelRequest = {
          name: name.trim(),
          type: channelType,
          config: buildConfig(),
        };
        result = await updateNotificationChannel(initialData!.id, req);
      }

      onSubmit(result);
    } catch (err: unknown) {
      if (err instanceof Error) {
        apiError = err.message;
      } else {
        apiError = t('common.error');
      }
    } finally {
      submitting = false;
    }
  }
</script>

<form onsubmit={handleSubmit} class="pulse-editor-form max-w-2xl space-y-6" data-testid="notification-channel-form">
  <!-- API Error Summary -->
  {#if apiError}
    <div
      class="rounded-md border border-[var(--color-border)] bg-surface p-4 text-sm text-[var(--color-error)]"
      role="alert"
      data-testid="api-error"
    >
      <p class="font-medium">{t('common.error')}</p>
      <p>{apiError}</p>
    </div>
  {/if}

  <!-- Channel Name -->
  <div>
    <label for="channel-name" class="block text-sm font-medium text-primary">
      {t('notifications.form.name')}
    </label>
    <input
      id="channel-name"
      type="text"
      bind:value={name}
      onblur={() => markTouched('name')}
      placeholder={t('notifications.form.namePlaceholder')}
      maxlength={100}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
      data-testid="input-channel-name"
    />
    {#if touched.name && nameError}
      <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-name">{nameError}</p>
    {/if}
  </div>

  <!-- Channel Type -->
  <div>
    <label for="channel-type" class="block text-sm font-medium text-primary">
      {t('notifications.form.type')}
    </label>
    <SelectField
      id="channel-type"
      bind:value={channelType}
      onblur={() => markTouched('type')}
      class="mt-1 w-full"
      data-testid="input-channel-type"
    >
      <option value="email">{t('notifications.channels.types.email')}</option>
      <option value="webhook">{t('notifications.channels.types.webhook')}</option>
      <option value="slack">{t('notifications.channels.types.slack')}</option>
      <option value="matrix">{t('notifications.channels.types.matrix')}</option>
      <option value="telegram">{t('notifications.channels.types.telegram')}</option>
    </SelectField>
  </div>

  <!-- Email Configuration -->
  {#if channelType === 'email'}
    <fieldset class="space-y-3" data-testid="email-config">
      <legend class="block text-sm font-medium text-primary">
        {t('notifications.form.email.recipients')}
      </legend>
      <p class="text-xs text-secondary">{t('notifications.form.email.recipientsHelp')}</p>

      <!-- Recipient list -->
      {#if recipients.length > 0}
        <ul class="space-y-1" data-testid="recipient-list">
          {#each recipients as recipient, i (recipient)}
            <li class="flex items-center justify-between rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm">
              <span class="text-primary">{recipient}</span>
              <button
                type="button"
                onclick={() => removeRecipient(i)}
                class="rounded p-0.5 text-secondary hover:text-[var(--color-error)]"
                aria-label={t('notifications.form.email.removeRecipient')}
                data-testid="btn-remove-recipient-{i}"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                </svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}

      <!-- Add recipient input -->
      <div class="flex items-start gap-2">
        <div class="flex-1">
          <input
            type="email"
            dir="ltr"
            aria-label={t('notifications.form.email.recipients')}
            bind:value={newRecipient}
            onkeydown={handleRecipientKeydown}
            placeholder={t('notifications.form.email.recipientsPlaceholder')}
            class="block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            data-testid="input-new-recipient"
          />
          {#if newRecipientError}
            <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-recipient">{newRecipientError}</p>
          {/if}
        </div>
        <button
          type="button"
          onclick={addRecipient}
          disabled={recipients.length >= 50}
          class="rounded-md bg-brand-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="btn-add-recipient"
        >
          {t('notifications.form.email.addRecipient')}
        </button>
      </div>

      {#if touched.recipients && recipientsError}
        <p class="text-xs text-[var(--color-error)]" data-testid="error-recipients">{recipientsError}</p>
      {/if}
    </fieldset>
  {/if}

  {#if channelType === 'slack'}
    <fieldset class="space-y-4" data-testid="slack-config">
      <legend class="text-sm font-medium text-primary">{t('notifications.channels.types.slack')}</legend>
      <p class="text-xs text-secondary">{t('notifications.form.slack.help')}</p>
      <div>
        <label for="slack-webhook-url" class="block text-sm font-medium text-primary">{t('notifications.form.slack.webhookUrl')}</label>
        <input id="slack-webhook-url" dir="ltr" type="password" bind:value={slackWebhookUrl} autocomplete="new-password" class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-slack-webhook-url" />
        {#if slackSeed?.webhook_url_configured}<p class="mt-1 text-xs text-secondary">{t('notifications.form.savedSecret')}</p>{/if}
      </div>
      {#if nativeError}<p class="text-xs text-[var(--color-error)]">{nativeError}</p>{/if}
    </fieldset>
  {/if}

  {#if channelType === 'matrix'}
    <fieldset class="space-y-4" data-testid="matrix-config">
      <legend class="text-sm font-medium text-primary">{t('notifications.channels.types.matrix')}</legend>
      <p class="text-xs text-secondary">{t('notifications.form.matrix.help')}</p>
      <div>
        <label for="matrix-homeserver-url" class="block text-sm font-medium text-primary">{t('notifications.form.matrix.homeserverUrl')}</label>
        <input id="matrix-homeserver-url" dir="ltr" type="url" bind:value={matrixHomeserverUrl} class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-matrix-homeserver-url" />
      </div>
      <div>
        <label for="matrix-room-id" class="block text-sm font-medium text-primary">{t('notifications.form.matrix.roomId')}</label>
        <input id="matrix-room-id" dir="ltr" type="text" bind:value={matrixRoomId} class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-matrix-room-id" />
      </div>
      <div>
        <label for="matrix-access-token" class="block text-sm font-medium text-primary">{t('notifications.form.matrix.accessToken')}</label>
        <input id="matrix-access-token" dir="ltr" type="password" bind:value={matrixAccessToken} autocomplete="new-password" class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-matrix-access-token" />
        {#if matrixSeed?.access_token_configured}<p class="mt-1 text-xs text-secondary">{t('notifications.form.savedSecret')}</p>{/if}
      </div>
      {#if nativeError}<p class="text-xs text-[var(--color-error)]">{nativeError}</p>{/if}
    </fieldset>
  {/if}

  {#if channelType === 'telegram'}
    <fieldset class="space-y-4" data-testid="telegram-config">
      <legend class="text-sm font-medium text-primary">{t('notifications.channels.types.telegram')}</legend>
      <p class="text-xs text-secondary">{t('notifications.form.telegram.help')}</p>
      <div>
        <label for="telegram-bot-token" class="block text-sm font-medium text-primary">{t('notifications.form.telegram.botToken')}</label>
        <input id="telegram-bot-token" dir="ltr" type="password" bind:value={telegramBotToken} autocomplete="new-password" class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-telegram-bot-token" />
        {#if telegramSeed?.bot_token_configured}<p class="mt-1 text-xs text-secondary">{t('notifications.form.savedSecret')}</p>{/if}
      </div>
      <div>
        <label for="telegram-chat-id" class="block text-sm font-medium text-primary">{t('notifications.form.telegram.chatId')}</label>
        <input id="telegram-chat-id" dir="ltr" type="text" bind:value={telegramChatId} class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-telegram-chat-id" />
      </div>
      <div>
        <label for="telegram-thread-id" class="block text-sm font-medium text-primary">{t('notifications.form.telegram.threadId')}</label>
        <input id="telegram-thread-id" dir="ltr" type="number" bind:value={telegramThreadId} min="1" step="1" class="mt-1 min-h-10 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary" data-testid="input-telegram-thread-id" />
      </div>
      {#if nativeError}<p class="text-xs text-[var(--color-error)]">{nativeError}</p>{/if}
    </fieldset>
  {/if}

  <!-- Webhook Configuration -->
  {#if channelType === 'webhook'}
    <div class="space-y-4" data-testid="webhook-config">
      <!-- Webhook URL -->
      <div>
        <label for="webhook-url" class="block text-sm font-medium text-primary">
          {t('notifications.form.webhook.url')}
        </label>
        <input
          id="webhook-url"
          dir="ltr"
          type="url"
          bind:value={webhookUrl}
          onblur={() => markTouched('url')}
          placeholder={t('notifications.form.webhook.urlPlaceholder')}
          maxlength={2048}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          data-testid="input-webhook-url"
        />
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.urlHelp')}</p>
        {#if touched.url && urlError}
          <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-url">{urlError}</p>
        {/if}
      </div>

      <!-- HTTP Method -->
      <div>
        <label for="webhook-method" class="block text-sm font-medium text-primary">
          {t('notifications.form.webhook.method')}
        </label>
        <SelectField
          id="webhook-method"
          bind:value={webhookMethod}
          class="mt-1 w-full"
          data-testid="input-webhook-method"
        >
          {#each WEBHOOK_METHODS as method (method)}
            <option value={method}>{method}</option>
          {/each}
        </SelectField>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.methodHelp')}</p>
        {#if methodError}
          <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-method">{methodError}</p>
        {/if}
      </div>

      <!-- Custom Headers -->
      <div>
        <div class="flex items-center justify-between">
          <span class="block text-sm font-medium text-primary">
            {t('notifications.form.webhook.headers')}
          </span>
          {#if webhookHeaders.length < 20}
            <button
              type="button"
              onclick={addHeader}
              class="text-xs font-medium text-[var(--color-brand-primary)] hover:underline"
              data-testid="btn-add-webhook-header"
            >
              {t('notifications.form.webhook.addHeader')}
            </button>
          {/if}
        </div>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.headersHelp')}</p>

        {#if webhookHeaders.length > 0}
          <div class="mt-2 space-y-2" data-testid="webhook-headers-list">
            {#each webhookHeaders as header, i (header.id)}
              <div class="space-y-1">
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    dir="ltr"
                    aria-label={t('notifications.form.webhook.headerNamePlaceholder')}
                    bind:value={header.name}
                    placeholder={t('notifications.form.webhook.headerNamePlaceholder')}
                    maxlength={128}
                    class="min-h-10 block w-1/3 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                    data-testid="input-webhook-header-name-{i}"
                  />
                  <input
                    type="password"
                    dir="ltr"
                    autocomplete="new-password"
                    aria-label={t('notifications.form.webhook.headerValuePlaceholder')}
                    oninput={() => { header.replace = true; }}
                    bind:value={header.value}
                    placeholder={t('notifications.form.webhook.headerValuePlaceholder')}
                    maxlength={8192}
                    class="min-h-10 block min-w-0 flex-1 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                    data-testid="input-webhook-header-value-{i}"
                  />
                  <button
                    type="button"
                    onclick={() => removeHeader(i)}
                    class="rounded p-1 text-secondary hover:text-[var(--color-error)]"
                    aria-label={t('notifications.form.webhook.removeHeader')}
                    data-testid="btn-remove-webhook-header-{i}"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                    </svg>
                  </button>
                </div>
                {#if header.saved && !header.replace}
                  <p class="text-xs text-secondary">{t('notifications.form.savedSecret')}</p>
                {/if}
                {#if getHeaderError(header)}
                  <p class="text-xs text-[var(--color-error)]">{getHeaderError(header)}</p>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        {#if headersError}
          <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-headers">{headersError}</p>
        {/if}
      </div>

      <!-- Body Template -->
      <div>
        <label for="webhook-body-template" class="block text-sm font-medium text-primary">
          {t('notifications.form.webhook.bodyTemplate')}
        </label>
        <textarea
          id="webhook-body-template"
          dir="ltr"
          bind:value={bodyTemplate}
          onblur={() => markTouched('bodyTemplate')}
          placeholder={t('notifications.form.webhook.bodyTemplatePlaceholder')}
          rows={6}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 font-mono text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          data-testid="input-body-template"
        ></textarea>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.bodyTemplateHelp')}</p>
        {#if touched.bodyTemplate && bodyTemplateError}
          <p class="mt-1 text-xs text-[var(--color-error)]" data-testid="error-body-template">{bodyTemplateError}</p>
        {/if}
      </div>

      <!-- Template Variables Reference Panel -->
      <details class="rounded-md border border-[var(--color-border)] bg-surface" data-testid="template-variables-panel">
        <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-primary hover:bg-[var(--color-bg-surface-hover)]">
          {t('notifications.templateVariables.title')}
        </summary>
        <div class="border-t border-[var(--color-border)] px-4 py-3">
          <p class="mb-3 text-xs text-secondary">{t('notifications.templateVariables.description')}</p>

          {#if templateVarsLoading}
            <p class="text-xs text-secondary">{t('common.loading')}</p>
          {:else if templateVarsError}
            <p class="text-xs text-[var(--color-error)]">{templateVarsError}</p>
          {:else if templateVariables.length > 0}
            <div class="space-y-4">
              {#each templateVariables as group (group.name)}
                <div>
                  <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-secondary">
                    {group.name}
                  </h4>
                  <div class="overflow-x-auto">
                    <table class="w-full text-xs">
                      <thead>
                        <tr class="border-b border-[var(--color-border)]">
                          <th class="pb-1 pe-4 text-start font-medium text-secondary">{t('notifications.templateVariables.variable')}</th>
                          <th class="pb-1 pe-4 text-start font-medium text-secondary">{t('notifications.templateVariables.type')}</th>
                          <th class="pb-1 text-start font-medium text-secondary">{t('notifications.templateVariables.example')}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each group.variables as variable (variable.name)}
                          <tr class="border-b border-[var(--color-border)] last:border-0">
                            <td class="py-1 pe-4 font-mono text-primary">
                              {`{{.${variable.name}}}`} </td>
                            <td class="py-1 pe-4 text-secondary">{variable.type}</td>
                            <td class="py-1 text-secondary">{variable.example}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </details>
    </div>
  {/if}

  {#if currentTestResult}
    <div role={currentTestResult.success ? 'status' : 'alert'} class="rounded-md border border-[var(--color-border)] bg-surface p-3 text-sm text-primary">
      {#if currentTestResult.success}{t('notifications.test.success')}{:else}{t('notifications.test.failure', { error: currentTestResult.error ?? t('common.error') })}{/if}
    </div>
  {/if}
  <p class="text-xs text-secondary">{t('notifications.test.draftHelp')}</p>

  <!-- Form Actions -->
  <div class="pulse-editor-actions">
    <button
      type="button"
      onclick={onCancel}
      class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
      data-testid="btn-cancel"
    >
      {t('common.cancel')}
    </button>
    <button type="button" onclick={handleTest} disabled={!isFormValid || testing || submitting} class="min-h-10 rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary disabled:opacity-50" data-testid="btn-test-channel">
      {testing ? t('notifications.test.sending') : t('notifications.test.button')}
    </button>
    <button
      type="submit"
      disabled={!isFormValid || submitting || testing}
      class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
      data-testid="btn-submit"
    >
      {#if submitting}
        {t('common.saving')}
      {:else if mode === 'create'}
        {t('notifications.form.submitCreate')}
      {:else}
        {t('notifications.form.submitUpdate')}
      {/if}
    </button>
  </div>
</form>
