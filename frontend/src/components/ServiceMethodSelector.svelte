<script lang="ts">
  import type { ProtoService, ServiceMethodSelection } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Props {
    services: ProtoService[];
    onselect?: (selection: ServiceMethodSelection) => void;
  }

  let { services, onselect }: Props = $props();

  // Track which services are expanded
  let expandedServices = $state<Set<string>>(new Set<string>());

  // Initialize expanded set whenever services change
  $effect(() => {
    expandedServices = new Set(services.map((s) => s.full_name));
  });

  // The currently selected method's full_name (radio-button style — exactly one)
  let selectedFullName = $state<string | null>(null);

  // Auto-select if there's exactly one service with one method
  $effect(() => {
    if (services.length === 1 && services[0].methods.length === 1) {
      const method = services[0].methods[0];
      selectedFullName = method.full_name;
    }
  });

  // Derived: can confirm (something is selected)
  let canConfirm = $derived(selectedFullName !== null);

  // Derived: find the selected method details for emitting
  let selectedMethodDetails = $derived.by(() => {
    if (!selectedFullName) return null;
    for (const service of services) {
      for (const method of service.methods) {
        if (method.full_name === selectedFullName) {
          return { service, method };
        }
      }
    }
    return null;
  });

  function toggleService(serviceName: string) {
    const next = new Set(expandedServices);
    if (next.has(serviceName)) {
      next.delete(serviceName);
    } else {
      next.add(serviceName);
    }
    expandedServices = next;
  }

  function selectMethod(fullName: string) {
    selectedFullName = fullName;
  }

  function handleConfirm() {
    if (!selectedMethodDetails || !onselect) return;
    const { service, method } = selectedMethodDetails;

    const selection: ServiceMethodSelection = {
      service_name: service.full_name,
      method_name: method.name,
      full_method: method.full_name,
      input_type: method.input_type,
      output_type: method.output_type,
    };

    onselect(selection);
  }
</script>

<div class="space-y-3" data-testid="service-method-selector">
  <p class="text-sm font-medium text-primary">{t('proto.upload.selectMethod')}</p>

  <div class="space-y-2">
    {#each services as service}
      {@const isExpanded = expandedServices.has(service.full_name)}
      <div
        class="rounded-md border border-[var(--color-border)] overflow-hidden"
        data-testid="service-group-{service.full_name}"
      >
        <!-- Service header (expandable) -->
        <button
          type="button"
          onclick={() => toggleService(service.full_name)}
          class="flex w-full items-center gap-2 px-3 py-2 text-start bg-[var(--color-bg-surface)] hover:bg-[var(--color-bg-surface-hover)] transition"
          aria-expanded={isExpanded}
          data-testid="service-toggle-{service.full_name}"
        >
          <svg
            class="h-4 w-4 text-secondary transition-transform {isExpanded ? 'rotate-90' : ''}"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
            aria-hidden="true"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          <span class="text-xs font-semibold text-secondary">{service.full_name}</span>
          <span class="ml-auto text-xs text-secondary">
            {t('proto.upload.methodCount', { count: service.methods.length })}
          </span>
        </button>

        <!-- Methods list -->
        {#if isExpanded}
          <ul class="border-t border-[var(--color-border)]" role="radiogroup" aria-label={service.full_name}>
            {#each service.methods as method}
              {@const isSelected = selectedFullName === method.full_name}
              <li
                class="border-b border-[var(--color-border)] last:border-b-0"
              >
                <button
                  type="button"
                  role="radio"
                  aria-checked={isSelected}
                  onclick={() => selectMethod(method.full_name)}
                  class="flex w-full items-center gap-3 px-4 py-2.5 text-start transition {isSelected
                    ? 'bg-[var(--color-brand-primary)]/5'
                    : 'hover:bg-[var(--color-bg-surface-hover)]'}"
                  data-testid="method-option-{method.full_name}"
                >
                  <!-- Radio indicator -->
                  <span
                    class="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2 transition {isSelected
                      ? 'border-[var(--color-brand-primary)]'
                      : 'border-[var(--color-border)]'}"
                    aria-hidden="true"
                  >
                    {#if isSelected}
                      <span class="h-2 w-2 rounded-full bg-[var(--color-brand-primary)]"></span>
                    {/if}
                  </span>

                  <!-- Method info -->
                  <div class="min-w-0 flex-1">
                    <span class="font-mono text-sm text-primary">{method.name}</span>
                    <p class="mt-0.5 text-xs text-secondary truncate">
                      {method.input_type} → {method.output_type}
                    </p>
                  </div>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/each}
  </div>

  <!-- Confirm button -->
  <div class="flex justify-end pt-2">
    <button
      type="button"
      onclick={handleConfirm}
      disabled={!canConfirm}
      class="rounded-md bg-[var(--color-brand-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      data-testid="service-method-confirm"
    >
      {t('common.confirm')}
    </button>
  </div>
</div>
