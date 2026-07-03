<script lang="ts">
  import type { MonitorType, Tag } from '$lib/types';
  import { t } from '$lib/i18n';

  interface Filters {
    types: MonitorType[];
    tags: Tag[];
    showPaused: boolean;
  }

  interface Props {
    availableTypes: MonitorType[];
    activeFilters: Filters;
    onFilterChange: (filters: Filters) => void;
  }

  let { availableTypes, activeFilters, onFilterChange }: Props = $props();

  // Tag input state
  let tagInput = $state('');
  let showTagInput = $state(false);
  let tagInputEl = $state<HTMLInputElement | null>(null);

  // UI state
  let isExpanded = $state(false);

  // Derived: whether any filters are active
  let hasActiveFilters = $derived(
    activeFilters.types.length > 0 || activeFilters.tags.length > 0 || activeFilters.showPaused
  );

  // Auto-expand when filters are active
  let showBar = $derived(isExpanded || hasActiveFilters);

  function toggleType(type: MonitorType): void {
    const current = activeFilters.types;
    const updated = current.includes(type)
      ? current.filter((t) => t !== type)
      : [...current, type];

    onFilterChange({ ...activeFilters, types: updated });
  }

  function togglePaused(): void {
    onFilterChange({ ...activeFilters, showPaused: !activeFilters.showPaused });
  }

  function removeTag(tag: Tag): void {
    const updated = activeFilters.tags.filter(
      (t) => !(t.key === tag.key && t.value === tag.value)
    );
    onFilterChange({ ...activeFilters, tags: updated });
  }

  function addTagFromInput(): void {
    const raw = tagInput.trim();
    if (!raw) return;

    // Parse key:value format
    const colonIdx = raw.indexOf(':');
    if (colonIdx <= 0) return; // must have key:value

    const key = raw.slice(0, colonIdx).trim();
    const value = raw.slice(colonIdx + 1).trim();
    if (!key || !value) return;

    // Avoid duplicates
    const exists = activeFilters.tags.some(
      (t) => t.key === key && t.value === value
    );
    if (exists) {
      tagInput = '';
      showTagInput = false;
      return;
    }

    const updated = [...activeFilters.tags, { key, value }];
    onFilterChange({ ...activeFilters, tags: updated });
    tagInput = '';
    showTagInput = false;
  }

  function handleTagKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault();
      addTagFromInput();
    }
    if (event.key === 'Escape') {
      showTagInput = false;
      tagInput = '';
    }
  }

  function openTagInput(): void {
    showTagInput = true;
    // Focus the input after it renders
    setTimeout(() => tagInputEl?.focus(), 0);
  }

  function handleTagInputBlur(): void {
    // Delay to allow Enter to fire first
    setTimeout(() => {
      if (!tagInput.trim()) {
        showTagInput = false;
      }
    }, 150);
  }

  function handleExpandClick(): void {
    isExpanded = true;
  }

  function handleCollapseClick(): void {
    if (!hasActiveFilters) {
      isExpanded = false;
    }
  }

  // Type display labels
  const typeLabels: Record<MonitorType, string> = {
    http: 'HTTP(S)',
    http3: 'HTTP/3',
    tcp: 'TCP',
    udp: 'UDP',
    websocket: 'WebSocket',
    grpc: 'gRPC',
    dns: 'DNS',
    icmp: 'ICMP',
    smtp: 'SMTP'
  };

  // Active pill colors per type
  const typePillColors: Record<MonitorType, string> = {
    http: 'bg-blue-600 text-white',
    http3: 'bg-cyan-600 text-white',
    tcp: 'bg-purple-600 text-white',
    udp: 'bg-amber-600 text-white',
    websocket: 'bg-pink-600 text-white',
    grpc: 'bg-indigo-600 text-white',
    dns: 'bg-teal-600 text-white',
    icmp: 'bg-orange-600 text-white',
    smtp: 'bg-rose-600 text-white'
  };
</script>

