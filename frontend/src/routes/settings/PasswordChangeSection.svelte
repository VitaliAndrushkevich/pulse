<script lang="ts">
  import { changePassword, ApiRequestError, NetworkError } from '$lib/api';
  import { toastStore } from '$lib/stores/toast.svelte';
  import { t } from '$lib/i18n/locale.svelte';

  // Form state
  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');

  // Dirty tracking
  let newPasswordDirty = $state(false);

  // Action states
  let submitting = $state(false);
  let error = $state<string | null>(null);

  // Derived validation
  let newPasswordTooShort = $derived(newPasswordDirty && newPassword.length > 0 && newPassword.length < 8);
  let passwordsMismatch = $derived(confirmPassword.length > 0 && newPassword !== confirmPassword);

  let canSubmit = $derived(
    !submitting &&
    currentPassword.trim().length > 0 &&
    newPassword.trim().length > 0 &&
    confirmPassword.trim().length > 0 &&
    newPassword === confirmPassword
  );

  function handleNewPasswordInput() {
    newPasswordDirty = true;
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!canSubmit) return;

    submitting = true;
    error = null;

    try {
      await changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });

      // Success: show toast, clear fields
      toastStore.addToast({
        type: 'success',
        message: t('settings.password.success'),
        persistent: false,
      });

      currentPassword = '';
      newPassword = '';
      confirmPassword = '';
      newPasswordDirty = false;
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        if (err.statusCode === 401) {
          error = t('settings.password.errors.incorrectCurrent');
        } else {
          error = err.apiError?.message ?? t('settings.password.errors.failed');
        }
      } else if (err instanceof NetworkError) {
        error = t('settings.password.errors.failed');
      } else {
        error = t('settings.password.errors.failed');
      }
    } finally {
      submitting = false;
    }
  }
</script>

<section class="space-y-6">
  <!-- Section header -->
  <div>
    <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{t('settings.password.title')}</h2>
    <p class="mt-1 text-sm text-[var(--color-text-secondary)]">
      {t('settings.password.description')}
    </p>
  </div>

  <form onsubmit={handleSubmit} class="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] p-6 shadow-sm">
    <!-- Inline error above form -->
    {#if error}
      <div
        class="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        role="alert"
      >
        {error}
      </div>
    {/if}

    <div class="space-y-4">
      <!-- Current Password -->
      <div>
        <label for="current-password" class="block text-sm font-medium text-[var(--color-text-primary)]">
          {t('settings.password.currentPassword')}
        </label>
        <input
          id="current-password"
          type="password"
          maxlength={128}
          bind:value={currentPassword}
          placeholder={t('settings.password.currentPasswordPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
      </div>

      <!-- New Password -->
      <div>
        <label for="new-password" class="block text-sm font-medium text-[var(--color-text-primary)]">
          {t('settings.password.newPassword')}
        </label>
        <input
          id="new-password"
          type="password"
          maxlength={128}
          bind:value={newPassword}
          oninput={handleNewPasswordInput}
          placeholder={t('settings.password.newPasswordPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          class:border-red-500={newPasswordTooShort}
        />
        {#if newPasswordTooShort}
          <p class="mt-1 text-xs text-red-600">{t('settings.password.validation.minLength')}</p>
        {/if}
      </div>

      <!-- Confirm Password -->
      <div>
        <label for="confirm-password" class="block text-sm font-medium text-[var(--color-text-primary)]">
          {t('settings.password.confirmPassword')}
        </label>
        <input
          id="confirm-password"
          type="password"
          maxlength={128}
          bind:value={confirmPassword}
          placeholder={t('settings.password.confirmPasswordPlaceholder')}
          class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)] px-3 py-2 text-sm text-[var(--color-text-primary)] shadow-sm placeholder:text-[var(--color-text-muted)] focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          class:border-red-500={passwordsMismatch}
        />
        {#if passwordsMismatch}
          <p class="mt-1 text-xs text-red-600">{t('settings.password.validation.mismatch')}</p>
        {/if}
      </div>
    </div>

    <!-- Submit button -->
    <div class="mt-6">
      <button
        type="submit"
        disabled={!canSubmit}
        class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if submitting}
          {t('settings.password.submitting')}
        {:else}
          {t('settings.password.submit')}
        {/if}
      </button>
    </div>
  </form>
</section>
