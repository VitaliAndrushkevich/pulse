<script lang="ts">
  import type { CreateCredentialRequest } from '$lib/api';
  import { t } from '$lib/i18n';

  type AuthType = 'none' | 'bearer' | 'basic' | 'header';

  interface Props {
    onSubmit: (req: CreateCredentialRequest) => void;
    loading?: boolean;
    /** When true, renders as a div instead of a form (for use inside another form). */
    inline?: boolean;
  }

  let { onSubmit, loading = false, inline = false }: Props = $props();

  // Form field state
  let authType: AuthType = $state('none');
  let token = $state('');
  let username = $state('');
  let password = $state('');
  let headerName = $state('');
  let headerValue = $state('');

  // Track touched fields for showing errors only after interaction
  let touched = $state<Record<string, boolean>>({});

  function markTouched(field: string) {
    touched[field] = true;
  }

  // Auto-generate credential name from auth type
  const authTypeDisplayNames: Record<Exclude<AuthType, 'none'>, string> = {
    bearer: 'Bearer Token',
    basic: 'Basic Auth',
    header: 'Custom Header',
  };

  // Validation per auth type
  let secretFieldsValid = $derived.by(() => {
    switch (authType) {
      case 'none':
        return false;
      case 'bearer':
        return token.trim().length > 0;
      case 'basic':
        return username.trim().length > 0 && password.trim().length > 0;
      case 'header':
        return headerName.trim().length > 0 && headerValue.trim().length > 0;
      default:
        return false;
    }
  });

  let isFormValid = $derived(authType !== 'none' && secretFieldsValid);

  function resetFields() {
    token = '';
    username = '';
    password = '';
    headerName = '';
    headerValue = '';
    touched = {};
  }

  function handleSubmit(event?: Event) {
    event?.preventDefault();
    if (!isFormValid || loading || authType === 'none') return;

    const req: CreateCredentialRequest = { auth_type: authType, name: authTypeDisplayNames[authType] };

    switch (authType) {
      case 'bearer':
        req.token = token.trim();
        break;
      case 'basic':
        req.username = username.trim();
        req.password = password.trim();
        break;
      case 'header':
        req.header_name = headerName.trim();
        req.header_value = headerValue.trim();
        break;
    }

    onSubmit(req);
    resetFields();
  }

  const authTypeOptions: { value: AuthType; label: string }[] = [
    { value: 'none', label: t('auth.types.none') },
    { value: 'bearer', label: t('auth.types.bearer') },
    { value: 'basic', label: t('auth.types.basic') },
    { value: 'header', label: t('auth.types.header') },
  ];
</script>

