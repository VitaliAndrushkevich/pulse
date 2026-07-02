<script lang="ts">
  import type { IcmpSettings } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    settings: IcmpSettings;
  }

  let { settings = $bindable() }: Props = $props();

  // Internal state
  let packetCount = $state(settings?.packet_count ?? 3);
  let lossThresholdPercent = $state(settings?.loss_threshold_percent ?? 100);
  let useIpv6 = $state(settings?.use_ipv6 ?? false);

  // Reactive output — syncs internal state to bound settings prop
  $effect(() => {
    const result: IcmpSettings = {
      packet_count: packetCount,
      loss_threshold_percent: lossThresholdPercent,
      use_ipv6: useIpv6,
    };

    settings = result;
  });
</script>

<div class="space-y-6" data-testid="icmp-settings-form">
  <!-- Packet Count -->
  <div>
    <label for="icmp-packet-count" class="block text-sm font-medium text-primary">{t('icmp.packetCount')}</label>
    <input
      id="icmp-packet-count"
      type="number"
      bind:value={packetCount}
      min={1}
      max={10}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="icmp-packet-count"
    />
    <p class="mt-1 text-xs text-secondary">{t('icmp.packetCountHelp')}</p>
  </div>

  <!-- Loss Threshold -->
  <div>
    <label for="icmp-loss-threshold" class="block text-sm font-medium text-primary">{t('icmp.lossThreshold')}</label>
    <input
      id="icmp-loss-threshold"
      type="number"
      bind:value={lossThresholdPercent}
      min={0}
      max={100}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="icmp-loss-threshold"
    />
    <p class="mt-1 text-xs text-secondary">{t('icmp.lossThresholdHelp')}</p>
  </div>

  <!-- Use IPv6 -->
  <div>
    <label class="inline-flex items-center gap-2 text-sm text-secondary">
      <input
        type="checkbox"
        bind:checked={useIpv6}
        class="rounded border-[var(--color-border)] text-blue-600 focus:ring-blue-500"
        data-testid="icmp-use-ipv6"
      />
      <span>{t('icmp.useIpv6')}</span>
    </label>
    <p class="mt-1 text-xs text-secondary">{t('icmp.useIpv6Help')}</p>
  </div>
</div>
