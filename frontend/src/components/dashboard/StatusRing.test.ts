import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import StatusRing from './StatusRing.svelte';
import { computeArcAngles, buildAriaDescription } from './StatusRing.svelte';
import type { StatusDistribution } from '$lib/types';

vi.mock('$lib/i18n', () => ({
  t: (key: string, params?: Record<string, string | number>) => {
    const translations: Record<string, string> = {
      'common.loading': 'Loading...',
      'common.retry': 'Retry',
      'dashboard.statusRing.title': 'Status Distribution',
      'dashboard.statusRing.ariaLabel': `${params?.up ?? 0} up, ${params?.down ?? 0} down, ${params?.unknown ?? 0} unknown`,
    };
    return translations[key] ?? key;
  },
}));

describe('computeArcAngles', () => {
  it('returns zero angles when total is 0', () => {
    const dist: StatusDistribution = { up: 0, down: 0, unknown: 0, total: 0 };
    const result = computeArcAngles(dist);
    expect(result).toEqual([
      { state: 'up', count: 0, angle: 0 },
      { state: 'down', count: 0, angle: 0 },
      { state: 'unknown', count: 0, angle: 0 },
    ]);
  });

  it('computes proportional angles that sum to 360', () => {
    const dist: StatusDistribution = { up: 8, down: 1, unknown: 1, total: 10 };
    const result = computeArcAngles(dist);
    expect(result[0]).toEqual({ state: 'up', count: 8, angle: 288 });
    expect(result[1]).toEqual({ state: 'down', count: 1, angle: 36 });
    expect(result[2]).toEqual({ state: 'unknown', count: 1, angle: 36 });
    const totalAngle = result.reduce((sum, s) => sum + s.angle, 0);
    expect(totalAngle).toBe(360);
  });

  it('handles all monitors in one state (complete ring)', () => {
    const dist: StatusDistribution = { up: 5, down: 0, unknown: 0, total: 5 };
    const result = computeArcAngles(dist);
    expect(result[0]).toEqual({ state: 'up', count: 5, angle: 360 });
    expect(result[1]).toEqual({ state: 'down', count: 0, angle: 0 });
    expect(result[2]).toEqual({ state: 'unknown', count: 0, angle: 0 });
  });
});

describe('buildAriaDescription', () => {
  it('describes all non-zero states', () => {
    const dist: StatusDistribution = { up: 8, down: 1, unknown: 1, total: 10 };
    expect(buildAriaDescription(dist)).toBe('8 up, 1 down, 1 unknown');
  });

  it('omits zero-count states', () => {
    const dist: StatusDistribution = { up: 5, down: 0, unknown: 0, total: 5 };
    expect(buildAriaDescription(dist)).toBe('5 up');
  });

  it('handles only down monitors', () => {
    const dist: StatusDistribution = { up: 0, down: 3, unknown: 0, total: 3 };
    expect(buildAriaDescription(dist)).toBe('3 down');
  });

  it('returns empty string when all counts are zero', () => {
    const dist: StatusDistribution = { up: 0, down: 0, unknown: 0, total: 0 };
    expect(buildAriaDescription(dist)).toBe('');
  });
});

describe('StatusRing component', () => {
  it('renders loading state via WidgetShell', () => {
    render(StatusRing, {
      props: { data: null, loading: true, error: null, onRetry: null },
    });
    expect(screen.getByTestId('widget-skeleton')).toBeTruthy();
  });

  it('renders error state via WidgetShell', () => {
    render(StatusRing, {
      props: { data: null, loading: false, error: 'Fetch failed', onRetry: vi.fn() },
    });
    expect(screen.getByTestId('widget-error')).toBeTruthy();
    expect(screen.getByText('Fetch failed')).toBeTruthy();
  });

  it('renders empty ring when total is 0', () => {
    const data: StatusDistribution = { up: 0, down: 0, unknown: 0, total: 0 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    const svg = screen.getByTestId('status-ring-svg');
    expect(svg).toBeTruthy();
    // Should display "0" in center
    expect(screen.getByTestId('status-ring-total').textContent).toBe('0');
  });

  it('renders total count in center', () => {
    const data: StatusDistribution = { up: 8, down: 1, unknown: 1, total: 10 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    expect(screen.getByTestId('status-ring-total').textContent).toBe('10');
  });

  it('renders segments for non-zero states', () => {
    const data: StatusDistribution = { up: 8, down: 1, unknown: 1, total: 10 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    expect(screen.getByTestId('status-ring-segment-up')).toBeTruthy();
    expect(screen.getByTestId('status-ring-segment-down')).toBeTruthy();
    expect(screen.getByTestId('status-ring-segment-unknown')).toBeTruthy();
  });

  it('does not render segments for zero-count states', () => {
    const data: StatusDistribution = { up: 5, down: 0, unknown: 0, total: 5 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    expect(screen.getByTestId('status-ring-segment-up')).toBeTruthy();
    expect(screen.queryByTestId('status-ring-segment-down')).toBeNull();
    expect(screen.queryByTestId('status-ring-segment-unknown')).toBeNull();
  });

  it('has accessible aria-label on SVG with state descriptions', () => {
    const data: StatusDistribution = { up: 8, down: 1, unknown: 1, total: 10 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    const svg = screen.getByTestId('status-ring-svg');
    expect(svg.getAttribute('aria-label')).toBe('8 up, 1 down, 1 unknown');
  });

  it('SVG has role="img" for accessibility', () => {
    const data: StatusDistribution = { up: 5, down: 0, unknown: 0, total: 5 };
    render(StatusRing, {
      props: { data, loading: false, error: null, onRetry: null },
    });
    const svg = screen.getByTestId('status-ring-svg');
    expect(svg.getAttribute('role')).toBe('img');
  });
});
