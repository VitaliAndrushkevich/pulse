<script lang="ts">
  import type { CreateCredentialRequest } from '$lib/api';

  type AuthType = 'bearer' | 'basic' | 'header';

  interface Props {
    onSubmit: (req: CreateCredentialRequest) => void;
    loading?: boolean;
  }

  let { onSubmit, loading = false }: Props = $props();

  // Form field state
  let authType: AuthType = $state('bearer');
  let name = $state('');
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

  // Validation per auth type
  let nameValid = $derived(name.trim().length > 0);

  let secretFieldsValid = $derived.by(() => {
    switch (authType) {
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

  let isFormValid = $derived(nameValid && secretFieldsValid);

  function resetFields() {
    name = '';
    token = '';
    username = '';
    password = '';
    headerName = '';
    headerValue = '';
    touched = {};
  }

  function handleSubmit(event: Event) {
    event.preventDefault();
    if (!isFormValid || loading) return;

    const req: CreateCredentialRequest = { auth_type: authType, name: name.trim() };

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
    { value: 'bearer', label: 'Bearer Token' },
    { value: 'basic', label: 'Basic Auth' },
    { value: 'header', label: 'Custom Header' },
  ];
</script>

<form onsubmit={handleSubmit} class="space-y-4" data-testid="credential-form">
  <!-- Auth Type -->
  <div>
    <label for="credential-auth-type" class="block text-sm font-medium text-slate-700">
      Authentication Type
    </label>
    <select
      id="credential-auth-type"
      bind:value={authType}
      class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="select-auth-type"
    >
      {#each authTypeOptions as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  </div>

  <!-- Name -->
  <div>
    <label for="credential-name" class="block text-sm font-medium text-slate-700">Name</label>
    <input
      id="credential-name"
      type="text"
      bind:value={name}
      onblur={() => markTouched('name')}
      placeholder="e.g. Production API Key"
      class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="input-credential-name"
    />
    {#if touched.name && !nameValid}
      <p class="mt-1 text-xs text-rose-600" data-testid="error-name">Name is required</p>
    {/if}
  </div>

  <!-- Bearer Token fields -->
  {#if authType === 'bearer'}
    <div>
      <label for="credential-token" class="block text-sm font-medium text-slate-700">Token</label>
      <input
        id="credential-token"
        type="password"
        bind:value={token}
        onblur={() => markTouched('token')}
        placeholder="Enter bearer token"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-credential-token"
      />
      {#if touched.token && !token.trim()}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-token">Token is required</p>
      {/if}
    </div>
  {/if}

  <!-- Basic Auth fields -->
  {#if authType === 'basic'}
    <div>
      <label for="credential-username" class="block text-sm font-medium text-slate-700">Username</label>
      <input
        id="credential-username"
        type="text"
        bind:value={username}
        onblur={() => markTouched('username')}
        placeholder="Enter username"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-credential-username"
      />
      {#if touched.username && !username.trim()}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-username">Username is required</p>
      {/if}
    </div>

    <div>
      <label for="credential-password" class="block text-sm font-medium text-slate-700">Password</label>
      <input
        id="credential-password"
        type="password"
        bind:value={password}
        onblur={() => markTouched('password')}
        placeholder="Enter password"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-credential-password"
      />
      {#if touched.password && !password.trim()}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-password">Password is required</p>
      {/if}
    </div>
  {/if}

  <!-- Custom Header fields -->
  {#if authType === 'header'}
    <div>
      <label for="credential-header-name" class="block text-sm font-medium text-slate-700">Header Name</label>
      <input
        id="credential-header-name"
        type="text"
        bind:value={headerName}
        onblur={() => markTouched('headerName')}
        placeholder="e.g. X-API-Key"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-credential-header-name"
      />
      {#if touched.headerName && !headerName.trim()}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-header-name">Header name is required</p>
      {/if}
    </div>

    <div>
      <label for="credential-header-value" class="block text-sm font-medium text-slate-700">Header Value</label>
      <input
        id="credential-header-value"
        type="password"
        bind:value={headerValue}
        onblur={() => markTouched('headerValue')}
        placeholder="Enter header value"
        class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="input-credential-header-value"
      />
      {#if touched.headerValue && !headerValue.trim()}
        <p class="mt-1 text-xs text-rose-600" data-testid="error-header-value">Header value is required</p>
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
        Saving…
      {:else}
        Add Credential
      {/if}
    </button>
  </div>
</form>
