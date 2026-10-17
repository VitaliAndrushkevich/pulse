<script lang="ts" module>
  import type { Tag } from '$lib/types';

  export interface HeartbeatFormValues {
    name: string;
    expected_interval_seconds: number;
    grace_seconds: number;
    status: 'active' | 'paused';
    tags?: Tag[];
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import SelectField from './SelectField.svelte';
  import TagEditor from './TagEditor.svelte';
  import { t } from '$lib/i18n';

  interface Props {
    mode: 'create' | 'edit';
    initialValues?: Partial<HeartbeatFormValues>;
    initialTags?: Tag[];
    onSubmit: (values: HeartbeatFormValues) => Promise<void>;
    onCancel: () => void;
    extraSections?: Snippet;
  }

  let { mode, initialValues, initialTags = [], onSubmit, onCancel, extraSections }: Props = $props();

  // Form field state
  let name = $state(initialValues?.name ?? '');
  let expected_interval_seconds = $state<number>(initialValues?.expected_interval_seconds ?? 86400);
  let grace_seconds = $state<number>(initialValues?.grace_seconds ?? 3600);
  let status: 'active' | 'paused' = $state(initialValues?.status ?? 'active');
  let formTags = $state<Tag[]>(initialTags);

  // Submission state
  let submitting = $state(false);
  let submitError = $state<string | null>(null);

  // Per-field validation errors (populated on submit attempt)
  let errors = $state<{ name?: string; interval?: string; grace?: string }>({});

  const MAX_INTERVAL = 31536000; // 365 days, matches backend CHECK constraint

  // Client-side validation mirroring the backend bounds (design §3.3).
  function validate(): boolean {
    const next: { name?: string; interval?: string; grace?: string } = {};

    if (!name.trim()) {
      next.name = t('heartbeats.form.nameRequired');
    }

    if (
      !Number.isInteger(expected_interval_seconds) ||
      expected_interval_seconds < 1 ||
      expected_interval_seconds > MAX_INTERVAL
    ) {
      next.interval = t('heartbeats.form.intervalInvalid');
    }

    if (
      !Number.isInteger(grace_seconds) ||
      grace_seconds < 0 ||
      grace_seconds > MAX_INTERVAL
    ) {
      next.grace = t('heartbeats.form.graceInvalid');
    }

    errors = next;
    return Object.keys(next).length === 0;
  }

  function handleTagsChange(tags: Tag[]) {
    formTags = tags;
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    submitError = null;
    if (!validate()) return;

    submitting = true;
    try {
      await onSubmit({
        name: name.trim(),
        expected_interval_seconds,
        grace_seconds,
        status,
        tags: formTags
      });
    } catch (err: unknown) {
      submitError = err instanceof Error ? err.message : t('errors.unexpected');
    } finally {
      submitting = false;
    }
  }
</script>

<form onsubmit={handleSubmit} class="pulse-editor-form space-y-6" data-testid="heartbeat-form">
  <div class="pulse-panel space-y-6 p-5 sm:p-6">
    <!-- Name -->
    <div>
      <label for="hb-name" class="block text-sm font-medium text-primary">{t('heartbeats.fields.name')}</label>
      <input
        id="hb-name"
        type="text"
        bind:value={name}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        data-testid="hb-name-input"
      />
      {#if errors.name}
        <p class="mt-1 text-xs text-rose-600" data-testid="hb-name-error">{errors.name}</p>
      {/if}
    </div>

    <div class="grid grid-cols-1 gap-5 sm:grid-cols-2">
    <!-- Expected interval -->
    <div>
      <label for="hb-interval" class="block text-sm font-medium text-primary">{t('heartbeats.fields.expectedInterval')}</label>
      <input
        id="hb-interval"
        type="number"
        min="1"
        max={MAX_INTERVAL}
        bind:value={expected_interval_seconds}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        data-testid="hb-interval-input"
      />
      <p class="mt-1 text-xs text-secondary">{t('heartbeats.form.intervalHelp')}</p>
      {#if errors.interval}
        <p class="mt-1 text-xs text-rose-600" data-testid="hb-interval-error">{errors.interval}</p>
      {/if}
    </div>

    <!-- Grace period -->
    <div>
      <label for="hb-grace" class="block text-sm font-medium text-primary">{t('heartbeats.fields.grace')}</label>
      <input
        id="hb-grace"
        type="number"
        min="0"
        max={MAX_INTERVAL}
        bind:value={grace_seconds}
        class="mt-1 block w-full rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm text-primary shadow-sm focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
        data-testid="hb-grace-input"
      />
      <p class="mt-1 text-xs text-secondary">{t('heartbeats.form.graceHelp')}</p>
      {#if errors.grace}
        <p class="mt-1 text-xs text-rose-600" data-testid="hb-grace-error">{errors.grace}</p>
      {/if}
    </div>

    </div>

    <!-- Status -->
    <div>
      <label for="hb-status" class="block text-sm font-medium text-primary">{t('heartbeats.fields.status')}</label>
      <SelectField
        id="hb-status"
        bind:value={status}
        class="mt-1 w-full sm:max-w-xs"
        data-testid="hb-status-select"
      >
        <option value="active">{t('common.active')}</option>
        <option value="paused">{t('common.paused')}</option>
      </SelectField>
    </div>

    <!-- Tags -->
    <TagEditor tags={formTags} onchange={handleTagsChange} />
  </div>

  {#if submitError}
    <div class="rounded-md border border-rose-200 bg-rose-50 px-4 py-2 text-sm text-rose-700" data-testid="hb-submit-error">
      {submitError}
    </div>
  {/if}

  {#if extraSections}{@render extraSections()}{/if}

  <div class="pulse-editor-actions">

    <button
      type="button"
      onclick={onCancel}
      disabled={submitting}
      class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:opacity-50"
      data-testid="hb-cancel-button"
    >
      {t('common.cancel')}
    </button>
    <button
      type="submit"
      disabled={submitting}
      class="rounded-md bg-brand-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
      data-testid="hb-submit-button"
    >
      {#if submitting}
        {mode === 'create' ? t('common.creating') : t('common.saving')}
      {:else}
        {t('common.save')}
      {/if}
    </button>
  </div>
</form>
