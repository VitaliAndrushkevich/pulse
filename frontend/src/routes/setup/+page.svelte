<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { setupAdmin, getSetupStatus, ApiRequestError, NetworkError } from '$lib/api';
  import { setToken } from '$lib/stores/auth.svelte';
  import { validateEmail, validatePassword } from '$lib/validation';
  import BrandLockup from '../../components/BrandLockup.svelte';

  let email = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let error = $state<string | null>(null);
  let submitting = $state(false);
  let loading = $state(true);

  const emailValidation = $derived(validateEmail(email));
  const passwordValidation = $derived(validatePassword(password));
  const passwordsMatch = $derived(password === confirmPassword);
  const formValid = $derived(
    emailValidation.valid && passwordValidation.valid && passwordsMatch && password.length >= 8
  );

  onMount(async () => {
    try {
      const status = await getSetupStatus();
      if (!status.setup_required) {
        await goto('/login');
        return;
      }
    } catch {
      error = 'Unable to check setup status. Is the server running?';
    } finally {
      loading = false;
    }
  });

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!formValid || submitting) return;

    error = null;
    submitting = true;

    try {
      const response = await setupAdmin({ email, password });
      setToken(response.token);
      await goto('/');
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        if (err.statusCode === 409) {
          error = 'Setup has already been completed. Redirecting to login...';
          setTimeout(() => goto('/login'), 2000);
        } else {
          error = err.apiError?.message ?? 'Setup failed. Please try again.';
        }
      } else if (err instanceof NetworkError) {
        error = 'Service unavailable. Please try again later.';
      } else {
        error = 'An unexpected error occurred.';
      }
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Setup — Pulse</title>
</svelte:head>

{#if loading}
  <div class="flex min-h-[calc(100vh-80px)] items-center justify-center px-4">
    <p class="text-secondary">Checking setup status…</p>
  </div>
{:else}
  <div class="flex min-h-[calc(100vh-80px)] items-center justify-center px-4">
    <div class="w-full max-w-sm">
      <div class="mb-6 flex justify-center">
        <div style="max-width: 100%; height: auto;">
          <BrandLockup size={48} variant="full" />
        </div>
      </div>
      <div class="rounded-xl border border-[var(--color-border)] bg-surface p-8 shadow-sm">
      <div class="mb-6 text-center">
        <h1 class="text-2xl font-semibold tracking-tight text-primary">Welcome to Pulse</h1>
        <p class="mt-1 text-sm text-secondary">Create your admin account to get started</p>
      </div>

      {#if error}
        <div
          class="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          role="alert"
        >
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="space-y-4">
        <div>
          <label for="email" class="block text-sm font-medium text-primary">Email</label>
          <input
            id="email"
            type="email"
            autocomplete="email"
            bind:value={email}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            placeholder="admin@example.com"
          />
        </div>

        <div>
          <label for="password" class="block text-sm font-medium text-primary">Password</label>
          <input
            id="password"
            type="password"
            autocomplete="new-password"
            bind:value={password}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            placeholder="••••••••"
          />
          {#if password.length > 0 && password.length < 8}
            <p class="mt-1 text-xs text-red-500">Password must be at least 8 characters</p>
          {/if}
        </div>

        <div>
          <label for="confirm-password" class="block text-sm font-medium text-primary"
            >Confirm Password</label
          >
          <input
            id="confirm-password"
            type="password"
            autocomplete="new-password"
            bind:value={confirmPassword}
            class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            placeholder="••••••••"
          />
          {#if confirmPassword.length > 0 && !passwordsMatch}
            <p class="mt-1 text-xs text-red-500">Passwords do not match</p>
          {/if}
        </div>

        <button
          type="submit"
          disabled={!formValid || submitting}
          class="w-full rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {#if submitting}
            Creating account…
          {:else}
            Create Admin Account
          {/if}
        </button>
      </form>
      </div>
    </div>
  </div>
{/if}
