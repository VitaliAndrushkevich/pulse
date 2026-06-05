<script lang="ts">
  import type { Credential } from '$lib/api';

  interface Props {
    credentials: Credential[];
    onDelete: (credentialId: string) => void;
    onReplace: (credential: Credential) => void;
    loading?: boolean;
  }

  let { credentials, onDelete, onReplace, loading = false }: Props = $props();

  let confirmDeleteId: string | null = $state(null);

  function formatAuthType(authType: string): string {
    switch (authType) {
      case 'bearer':
        return 'Bearer Token';
      case 'basic':
        return 'Basic Auth';
      case 'header':
        return 'Custom Header';
      default:
        return authType;
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }

  function handleDeleteClick(credentialId: string) {
    if (confirmDeleteId === credentialId) {
      onDelete(credentialId);
      confirmDeleteId = null;
    } else {
      confirmDeleteId = credentialId;
    }
  }

  function cancelDelete() {
    confirmDeleteId = null;
  }
</script>

<div class="space-y-3" data-testid="credential-list">
  {#if credentials.length === 0}
    <p class="text-sm text-slate-500" data-testid="credential-list-empty">
      No credentials configured for this monitor.
    </p>
  {:else}
    <ul class="divide-y divide-slate-200 rounded-md border border-slate-200">
      {#each credentials as credential (credential.id)}
        <li class="flex items-center justify-between px-4 py-3" data-testid="credential-item">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-slate-900" data-testid="credential-name">
                {credential.name}
              </span>
              <span class="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600" data-testid="credential-auth-type">
                {formatAuthType(credential.auth_type)}
              </span>
            </div>
            <p class="mt-0.5 text-xs text-slate-500" data-testid="credential-created-at">
              Created {formatDate(credential.created_at)}
            </p>
          </div>

          <div class="flex items-center gap-2 ml-4">
            {#if confirmDeleteId === credential.id}
              <span class="text-xs text-rose-600 mr-1">Confirm?</span>
              <button
                type="button"
                onclick={() => handleDeleteClick(credential.id)}
                disabled={loading}
                class="inline-flex items-center rounded-md bg-rose-600 px-2.5 py-1.5 text-xs font-medium text-white transition hover:bg-rose-700 disabled:opacity-50"
                data-testid="btn-confirm-delete"
                aria-label="Confirm delete credential {credential.name}"
              >
                Delete
              </button>
              <button
                type="button"
                onclick={cancelDelete}
                class="inline-flex items-center rounded-md border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition hover:bg-slate-50"
                data-testid="btn-cancel-delete"
                aria-label="Cancel delete"
              >
                Cancel
              </button>
            {:else}
              <button
                type="button"
                onclick={() => onReplace(credential)}
                disabled={loading}
                class="inline-flex items-center rounded-md border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition hover:bg-slate-50 disabled:opacity-50"
                data-testid="btn-replace-credential"
                aria-label="Replace credential {credential.name}"
              >
                Replace
              </button>
              <button
                type="button"
                onclick={() => handleDeleteClick(credential.id)}
                disabled={loading}
                class="inline-flex items-center rounded-md border border-rose-200 bg-white px-2.5 py-1.5 text-xs font-medium text-rose-600 transition hover:bg-rose-50 disabled:opacity-50"
                data-testid="btn-delete-credential"
                aria-label="Delete credential {credential.name}"
              >
                Delete
              </button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
