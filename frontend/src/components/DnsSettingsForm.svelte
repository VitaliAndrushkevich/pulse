<script lang="ts">
  import type { DnsSettings, DnsRecordType } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    settings: DnsSettings;
  }

  let { settings = $bindable() }: Props = $props();

  const DNS_RECORD_TYPES: DnsRecordType[] = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'SOA', 'PTR', 'NS'];

  // Internal state
  let recordType: DnsRecordType = $state(settings?.record_type ?? 'A');
  let expectedValue = $state(settings?.expected_value ?? '');
  let dnsServer = $state(settings?.dns_server ?? '');

  // Reactive output — syncs internal state to bound settings prop
  $effect(() => {
    const result: DnsSettings = {
      record_type: recordType,
    };

    if (expectedValue.trim()) {
      result.expected_value = expectedValue.trim();
    }

    if (dnsServer.trim()) {
      result.dns_server = dnsServer.trim();
    }

    settings = result;
  });
</script>

<div class="space-y-6" data-testid="dns-settings-form">
  <!-- Record Type -->
  <div>
    <label for="dns-record-type" class="block text-sm font-medium text-primary">{t('dns.recordType')}</label>
    <select
      id="dns-record-type"
      bind:value={recordType}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="dns-record-type"
    >
      {#each DNS_RECORD_TYPES as rt}
        <option value={rt}>{rt}</option>
      {/each}
    </select>
    <p class="mt-1 text-xs text-secondary">{t('dns.recordTypeHelp')}</p>
  </div>

  <!-- Expected Value -->
  <div>
    <label for="dns-expected-value" class="block text-sm font-medium text-primary">{t('dns.expectedValue')}</label>
    <input
      id="dns-expected-value"
      type="text"
      bind:value={expectedValue}
      placeholder={t('dns.expectedValuePlaceholder')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="dns-expected-value"
    />
    <p class="mt-1 text-xs text-secondary">{t('dns.expectedValueHelp')}</p>
  </div>

  <!-- DNS Server -->
  <div>
    <label for="dns-server" class="block text-sm font-medium text-primary">{t('dns.dnsServer')}</label>
    <input
      id="dns-server"
      type="text"
      bind:value={dnsServer}
      placeholder={t('dns.dnsServerPlaceholder')}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="dns-server"
    />
    <p class="mt-1 text-xs text-secondary">{t('dns.dnsServerHelp')}</p>
  </div>
</div>
