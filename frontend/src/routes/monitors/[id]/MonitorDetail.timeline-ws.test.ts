/**
 * Bug Condition Exploration Test: WebSocket Messages Not Appended to Timeline
 *
 * **Property 3: Bug Condition** — WebSocket Messages Not Appended to Timeline
 *
 * This test MUST FAIL on unfixed code — failure confirms the bug exists.
 * The monitor detail page loads history once on mount via getMonitorHistory().
 * When a WebSocket `monitor_status` message arrives for the currently viewed monitor,
 * the `history` array that feeds StatusTimeline is NOT updated.
 *
 * **Validates: Requirements 1.4, 1.5, 2.4**
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import fc from 'fast-check';
import type { HistoryPoint, MonitorPatch, Monitor } from '$lib/types';
import { patchBus } from '$lib/stores/patchBus.svelte';

// ---------------------------------------------------------------------------
// DOM Stubs for uPlot (HistoryChart component uses canvas)
// ---------------------------------------------------------------------------

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn()
  }))
});

// Full canvas context mock for uPlot
HTMLCanvasElement.prototype.getContext = vi.fn().mockReturnValue({
  clearRect: vi.fn(),
  fillRect: vi.fn(),
  strokeRect: vi.fn(),
  getImageData: vi.fn(() => ({ data: [] })),
  putImageData: vi.fn(),
  createImageData: vi.fn(() => []),
  setTransform: vi.fn(),
  resetTransform: vi.fn(),
  drawImage: vi.fn(),
  save: vi.fn(),
  fillText: vi.fn(),
  strokeText: vi.fn(),
  restore: vi.fn(),
  beginPath: vi.fn(),
  moveTo: vi.fn(),
  lineTo: vi.fn(),
  closePath: vi.fn(),
  stroke: vi.fn(),
  translate: vi.fn(),
  scale: vi.fn(),
  rotate: vi.fn(),
  arc: vi.fn(),
  fill: vi.fn(),
  measureText: vi.fn(() => ({ width: 0 })),
  transform: vi.fn(),
  rect: vi.fn(),
  clip: vi.fn(),
  setLineDash: vi.fn(),
  getLineDash: vi.fn(() => []),
  createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
  createRadialGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
  createPattern: vi.fn(),
  font: '',
  textAlign: 'left',
  textBaseline: 'top',
  strokeStyle: '',
  fillStyle: '',
  lineWidth: 1,
  lineCap: 'butt',
  lineJoin: 'miter',
  miterLimit: 10,
  globalAlpha: 1,
  globalCompositeOperation: 'source-over',
  shadowBlur: 0,
  shadowColor: 'rgba(0, 0, 0, 0)',
  shadowOffsetX: 0,
  shadowOffsetY: 0,
  lineDashOffset: 0,
  canvas: { width: 800, height: 400 }
}) as unknown as typeof HTMLCanvasElement.prototype.getContext;

// Stub ResizeObserver (used by uPlot)
global.ResizeObserver = class {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
};

// Stub Path2D (used by uPlot for drawing paths)
global.Path2D = class {
  moveTo = vi.fn();
  lineTo = vi.fn();
  closePath = vi.fn();
  arc = vi.fn();
  rect = vi.fn();
  addPath = vi.fn();
} as unknown as typeof Path2D;

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Mock i18n to avoid $effect outside component context
vi.mock('$lib/i18n', () => ({
  t: (key: string) => key,
}));

vi.mock('$app/stores', () => {
  const { readable } = require('svelte/store');
  return {
    page: readable({
      url: new URL('http://localhost/monitors/test-monitor-123'),
      params: { id: 'test-monitor-123' },
      route: { id: '/monitors/[id]' },
      status: 200,
      error: null,
      data: {},
      form: null
    }),
    navigating: readable(null)
  };
});

// Use recent timestamps relative to Date.now() so the 24h pruning window doesn't discard them
const now = Date.now();
const recentTime = (minutesAgo: number) => new Date(now - minutesAgo * 60 * 1000).toISOString();

const mockMonitor: Monitor = {
  id: 'test-monitor-123',
  name: 'Test Monitor',
  type: 'http',
  target: 'https://example.com',
  interval_seconds: 60,
  timeout_seconds: 10,
  status: 'active',
  state: 'up',
  last_checked_at: recentTime(1),
  next_check_at: new Date(now + 60 * 1000).toISOString(),
  settings: {},
  tags: [],
  history_retention_days: 30,
  created_at: recentTime(120),
  updated_at: recentTime(1)
};

const initialHistory: HistoryPoint[] = [
  { state: 'up', latency_ms: 100, status_code: 200, error: null, ssl_days_remaining: null, checked_at: recentTime(60) },
  { state: 'up', latency_ms: 120, status_code: 200, error: null, ssl_days_remaining: null, checked_at: recentTime(30) },
  { state: 'up', latency_ms: 95, status_code: 200, error: null, ssl_days_remaining: null, checked_at: recentTime(5) }
];

vi.mock('$lib/api', () => ({
  getMonitor: vi.fn(() => Promise.resolve(mockMonitor)),
  getMonitorHistory: vi.fn(() => Promise.resolve([...initialHistory])),
  getMonitorIncidents: vi.fn(() => Promise.resolve({ data: [], total: 0, page: 1, limit: 20, total_pages: 0 })),
  getMonitorStats: vi.fn(() => Promise.resolve(null)),
  deleteMonitor: vi.fn(),
  ApiRequestError: class extends Error {
    statusCode: number;
    constructor(code: number, body: unknown, reqId: string | null) {
      super('API Error');
      this.statusCode = code;
    }
  }
}));

const mockApplyPatch = vi.fn();
vi.mock('$lib/stores/monitors.svelte', () => ({
  monitorStore: {
    getById: vi.fn(() => mockMonitor),
    updateMonitor: vi.fn(),
    applyPatch: (...args: unknown[]) => mockApplyPatch(...args),
    setMonitors: vi.fn(),
    list: [],
    totalCount: 0,
    healthyCount: 0,
    unhealthyCount: 0,
    removeMonitor: vi.fn(),
    clear: vi.fn()
  }
}));

vi.mock('$lib/stores/auth.svelte', () => ({
  getToken: vi.fn(() => 'test-token'),
  setToken: vi.fn(),
  clearToken: vi.fn(),
  isAuthenticated: vi.fn(() => true)
}));

vi.mock('$lib/stores/connection.svelte', () => ({
  connectionStore: {
    setStatus: vi.fn(),
    status: 'connected'
  }
}));

vi.mock('$lib/stores/toast.svelte', () => ({
  toastStore: {
    addToast: vi.fn(),
    dismissToast: vi.fn()
  }
}));

// ---------------------------------------------------------------------------
// Arbitraries
// ---------------------------------------------------------------------------

/**
 * Generate arbitrary MonitorPatch payloads for the current monitor.
 * These represent incoming WebSocket `monitor_status` messages.
 */
