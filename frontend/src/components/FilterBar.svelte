<script lang="ts">
  import { untrack } from 'svelte';
  import type { MonitorType, Tag } from '$lib/types';
  import { getTags, getTagValues } from '$lib/api';
  import { t } from '$lib/i18n';

  interface Props {
    availableTypes: MonitorType[];
    activeFilters: { types: MonitorType[]; tags: Tag[] };
    onFilterChange: (filters: { types: MonitorType[]; tags: Tag[] }) => void;
  }

  let { availableTypes, activeFilters, onFilterChange }: Props = $props();

  // Component-level cache for tag autocomplete data
  let tagKeys = $state<string[]>([]);
  let tagValuesCache = $state<Record<string, string[]>>({});
  let isLoadingTags = $state(false);

  // UI state
  let isExpanded = $state(false);
  let isTagSelectorOpen = $state(false);
  let selectedTagKey = $state<string | null>(null);

  // Derived: whether any filters are active
  let hasActiveFilters = $derived(
    activeFilters.types.length > 0 || activeFilters.tags.length > 0
  );

  // Auto-expand when filters are active
  let showBar = $derived(isExpanded || hasActiveFilters);

  // Fetch and cache tag keys + values on mount (once)
  $effect(() => {
    untrack(() => fetchTagOptions());
  });

  async function fetchTagOptions(): Promise<void> {
    if (isLoadingTags) return;
    isLoadingTags = true;
    try {
      const keys = await getTags();
      tagKeys = keys;

      // Pre-fetch values for each key
      const valuesMap: Record<string, string[]> = {};
      await Promise.all(
        keys.map(async (key) => {
          const values = await getTagValues(key);
          valuesMap[key] = values;
        })
      );
      tagValuesCache = valuesMap;
    } catch {
      // Silently handle errors — filter bar is non-critical
    } finally {
      isLoadingTags = false;
    }
  }

  function toggleType(type: MonitorType): void {
    const current = activeFilters.types;
    const updated = current.includes(type)
      ? current.filter((t) => t !== type)
      : [...current, type];

    onFilterChange({ types: updated, tags: activeFilters.tags });
  }

  function removeTag(tag: Tag): void {
    const updated = activeFilters.tags.filter(
      (t) => !(t.key === tag.key && t.value === tag.value)
    );
    onFilterChange({ types: activeFilters.types, tags: updated });
  }

  function addTag(key: string, value: string): void {
    // Avoid duplicates
    const exists = activeFilters.tags.some(
      (t) => t.key === key && t.value === value
    );
    if (exists) return;

    const updated = [...activeFilters.tags, { key, value }];
    onFilterChange({ types: activeFilters.types, tags: updated });
    isTagSelectorOpen = false;
    selectedTagKey = null;
  }

  function handleExpandClick(): void {
    isExpanded = true;
  }

  function handleCollapseClick(): void {
    if (!hasActiveFilters) {
      isExpanded = false;
    }
  }

  function openTagSelector(): void {
    isTagSelectorOpen = true;
    selectedTagKey = null;
  }

  function closeTagSelector(): void {
    isTagSelectorOpen = false;
    selectedTagKey = null;
  }

  function selectTagKey(key: string): void {
    selectedTagKey = key;
  }

  // Type display labels
  const typeLabels: Record<MonitorType, string> = {
    http: 'HTTP(S)',
    http3: 'HTTP/3',
    tcp: 'TCP',
    udp: 'UDP',
    websocket: 'WebSocket',
    grpc: 'gRPC'
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
    class="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--color-border)] bg-surface p-3"
    data-testid="filter-bar"
  >
    <!-- Type pill toggles -->
    <div class="flex flex-wrap items-center gap-1.5" data-testid="type-filters">
      {#each availableTypes as type}
        {@const isActive = activeFilters.types.includes(type)}
        <button
          type="button"
          onclick={() => toggleType(type)}
          class="rounded-full px-3 py-1 text-xs font-medium transition {isActive
            ? 'bg-blue-600 text-white'
            : 'bg-[var(--color-bg-surface-hover)] text-secondary hover:bg-[var(--color-bg-surface-hover)]'}"
          aria-pressed={isActive}
          data-testid="type-pill-{type}"
        >
          {typeLabels[type]}
        </button>
      {/each}
    </div>

    <!-- Separator when both type and tag filters present -->
    {#if availableTypes.length > 0 && (activeFilters.tags.length > 0 || tagKeys.length > 0)}
      <div class="h-5 w-px bg-[var(--color-border)]" aria-hidden="true"></div>
    {/if}

    <!-- Active tag chips -->
    {#if activeFilters.tags.length > 0}
      <div class="flex flex-wrap items-center gap-1.5" data-testid="tag-chips">
        {#each activeFilters.tags as tag}
          <span
            class="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2.5 py-0.5 text-xs font-medium text-indigo-700"
            data-testid="tag-chip-{tag.key}-{tag.value}"
          >
            {tag.key}:{tag.value}
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
      </div>
    {/if}

    <!-- Add tag button / selector -->
    {#if tagKeys.length > 0}
      <div class="relative">
        {#if !isTagSelectorOpen}
          <button
            type="button"
            onclick={openTagSelector}
            class="inline-flex items-center gap-1 rounded-full border border-dashed border-[var(--color-border)] px-2.5 py-0.5 text-xs font-medium text-secondary transition hover:border-[var(--color-border)] hover:text-primary"
            data-testid="add-tag-button"
          >
            <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            {t('monitors.filter.addTag')}
          </button>
        {:else}
          <!-- Tag selector dropdown -->
          <div
            class="absolute left-0 top-full z-10 mt-1 min-w-[160px] rounded-md border border-[var(--color-border)] bg-surface py-1 shadow-lg"
            data-testid="tag-selector"
          >
            {#if selectedTagKey === null}
              <!-- Key selection -->
              <div class="px-2 py-1 text-xs font-medium text-[var(--color-text-muted)]">{t('monitors.filter.selectKey')}</div>
              {#each tagKeys as key}
                <button
                  type="button"
                  onclick={() => selectTagKey(key)}
                  class="block w-full px-3 py-1.5 text-left text-xs text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
                  data-testid="tag-key-option-{key}"
                >
                  {key}
                </button>
              {/each}
            {:else}
              <!-- Value selection for chosen key -->
              <div class="px-2 py-1 text-xs font-medium text-[var(--color-text-muted)]">{selectedTagKey} =</div>
              {#each tagValuesCache[selectedTagKey] ?? [] as value}
                <button
                  type="button"
                  onclick={() => addTag(selectedTagKey!, value)}
                  class="block w-full px-3 py-1.5 text-left text-xs text-primary transition hover:bg-[var(--color-bg-surface-hover)]"
                  data-testid="tag-value-option-{value}"
                >
                  {value}
                </button>
              {/each}
              <button
                type="button"
                onclick={() => (selectedTagKey = null)}
                class="block w-full border-t border-[var(--color-border)] px-3 py-1.5 text-left text-xs text-[var(--color-text-muted)] transition hover:bg-[var(--color-bg-surface-hover)]"
              >
                {t('monitors.filter.back')}
              </button>
            {/if}
            <button
              type="button"
              onclick={closeTagSelector}
              class="block w-full border-t border-[var(--color-border)] px-3 py-1.5 text-left text-xs text-[var(--color-text-muted)] transition hover:bg-[var(--color-bg-surface-hover)]"
              data-testid="tag-selector-close"
            >
              {t('monitors.filter.cancel')}
            </button>
          </div>
        {/if}
      </div>
    {/if}

    <!-- Collapse button (only when no filters active) -->
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
{/if}