{#if !showBar}
  <!-- Collapsed state: single Filter button -->
  <button
    type="button"
    onclick={handleExpandClick}
    class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm font-medium text-secondary transition hover:bg-[var(--color-bg-surface-hover)]"
    data-testid="filter-expand-button"
  >
    <span class="inline-flex items-center gap-1.5">
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
      </svg>
      {t('monitors.filter.expand')}
    </span>
  </button>
{:else}
  <!-- Expanded filter bar -->
  <div
    class="space-y-2 rounded-lg border border-[var(--color-border)] bg-surface p-3"
    data-testid="filter-bar"
  >
    <!-- Row 1: Type pills + Paused toggle + collapse button -->
    <div class="flex flex-wrap items-center gap-2">
      <!-- Type pill toggles -->
      <div class="flex flex-wrap items-center gap-1.5" data-testid="type-filters">
        {#each availableTypes as type}
          {@const isActive = activeFilters.types.includes(type)}
          <button
            type="button"
            onclick={() => toggleType(type)}
            class="rounded-full px-3 py-1 text-xs font-medium transition {isActive
              ? typePillColors[type]
              : 'bg-[var(--color-bg-surface-hover)] text-secondary hover:bg-[var(--color-bg-surface-hover)]'}"
            aria-pressed={isActive}
            data-testid="type-pill-{type}"
          >
            {typeLabels[type]}
          </button>
        {/each}
      </div>

      <!-- Separator -->
      <div class="h-5 w-px bg-[var(--color-border)]" aria-hidden="true"></div>

      <!-- Paused toggle -->
      <button
        type="button"
        onclick={togglePaused}
        class="rounded-full px-3 py-1 text-xs font-medium transition {activeFilters.showPaused
          ? 'bg-slate-600 text-white'
          : 'bg-[var(--color-bg-surface-hover)] text-secondary hover:bg-[var(--color-bg-surface-hover)]'}"
        aria-pressed={activeFilters.showPaused}
        data-testid="paused-filter"
      >
        {t('monitors.filter.paused')}
      </button>

      <!-- Collapse button -->
      {#if !hasActiveFilters}
        <button
          type="button"
          onclick={handleCollapseClick}
          class="ml-auto inline-flex h-5 w-5 items-center justify-center rounded text-[var(--color-text-muted)] transition hover:text-secondary"
          aria-label={t('monitors.filter.collapse')}
          data-testid="filter-collapse-button"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      {/if}
    </div>

    <!-- Row 2: Active tag chips + inline tag input (shown on demand) -->
    {#if activeFilters.tags.length > 0 || showTagInput}
      <div class="flex flex-wrap items-center gap-1.5">
        <!-- Active tag chips -->
        {#each activeFilters.tags as tag}
          <span
            class="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium"
            data-testid="tag-chip-{tag.key}-{tag.value}"
          >
            <span class="text-indigo-700">{tag.key}</span><span class="text-indigo-400">:</span><span class="text-indigo-600">{tag.value}</span>
            <button
              type="button"
              onclick={() => removeTag(tag)}
              class="ml-0.5 inline-flex h-3.5 w-3.5 items-center justify-center rounded-full text-indigo-400 transition hover:bg-indigo-200 hover:text-indigo-600"
              aria-label={t('monitors.filter.removeTag', { key: tag.key, value: tag.value })}
              data-testid="tag-remove-{tag.key}-{tag.value}"
            >
              <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </span>
        {/each}

        <!-- Inline tag input (compact, appears when toggled) -->
        {#if showTagInput}
          <span class="inline-flex items-center gap-1">
            <input
              type="text"
              bind:this={tagInputEl}
              bind:value={tagInput}
              onkeydown={handleTagKeydown}
              onblur={handleTagInputBlur}
              placeholder={t('monitors.filter.tagPlaceholder')}
              class="w-48 rounded-full border border-[var(--color-border)] bg-surface px-2.5 py-0.5 text-xs text-primary placeholder:text-[var(--color-text-muted)] focus:border-indigo-400 focus:outline-none focus:ring-1 focus:ring-indigo-400"
              data-testid="tag-filter-input"
            />
          </span>
        {:else}
          <!-- "+ Tag" pill button to open input -->
          <button
            type="button"
            onclick={openTagInput}
            class="inline-flex items-center gap-0.5 rounded-full border border-dashed border-[var(--color-border)] px-2 py-0.5 text-xs text-secondary transition hover:border-indigo-400 hover:text-indigo-600"
            data-testid="tag-filter-add-btn"
          >
            <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            {t('monitors.filter.addTag')}
          </button>
        {/if}
      </div>
    {:else}
      <!-- No tags yet — show "+ Tag" button inline -->
      <div class="flex items-center">
        {#if showTagInput}
          <input
            type="text"
            bind:this={tagInputEl}
            bind:value={tagInput}
            onkeydown={handleTagKeydown}
            onblur={handleTagInputBlur}
            placeholder={t('monitors.filter.tagPlaceholder')}
            class="w-48 rounded-full border border-[var(--color-border)] bg-surface px-2.5 py-0.5 text-xs text-primary placeholder:text-[var(--color-text-muted)] focus:border-indigo-400 focus:outline-none focus:ring-1 focus:ring-indigo-400"
            data-testid="tag-filter-input"
          />
        {:else}
          <button
            type="button"
            onclick={openTagInput}
            class="inline-flex items-center gap-0.5 rounded-full border border-dashed border-[var(--color-border)] px-2 py-0.5 text-xs text-secondary transition hover:border-indigo-400 hover:text-indigo-600"
            data-testid="tag-filter-add-btn"
          >
            <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            {t('monitors.filter.addTag')}
          </button>
        {/if}
      </div>
    {/if}
  </div>
{/if}
