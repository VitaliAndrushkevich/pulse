/**
 * Unit tests for the HistoryExplorer component.
 *
 * Validates:
 * - Loading skeleton display during fetch (Req 3.6)
 * - Error state rendering with retry button (Req 3.7)
 * - Empty state message display (Req 3.7, 6.7)
 * - Default "24 hours" selection on first open (Req 3.3)
 *
 * Requirements: 3.2, 3.3, 3.6, 3.7, 6.7
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import HistoryExplorer from './HistoryExplorer.svelte';
import type { HistoryResponseExtended } from '$lib/api';

// Mock the API module
vi.mock('$lib/api', () => ({
  getMonitorHistoryExtended: vi.fn(),
}));

// Mock uPlot (used by HistoryChartExplorer child component)
vi.mock('uplot', () => ({
  default: vi.fn().mockImplementation(() => ({
    destroy: vi.fn(),
    setSize: vi.fn(),
    setScale: vi.fn(),
    setSelect: vi.fn(),
    cursor: { idx: null, left: 0, top: 0 },
    bbox: { left: 0, top: 0, width: 600, height: 250 },
    data: [],
    select: { left: 0, width: 0, top: 0, height: 0 },
    valToPos: vi.fn(() => 0),
    posToVal: vi.fn(() => 0),
    ctx: { save: vi.fn(), restore: vi.fn(), fillRect: vi.fn(), fillStyle: '' },
  })),
}));
vi.mock('uplot/dist/uPlot.min.css', () => ({}));

// Mock ResizeObserver (not available in jsdom)
const mockResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
vi.stubGlobal('ResizeObserver', mockResizeObserver);

import { getMonitorHistoryExtended } from '$lib/api';
const mockGetHistory = vi.mocked(getMonitorHistoryExtended);

function defaultProps() {
  return {
    monitorId: 'test-monitor-123',
    retentionDays: 30,
  };
}

function mockSuccessResponse(points: any[] = [], aggregatedPoints: any[] = []): HistoryResponseExtended {
  return {
    monitor_id: 'test-monitor-123',
    from: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    to: new Date().toISOString(),
    points,
    aggregated_points: aggregatedPoints,
    step: undefined,
    truncated: false,
  };
}

describe('HistoryExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('displays loading skeleton while data is being fetched', async () => {
      // Never resolve the promise so loading stays active
      mockGetHistory.mockReturnValue(new Promise(() => {}));

      render(HistoryExplorer, { props: defaultProps() });

      expect(screen.getByTestId('history-loading-skeleton')).toBeTruthy();
    });

    it('hides loading skeleton once data arrives', async () => {
      mockGetHistory.mockResolvedValue(mockSuccessResponse([
        { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() }
      ]));

      render(HistoryExplorer, { props: defaultProps() });

      await waitFor(() => {
        expect(screen.queryByTestId('history-loading-skeleton')).toBeNull();
      });
    });
  });

  describe('error state', () => {
    it('displays error message and retry button on fetch failure', async () => {
      mockGetHistory.mockRejectedValue(new Error('Network error'));

      render(HistoryExplorer, { props: defaultProps() });

      await waitFor(() => {
        expect(screen.getByTestId('history-error')).toBeTruthy();
      });

      expect(screen.getByTestId('history-error').textContent).toContain('Network error');
      // Retry button should be present
      const retryButton = screen.getByTestId('history-error').querySelector('button');
      expect(retryButton).toBeTruthy();
      expect(retryButton!.textContent).toContain('Retry');
    });

    it('retries fetch when retry button is clicked', async () => {
      mockGetHistory.mockRejectedValueOnce(new Error('Network error'));
      mockGetHistory.mockResolvedValueOnce(mockSuccessResponse([
        { state: 'up', latency_ms: 50, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() }
      ]));

      render(HistoryExplorer, { props: defaultProps() });

      await waitFor(() => {
        expect(screen.getByTestId('history-error')).toBeTruthy();
      });

      const retryButton = screen.getByTestId('history-error').querySelector('button')!;
      await fireEvent.click(retryButton);

      await waitFor(() => {
        expect(screen.queryByTestId('history-error')).toBeNull();
      });

      expect(mockGetHistory).toHaveBeenCalledTimes(2);
    });
  });

  describe('empty state', () => {
    it('displays empty state message when no data points returned', async () => {
      mockGetHistory.mockResolvedValue(mockSuccessResponse([], []));

      render(HistoryExplorer, { props: defaultProps() });

      await waitFor(() => {
        expect(screen.getByTestId('history-empty')).toBeTruthy();
      });

      expect(screen.getByTestId('history-empty').textContent).toContain('No data available for the selected period');
    });
  });

  describe('default selection', () => {
    it('defaults to 24h and passes selected="24h" to TimeRangePicker', async () => {
      mockGetHistory.mockResolvedValue(mockSuccessResponse([
        { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() }
      ]));

      render(HistoryExplorer, { props: defaultProps() });

      // The "24h" preset button should be aria-pressed=true (passed via selected prop)
      await waitFor(() => {
        expect(screen.getByTestId('preset-24h').getAttribute('aria-pressed')).toBe('true');
      });
    });

    it('fetches history data with a 24h range on initial render', async () => {
      mockGetHistory.mockResolvedValue(mockSuccessResponse());

      const before = Date.now();
      render(HistoryExplorer, { props: defaultProps() });
      const after = Date.now();

      await waitFor(() => {
        expect(mockGetHistory).toHaveBeenCalled();
      });

      const [id, from, to] = mockGetHistory.mock.calls[0];
      expect(id).toBe('test-monitor-123');

      const fromMs = new Date(from).getTime();
      const toMs = new Date(to).getTime();
      const durationMs = toMs - fromMs;

      // Should be approximately 24 hours
      expect(durationMs).toBe(24 * 60 * 60 * 1000);
      // "to" should be close to render time
      expect(toMs).toBeGreaterThanOrEqual(before - 1000);
      expect(toMs).toBeLessThanOrEqual(after + 1000);
    });
  });

  describe('truncated notice', () => {
    it('shows truncated notice when API returns truncated: true', async () => {
      mockGetHistory.mockResolvedValue({
        ...mockSuccessResponse([
          { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() }
        ]),
        truncated: true,
      });

      render(HistoryExplorer, { props: defaultProps() });

      await waitFor(() => {
        expect(screen.getByTestId('truncated-notice')).toBeTruthy();
      });
    });
  });
});
