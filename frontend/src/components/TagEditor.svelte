<script lang="ts">
  import type { Tag } from '$lib/types';
  import { getTags, getTagValues } from '$lib/api';
  import { t } from '$lib/i18n';

  interface Props {
    tags: Tag[];
    onchange: (tags: Tag[]) => void;
  }

  let { tags, onchange }: Props = $props();

  // Autocomplete data
  let knownKeys = $state<string[]>([]);
  let knownValues = $state<Record<string, string[]>>({});
  let keysLoaded = $state(false);

  // Row state — each "pending" row the user is editing
  interface PendingTag {
    key: string;
    value: string;
  }
  let pendingRows = $state<PendingTag[]>([]);

  // Validation
  const KEY_PATTERN = /^[a-z][a-z0-9_-]{0,63}$/;
  const MAX_TAGS = 20;
  const MAX_VALUE_LENGTH = 256;

  let atLimit = $derived(tags.length + pendingRows.length >= MAX_TAGS);

  // Lazy-load known keys on first interaction
  async function ensureKeysLoaded(): Promise<void> {
    if (keysLoaded) return;
    keysLoaded = true;
    try {
      knownKeys = await getTags();
    } catch {
      // Non-critical
    }
  }

  async function fetchValuesForKey(key: string): Promise<void> {
    if (knownValues[key]) return;
    try {
      const values = await getTagValues(key);
      knownValues = { ...knownValues, [key]: values };
    } catch {
      // Non-critical
    }
  }

  function addPendingRow(): void {
    if (atLimit) return;
    ensureKeysLoaded();
    pendingRows = [...pendingRows, { key: '', value: '' }];
  }

  function removePendingRow(index: number): void {
    pendingRows = pendingRows.filter((_, i) => i !== index);
  }

  function commitRow(index: number): void {
    const row = pendingRows[index];
    if (!row) return;

    const key = row.key.trim();
    const value = row.value.trim();

    if (!key || !value) return;
    if (!KEY_PATTERN.test(key)) return;
    if (value.length > MAX_VALUE_LENGTH) return;
    if (tags.some((t) => t.key === key && t.value === value)) return;

    onchange([...tags, { key, value }]);
    pendingRows = pendingRows.filter((_, i) => i !== index);
  }

  function removeTag(index: number): void {
    const updated = [...tags];
    updated.splice(index, 1);
    onchange(updated);
  }

  function handleKeydown(event: KeyboardEvent, index: number): void {
    if (event.key === 'Enter') {
      event.preventDefault();
      commitRow(index);
    }
  }
</script>

<div data-testid="tag-editor">
  <!-- Header row: title + add button -->
  <div class="flex items-center justify-between">
    <span class="block text-sm font-medium text-primary">{t('monitors.tags.title')}</span>
    {#if !atLimit}
      <button
        type="button"
        onclick={addPendingRow}
        class="text-xs font-medium text-blue-600 hover:text-blue-800"
        data-testid="btn-add-tag"
      >
        + {t('monitors.tags.addTag')}
      </button>
    {/if}
  </div>
  <p class="mt-1 text-xs text-secondary">{t('monitors.tags.description')}</p>

  <!-- Existing tag chips + pending input rows -->
  {#if tags.length > 0 || pendingRows.length > 0}
    <div class="mt-2 space-y-2">
      <!-- Tag chips (compact, colored, inline) -->
      {#if tags.length > 0}
        <div class="flex flex-wrap gap-1.5">
          {#each tags as tag, i}
            <span
              class="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2 py-0.5 text-xs"
              data-testid="tag-row-{i}"
            >
              <span class="font-medium text-indigo-700">{tag.key}</span><span class="text-indigo-400">:</span><span class="text-indigo-600">{tag.value}</span>
              <button
                type="button"
                onclick={() => removeTag(i)}
                class="ml-0.5 inline-flex h-3.5 w-3.5 items-center justify-center rounded-full text-indigo-400 transition hover:bg-indigo-200 hover:text-indigo-700"
                aria-label={t('monitors.tags.remove', { key: tag.key, value: tag.value })}
                data-testid="tag-remove-{i}"
              >
                <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </span>
          {/each}
        </div>
      {/if}

      <!-- Pending input rows -->
      {#each pendingRows as row, i (i)}
        <div class="flex items-center gap-2" data-testid="tag-pending-{i}">
          <input
            type="text"
            bind:value={row.key}
            onfocus={() => ensureKeysLoaded()}
            onblur={() => { if (row.key.trim()) fetchValuesForKey(row.key.trim()); }}
            onkeydown={(e) => handleKeydown(e, i)}
            placeholder={t('monitors.tags.keyPlaceholder')}
            list="tag-keys-{i}"
            class="block w-1/3 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            data-testid="tag-key-input-{i}"
          />
          <datalist id="tag-keys-{i}">
            {#each knownKeys as key}
              <option value={key}></option>
            {/each}
          </datalist>

          <input
            type="text"
            bind:value={row.value}
            onfocus={() => { if (row.key.trim()) fetchValuesForKey(row.key.trim()); }}
            onkeydown={(e) => handleKeydown(e, i)}
            placeholder={t('monitors.tags.valuePlaceholder')}
            list="tag-values-{i}"
            class="block flex-1 rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-sm text-primary shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            data-testid="tag-value-input-{i}"
          />
          <datalist id="tag-values-{i}">
            {#each knownValues[row.key.trim()] ?? [] as value}
              <option value={value}></option>
            {/each}
          </datalist>

          <button
            type="button"
            onclick={() => commitRow(i)}
            disabled={!row.key.trim() || !row.value.trim() || !KEY_PATTERN.test(row.key.trim())}
            class="rounded p-1 text-green-600 hover:text-green-800 disabled:cursor-not-allowed disabled:opacity-30"
            aria-label={t('monitors.tags.confirm')}
            data-testid="tag-confirm-{i}"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
            </svg>
          </button>
          <button
            type="button"
            onclick={() => removePendingRow(i)}
            class="rounded p-1 text-slate-400 hover:text-rose-600"
            aria-label={t('monitors.tags.cancelRow')}
            data-testid="tag-cancel-{i}"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>
