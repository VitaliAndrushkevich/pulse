/**
 * Unit tests for PayloadEditor component.
 *
 * The PayloadEditor always renders a CodeMirror 6 JSON editor (no textarea fallback).
 * It provides Format/Minify buttons for JSON normalization.
 *
 * Requirements: 4.1, 4.6, 4.7
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import type { ProtoMessageSchema } from '$lib/types';

vi.mock('$lib/i18n', () => ({
  t: (key: string) => key,
}));

// Mock CodeMirror modules to avoid jsdom issues with EditorView
vi.mock('@codemirror/view', () => {
  const EditorView = vi.fn().mockImplementation(function (this: any, config: any) {
    this.state = { doc: { toString: () => config.state?.doc ?? '', length: config.state?.doc?.length ?? 0 } };
    this.dispatch = vi.fn();
    this.destroy = vi.fn();
    if (config.parent) {
      const el = document.createElement('div');
      el.setAttribute('data-testid', 'cm-mock-editor');
      config.parent.appendChild(el);
    }
    return this;
  });
  (EditorView as any).updateListener = { of: vi.fn(() => ({})) };
  (EditorView as any).theme = vi.fn(() => ({}));

  return {
    EditorView,
    placeholder: vi.fn(() => ({})),
    keymap: { of: vi.fn(() => ({})) },
    lineNumbers: vi.fn(() => ({})),
    highlightActiveLineGutter: vi.fn(() => ({})),
    highlightActiveLine: vi.fn(() => ({})),
  };
});

vi.mock('@codemirror/state', () => {
  const readOnlyField = { of: vi.fn(() => ({})) };
  const EditorState = {
    create: vi.fn((config: any) => ({ doc: config?.doc ?? '', extensions: config?.extensions ?? [] })),
    readOnly: readOnlyField,
  };
  const Compartment = vi.fn().mockImplementation(function (this: any) {
    this.of = vi.fn(() => ({}));
    this.reconfigure = vi.fn(() => ({}));
    return this;
  });
  return { EditorState, Compartment };
});

vi.mock('@codemirror/lang-json', () => ({
  json: vi.fn(() => ({})),
}));

vi.mock('@codemirror/commands', () => ({
  defaultKeymap: [],
  history: vi.fn(() => ({})),
  historyKeymap: [],
}));

vi.mock('@codemirror/language', () => ({
  bracketMatching: vi.fn(() => ({})),
  foldGutter: vi.fn(() => ({})),
  indentOnInput: vi.fn(() => ({})),
}));

vi.mock('@codemirror/autocomplete', () => ({
  closeBrackets: vi.fn(() => ({})),
  closeBracketsKeymap: [],
}));

vi.mock('@codemirror/search', () => ({
  highlightSelectionMatches: vi.fn(() => ({})),
}));

import PayloadEditor from './PayloadEditor.svelte';

describe('PayloadEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  describe('CodeMirror editor', () => {
    it('always renders CodeMirror container', () => {
      render(PayloadEditor, {
        props: { value: '' },
      });

      expect(screen.getByTestId('payload-editor-codemirror')).toBeTruthy();
    });

    it('renders CodeMirror container with schema prop', () => {
      const schema: ProtoMessageSchema = {
        full_name: 'test.Msg',
        fields: [{ name: 'id', json_name: 'id', type: 'string', repeated: false }],
      };

      render(PayloadEditor, {
        props: { schema, value: '{}' },
      });

      expect(screen.getByTestId('payload-editor-codemirror')).toBeTruthy();
    });

    it('renders CodeMirror container without schema (still JSON editor, no textarea)', () => {
      render(PayloadEditor, {
        props: { schema: null, value: '' },
      });

      expect(screen.getByTestId('payload-editor-codemirror')).toBeTruthy();
      // No textarea should exist
      expect(screen.queryByRole('textbox')).toBeNull();
    });
  });

  describe('toolbar', () => {
    it('renders Format button', () => {
      render(PayloadEditor, {
        props: { value: '' },
      });

      expect(screen.getByTestId('payload-editor-format')).toBeTruthy();
    });

    it('renders Minify button', () => {
      render(PayloadEditor, {
        props: { value: '' },
      });

      expect(screen.getByTestId('payload-editor-minify')).toBeTruthy();
    });

    it('Format and Minify buttons are disabled when disabled prop is true', () => {
      render(PayloadEditor, {
        props: { value: '', disabled: true },
      });

      const formatBtn = screen.getByTestId('payload-editor-format') as HTMLButtonElement;
      const minifyBtn = screen.getByTestId('payload-editor-minify') as HTMLButtonElement;
      expect(formatBtn.disabled).toBe(true);
      expect(minifyBtn.disabled).toBe(true);
    });

    it('Format and Minify buttons are enabled when disabled prop is false', () => {
      render(PayloadEditor, {
        props: { value: '', disabled: false },
      });

      const formatBtn = screen.getByTestId('payload-editor-format') as HTMLButtonElement;
      const minifyBtn = screen.getByTestId('payload-editor-minify') as HTMLButtonElement;
      expect(formatBtn.disabled).toBe(false);
      expect(minifyBtn.disabled).toBe(false);
    });
  });
});
