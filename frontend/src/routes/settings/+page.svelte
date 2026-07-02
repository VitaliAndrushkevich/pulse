<script lang="ts">
  import ApiTokenSection from './ApiTokenSection.svelte';
  import LanguageSelector from '../../components/LanguageSelector.svelte';
  import { t } from '$lib/i18n/locale.svelte';
</script>

<svelte:head>
  <title>{t('settings.title')} — Pulse</title>
</svelte:head>

<div class="mx-auto max-w-4xl space-y-8 px-4 py-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight text-[var(--color-text-primary)]">{t('settings.title')}</h1>
    <p class="mt-1 text-sm text-[var(--color-text-secondary)]">{t('settings.description')}</p>
  </div>

  <section class="space-y-4">
    <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{t('settings.language.title')}</h2>
    <LanguageSelector />
  </section>

  <ApiTokenSection />
</div>
