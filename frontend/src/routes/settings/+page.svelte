<script lang="ts">
  import { isAdmin } from '$lib/stores/auth.svelte';
  import UsersSection from './UsersSection.svelte';
  import PageHeader from '../../components/PageHeader.svelte';
  import ApiTokenSection from './ApiTokenSection.svelte';
  import SmtpSettingsSection from './SmtpSettingsSection.svelte';
  import PasswordChangeSection from './PasswordChangeSection.svelte';
  import CollapsibleSection from '../../components/CollapsibleSection.svelte';
  import LanguageSelector from '../../components/LanguageSelector.svelte';
  import { t } from '$lib/i18n/locale.svelte';
</script>

<svelte:head>
  <title>{t('settings.title')} — Pulse</title>
</svelte:head>

<section class="space-y-6">
  <PageHeader eyebrow={t('shell.settingsOverview')} title={t('settings.title')} description={t('settings.description')} />

  <section class="pulse-panel space-y-4 p-5">
    <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{t('settings.language.title')}</h2>
    <div class="max-w-xl"><LanguageSelector /></div>
  </section>

  {#if isAdmin()}
  <CollapsibleSection title={t('users.title')} description={t('users.description')} defaultOpen={true}>
    <UsersSection />
  </CollapsibleSection>

  <CollapsibleSection title={t('settings.tokens.title')} description={t('settings.tokens.description')} defaultOpen={true}>
    <ApiTokenSection />
  </CollapsibleSection>

  <CollapsibleSection title={t('notifications.smtpSettings.title')} description={t('notifications.smtpSettings.description')}>
    <SmtpSettingsSection />
  </CollapsibleSection>

  {/if}

  <CollapsibleSection title={t('settings.password.title')} description={t('settings.password.description')}>
    <PasswordChangeSection />
  </CollapsibleSection>
</section>
