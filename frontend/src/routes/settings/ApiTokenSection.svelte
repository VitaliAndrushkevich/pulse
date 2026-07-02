<script lang="ts">
  import { onMount } from 'svelte';
  import {
    listApiTokens,
    createApiToken,
    revokeApiToken,
    ApiRequestError,
    NetworkError,
  } from '$lib/api';
  import type { ApiToken } from '$lib/api';
  import { formatDate } from '$lib/format';
  import { t } from '$lib/i18n/locale.svelte';
  import ShowOnceModal from '../../components/ShowOnceModal.svelte';

  let tokens = $state<ApiToken[]>([]);
  let loading = $state(true);
  let listError = $state<string | null>(null);

  // Create form state
  let tokenName = $state('');
  let creating = $state(false);
  let createError = $state<string | null>(null);

  // Show-once modal state
  let showModal = $state(false);
  let createdTokenValue = $state('');

  // Revoke state
  let revokingId = $state<string | null>(null);

  async function fetchTokens() {
    loading = true;
    listError = null;
    try {
      const result = await listApiTokens();
      tokens = result.data;
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        listError = err.message;
      } else if (err instanceof NetworkError) {
        listError = t('settings.tokens.errors.networkList');
      } else {
        listError = t('settings.tokens.errors.unexpectedList');
      }
    } finally {
      loading = false;
    }
  }

  async function handleCreateToken(event: Event) {
    event.preventDefault();
    if (creating || !tokenName.trim()) return;

    creating = true;
    createError = null;

    try {
      const result = await createApiToken({ name: tokenName.trim() });
      // Store the raw token for one-time display
      createdTokenValue = result.token;
      showModal = true;
      tokenName = '';
      // Refresh the tokens list
      await fetchTokens();
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        createError = err.message;
      } else if (err instanceof NetworkError) {
        createError = t('settings.tokens.errors.networkCreate');
      } else {
        createError = t('settings.tokens.errors.unexpectedCreate');
      }
    } finally {
      creating = false;
    }
  }

  function handleModalDismiss() {
    // Permanently discard the plaintext token value
    createdTokenValue = '';
    showModal = false;
  }

  async function handleRevoke(tokenId: string) {
    if (revokingId) return;
    revokingId = tokenId;

    try {
      await revokeApiToken(tokenId);
      await fetchTokens();
    } catch (err: unknown) {
      // Error toasts are handled by the API client
    } finally {
      revokingId = null;
    }
  }

  function getExpirationStatus(token: ApiToken): string {
    if (token.revoked_at) return t('settings.tokens.revoked');
    if (!token.expires_at) return t('common.never');
    const expires = new Date(token.expires_at);
    if (expires < new Date()) return t('settings.tokens.expired');
    return formatDate(token.expires_at);
  }

  function isRevoked(token: ApiToken): boolean {
    return !!token.revoked_at;
  }

  onMount(() => {
    fetchTokens();
  });
</script>

<!-- Show-Once Modal for newly created token -->
{#if showModal}
  <ShowOnceModal secret={createdTokenValue} onDismiss={handleModalDismiss} />
{/if}

<section class="space-y-6">
  <!-- Section header -->
  <div>
    <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{t('settings.tokens.title')}</h2>
    <p class="mt-1 text-sm text-[var(--color-text-secondary)]">
      {t('settings.tokens.description')}
    </p>
  </div>

  <!-- Create Token Form -->
  <div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] p-6 shadow-sm">
    <h3 class="text-sm font-medium text-[var(--color-text-primary)]">{t('settings.tokens.createTitle')}</h3>

    {#if createError}
      <div
        class="mt-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        role="alert"
      >
        {createError}
      </div>
    {/if}

    <form onsubmit={handleCreateToken} class="mt-3 flex items-end gap-3">
      <div class="flex-1">
        <label for="token-name" class="block text-sm font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.createLabel')}</label>
        <input
          id="token-name"
          type="text"
          maxlength={128}
          bind:value={tokenName}
          placeholder={t('settings.tokens.createPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
      </div>
      <button
        type="submit"
        disabled={creating || !tokenName.trim()}
        class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if creating}
          {t('common.creating')}
        {:else}
          {t('settings.tokens.createButton')}
        {/if}
      </button>
    </form>
  </div>

  <!-- Token List -->
  <div class="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] p-6 shadow-sm">
    <h3 class="text-sm font-medium text-[var(--color-text-primary)]">{t('settings.tokens.existingTitle')}</h3>
    <p class="mt-1 text-xs text-[var(--color-text-muted)]">
      {t('settings.tokens.existingDescription')}
    </p>

    {#if loading}
      <div class="mt-4 flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
        <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
        </svg>
        {t('settings.tokens.loadingTokens')}
      </div>
    {:else if listError}
      <div class="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
        <p>{listError}</p>
        <button
          onclick={fetchTokens}
          class="mt-2 text-sm font-medium text-red-800 underline hover:text-red-900"
        >
          {t('common.retry')}
        </button>
      </div>
    {:else if tokens.length === 0}
      <p class="mt-4 text-sm text-[var(--color-text-secondary)]">{t('settings.tokens.emptyState')}</p>
    {:else}
      <div class="mt-4 overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-[var(--color-border)]">
              <th class="pb-2 pr-4 font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.tableHeaders.name')}</th>
              <th class="pb-2 pr-4 font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.tableHeaders.created')}</th>
              <th class="pb-2 pr-4 font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.tableHeaders.lastUsed')}</th>
              <th class="pb-2 pr-4 font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.tableHeaders.expires')}</th>
              <th class="pb-2 font-medium text-[var(--color-text-secondary)]">{t('settings.tokens.tableHeaders.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {#each tokens as token (token.id)}
              <tr class="border-b border-[var(--color-border)]/50 last:border-0" class:opacity-50={isRevoked(token)}>
                <td class="py-3 pr-4 text-sm font-medium text-[var(--color-text-primary)]">
                  {token.name}
                  {#if isRevoked(token)}
                    <span class="ml-2 inline-flex items-center rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
                      {t('settings.tokens.revoked')}
                    </span>
                  {/if}
                </td>
                <td class="py-3 pr-4 text-sm text-[var(--color-text-secondary)]">
                  {formatDate(token.created_at)}
                </td>
                <td class="py-3 pr-4 text-sm text-[var(--color-text-secondary)]">
                  {formatDate(token.last_used_at ?? null, t('common.never'))}
                </td>
                <td class="py-3 pr-4 text-sm text-[var(--color-text-secondary)]">
                  {getExpirationStatus(token)}
                </td>
                <td class="py-3">
                  {#if !isRevoked(token)}
                    <button
                      type="button"
                      onclick={() => handleRevoke(token.id)}
                      disabled={revokingId === token.id}
                      class="text-sm font-medium text-red-600 transition hover:text-red-800 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {#if revokingId === token.id}
                        {t('common.revoking')}
                      {:else}
                        {t('settings.tokens.revoke')}
                      {/if}
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</section>
