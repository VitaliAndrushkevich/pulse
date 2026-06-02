<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { login, getSetupStatus, ApiRequestError, NetworkError } from '$lib/api';
  import { setToken } from '$lib/stores/auth.svelte';
  import { validateEmail, validatePassword } from '$lib/validation';

  let email = $state('');
  let password = $state('');
  let error = $state<string | null>(null);
  let submitting = $state(false);

  const emailValidation = $derived(validateEmail(email));
  const passwordValidation = $derived(validatePassword(password));
  const formValid = $derived(emailValidation.valid && passwordValidation.valid);

  onMount(async () => {
    try {
      const status = await getSetupStatus();
      if (status.setup_required) {
        await goto('/setup');
      }
    } catch {
      // Server unreachable — stay on login page
    }
  });

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!formValid || submitting) return;

    error = null;
    submitting = true;

    try {
      const response = await login({ email, password });
      setToken(response.token);
      await goto('/');
    } catch (err: unknown) {
      if (err instanceof ApiRequestError && err.statusCode === 401) {
        error = 'Invalid email or password';
      } else if (err instanceof NetworkError) {
        error = 'Service unavailable. Please try again later.';
      } else {
        error = 'Service unavailable. Please try again later.';
      }
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Login — Pulse</title>
</svelte:head>

<div class="flex min-h-[calc(100vh-80px)] items-center justify-center px-4">
  <div class="w-full max-w-sm rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
    <div class="mb-6 text-center">
      <h1 class="text-2xl font-semibold tracking-tight text-slate-900">Sign in to Pulse</h1>
      <p class="mt-1 text-sm text-slate-500">Enter your credentials to continue</p>
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
        <label for="email" class="block text-sm font-medium text-slate-700">Email</label>
        <input
          id="email"
          type="email"
          autocomplete="email"
          bind:value={email}
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          placeholder="you@example.com"
        />
      </div>

      <div>
        <label for="password" class="block text-sm font-medium text-slate-700">Password</label>
        <input
          id="password"
          type="password"
          autocomplete="current-password"
          bind:value={password}
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          placeholder="••••••••"
        />
      </div>

      <button
        type="submit"
        disabled={!formValid || submitting}
        class="w-full rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if submitting}
          Signing in…
        {:else}
          Sign in
        {/if}
      </button>
    </form>
  </div>
</div>
