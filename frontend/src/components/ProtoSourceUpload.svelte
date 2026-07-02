<script lang="ts">
  import type { ProtoSourceMeta, ServiceMethodSelection } from '$lib/types';
  import { uploadProtoSource, deleteProtoSource, adHocReflect, adHocParseProto } from '$lib/api';
  import { t } from '$lib/i18n';
  import ServiceMethodSelector from './ServiceMethodSelector.svelte';

  interface Props {
    monitorId?: string;
    target: string;
    tlsMode?: string;
    currentSource?: ProtoSourceMeta | null;
    onSourceChanged?: (source: ProtoSourceMeta | null) => void;
    onMethodSelected?: (selection: ServiceMethodSelection) => void;
  }

  let { monitorId, target, tlsMode = 'tls', currentSource = null, onSourceChanged, onMethodSelected }: Props = $props();

  // Internal state
  let isDragging = $state(false);
  let selectedFiles = $state<File[]>([]);
  let isUploading = $state(false);
  let isReflecting = $state(false);
  let error = $state<string | null>(null);

  let fileInputRef: HTMLInputElement | undefined = $state();

  const MAX_FILES = 20;
  const ACCEPTED_EXTENSIONS = ['.proto', '.desc'];

  let isLoading = $derived(isUploading || isReflecting);
  let reflectionEnabled = $derived(target.trim().length > 0);

  // Track newly discovered source (shown after upload/reflection for method selection)
  let discoveredSource = $state<ProtoSourceMeta | null>(null);

  // The source to show in the method selector: newly discovered takes priority
  let selectorSource = $derived(discoveredSource ?? currentSource);

  // Whether to show the method selector inline (after successful upload/reflection)
  let showSelector = $derived(
    discoveredSource !== null && discoveredSource.services.length > 0
  );

  // --- Drag and drop handlers ---

  function handleDragEnter(e: DragEvent) {
    e.preventDefault();
    isDragging = true;
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    isDragging = true;
  }

  function handleDragLeave(e: DragEvent) {
    e.preventDefault();
    isDragging = false;
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    isDragging = false;

    const droppedFiles = e.dataTransfer?.files;
    if (droppedFiles) {
      addFiles(Array.from(droppedFiles));
    }
  }

  // --- File selection ---

  function handleFileInputChange(e: Event) {
    const input = e.target as HTMLInputElement;
    if (input.files) {
      addFiles(Array.from(input.files));
    }
    // Reset input so the same file can be re-selected
    input.value = '';
  }

  function addFiles(files: File[]) {
    const validFiles = files.filter((f) => {
      const ext = '.' + f.name.split('.').pop()?.toLowerCase();
      return ACCEPTED_EXTENSIONS.includes(ext);
    });

    const combined = [...selectedFiles, ...validFiles].slice(0, MAX_FILES);
    selectedFiles = combined;
    error = null;
  }

  function removeFile(index: number) {
    selectedFiles = selectedFiles.filter((_, i) => i !== index);
  }

  function clearFiles() {
    selectedFiles = [];
  }

  function openFileBrowser() {
    fileInputRef?.click();
  }

  // --- Upload ---

  async function handleUpload() {
    if (selectedFiles.length === 0 || isLoading) return;

    isUploading = true;
    error = null;

    try {
      let result: ProtoSourceMeta;
      if (monitorId) {
        result = await uploadProtoSource(monitorId, selectedFiles);
      } else {
        // Ad-hoc: parse files without a saved monitor
        result = await adHocParseProto(selectedFiles);
      }
      selectedFiles = [];
      discoveredSource = result;
      onSourceChanged?.(result);
    } catch (err: unknown) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = t('proto.upload.unknownError');
      }
    } finally {
      isUploading = false;
    }
  }

  // --- Server Reflection ---

  async function handleReflection() {
    if (!reflectionEnabled || isLoading) return;

    isReflecting = true;
    error = null;

    try {
      // Always use ad-hoc reflect with current form values (target + tlsMode)
      // so the user doesn't need to save the monitor first.
      const result = await adHocReflect(target, tlsMode);
      discoveredSource = result;
      onSourceChanged?.(result);
    } catch (err: unknown) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = t('proto.upload.unknownError');
      }
    } finally {
      isReflecting = false;
    }
  }

  // --- Remove source ---

  async function handleRemove() {
    if (isLoading) return;

    isUploading = true;
    error = null;

    try {
      if (monitorId) {
        await deleteProtoSource(monitorId);
      }
      discoveredSource = null;
      onSourceChanged?.(null);
    } catch (err: unknown) {
      if (err instanceof Error) {
        error = err.message;
      } else {
        error = t('proto.upload.unknownError');
      }
    } finally {
      isUploading = false;
    }
  }

  // --- Replace source ---

  function handleReplace() {
    // Clear existing source display and show upload area
    discoveredSource = null;
    onSourceChanged?.(null);
  }

  // --- Method selection ---

  function handleMethodConfirmed(selection: ServiceMethodSelection) {
    discoveredSource = null;
    onMethodSelected?.(selection);
  }
