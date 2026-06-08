<script lang="ts">
  import type { GrpcSettings, TlsMode } from '$lib/types';

  interface Props {
    settings: GrpcSettings;
  }

  let { settings = $bindable() }: Props = $props();

  const GRPC_STATUS_CODES = [
    { code: 0, name: 'OK' },
    { code: 1, name: 'CANCELLED' },
    { code: 2, name: 'UNKNOWN' },
    { code: 3, name: 'INVALID_ARGUMENT' },
    { code: 4, name: 'DEADLINE_EXCEEDED' },
    { code: 5, name: 'NOT_FOUND' },
    { code: 6, name: 'ALREADY_EXISTS' },
    { code: 7, name: 'PERMISSION_DENIED' },
    { code: 8, name: 'RESOURCE_EXHAUSTED' },
    { code: 9, name: 'FAILED_PRECONDITION' },
    { code: 10, name: 'ABORTED' },
    { code: 11, name: 'OUT_OF_RANGE' },
    { code: 12, name: 'UNIMPLEMENTED' },
    { code: 13, name: 'INTERNAL' },
    { code: 14, name: 'UNAVAILABLE' },
    { code: 15, name: 'DATA_LOSS' },
    { code: 16, name: 'UNAUTHENTICATED' },
  ];

  const MAX_METADATA_ROWS = 20;

  // Internal state
  let serviceMethod = $state(settings?.service_method ?? 'grpc.health.v1.Health/Check');
  let tlsMode: TlsMode = $state(settings?.tls_mode ?? 'tls');
  let sslExpiryThreshold = $state<number | undefined>(settings?.ssl_expiry_threshold);
  let metadataRows = $state<Array<{ key: string; value: string }>>(
    settings?.metadata
      ? Object.entries(settings.metadata).map(([key, value]) => ({ key, value }))
      : []
  );
  let expectedStatuses = $state<number[]>(settings?.expected_statuses ?? [0]);
  let requestPayload = $state(settings?.request_payload ?? '');

  // Reactive output — syncs internal state to bound settings prop
  $effect(() => {
    const result: GrpcSettings = {
      service_method: serviceMethod,
      tls_mode: tlsMode,
      expected_statuses: expectedStatuses,
    };

    if (tlsMode !== 'plaintext' && sslExpiryThreshold != null && sslExpiryThreshold > 0) {
      result.ssl_expiry_threshold = sslExpiryThreshold;
    }

    const filteredMetadata = metadataRows.filter(r => r.key.trim());
    if (filteredMetadata.length > 0) {
      result.metadata = Object.fromEntries(filteredMetadata.map(r => [r.key, r.value]));
    }

    if (requestPayload.trim()) {
      result.request_payload = requestPayload;
    }

    settings = result;
  });

  function addMetadataRow() {
    if (metadataRows.length < MAX_METADATA_ROWS) {
      metadataRows = [...metadataRows, { key: '', value: '' }];
    }
  }

  function removeMetadataRow(index: number) {
    metadataRows = metadataRows.filter((_, i) => i !== index);
  }

  function toggleStatus(code: number) {
    if (expectedStatuses.includes(code)) {
      expectedStatuses = expectedStatuses.filter(c => c !== code);
    } else {
      expectedStatuses = [...expectedStatuses, code];
    }
  }
</script>

