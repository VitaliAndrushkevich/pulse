<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, placeholder as cmPlaceholder, keymap } from '@codemirror/view';
  import { EditorState, Compartment } from '@codemirror/state';
  import { json } from '@codemirror/lang-json';
  import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
  import { bracketMatching, foldGutter, indentOnInput } from '@codemirror/language';
  import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
  import { lineNumbers, highlightActiveLineGutter, highlightActiveLine } from '@codemirror/view';
  import { highlightSelectionMatches } from '@codemirror/search';
  import { t } from '$lib/i18n';
  import type { ProtoMessageSchema } from '$lib/types';

  interface Props {
    value?: string;
    schema?: ProtoMessageSchema | null;
    placeholder?: string;
    disabled?: boolean;
    onchange?: (value: string) => void;
  }

  let {
    value = $bindable(''),
    schema = null,
    placeholder = '',
    disabled = false,
    onchange,
  }: Props = $props();

  let editorContainer: HTMLDivElement | undefined = $state();
  let editorView: EditorView | null = null;
  let isUpdatingFromProp = false;
  let formatError = $state<string | null>(null);

  const readOnlyCompartment = new Compartment();

  function createExtensions() {
    const placeholderText = placeholder || t('payloadEditor.placeholder');
    return [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightActiveLine(),
      history(),
      foldGutter(),
      indentOnInput(),
      bracketMatching(),
      closeBrackets(),
      highlightSelectionMatches(),
      json(),
      cmPlaceholder(placeholderText),
      readOnlyCompartment.of(EditorState.readOnly.of(disabled)),
      keymap.of([
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...historyKeymap,
      ]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !isUpdatingFromProp) {
          const newValue = update.state.doc.toString();
          value = newValue;
          onchange?.(newValue);
          formatError = null;
        }
      }),
      EditorView.theme({
        '&': {
          fontSize: '14px',
          border: '1px solid var(--color-border)',
          borderRadius: '0.375rem',
          backgroundColor: 'var(--color-bg-surface)',
          minHeight: '160px',
        },
        '.cm-scroller': {
          minHeight: '160px',
        },
        '.cm-content': {
          fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
          padding: '8px 0',
          caretColor: 'var(--color-text-primary)',
          color: 'var(--color-text-primary)',
          minHeight: '160px',
        },
        '.cm-gutters': {
          backgroundColor: 'var(--color-bg-page)',
          borderRight: '1px solid var(--color-border)',
          color: 'var(--color-text-muted)',
          minHeight: '160px',
        },
        '.cm-activeLineGutter': {
          backgroundColor: 'var(--color-bg-surface-hover)',
        },
        '.cm-activeLine': {
          backgroundColor: 'var(--color-bg-surface-hover)',
        },
        '.cm-cursor': {
          borderLeftColor: 'var(--color-text-primary)',
        },
        '&.cm-focused': {
          outline: '2px solid var(--color-brand-primary)',
          outlineOffset: '-1px',
        },
        '.cm-placeholder': {
          color: 'var(--color-text-muted)',
        },
      }),
    ];
  }

  onMount(() => {
    if (!editorContainer) return;

    editorView = new EditorView({
      state: EditorState.create({
        doc: value,
        extensions: createExtensions(),
      }),
      parent: editorContainer,
    });
  });

  onDestroy(() => {
    if (editorView) {
      editorView.destroy();
      editorView = null;
    }
  });

  // Sync external value prop changes into the editor
  $effect(() => {
    if (!editorView) return;
    const currentDoc = editorView.state.doc.toString();
    if (value !== currentDoc) {
      isUpdatingFromProp = true;
      editorView.dispatch({
        changes: {
          from: 0,
          to: editorView.state.doc.length,
          insert: value,
        },
      });
      isUpdatingFromProp = false;
    }
  });

  // Update readonly state when disabled prop changes
  $effect(() => {
    if (!editorView) return;
    editorView.dispatch({
      effects: readOnlyCompartment.reconfigure(EditorState.readOnly.of(disabled)),
    });
  });

  /**
   * Format JSON: parse + re-serialize with 2-space indentation.
   */
  function handleFormat() {
    if (!editorView) return;
    const doc = editorView.state.doc.toString().trim();
    if (!doc) return;

    try {
      const parsed = JSON.parse(doc);
      const formatted = JSON.stringify(parsed, null, 2);
      formatError = null;

      isUpdatingFromProp = true;
      editorView.dispatch({
        changes: {
          from: 0,
          to: editorView.state.doc.length,
          insert: formatted,
        },
      });
      isUpdatingFromProp = false;

      value = formatted;
      onchange?.(formatted);
    } catch (e) {
      formatError = e instanceof Error ? e.message : 'Invalid JSON';
    }
  }

  /**
   * Minify JSON: parse + re-serialize without whitespace.
   */
  function handleMinify() {
    if (!editorView) return;
    const doc = editorView.state.doc.toString().trim();
    if (!doc) return;

    try {
      const parsed = JSON.parse(doc);
      const minified = JSON.stringify(parsed);
      formatError = null;

      isUpdatingFromProp = true;
      editorView.dispatch({
        changes: {
          from: 0,
          to: editorView.state.doc.length,
          insert: minified,
        },
      });
      isUpdatingFromProp = false;

      value = minified;
      onchange?.(minified);
    } catch (e) {
      formatError = e instanceof Error ? e.message : 'Invalid JSON';
    }
  }
</script>

<div class="payload-editor" data-testid="payload-editor">
  <!-- Toolbar -->
  <div class="mb-1 flex items-center justify-between">
    <div class="flex gap-2">
      <button
        type="button"
        onclick={handleFormat}
        disabled={disabled}
        class="rounded border border-[var(--color-border)] bg-surface px-2 py-1 text-xs font-medium text-secondary transition hover:bg-[var(--color-bg-surface-hover)] hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="payload-editor-format"
      >
        {t('payloadEditor.format')}
      </button>
      <button
        type="button"
        onclick={handleMinify}
        disabled={disabled}
        class="rounded border border-[var(--color-border)] bg-surface px-2 py-1 text-xs font-medium text-secondary transition hover:bg-[var(--color-bg-surface-hover)] hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="payload-editor-minify"
      >
        {t('payloadEditor.minify')}
      </button>
    </div>
    {#if formatError}
      <span class="text-xs text-rose-600" data-testid="payload-editor-format-error">{formatError}</span>
    {/if}
  </div>

  <!-- CodeMirror 6 JSON editor — always shown -->
  <div
    bind:this={editorContainer}
    class="min-h-[160px] overflow-hidden rounded-md"
    data-testid="payload-editor-codemirror"
  ></div>
</div>
