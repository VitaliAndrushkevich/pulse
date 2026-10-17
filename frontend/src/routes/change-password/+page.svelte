<script lang="ts">
  import PasswordChangeSection from '../settings/PasswordChangeSection.svelte';
  import BrandLockup from '../../components/BrandLockup.svelte';
  import { clearToken } from '$lib/stores/auth.svelte';
  import { goto } from '$app/navigation';
  import { t } from '$lib/i18n';

  function logout() {
    clearToken();
    void goto('/login');
  }
</script>

<svelte:head><title>{t('users.firstLoginTitle')} — Pulse</title></svelte:head>
<div class="mx-auto flex min-h-[80vh] max-w-md flex-col justify-center gap-6">
  <BrandLockup size={40} variant="full" />
  <section class="pulse-panel space-y-5 p-6">
    <h1 class="text-2xl font-semibold text-primary">{t('users.firstLoginTitle')}</h1>
    <p class="text-sm text-secondary">{t('users.firstLoginDescription')}</p>
    <PasswordChangeSection forced />
    <button type="button" class="text-sm text-secondary" onclick={logout}>{t('nav.logout')}</button>
  </section>
</div>
