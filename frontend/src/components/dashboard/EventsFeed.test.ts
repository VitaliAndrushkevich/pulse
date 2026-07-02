import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import EventsFeed, {
  formatRelativeTime,
  filterInitialEvents,
  prependEvent,
  getEventColor
} from './EventsFeed.svelte';
import type { RecentEvent } from '$lib/types';

function makeEvent(overrides: Partial<RecentEvent> = {}): RecentEvent {
  return {
    monitor_id: 'mon-1',
    monitor_name: 'API Gateway',
    from_state: 'up',
    to_state: 'down',
    occurred_at: new Date(Date.now() - 60_000).toISOString(),
    ...overrides
  };
}

describe('EventsFeed', () => {
  describe('formatRelativeTime', () => {
    const now = new Date('2024-01-15T12:00:00Z').getTime();

    it('returns justNow for timestamps less than 10 seconds ago', () => {
      const occurred = new Date(now - 5_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({ key: 'dashboard.events.justNow' });
    });

    it('returns seconds for timestamps 10-59 seconds ago', () => {
      const occurred = new Date(now - 45_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({
        key: 'dashboard.events.secondsAgo',
        params: { count: '45' }
      });
    });

    it('returns minutes for timestamps 1-59 minutes ago', () => {
      const occurred = new Date(now - 3 * 60_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({
        key: 'dashboard.events.minutesAgo',
        params: { count: '3' }
      });
    });

    it('returns hours for timestamps 1-23 hours ago', () => {
      const occurred = new Date(now - 2 * 3600_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({
        key: 'dashboard.events.hoursAgo',
        params: { count: '2' }
      });
    });

    it('returns days for timestamps 24+ hours ago', () => {
      const occurred = new Date(now - 3 * 24 * 3600_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({
        key: 'dashboard.events.daysAgo',
        params: { count: '3' }
      });
    });

    it('handles future timestamps gracefully (clamps to 0)', () => {
      const occurred = new Date(now + 60_000).toISOString();
      expect(formatRelativeTime(occurred, now)).toEqual({ key: 'dashboard.events.justNow' });
    });
  });

  describe('filterInitialEvents', () => {
    const now = new Date('2024-01-15T12:00:00Z').getTime();

    it('filters out events older than 24 hours', () => {
      const recent = makeEvent({ occurred_at: new Date(now - 3600_000).toISOString() });
      const old = makeEvent({
        monitor_id: 'mon-2',
        occurred_at: new Date(now - 25 * 3600_000).toISOString()
      });

      const result = filterInitialEvents([recent, old], now);
      expect(result).toHaveLength(1);
      expect(result[0]).toEqual(recent);
    });

    it('sorts events in reverse chronological order', () => {
      const older = makeEvent({
        monitor_id: 'mon-1',
        occurred_at: new Date(now - 7200_000).toISOString()
      });
      const newer = makeEvent({
        monitor_id: 'mon-2',
        occurred_at: new Date(now - 3600_000).toISOString()
      });

      const result = filterInitialEvents([older, newer], now);
      expect(result[0].monitor_id).toBe('mon-2');
      expect(result[1].monitor_id).toBe('mon-1');
    });

    it('caps at 10 entries', () => {
      const events = Array.from({ length: 15 }, (_, i) =>
        makeEvent({
          monitor_id: `mon-${i}`,
          occurred_at: new Date(now - i * 60_000).toISOString()
        })
      );

      const result = filterInitialEvents(events, now);
      expect(result).toHaveLength(10);
    });

    it('returns empty array for no events', () => {
      expect(filterInitialEvents([], now)).toEqual([]);
    });
  });

  describe('prependEvent', () => {
    it('prepends a new event to the beginning', () => {
      const existing = [makeEvent({ monitor_id: 'mon-1' })];
      const newEvent = makeEvent({ monitor_id: 'mon-2' });

      const result = prependEvent(existing, newEvent);
      expect(result[0].monitor_id).toBe('mon-2');
      expect(result[1].monitor_id).toBe('mon-1');
    });

    it('removes oldest event when exceeding maxSize', () => {
      const existing = Array.from({ length: 10 }, (_, i) =>
        makeEvent({ monitor_id: `mon-${i}` })
      );
      const newEvent = makeEvent({ monitor_id: 'mon-new' });

      const result = prependEvent(existing, newEvent, 10);
      expect(result).toHaveLength(10);
      expect(result[0].monitor_id).toBe('mon-new');
      expect(result[9].monitor_id).toBe('mon-8');
    });

    it('does not remove events when under maxSize', () => {
      const existing = [makeEvent({ monitor_id: 'mon-1' })];
      const newEvent = makeEvent({ monitor_id: 'mon-2' });

      const result = prependEvent(existing, newEvent, 10);
      expect(result).toHaveLength(2);
    });
  });

  describe('getEventColor', () => {
    it('returns success color for recovery (to up)', () => {
      expect(getEventColor('up')).toBe('var(--color-success)');
    });

    it('returns error color for failure (to down)', () => {
      expect(getEventColor('down')).toBe('var(--color-error)');
    });

    it('returns secondary color for unknown transitions', () => {
      expect(getEventColor('unknown')).toBe('var(--color-text-secondary)');
    });
  });

  describe('rendering', () => {
    it('shows empty state when no events exist', () => {
      render(EventsFeed, {
        props: { events: [], loading: false, error: null, onRetry: null }
      });

      expect(screen.getByTestId('events-empty')).toBeTruthy();
    });

    it('shows loading skeleton when loading', () => {
      render(EventsFeed, {
        props: { events: [], loading: true, error: null, onRetry: null }
      });

      expect(screen.getByTestId('widget-skeleton')).toBeTruthy();
    });

    it('shows error state when error is provided', () => {
      render(EventsFeed, {
        props: { events: [], loading: false, error: 'Network error', onRetry: () => {} }
      });

      expect(screen.getByTestId('widget-error')).toBeTruthy();
      expect(screen.getByText('Network error')).toBeTruthy();
    });

    it('renders event list when events exist', () => {
      const events: RecentEvent[] = [
        makeEvent({ monitor_name: 'API Gateway', from_state: 'up', to_state: 'down' }),
        makeEvent({
          monitor_id: 'mon-2',
          monitor_name: 'DB Server',
          from_state: 'down',
          to_state: 'up',
          occurred_at: new Date(Date.now() - 120_000).toISOString()
        })
      ];

      render(EventsFeed, {
        props: { events, loading: false, error: null, onRetry: null }
      });

      expect(screen.getByTestId('events-list')).toBeTruthy();
      expect(screen.getByText('API Gateway')).toBeTruthy();
      expect(screen.getByText('DB Server')).toBeTruthy();
    });
  });
});
