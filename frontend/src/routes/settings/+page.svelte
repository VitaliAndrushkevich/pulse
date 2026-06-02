<script lang="ts">
  import { onMount } from 'svelte';
  import { getSecrets, createSecret, ApiRequestError, NetworkError } from '$lib/api';
  import { formatDate, formatSecretReference } from '$lib/format';
  import type { Secret } from '$lib/types';

  let secrets = $state<Secret[]>([]);
  let loading = $state(true);
  let listError = $state<string | null>(null);

  // Create form state
  let secretName = $state('');
  let secretValue = $state('');
  let creating = $state(false);
  let createError = $state<string | null>(null);
  let createSuccess = $state<string | null>(null);

  async function fetchSecrets() {
    loading = true;
    listError = null;
    try {
      secrets = await getSecrets();
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        listError = err.message;
      } else if (err instanceof NetworkError) {
        listError = 'Unable to connect to the server. Please check your network connection.';
      } else {
        listError = 'An unexpected error occurred while loading secrets.';
      }
    } finally {
      loading = false;
    }
  }

  async function handleCreateSecret(event: Event) {
    event.preventDefault();
    if (creating || !secretName.trim() || !secretValue) return;

    creating = true;
    createError = null;
    createSuccess = null;

    try {
      const created = await createSecret({ name: secretName.trim(), value: secretValue });
      // Clear value from state immediately on success (same event cycle)
      secretValue = '';
      secretName = '';
      createSuccess = `Secret "${created.name}" created successfully.`;
      // Refresh the secrets list
      await fetchSecrets();
    } catch (err: unknown) {
      // Always clear value on error — never keep secret values in state
      secretValue = '';
      if (err instanceof ApiRequestError) {
        createError = err.message;
      } else if (err instanceof NetworkError) {
        createError = 'Unable to connect to the server. Please try again.';
      } else {
        createError = 'An unexpected error occurred. Please try again.';
      }
      // Name is retained for correction (secretName stays)
    } finally {
      creating = false;
    }
  }

  onMount(() => {
    fetchSecrets();
  });
</script>

<svelte:head>
  <title>Settings — Pulse</title>
</svelte:head>

<div class="mx-auto max-w-4xl space-y-8 px-4 py-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight text-slate-900">Settings</h1>
    <p class="mt-1 text-sm text-slate-500">Manage secrets used in monitor configurations.</p>
  </div>

  <!-- Create Secret Form -->
  <section class="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
    <h2 class="text-lg font-medium text-slate-900">Create Secret</h2>
    <p class="mt-1 text-sm text-slate-500">
      Secrets are stored encrypted and can be referenced in monitor targets.
    </p>

    {#if createSuccess}
      <div
        class="mt-4 rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700"
        role="status"
      >
        {createSuccess}
      </div>
    {/if}

    {#if createError}
      <div
        class="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        role="alert"
      >
        {createError}
      </div>
    {/if}

    <form onsubmit={handleCreateSecret} class="mt-4 space-y-4">
      <div>
        <label for="secret-name" class="block text-sm font-medium text-slate-700">Name</label>
        <input
          id="secret-name"
          type="text"
          maxlength={128}
          bind:value={secretName}
          placeholder="e.g. database-password"
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
      </div>

      <div>
        <label for="secret-value" class="block text-sm font-medium text-slate-700">Value</label>
        <input
          id="secret-value"
          type="password"
          bind:value={secretValue}
          placeholder="••••••••"
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
        <p class="mt-1 text-xs text-slate-400">Value is encrypted at rest and never displayed again.</p>
      </div>

      <button
        type="submit"
        disabled={creating || !secretName.trim() || !secretValue}
        class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if creating}
          Creating…
        {:else}
          Create Secret
        {/if}
      </button>
    </form>
  </section>

  <!-- Secrets List -->
  <section class="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
    <h2 class="text-lg font-medium text-slate-900">Secrets</h2>
    <p class="mt-1 text-sm text-slate-500">
      Only metadata is shown. Secret values are never returned by the API.
    </p>

    {#if loading}
      <div class="mt-4 flex items-center gap-2 text-sm text-slate-500">
        <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        Loading secrets…
      </div>
    {:else if listError}
      <div class="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
        <p>{listError}</p>
        <button
          onclick={fetchSecrets}
          class="mt-2 text-sm font-medium text-red-800 underline hover:text-red-900"
        >
          Retry
        </button>
      </div>
    {:else if secrets.length === 0}
      <p class="mt-4 text-sm text-slate-500">No secrets created yet. Use the form above to create one.</p>
    {:else}
      <div class="mt-4 overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-slate-200">
              <th class="pb-2 pr-4 font-medium text-slate-600">Reference</th>
              <th class="pb-2 pr-4 font-medium text-slate-600">ID</th>
              <th class="pb-2 font-medium text-slate-600">Created</th>
            </tr>
          </thead>
          <tbody>
            {#each secrets as secret (secret.id)}
              <tr class="border-b border-slate-100 last:border-0">
                <td class="py-3 pr-4 font-mono text-sm text-slate-800">
                  {formatSecretReference(secret.name, secret.id)}
                </td>
                <td class="py-3 pr-4 font-mono text-xs text-slate-500">{secret.id}</td>
                <td class="py-3 text-sm text-slate-600">{formatDate(secret.created_at)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</div>