</script>

<div class="space-y-4" data-testid="proto-source-upload">
  <!-- Error display -->
  {#if error}
    <div
      class="rounded-md border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300"
      role="alert"
      data-testid="proto-upload-error"
    >
      <p class="font-medium">{t('proto.upload.errorTitle')}</p>
      <p class="mt-1">{error}</p>
    </div>
  {/if}

  <!-- Current source display -->
  {#if currentSource}
    <div
      class="rounded-md border border-[var(--color-border)] bg-surface px-4 py-3"
      data-testid="proto-current-source"
    >
      <div class="flex items-start justify-between">
        <div class="space-y-1">
          <p class="text-sm font-medium text-primary">
            {t('proto.upload.currentSource')}
          </p>
          <p class="text-xs text-secondary">
            {t('proto.upload.sourceType')}: {currentSource.source_type === 'upload' ? t('proto.upload.sourceTypeUpload') : t('proto.upload.sourceTypeReflection')}
          </p>
          {#if currentSource.filenames.length > 0}
            <p class="text-xs text-secondary">
              {t('proto.upload.filenames')}: {currentSource.filenames.join(', ')}
            </p>
          {/if}
          <p class="text-xs text-secondary">
            {t('proto.upload.serviceCount', { count: currentSource.services.length })}
          </p>
        </div>
        <div class="flex gap-2">
          <button
            type="button"
            onclick={handleReplace}
            disabled={isLoading}
            class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50"
            data-testid="proto-replace-btn"
          >
            {t('proto.upload.replace')}
          </button>
          <button
            type="button"
            onclick={handleRemove}
            disabled={isLoading}
            class="rounded-md border border-rose-300 bg-surface px-3 py-1.5 text-xs font-medium text-rose-600 transition hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
            data-testid="proto-remove-btn"
          >
            {t('proto.upload.remove')}
          </button>
        </div>
      </div>
    </div>
  {:else}
    <!-- File drop zone -->
    <div
      class="relative rounded-lg border-2 border-dashed p-6 text-center transition-colors {isDragging
        ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/20'
        : 'border-[var(--color-border)] hover:border-blue-400'}"
      role="button"
      tabindex="0"
      aria-label={t('proto.upload.dropzoneLabel')}
      ondragenter={handleDragEnter}
      ondragover={handleDragOver}
      ondragleave={handleDragLeave}
      ondrop={handleDrop}
      onclick={openFileBrowser}
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') openFileBrowser(); }}
      data-testid="proto-dropzone"
    >
      <!-- Upload icon -->
      <svg
        class="mx-auto h-10 w-10 text-secondary"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="1.5"
        aria-hidden="true"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M12 16.5V9.75m0 0 3 3m-3-3-3 3M6.75 19.5a4.5 4.5 0 0 1-1.41-8.775 5.25 5.25 0 0 1 10.233-2.33 3 3 0 0 1 3.758 3.848A3.752 3.752 0 0 1 18 19.5H6.75Z"
        />
      </svg>

      <p class="mt-2 text-sm text-primary">{t('proto.upload.dropzone')}</p>
      <p class="mt-1 text-xs text-secondary">{t('proto.upload.dropzoneHint')}</p>

      <input
        bind:this={fileInputRef}
        type="file"
        accept=".proto,.desc"
        multiple
        class="hidden"
        onchange={handleFileInputChange}
        data-testid="proto-file-input"
      />
    </div>

    <!-- Selected files list -->
    {#if selectedFiles.length > 0}
      <div class="space-y-2" data-testid="proto-selected-files">
        <div class="flex items-center justify-between">
          <p class="text-sm font-medium text-primary">
            {t('proto.upload.selectedFiles', { count: selectedFiles.length })}
          </p>
          <button
            type="button"
            onclick={clearFiles}
            class="text-xs text-secondary hover:text-primary"
            data-testid="proto-clear-files"
          >
            {t('proto.upload.clearAll')}
          </button>
        </div>
        <ul class="space-y-1">
          {#each selectedFiles as file, index}
            <li class="flex items-center justify-between rounded-md border border-[var(--color-border)] px-3 py-1.5 text-sm">
              <span class="truncate text-primary">{file.name}</span>
              <button
                type="button"
                onclick={() => removeFile(index)}
                aria-label={t('proto.upload.removeFile', { name: file.name })}
                class="ml-2 text-secondary hover:text-rose-600"
                data-testid="proto-remove-file-{index}"
              >
                ✕
              </button>
            </li>
          {/each}
        </ul>

        <!-- Upload button -->
        <button
          type="button"
          onclick={handleUpload}
          disabled={isLoading}
          class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="proto-upload-btn"
        >
          {#if isUploading}
            <span class="inline-flex items-center gap-2">
              <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
              </svg>
              {t('proto.upload.uploading')}
            </span>
          {:else}
            {t('proto.upload.uploadButton')}
          {/if}
        </button>
      </div>
    {/if}

    <!-- Divider -->
    <div class="flex items-center gap-3">
      <div class="h-px flex-1 bg-[var(--color-border)]"></div>
      <span class="text-xs text-secondary">{t('proto.upload.or')}</span>
      <div class="h-px flex-1 bg-[var(--color-border)]"></div>
    </div>

    <!-- Server Reflection button -->
    <div>
      <button
        type="button"
        onclick={handleReflection}
        disabled={!reflectionEnabled || isLoading}
        class="w-full rounded-md border border-[var(--color-border)] bg-surface px-4 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="proto-reflection-btn"
      >
        {#if isReflecting}
          <span class="inline-flex items-center gap-2">
            <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
            </svg>
            {t('proto.upload.reflecting')}
          </span>
        {:else}
          {t('proto.upload.reflectionButton')}
        {/if}
      </button>
      {#if !reflectionEnabled}
        <p class="mt-1 text-xs text-secondary" data-testid="proto-reflection-hint">
          {t('proto.upload.reflectionHint')}
        </p>
      {/if}
    </div>
  {/if}

  <!-- Loading overlay (shown during any operation) -->
  {#if isLoading && currentSource}
    <div class="flex items-center justify-center py-2" data-testid="proto-loading">
      <svg class="h-5 w-5 animate-spin text-blue-600" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
      </svg>
      <span class="ml-2 text-sm text-secondary">{t('proto.upload.processing')}</span>
    </div>
  {/if}

  <!-- Service/Method selector (shown after successful upload or reflection) -->
  {#if showSelector && selectorSource}
    <ServiceMethodSelector
      services={selectorSource.services}
      onselect={handleMethodConfirmed}
    />
  {/if}
</div>
