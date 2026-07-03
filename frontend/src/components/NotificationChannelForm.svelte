<script lang="ts">
  import type {
    NotificationChannelType,
    NotificationChannel,
    WebhookMethod,
    WebhookHeader,
    EmailChannelConfig,
    WebhookChannelConfig,
    TemplateVariableGroup,
  } from '$lib/types';
  import {
    createNotificationChannel,
    updateNotificationChannel,
    getTemplateVariables,
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

  let name = $state(initialData?.name ?? '');
  let channelType: NotificationChannelType = $state(initialData?.type ?? 'email');

  // Email fields
  let recipients = $state<string[]>(
    initialData?.type === 'email'
      ? (initialData.config as EmailChannelConfig).recipients ?? []
      : []
  );
  let newRecipient = $state('');

  // Webhook fields
  let webhookUrl = $state(
    initialData?.type === 'webhook'
      ? (initialData.config as WebhookChannelConfig).url ?? ''
      : ''
  );
  let webhookMethod: WebhookMethod = $state(
    initialData?.type === 'webhook'
      ? (initialData.config as WebhookChannelConfig).method ?? 'POST'
      : 'POST'
  );
  let webhookHeaders = $state<WebhookHeader[]>(
    initialData?.type === 'webhook'
      ? (initialData.config as WebhookChannelConfig).headers ?? []
      : []
  );
  let bodyTemplate = $state(
    initialData?.type === 'webhook'
      ? (initialData.config as WebhookChannelConfig).body_template ?? ''
      : '{ "text": "{{.Monitor.Name}} is {{.Status}}" }'
  );

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

  let isFormValid = $derived(
    !nameError &&
    (channelType === 'email' ? !recipientsError : true) &&
    (channelType === 'webhook' ? !urlError && !methodError && !bodyTemplateError && !headersError : true)
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
      newRecipientError = t('notifications.validation.recipientInvalid', { email: `${email} (duplicate)` });
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
    webhookHeaders = [...webhookHeaders, { name: '', value: '' }];
  }

  function removeHeader(index: number) {
    webhookHeaders = webhookHeaders.filter((_, i) => i !== index);
  }

  function getHeaderError(header: WebhookHeader): string | null {
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
      templateVarsError = err instanceof Error ? err.message : 'Failed to load template variables';
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

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!isFormValid || submitting) return;

    submitting = true;
    apiError = null;

    const config: EmailChannelConfig | WebhookChannelConfig =
      channelType === 'email'
        ? { recipients }
        : {
            url: webhookUrl.trim(),
            method: webhookMethod,
            body_template: bodyTemplate,
            headers: webhookHeaders.filter(h => h.name.trim() !== ''),
          };

    try {
      let result: NotificationChannel;

      if (mode === 'create') {
        const req: CreateNotificationChannelRequest = {
          name: name.trim(),
          type: channelType,
          config,
        };
        result = await createNotificationChannel(req);
      } else {
        const req: UpdateNotificationChannelRequest = {
          name: name.trim(),
          type: channelType,
          config,
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

<form onsubmit={handleSubmit} class="mx-auto max-w-2xl space-y-6" data-testid="notification-channel-form">
  <!-- API Error Summary -->
  {#if apiError}
    <div
      class="rounded-md border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700"
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
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-channel-name"
    />
    {#if touched.name && nameError}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-name">{nameError}</p>
    {/if}
  </div>

  <!-- Channel Type -->
  <div>
    <label for="channel-type" class="block text-sm font-medium text-primary">
      {t('notifications.form.type')}
    </label>
    <select
      id="channel-type"
      bind:value={channelType}
      onblur={() => markTouched('type')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-channel-type"
    >
      <option value="email">{t('notifications.channels.types.email')}</option>
      <option value="webhook">{t('notifications.channels.types.webhook')}</option>
    </select>
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
                class="rounded p-0.5 text-secondary hover:text-rose-600"
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
            bind:value={newRecipient}
            onkeydown={handleRecipientKeydown}
            placeholder={t('notifications.form.email.recipientsPlaceholder')}
            class="block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            data-testid="input-new-recipient"
          />
          {#if newRecipientError}
            <p class="mt-1 text-xs text-rose-600" data-testid="error-recipient">{newRecipientError}</p>
          {/if}
        </div>
        <button
          type="button"
          onclick={addRecipient}
          disabled={recipients.length >= 50}
          class="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="btn-add-recipient"
        >
          {t('notifications.form.email.addRecipient')}
        </button>
      </div>

      {#if touched.recipients && recipientsError}
        <p class="text-xs text-rose-600" data-testid="error-recipients">{recipientsError}</p>
      {/if}
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
          type="url"
          bind:value={webhookUrl}
          onblur={() => markTouched('url')}
          placeholder={t('notifications.form.webhook.urlPlaceholder')}
          maxlength={2048}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-webhook-url"
        />
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.urlHelp')}</p>
        {#if touched.url && urlError}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-url">{urlError}</p>
        {/if}
      </div>

      <!-- HTTP Method -->
      <div>
        <label for="webhook-method" class="block text-sm font-medium text-primary">
          {t('notifications.form.webhook.method')}
        </label>
        <select
          id="webhook-method"
          bind:value={webhookMethod}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-webhook-method"
        >
          {#each WEBHOOK_METHODS as method}
            <option value={method}>{method}</option>
          {/each}
        </select>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.methodHelp')}</p>
        {#if methodError}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-method">{methodError}</p>
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
              class="text-xs font-medium text-blue-600 hover:text-blue-800"
              data-testid="btn-add-webhook-header"
            >
              {t('notifications.form.webhook.addHeader')}
            </button>
          {/if}
        </div>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.headersHelp')}</p>

        {#if webhookHeaders.length > 0}
          <div class="mt-2 space-y-2" data-testid="webhook-headers-list">
            {#each webhookHeaders as header, i (i)}
              <div class="space-y-1">
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    bind:value={header.name}
                    placeholder={t('notifications.form.webhook.headerNamePlaceholder')}
                    maxlength={128}
                    class="block w-1/3 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                    data-testid="input-webhook-header-name-{i}"
                  />
                  <input
                    type="text"
                    bind:value={header.value}
                    placeholder={t('notifications.form.webhook.headerValuePlaceholder')}
                    maxlength={8192}
                    class="block flex-1 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                    data-testid="input-webhook-header-value-{i}"
                  />
                  <button
                    type="button"
                    onclick={() => removeHeader(i)}
                    class="rounded p-1 text-secondary hover:text-rose-600"
                    aria-label={t('notifications.form.webhook.removeHeader')}
                    data-testid="btn-remove-webhook-header-{i}"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                    </svg>
                  </button>
                </div>
                {#if getHeaderError(header)}
                  <p class="text-xs text-rose-600">{getHeaderError(header)}</p>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        {#if headersError}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-headers">{headersError}</p>
        {/if}
      </div>

      <!-- Body Template -->
      <div>
        <label for="webhook-body-template" class="block text-sm font-medium text-primary">
          {t('notifications.form.webhook.bodyTemplate')}
        </label>
        <textarea
          id="webhook-body-template"
          bind:value={bodyTemplate}
          onblur={() => markTouched('bodyTemplate')}
          placeholder={t('notifications.form.webhook.bodyTemplatePlaceholder')}
          rows={6}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 font-mono text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-body-template"
        ></textarea>
        <p class="mt-1 text-xs text-secondary">{t('notifications.form.webhook.bodyTemplateHelp')}</p>
        {#if touched.bodyTemplate && bodyTemplateError}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-body-template">{bodyTemplateError}</p>
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
            <p class="text-xs text-rose-600">{templateVarsError}</p>
          {:else if templateVariables.length > 0}
            <div class="space-y-4">
              {#each templateVariables as group}
                <div>
                  <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-secondary">
                    {group.name}
                  </h4>
                  <div class="overflow-x-auto">
                    <table class="w-full text-xs">
                      <thead>
                        <tr class="border-b border-[var(--color-border)]">
                          <th class="pb-1 pe-4 text-start font-medium text-secondary">Variable</th>
                          <th class="pb-1 pe-4 text-start font-medium text-secondary">Type</th>
                          <th class="pb-1 text-start font-medium text-secondary">Example</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each group.variables as variable}
                          <tr class="border-b border-[var(--color-border)] last:border-0">
                            <td class="py-1 pe-4 font-mono text-primary">
                              {"{{."}{variable.name}{"}}"}</td>
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
        {t('notifications.form.submitCreate')}
      {:else}
        {t('notifications.form.submitUpdate')}
      {/if}
    </button>
  </div>
</form>
