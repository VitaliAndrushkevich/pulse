/**
 * Unit tests for the HistoryChartExplorer component.
 *
 * Validates:
 * - Loading skeleton display (Req 6.6)
 * - Empty state message (Req 6.7)
 * - Zoom reset button visibility (Req 6.5)
 *
 * Requirements: 6.4, 6.5, 6.6, 6.7
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import HistoryChartExplorer from './HistoryChartExplorer.svelte';

// Mock ResizeObserver (not available in jsdom)
const mockResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
vi.stubGlobal('ResizeObserver', mockResizeObserver);

// Mock uPlot — it requires real canvas/DOM with dimensions
vi.mock('uplot', () => {
  return {
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
      ctx: {
        save: vi.fn(),
        restore: vi.fn(),
        fillRect: vi.fn(),
        fillStyle: '',
      },
    })),
  };
});

// Mock uPlot CSS import
vi.mock('uplot/dist/uPlot.min.css', () => ({}));

describe('HistoryChartExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  describe('loading skeleton', () => {
    it('displays a 250px loading skeleton when loading=true', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [],
          aggregatedPoints: [],
          loading: true,
        },
      });

      const skeleton = screen.getByLabelText('Loading chart data');
      expect(skeleton).toBeTruthy();
      expect(skeleton.classList.contains('h-[250px]')).toBe(true);
      expect(skeleton.classList.contains('animate-pulse')).toBe(true);
    });

    it('does not display skeleton when loading=false', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [],
          aggregatedPoints: [],
          loading: false,
        },
      });

      expect(screen.queryByLabelText('Loading chart data')).toBeNull();
    });
  });

  describe('empty state', () => {
    it('displays "No data available" when no points and not loading', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [],
          aggregatedPoints: [],
          loading: false,
        },
      });

      expect(screen.getByText('No data available')).toBeTruthy();
    });

    it('does not display empty state when points exist', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [
            { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() },
            { state: 'up', latency_ms: 120, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date(Date.now() - 60000).toISOString() },
          ],
          aggregatedPoints: [],
          loading: false,
        },
      });

      expect(screen.queryByText('No data available')).toBeNull();
    });

    it('does not display empty state when aggregatedPoints exist', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [],
          aggregatedPoints: [
            { timestamp: new Date().toISOString(), min_latency_ms: 50, max_latency_ms: 200, avg_latency_ms: 100, check_count: 10, uptime_ratio: 0.95 },
            { timestamp: new Date(Date.now() - 60000).toISOString(), min_latency_ms: 40, max_latency_ms: 180, avg_latency_ms: 90, check_count: 10, uptime_ratio: 1.0 },
          ],
          loading: false,
        },
      });

      expect(screen.queryByText('No data available')).toBeNull();
    });
  });

  describe('zoom reset button', () => {
    it('does not show reset zoom button when not zoomed (initial state)', () => {
      render(HistoryChartExplorer, {
        props: {
          points: [
            { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date().toISOString() },
            { state: 'up', latency_ms: 120, status_code: 200, error: null, ssl_days_remaining: null, checked_at: new Date(Date.now() - 60000).toISOString() },
          ],
          aggregatedPoints: [],
          loading: false,
        },
      });

      expect(screen.queryByText('Reset zoom')).toBeNull();
    });
  });
});
