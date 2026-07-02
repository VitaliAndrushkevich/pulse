/**
 * Unit tests for ProtoSourceUpload component.
 *
 * Validates:
 * - File upload flow: drop zone, loading, success, error (Req 6.1, 6.2)
 * - Reflection flow: button enable/disable, success, error (Req 6.5, 6.6)
 * - Existing source display: info, replace, remove (Req 6.7, 6.8)
 * - Method selection: service tree, method click (Req 6.3, 6.4)
 *
 * Requirements: 6.1–6.10
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import type { ProtoSourceMeta } from '$lib/types';

vi.mock('$lib/i18n', () => ({
  t: (key: string, params?: Record<string, string | number>) => {
    let result = key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        result = result.replace(`{${k}}`, String(v));
      }
    }
    return result;
  },
}));

vi.mock('$lib/api', () => ({
  uploadProtoSource: vi.fn(),
  triggerReflection: vi.fn(),
  deleteProtoSource: vi.fn(),
  getProtoSource: vi.fn(),
  ApiRequestError: class extends Error {
    statusCode: number;
    apiError: any;
    requestId: string | null;
    constructor(s: number, e: any, r: string | null) {
      super(e?.message ?? '');
      this.statusCode = s;
      this.apiError = e;
      this.requestId = r;
    }
  },
}));

import ProtoSourceUpload from './ProtoSourceUpload.svelte';
import { uploadProtoSource, triggerReflection, deleteProtoSource } from '$lib/api';

const mockUpload = vi.mocked(uploadProtoSource);
const mockReflection = vi.mocked(triggerReflection);
const mockDelete = vi.mocked(deleteProtoSource);

function createMockSource(overrides: Partial<ProtoSourceMeta> = {}): ProtoSourceMeta {
  return {
    source_type: 'upload',
    filenames: ['service.proto'],
    services: [
      {
        full_name: 'mypackage.MyService',
        methods: [
          {
            name: 'GetItem',
            full_name: 'mypackage.MyService/GetItem',
            input_type: 'mypackage.GetItemRequest',
            output_type: 'mypackage.GetItemResponse',
          },
        ],
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    size_bytes: 1024,
    ...overrides,
  };
}

function defaultProps(overrides: Partial<{
  monitorId: string;
  target: string;
  currentSource: ProtoSourceMeta | null;
  onSourceChanged: (source: ProtoSourceMeta | null) => void;
  onMethodSelected: (selection: any) => void;
}> = {}) {
  return {
    monitorId: 'test-monitor-id',
    target: 'localhost:50051',
    currentSource: null,
    onSourceChanged: vi.fn(),
    onMethodSelected: vi.fn(),
    ...overrides,
  };
}

describe('ProtoSourceUpload', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  describe('file upload flow', () => {
    it('renders drop zone when no source configured', () => {
      render(ProtoSourceUpload, { props: defaultProps() });

      expect(screen.getByTestId('proto-dropzone')).toBeTruthy();
    });

    it('calls uploadProtoSource on file selection and upload', async () => {
      const mockSource = createMockSource();
      mockUpload.mockResolvedValue(mockSource);
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ onSourceChanged }),
      });

      // Simulate file selection via the hidden input
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['syntax = "proto3";'], 'service.proto', {
        type: 'application/octet-stream',
      });

      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      // Should see selected files and upload button
      await waitFor(() => {
        expect(screen.getByTestId('proto-selected-files')).toBeTruthy();
      });

      // Click upload button
      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      await waitFor(() => {
        expect(mockUpload).toHaveBeenCalledWith('test-monitor-id', [file]);
      });
    });

    it('shows loading indicator during upload', async () => {
      // Never resolves during test
      mockUpload.mockReturnValue(new Promise(() => {}));

      render(ProtoSourceUpload, { props: defaultProps() });

      // Add a file
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['content'], 'test.proto', { type: 'application/octet-stream' });
      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-btn')).toBeTruthy();
      });

      // Click upload
      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      // The button should show the uploading state (disabled)
      await waitFor(() => {
        const btn = screen.getByTestId('proto-upload-btn');
        expect(btn.hasAttribute('disabled')).toBe(true);
      });
    });

    it('calls onSourceChanged on successful upload', async () => {
      const mockSource = createMockSource();
      mockUpload.mockResolvedValue(mockSource);
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ onSourceChanged }),
      });

      // Add file and upload
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['content'], 'service.proto', { type: 'application/octet-stream' });
      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-btn')).toBeTruthy();
      });

      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      await waitFor(() => {
        expect(onSourceChanged).toHaveBeenCalledWith(mockSource);
      });
    });

    it('shows error message on upload failure', async () => {
      mockUpload.mockRejectedValue(new Error('Parse error: invalid proto syntax'));
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ onSourceChanged }),
      });

      // Add file and upload
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['invalid'], 'bad.proto', { type: 'application/octet-stream' });
      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-btn')).toBeTruthy();
      });

      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-error')).toBeTruthy();
        expect(screen.getByTestId('proto-upload-error').textContent).toContain(
          'Parse error: invalid proto syntax'
        );
      });

      expect(onSourceChanged).not.toHaveBeenCalled();
    });
  });

  describe('reflection flow', () => {
    it('reflection button is disabled when target is empty', () => {
      render(ProtoSourceUpload, {
        props: defaultProps({ target: '' }),
      });

      const reflectionBtn = screen.getByTestId('proto-reflection-btn');
      expect(reflectionBtn.hasAttribute('disabled')).toBe(true);
    });

    it('reflection button is enabled when target is set', () => {
      render(ProtoSourceUpload, {
        props: defaultProps({ target: 'localhost:50051' }),
      });

      const reflectionBtn = screen.getByTestId('proto-reflection-btn');
      expect(reflectionBtn.hasAttribute('disabled')).toBe(false);
    });

    it('shows reflection hint when target is empty', () => {
      render(ProtoSourceUpload, {
        props: defaultProps({ target: '' }),
      });

      expect(screen.getByTestId('proto-reflection-hint')).toBeTruthy();
    });

    it('calls triggerReflection on click', async () => {
      const mockSource = createMockSource({ source_type: 'reflection' });
      mockReflection.mockResolvedValue(mockSource);
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ onSourceChanged }),
      });

      await fireEvent.click(screen.getByTestId('proto-reflection-btn'));

      await waitFor(() => {
        expect(mockReflection).toHaveBeenCalledWith('test-monitor-id');
        expect(onSourceChanged).toHaveBeenCalledWith(mockSource);
      });
    });

    it('shows error on reflection failure', async () => {
      mockReflection.mockRejectedValue(new Error('Server does not support reflection'));

      render(ProtoSourceUpload, {
        props: defaultProps(),
      });

      await fireEvent.click(screen.getByTestId('proto-reflection-btn'));

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-error')).toBeTruthy();
        expect(screen.getByTestId('proto-upload-error').textContent).toContain(
          'Server does not support reflection'
        );
      });
    });
  });

  describe('existing source display', () => {
    it('shows source info when currentSource is provided', () => {
      const source = createMockSource();

      render(ProtoSourceUpload, {
        props: defaultProps({ currentSource: source }),
      });

      expect(screen.getByTestId('proto-current-source')).toBeTruthy();
      // Should not show the drop zone when a source exists
      expect(screen.queryByTestId('proto-dropzone')).toBeNull();
    });

    it('shows replace and remove buttons', () => {
      const source = createMockSource();

      render(ProtoSourceUpload, {
        props: defaultProps({ currentSource: source }),
      });

      expect(screen.getByTestId('proto-replace-btn')).toBeTruthy();
      expect(screen.getByTestId('proto-remove-btn')).toBeTruthy();
    });

    it('calls onSourceChanged with null on remove click', async () => {
      const source = createMockSource();
      mockDelete.mockResolvedValue(undefined);
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ currentSource: source, onSourceChanged }),
      });

      await fireEvent.click(screen.getByTestId('proto-remove-btn'));

      await waitFor(() => {
        expect(mockDelete).toHaveBeenCalledWith('test-monitor-id');
        expect(onSourceChanged).toHaveBeenCalledWith(null);
      });
    });

    it('replace button clears current source and shows upload area', async () => {
      const source = createMockSource();
      const onSourceChanged = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ currentSource: source, onSourceChanged }),
      });

      await fireEvent.click(screen.getByTestId('proto-replace-btn'));

      // onSourceChanged should be called with null (reset)
      expect(onSourceChanged).toHaveBeenCalledWith(null);
    });
  });

  describe('method selection', () => {
    it('shows method selector when services are available after upload', async () => {
      const mockSource = createMockSource();
      mockUpload.mockResolvedValue(mockSource);

      render(ProtoSourceUpload, {
        props: defaultProps(),
      });

      // Add file and upload
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['content'], 'service.proto', { type: 'application/octet-stream' });
      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-btn')).toBeTruthy();
      });

      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      await waitFor(() => {
        expect(screen.getByTestId('service-method-selector')).toBeTruthy();
      });
    });

    it('calls onMethodSelected when method is confirmed', async () => {
      const mockSource = createMockSource();
      mockUpload.mockResolvedValue(mockSource);
      const onMethodSelected = vi.fn();

      render(ProtoSourceUpload, {
        props: defaultProps({ onMethodSelected }),
      });

      // Add file and upload
      const fileInput = screen.getByTestId('proto-file-input') as HTMLInputElement;
      const file = new File(['content'], 'service.proto', { type: 'application/octet-stream' });
      Object.defineProperty(fileInput, 'files', { value: [file], writable: false });
      await fireEvent.change(fileInput);

      await waitFor(() => {
        expect(screen.getByTestId('proto-upload-btn')).toBeTruthy();
      });

      await fireEvent.click(screen.getByTestId('proto-upload-btn'));

      // Wait for the selector to appear
      await waitFor(() => {
        expect(screen.getByTestId('service-method-selector')).toBeTruthy();
      });

      // Single method should be auto-selected, click confirm
      await fireEvent.click(screen.getByTestId('service-method-confirm'));

      await waitFor(() => {
        expect(onMethodSelected).toHaveBeenCalledWith({
          service_name: 'mypackage.MyService',
          method_name: 'GetItem',
          full_method: 'mypackage.MyService/GetItem',
          input_type: 'mypackage.GetItemRequest',
          output_type: 'mypackage.GetItemResponse',
        });
      });
    });
  });
});
