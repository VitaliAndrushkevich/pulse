import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import HealthScore, { getHealthScoreColor } from './HealthScore.svelte';

describe('HealthScore', () => {
  describe('getHealthScoreColor', () => {
    it('returns success color for values >= 99', () => {
      expect(getHealthScoreColor(99)).toBe('var(--color-success)');
      expect(getHealthScoreColor(99.5)).toBe('var(--color-success)');
      expect(getHealthScoreColor(100)).toBe('var(--color-success)');
    });

    it('returns warning color for values >= 95 and < 99', () => {
      expect(getHealthScoreColor(95)).toBe('var(--color-warning)');
      expect(getHealthScoreColor(97.5)).toBe('var(--color-warning)');
      expect(getHealthScoreColor(98.99)).toBe('var(--color-warning)');
    });

    it('returns error color for values < 95', () => {
      expect(getHealthScoreColor(94.99)).toBe('var(--color-error)');
      expect(getHealthScoreColor(50)).toBe('var(--color-error)');
      expect(getHealthScoreColor(0)).toBe('var(--color-error)');
    });
  });

  describe('rendering', () => {
    it('shows empty state when data is null', () => {
      render(HealthScore, {
        props: { data: null, loading: false, error: null, onRetry: null }
      });

      const scoreEl = screen.getByTestId('health-score');
      expect(scoreEl.textContent).toContain('—');
    });

    it('shows empty state when active_monitor_count is 0', () => {
      render(HealthScore, {
        props: {
          data: { uptime_percent: 0, active_monitor_count: 0, partial_data: false },
          loading: false,
          error: null,
          onRetry: null
        }
      });

      const scoreEl = screen.getByTestId('health-score');
      expect(scoreEl.textContent).toContain('—');
    });

    it('displays formatted percentage when data is present', () => {
      render(HealthScore, {
        props: {
          data: { uptime_percent: 99.95, active_monitor_count: 47, partial_data: false },
          loading: false,
          error: null,
          onRetry: null
        }
      });

      const scoreEl = screen.getByTestId('health-score');
      expect(scoreEl.textContent).toContain('99.95%');
    });

    it('formats percentage to exactly 2 decimal places', () => {
      render(HealthScore, {
        props: {
          data: { uptime_percent: 100, active_monitor_count: 10, partial_data: false },
          loading: false,
          error: null,
          onRetry: null
        }
      });

      const scoreEl = screen.getByTestId('health-score');
      expect(scoreEl.textContent).toContain('100.00%');
    });

    it('shows partial data warning when partial_data is true', () => {
      render(HealthScore, {
        props: {
          data: { uptime_percent: 98.5, active_monitor_count: 47, partial_data: true },
          loading: false,
          error: null,
          onRetry: null
        }
      });

      const warning = screen.getByTestId('health-score-partial-warning');
      expect(warning).toBeTruthy();
    });

    it('does not show partial data warning when partial_data is false', () => {
      render(HealthScore, {
        props: {
          data: { uptime_percent: 99.9, active_monitor_count: 47, partial_data: false },
          loading: false,
          error: null,
          onRetry: null
        }
      });

      expect(screen.queryByTestId('health-score-partial-warning')).toBeNull();
    });

    it('shows loading skeleton when loading is true', () => {
      render(HealthScore, {
        props: { data: null, loading: true, error: null, onRetry: null }
      });

      expect(screen.getByTestId('widget-skeleton')).toBeTruthy();
    });

    it('shows error state when error is provided', () => {
      render(HealthScore, {
        props: { data: null, loading: false, error: 'Failed to fetch', onRetry: () => {} }
      });

      expect(screen.getByTestId('widget-error')).toBeTruthy();
      expect(screen.getByText('Failed to fetch')).toBeTruthy();
    });
  });
});
