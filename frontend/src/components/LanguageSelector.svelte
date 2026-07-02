<script lang="ts">
  import { getLocale, setLocale, t } from '$lib/i18n/locale.svelte';
  import { SUPPORTED_LOCALES } from '$lib/i18n/config';
  import type { LocaleCode } from '$lib/i18n/config';

  let selected = $derived(getLocale());

  function handleChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    setLocale(target.value as LocaleCode);
  }
</script>

<div class="space-y-2">
  <label
    for="language-select"
    id="language-select-label"
    class="block text-sm font-medium text-primary"
  >
    {t('settings.language.label')}
  </label>
  <select
    id="language-select"
    aria-labelledby="language-select-label"
    value={selected}
    onchange={handleChange}
    class="block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm transition focus:border-[var(--color-brand-primary)] focus:outline-none focus:ring-1 focus:ring-[var(--color-brand-primary)]"
    data-testid="language-select"
  >
    {#each SUPPORTED_LOCALES as locale}
      <option value={locale.code}>{locale.name}</option>
    {/each}
  </select>
</div>
