<script lang="ts">
  interface Props {
    retentionDays: number;
    onchange: (range: { from: string; to: string }) => void;
    selected?: string;
  }

  let { retentionDays, onchange, selected }: Props = $props();

  type PresetKey = '1h' | '6h' | '24h' | '7d' | '30d';

  const presets: { key: PresetKey; label: string; ms: number }[] = [
    { key: '1h', label: '1h', ms: 60 * 60 * 1000 },
    { key: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
    { key: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
    { key: '7d', label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
    { key: '30d', label: '30d', ms: 30 * 24 * 60 * 60 * 1000 }
  ];

  let mode = $state<'preset' | 'custom'>('preset');
  let activePreset = $state<PresetKey | null>(null);

  // Sync activePreset with external `selected` prop changes
  $effect(() => {
    if (selected) {
      activePreset = selected as PresetKey;
      mode = 'preset';
    }
  });

  let customStart = $state('');
  let customEnd = $state('');
  let validationError = $state<string | null>(null);

  // Compute whether the currently selected range exceeds retention
  let showRetentionNotice = $derived.by(() => {
    const retentionMs = retentionDays * 24 * 60 * 60 * 1000;
    if (mode === 'preset' && activePreset) {
      const preset = presets.find((p) => p.key === activePreset);
      return preset ? preset.ms > retentionMs : false;
    }
    if (mode === 'custom' && customStart && customEnd) {
      const startMs = new Date(customStart).getTime();
      const endMs = new Date(customEnd).getTime();
      if (!isNaN(startMs) && !isNaN(endMs)) {
        return endMs - startMs > retentionMs;
      }
    }
    return false;
  });

  function selectPreset(key: PresetKey): void {
    mode = 'preset';
    activePreset = key;
    validationError = null;

    const preset = presets.find((p) => p.key === key)!;
    const now = new Date();
    const to = now.toISOString();
    const from = new Date(now.getTime() - preset.ms).toISOString();

    onchange({ from, to });
  }

  function enableCustomMode(): void {
    mode = 'custom';
    activePreset = null;
    validationError = null;
  }

  function applyCustomRange(): void {
    if (!customStart || !customEnd) {
      validationError = 'Please select both start and end times';
      return;
    }

    const startDate = new Date(customStart);
    const endDate = new Date(customEnd);

    if (isNaN(startDate.getTime()) || isNaN(endDate.getTime())) {
      validationError = 'Invalid date values';
      return;
    }

    if (startDate >= endDate) {
      validationError = 'Start time must be before end time';
      return;
    }

    // Clamp end to current time if in the future
    const now = new Date();
    const clampedEnd = endDate > now ? now : endDate;

    // Re-validate after clamping
    if (startDate >= clampedEnd) {
      validationError = 'Start time must be before end time';
      return;
    }

    validationError = null;

    const from = startDate.toISOString();
    const to = clampedEnd.toISOString();

    onchange({ from, to });
  }

  // Helper to format datetime-local max value (current time)
  function nowLocalString(): string {
    const now = new Date();
    const offset = now.getTimezoneOffset();
    const local = new Date(now.getTime() - offset * 60 * 1000);
    return local.toISOString().slice(0, 16);
  }
</script>

<div class="flex flex-col gap-3" data-testid="time-range-picker">
  <!-- Preset buttons row -->
  <div class="flex flex-wrap items-center gap-2">
    {#each presets as preset}
      <button
        type="button"
        onclick={() => selectPreset(preset.key)}
        class="rounded-md px-3 py-1.5 text-sm font-medium transition
          {mode === 'preset' && activePreset === preset.key
          ? 'bg-[var(--color-brand-primary)] text-white'
          : 'border border-[var(--color-border)] bg-surface text-secondary hover:bg-[var(--color-bg-surface-hover)]'}"
        aria-pressed={mode === 'preset' && activePreset === preset.key}
        data-testid="preset-{preset.key}"
      >
        {preset.label}
      </button>
    {/each}

    <!-- Custom button -->
    <button
      type="button"
      onclick={enableCustomMode}
      class="rounded-md px-3 py-1.5 text-sm font-medium transition
        {mode === 'custom'
        ? 'bg-[var(--color-brand-primary)] text-white'
        : 'border border-[var(--color-border)] bg-surface text-secondary hover:bg-[var(--color-bg-surface-hover)]'}"
      aria-pressed={mode === 'custom'}
      data-testid="preset-custom"
    >
      Custom
    </button>
  </div>

  <!-- Custom range inputs -->
  {#if mode === 'custom'}
    <div class="flex flex-wrap items-end gap-3" data-testid="custom-range-inputs">
      <div class="flex flex-col gap-1">
        <label for="range-start" class="text-xs font-medium text-secondary">Start</label>
        <input
          id="range-start"
          type="datetime-local"
          bind:value={customStart}
          max={nowLocalString()}
          class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary focus:border-[var(--color-brand-primary)] focus:outline-none focus:ring-1 focus:ring-[var(--color-brand-primary)]"
          data-testid="custom-start"
        />
      </div>
      <div class="flex flex-col gap-1">
        <label for="range-end" class="text-xs font-medium text-secondary">End</label>
        <input
          id="range-end"
          type="datetime-local"
          bind:value={customEnd}
          max={nowLocalString()}
          class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary focus:border-[var(--color-brand-primary)] focus:outline-none focus:ring-1 focus:ring-[var(--color-brand-primary)]"
          data-testid="custom-end"
        />
      </div>
      <button
        type="button"
        onclick={applyCustomRange}
        class="rounded-md bg-[var(--color-brand-primary)] px-4 py-1.5 text-sm font-medium text-white transition hover:bg-[var(--color-brand-hover)]"
        data-testid="custom-apply"
      >
        Apply
      </button>
    </div>
  {/if}

  <!-- Validation error -->
  {#if validationError}
    <p class="text-sm text-[var(--color-error)]" role="alert" data-testid="validation-error">
      {validationError}
    </p>
  {/if}

  <!-- Retention notice -->
  {#if showRetentionNotice}
    <div
      class="flex items-center gap-2 rounded-md border border-[var(--color-warning)] bg-[var(--color-warning)]/10 px-3 py-2 text-sm text-[var(--color-warning)]"
      role="status"
      data-testid="retention-notice"
    >
      <svg class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
      <span>Selected range exceeds the {retentionDays}-day retention period. Data may be incomplete.</span>
    </div>
  {/if}
</div>
