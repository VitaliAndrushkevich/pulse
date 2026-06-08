<script lang="ts">
  import { onMount } from 'svelte';
  import CredentialForm from './CredentialForm.svelte';
  import CredentialList from './CredentialList.svelte';
  import ShowOnceModal from './ShowOnceModal.svelte';
  import {
    createCredential,
    listCredentials,
    deleteCredential,
    type Credential,
    type CreateCredentialRequest,
  } from '$lib/api';

  interface Props {
    monitorId: string;
  }

  let { monitorId }: Props = $props();

  // State
  let credentials: Credential[] = $state([]);
  let loading = $state(false);
  let showOnceSecret: string | null = $state(null);
  let replacingCredentialId: string | null = $state(null);

  // Load credentials on mount
  onMount(() => {
    loadCredentials();
  });

  async function loadCredentials() {
    if (!monitorId) return;
    loading = true;
    try {
      credentials = await listCredentials(monitorId);
    } catch {
      // Error toast handled by apiRequest
    } finally {
      loading = false;
    }
  }

  async function handleCreate(req: CreateCredentialRequest) {
    loading = true;
    try {
      // If replacing, delete the old credential first
      if (replacingCredentialId) {
        await deleteCredential(monitorId, replacingCredentialId);
        replacingCredentialId = null;
      }

      // Create the new credential
      const _created = await createCredential(monitorId, req);

      // Extract the secret value to show once
      // The secret is the raw value the user just typed — show it in the modal
      const secretValue = extractSecretFromRequest(req);
      showOnceSecret = secretValue;

      // Reload the credential list
      await loadCredentials();
    } catch {
      // Error toast handled by apiRequest
    } finally {
      loading = false;
    }
  }

  function extractSecretFromRequest(req: CreateCredentialRequest): string {
    switch (req.auth_type) {
      case 'bearer':
        return req.token ?? '';
      case 'basic':
        return req.password ?? '';
      case 'header':
        return req.header_value ?? '';
      default:
        return '';
    }
  }

  async function handleDelete(credentialId: string) {
    loading = true;
    try {
      await deleteCredential(monitorId, credentialId);
      await loadCredentials();
    } catch {
      // Error toast handled by apiRequest
    } finally {
      loading = false;
    }
  }

  function handleReplace(credential: Credential) {
    // Mark that we're replacing — next form submission will delete old + create new
    replacingCredentialId = credential.id;
  }

  function handleModalDismiss() {
    showOnceSecret = null;
  }
</script>

<section class="space-y-4" data-testid="auth-section">
  <div>
    <h3 class="text-sm font-medium text-primary">Authentication</h3>
    <p class="mt-1 text-xs text-secondary">
      Configure credentials for this monitor's health-check requests.
    </p>
  </div>

  <!-- Existing credentials list -->
  <CredentialList
    {credentials}
    onDelete={handleDelete}
    onReplace={handleReplace}
    {loading}
  />

  <!-- Add/Replace credential form -->
  <div>
    {#if replacingCredentialId}
      <div class="mb-2 flex items-center justify-between rounded-md border border-amber-200 bg-amber-50 px-3 py-2">
        <p class="text-sm text-amber-800">
          Replacing credential — fill in the new values below.
        </p>
        <button
          type="button"
          onclick={() => { replacingCredentialId = null; }}
          class="text-xs font-medium text-amber-700 hover:text-amber-900"
          data-testid="btn-cancel-replace"
        >
          Cancel
        </button>
      </div>
    {/if}
    <CredentialForm onSubmit={handleCreate} {loading} />
  </div>
</section>

<!-- Show-once modal for newly created credential -->
{#if showOnceSecret}
  <ShowOnceModal secret={showOnceSecret} onDismiss={handleModalDismiss} />
{/if}