const arbMonitorPatch: fc.Arbitrary<MonitorPatch> = fc.record({
  monitor_id: fc.constant('test-monitor-123'),
  state: fc.oneof(
    fc.constant('up' as const),
    fc.constant('down' as const),
    fc.constant('unknown' as const)
  ),
  latency_ms: fc.integer({ min: 1, max: 30000 }),
  status_code: fc.option(fc.integer({ min: 100, max: 599 }), { nil: undefined }),
  error: fc.option(
    fc.oneof(
      fc.constant('connection refused'),
      fc.constant('timeout after 10s'),
      fc.constant('DNS resolution failed'),
      fc.constant('TLS handshake error'),
      fc.stringOf(fc.char(), { minLength: 1, maxLength: 50 })
    ),
    { nil: undefined }
  ),
  // Use recent timestamps (within last few minutes) so 24h pruning doesn't discard them
  checked_at: fc.integer({ min: 1, max: 240 }).map(secsAgo =>
    new Date(now - secsAgo * 1000).toISOString()
  ),
  timestamp: fc.integer({ min: 1, max: 240 }).map(secsAgo =>
    new Date(now - secsAgo * 1000).toISOString()
  )
}) as fc.Arbitrary<MonitorPatch>;

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Bug Condition: WebSocket Messages Not Appended to Timeline', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  /**
   * Property 3: Bug Condition — WebSocket Messages Not Appended to Timeline
   *
   * For any valid MonitorPatch payload targeting the currently viewed monitor:
   * After the WS `monitor_status` message arrives, the StatusTimeline's data
   * should increase by 1 and the last point should match the WS payload.
   *
   * This test is EXPECTED TO FAIL on unfixed code because the detail page
   * does not subscribe to WS messages to update the history/timeline.
   *
   * **Validates: Requirements 1.4, 1.5, 2.4**
   */
  it('Property 3: timeline data updates when WS monitor_status message arrives for current monitor', async () => {
    const MonitorDetailPage = (await import('./+page.svelte')).default;

    // Test one representative case directly (scoped PBT approach)
    // We use fc.assert with a small number of runs since each involves DOM rendering
    await fc.assert(
      fc.asyncProperty(arbMonitorPatch, async (patch) => {
        cleanup(); // Clean up any previous render

        // Render the detail page
        const { unmount } = render(MonitorDetailPage);

        // Wait for initial data load to complete
        await waitFor(() => {
          const sections = screen.getAllByTestId('status-timeline-section');
          expect(sections.length).toBeGreaterThan(0);
        });

        // Verify initial state: timeline shows "3 checks"
        const timelineEl = screen.getAllByTestId('status-timeline')[0];
        expect(timelineEl.textContent).toContain('3 checks');

        // Simulate WS monitor_status message arriving for the current monitor.
        // In the real app, the WS client dispatches to monitorStore.applyPatch() AND
        // publishes to patchBus. The detail page subscribes to patchBus and appends
        // a new HistoryPoint to the local `history` array.
        mockApplyPatch(patch);
        patchBus.publish(patch);

        // Allow any reactive effects to settle
        await new Promise(r => setTimeout(r, 50));

        // PROPERTY ASSERTION:
        // After a WS monitor_status message for the current monitor, the timeline
        // should now show N+1 checks (4 instead of 3).
        // On UNFIXED code this will FAIL because the page doesn't update history.
        const updatedTimeline = screen.getAllByTestId('status-timeline')[0];
        expect(updatedTimeline.textContent).toContain('4 checks');

        unmount();
      }),
      {
        numRuns: 5,
        seed: 42,
        verbose: 1
      }
    );
  });
});
