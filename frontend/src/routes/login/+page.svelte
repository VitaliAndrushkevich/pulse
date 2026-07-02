<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { login, getSetupStatus, ApiRequestError, NetworkError } from '$lib/api';
  import { setToken } from '$lib/stores/auth.svelte';
  import { validateEmail, validatePassword } from '$lib/validation';
  import BrandLockup from '../../components/BrandLockup.svelte';
  import { t } from '$lib/i18n';

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
        error = t('login.errors.invalidCredentials');
      } else if (err instanceof NetworkError) {
        error = t('login.errors.networkError');
      } else {
        error = t('login.errors.unexpected');
      }
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>{t('app.title', { page: 'Login' })}</title>
</svelte:head>

<div class="flex min-h-[calc(100vh-80px)] items-center justify-center px-4">
  <div class="w-full max-w-sm">
    <div class="mb-6 flex justify-center">
      <div style="max-width: 100%; height: auto;">
        <BrandLockup size={48} variant="full" />
      </div>
    </div>
    <div class="rounded-xl border border-[var(--color-border)] bg-surface p-8 shadow-sm">
    <div class="mb-6 text-center">
      <h1 class="text-2xl font-semibold tracking-tight text-primary">{t('login.title')}</h1>
      <p class="mt-1 text-sm text-secondary">{t('login.subtitle')}</p>
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
        <label for="email" class="block text-sm font-medium text-primary">{t('login.email')}</label>
        <input
          id="email"
          type="email"
          autocomplete="email"
          bind:value={email}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          placeholder={t('login.emailPlaceholder')}
        />
      </div>

      <div>
        <label for="password" class="block text-sm font-medium text-primary">{t('login.password')}</label>
        <input
          id="password"
          type="password"
          autocomplete="current-password"
          bind:value={password}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          placeholder={t('login.passwordPlaceholder')}
        />
      </div>

      <button
        type="submit"
        disabled={!formValid || submitting}
        class="w-full rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if submitting}
          {t('login.submitting')}
        {:else}
          {t('login.submit')}
        {/if}
      </button>
    </form>
    </div>
  </div>
</div>