<div class="space-y-6" data-testid="grpc-settings-form">
  <!-- Service Method -->
  <div>
    <label for="grpc-service-method" class="block text-sm font-medium text-primary">Service Method</label>
    <input
      id="grpc-service-method"
      type="text"
      bind:value={serviceMethod}
      required
      maxlength={512}
      placeholder="grpc.health.v1.Health/Check"
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="grpc-service-method"
    />
    <p class="mt-1 text-xs text-secondary">Fully-qualified gRPC service and method (package.Service/Method)</p>
  </div>

  <!-- TLS Mode -->
  <div>
    <label for="grpc-tls-mode" class="block text-sm font-medium text-primary">TLS Mode</label>
    <select
      id="grpc-tls-mode"
      bind:value={tlsMode}
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="grpc-tls-mode"
    >
      <option value="plaintext">Plaintext</option>
      <option value="tls">TLS</option>
      <option value="tls_skip_verify">TLS (Skip Verify)</option>
    </select>
  </div>

  <!-- SSL Expiry Threshold (hidden when plaintext) -->
  {#if tlsMode !== 'plaintext'}
    <div>
      <label for="grpc-ssl-expiry" class="block text-sm font-medium text-primary">SSL Expiry Threshold (days)</label>
      <input
        id="grpc-ssl-expiry"
        type="number"
        bind:value={sslExpiryThreshold}
        min={1}
        max={3650}
        placeholder="30"
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        data-testid="grpc-ssl-expiry"
      />
      <p class="mt-1 text-xs text-secondary">Alert when certificate expires within this many days</p>
    </div>
  {/if}

  <!-- Metadata Key-Value Rows -->
  <div>
    <div class="flex items-center justify-between">
      <span class="block text-sm font-medium text-primary">Metadata</span>
      <button
        type="button"
        onclick={addMetadataRow}
        disabled={metadataRows.length >= MAX_METADATA_ROWS}
        class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="grpc-add-metadata"
      >
        Add Metadata
      </button>
    </div>

    {#if metadataRows.length > 0}
      <div class="mt-2 space-y-2">
        {#each metadataRows as row, index}
          <div class="flex items-center gap-2">
            <input
              type="text"
              bind:value={row.key}
              maxlength={128}
              placeholder="Key"
              aria-label="Metadata key {index + 1}"
              class="block w-1/3 rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              data-testid="grpc-metadata-key-{index}"
            />
            <input
              type="text"
              bind:value={row.value}
              maxlength={4096}
              placeholder="Value"
              aria-label="Metadata value {index + 1}"
              class="block flex-1 rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              data-testid="grpc-metadata-value-{index}"
            />
            <button
              type="button"
              onclick={() => removeMetadataRow(index)}
              aria-label="Remove metadata row {index + 1}"
              class="rounded-md border border-[var(--color-border)] bg-surface px-2 py-2 text-xs text-rose-600 transition hover:bg-rose-50"
              data-testid="grpc-metadata-remove-{index}"
            >
              ✕
            </button>
          </div>
        {/each}
      </div>
    {/if}

    <p class="mt-1 text-xs text-secondary">Custom gRPC metadata headers sent with each request</p>
  </div>

  <!-- Expected Status Codes -->
  <fieldset>
    <legend class="block text-sm font-medium text-primary">Expected Status Codes</legend>
    <div class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3" data-testid="grpc-expected-statuses">
      {#each GRPC_STATUS_CODES as { code, name }}
        <label class="flex items-center gap-2 text-sm text-secondary">
          <input
            type="checkbox"
            checked={expectedStatuses.includes(code)}
            onchange={() => toggleStatus(code)}
            class="rounded border-[var(--color-border)] text-blue-600 focus:ring-blue-500"
            data-testid="grpc-status-{code}"
          />
          {code} ({name})
        </label>
      {/each}
    </div>
    <p class="mt-1 text-xs text-secondary">gRPC status codes considered a healthy response</p>
  </fieldset>

  <!-- Request Payload -->
  <div>
    <label for="grpc-request-payload" class="block text-sm font-medium text-primary">Request Payload (Base64)</label>
    <textarea
      id="grpc-request-payload"
      bind:value={requestPayload}
      maxlength={65536}
      rows={3}
      placeholder="Base64-encoded protobuf message"
      class="mt-1 block w-full rounded-md border border-[var(--color-border)] px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      data-testid="grpc-request-payload"
    ></textarea>
    <p class="mt-1 text-xs text-secondary">Optional base64-encoded protobuf request body</p>
  </div>
</div>
