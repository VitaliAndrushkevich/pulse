<script lang="ts">
  import type { SmtpSettings } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    settings: SmtpSettings;
  }

  let { settings = $bindable() }: Props = $props();

  // Internal state
  let port = $state(settings?.port ?? 25);
  let starttls = $state(settings?.starttls ?? true);
  let ehloDomain = $state(settings?.ehlo_domain ?? '');
  let sslExpiryThreshold = $state<number | undefined>(settings?.ssl_expiry_threshold);

  // Reactive output — syncs internal state to bound settings prop
  $effect(() => {
    const result: SmtpSettings = {
      port,
      starttls,
    };

    if (ehloDomain.trim()) {
      result.ehlo_domain = ehloDomain.trim();
    }

    if (starttls && sslExpiryThreshold != null && sslExpiryThreshold > 0) {
      result.ssl_expiry_threshold = sslExpiryThreshold;
    }

    settings = result;
  });
</script>

<div class="space-y-6" data-testid="smtp-settings-form">
  <!-- Port -->
  <div>
    <label for="smtp-port" class="block text-sm font-medium text-primary">{t('smtp.port')}</label>
    <input
      id="smtp-port"
      type="number"
      bind:value={port}
      min={1}
      max={65535}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="smtp-port"
    />
    <p class="mt-1 text-xs text-secondary">{t('smtp.portHelp')}</p>
  </div>

  <!-- STARTTLS -->
  <div>
    <label class="inline-flex items-center gap-2 text-sm text-secondary">
      <input
        type="checkbox"
        bind:checked={starttls}
        class="rounded border-[var(--color-border)] text-blue-600 focus:ring-blue-500"
        data-testid="smtp-starttls"
      />
      <span>{t('smtp.starttls')}</span>
    </label>
    <p class="mt-1 text-xs text-secondary">{t('smtp.starttlsHelp')}</p>
  </div>

  <!-- EHLO Domain -->
  <div>
    <label for="smtp-ehlo-domain" class="block text-sm font-medium text-primary">{t('smtp.ehloDomain')}</label>
    <input
      id="smtp-ehlo-domain"
      type="text"
      bind:value={ehloDomain}
      placeholder={t('smtp.ehloDomainPlaceholder')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="smtp-ehlo-domain"
    />
    <p class="mt-1 text-xs text-secondary">{t('smtp.ehloDomainHelp')}</p>
  </div>

  <!-- SSL Expiry Threshold (visible only when starttls is enabled) -->
  {#if starttls}
    <div>
      <label for="smtp-ssl-expiry" class="block text-sm font-medium text-primary">{t('smtp.sslExpiryThreshold')}</label>
      <input
        id="smtp-ssl-expiry"
        type="number"
        bind:value={sslExpiryThreshold}
        min={0}
        placeholder={t('smtp.sslExpiryThresholdPlaceholder')}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="smtp-ssl-expiry"
      />
      <p class="mt-1 text-xs text-secondary">{t('smtp.sslExpiryThresholdHelp')}</p>
    </div>
  {/if}
</div>