{#if inline}
<div class="space-y-4" data-testid="credential-form">
  <!-- Auth Type -->
  <div>
    <label for="credential-auth-type" class="block text-sm font-medium text-primary">
      {t('auth.authType')}
    </label>
    <select
      id="credential-auth-type"
      bind:value={authType}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="select-auth-type"
    >
      {#each authTypeOptions as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  {#if authType !== 'none'}
    <!-- Bearer Token fields -->
    {#if authType === 'bearer'}
      <div>
        <label for="credential-token" class="block text-sm font-medium text-primary">{t('auth.fields.token')}</label>
        <input
          id="credential-token"
          type="password"
          bind:value={token}
          onblur={() => markTouched('token')}
          placeholder={t('auth.fields.tokenPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-token"
        />
        {#if touched.token && !token.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-token">{t('auth.validation.tokenRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Basic Auth fields -->
    {#if authType === 'basic'}
      <div>
        <label for="credential-username" class="block text-sm font-medium text-primary">{t('auth.fields.username')}</label>
        <input
          id="credential-username"
          type="text"
          bind:value={username}
          onblur={() => markTouched('username')}
          placeholder={t('auth.fields.usernamePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-username"
        />
        {#if touched.username && !username.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-username">{t('auth.validation.usernameRequired')}</p>
        {/if}
      </div>

      <div>
        <label for="credential-password" class="block text-sm font-medium text-primary">{t('auth.fields.password')}</label>
        <input
          id="credential-password"
          type="password"
          bind:value={password}
          onblur={() => markTouched('password')}
          placeholder={t('auth.fields.passwordPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-password"
        />
        {#if touched.password && !password.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-password">{t('auth.validation.passwordRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Custom Header fields -->
    {#if authType === 'header'}
      <div>
        <label for="credential-header-name" class="block text-sm font-medium text-primary">{t('auth.fields.headerName')}</label>
        <input
          id="credential-header-name"
          type="text"
          bind:value={headerName}
          onblur={() => markTouched('headerName')}
          placeholder={t('auth.fields.headerNamePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-header-name"
        />
        {#if touched.headerName && !headerName.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-header-name">{t('auth.validation.headerNameRequired')}</p>
        {/if}
      </div>

      <div>
        <label for="credential-header-value" class="block text-sm font-medium text-primary">{t('auth.fields.headerValue')}</label>
        <input
          id="credential-header-value"
          type="password"
          bind:value={headerValue}
          onblur={() => markTouched('headerValue')}
          placeholder={t('auth.fields.headerValuePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-header-value"
        />
        {#if touched.headerValue && !headerValue.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-header-value">{t('auth.validation.headerValueRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Submit Button (type="button" to avoid triggering parent form) -->
    <div class="pt-2">
      <button
        type="button"
        disabled={!isFormValid || loading}
        onclick={() => handleSubmit()}
        class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="btn-add-credential"
      >
        {#if loading}
          {t('common.saving')}
        {:else}
          {t('auth.addCredential')}
        {/if}
      </button>
    </div>
  {/if}
</div>
{:else}
<form onsubmit={handleSubmit} class="space-y-4" data-testid="credential-form">
  <!-- Auth Type -->
  <div>
    <label for="credential-auth-type" class="block text-sm font-medium text-primary">
      {t('auth.authType')}
    </label>
    <select
      id="credential-auth-type"
      bind:value={authType}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="select-auth-type"
    >
      {#each authTypeOptions as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  {#if authType !== 'none'}
    <!-- Bearer Token fields -->
    {#if authType === 'bearer'}
      <div>
        <label for="credential-token" class="block text-sm font-medium text-primary">{t('auth.fields.token')}</label>
        <input
          id="credential-token"
          type="password"
          bind:value={token}
          onblur={() => markTouched('token')}
          placeholder={t('auth.fields.tokenPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-token"
        />
        {#if touched.token && !token.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-token">{t('auth.validation.tokenRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Basic Auth fields -->
    {#if authType === 'basic'}
      <div>
        <label for="credential-username" class="block text-sm font-medium text-primary">{t('auth.fields.username')}</label>
        <input
          id="credential-username"
          type="text"
          bind:value={username}
          onblur={() => markTouched('username')}
          placeholder={t('auth.fields.usernamePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-username"
        />
        {#if touched.username && !username.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-username">{t('auth.validation.usernameRequired')}</p>
        {/if}
      </div>

      <div>
        <label for="credential-password" class="block text-sm font-medium text-primary">{t('auth.fields.password')}</label>
        <input
          id="credential-password"
          type="password"
          bind:value={password}
          onblur={() => markTouched('password')}
          placeholder={t('auth.fields.passwordPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-password"
        />
        {#if touched.password && !password.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-password">{t('auth.validation.passwordRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Custom Header fields -->
    {#if authType === 'header'}
      <div>
        <label for="credential-header-name" class="block text-sm font-medium text-primary">{t('auth.fields.headerName')}</label>
        <input
          id="credential-header-name"
          type="text"
          bind:value={headerName}
          onblur={() => markTouched('headerName')}
          placeholder={t('auth.fields.headerNamePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-header-name"
        />
        {#if touched.headerName && !headerName.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-header-name">{t('auth.validation.headerNameRequired')}</p>
        {/if}
      </div>

      <div>
        <label for="credential-header-value" class="block text-sm font-medium text-primary">{t('auth.fields.headerValue')}</label>
        <input
          id="credential-header-value"
          type="password"
          bind:value={headerValue}
          onblur={() => markTouched('headerValue')}
          placeholder={t('auth.fields.headerValuePlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          data-testid="input-credential-header-value"
        />
        {#if touched.headerValue && !headerValue.trim()}
          <p class="mt-1 text-xs text-rose-600" data-testid="error-header-value">{t('auth.validation.headerValueRequired')}</p>
        {/if}
      </div>
    {/if}

    <!-- Submit Button -->
    <div class="pt-2">
      <button
        type="submit"
        disabled={!isFormValid || loading}
        class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="btn-add-credential"
      >
        {#if loading}
          {t('common.saving')}
        {:else}
          {t('auth.addCredential')}
        {/if}
      </button>
    </div>
  {/if}
</form>
{/if}
