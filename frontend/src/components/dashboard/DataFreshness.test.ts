import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DataFreshness, { formatRelativeTime } from './DataFreshness.svelte';

vi.mock('$lib/i18n', () => ({
  t: (key: string) => {
    const translations: Record<string, string> = {
      'dashboard.freshness.label': 'Last updated:',
      'dashboard.freshness.stale': 'Data may be stale',
      'common.never': 'Never',
    };
    return translations[key] ?? key;
  },
}));

describe('DataFreshness', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders "Last updated: Never" when lastUpdated is null', () => {
    render(DataFreshness, { props: { lastUpdated: null, stale: false } });
    const freshness = screen.getByTestId('data-freshness');
    expect(freshness).toBeTruthy();
    expect(freshness.textContent).toContain('Never');
  });

  it('renders relative timestamp when lastUpdated is provided', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);

    const tenSecondsAgo = new Date('2024-01-15T11:59:50Z');
    render(DataFreshness, { props: { lastUpdated: tenSecondsAgo, stale: false } });

    const freshness = screen.getByTestId('data-freshness');
    expect(freshness.textContent).toContain('10s ago');
  });

  it('does not show stale indicator when stale is false', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);

    render(DataFreshness, { props: { lastUpdated: new Date(now), stale: false } });
    expect(screen.queryByTestId('stale-indicator')).toBeNull();
  });

  it('shows stale indicator badge when stale is true', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);

    render(DataFreshness, { props: { lastUpdated: new Date(now), stale: true } });
    const badge = screen.getByTestId('stale-indicator');
    expect(badge).toBeTruthy();
    expect(badge.textContent?.trim()).toBe('Data may be stale');
  });

  it('stale indicator has role="status" for accessibility', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);

    render(DataFreshness, { props: { lastUpdated: new Date(now), stale: true } });
    const badge = screen.getByTestId('stale-indicator');
    expect(badge.getAttribute('role')).toBe('status');
  });

  it('displays "Last updated:" label using t()', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    vi.setSystemTime(now);

    render(DataFreshness, { props: { lastUpdated: new Date(now), stale: false } });
    const freshness = screen.getByTestId('data-freshness');
    expect(freshness.textContent).toContain('Last updated:');
  });
});

describe('formatRelativeTime', () => {
  it('returns "0s ago" for same timestamp', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    expect(formatRelativeTime(now, now)).toBe('0s ago');
  });

  it('returns seconds for diffs under 60s', () => {
    const now = new Date('2024-01-15T12:00:45Z');
    const date = new Date('2024-01-15T12:00:00Z');
    expect(formatRelativeTime(date, now)).toBe('45s ago');
  });

  it('returns minutes for diffs between 60s and 60m', () => {
    const now = new Date('2024-01-15T12:05:00Z');
    const date = new Date('2024-01-15T12:00:00Z');
    expect(formatRelativeTime(date, now)).toBe('5m ago');
  });

  it('returns hours for diffs between 1h and 24h', () => {
    const now = new Date('2024-01-15T14:00:00Z');
    const date = new Date('2024-01-15T12:00:00Z');
    expect(formatRelativeTime(date, now)).toBe('2h ago');
  });

  it('returns days for diffs over 24h', () => {
    const now = new Date('2024-01-18T12:00:00Z');
    const date = new Date('2024-01-15T12:00:00Z');
    expect(formatRelativeTime(date, now)).toBe('3d ago');
  });

  it('clamps negative diffs to 0s ago', () => {
    const now = new Date('2024-01-15T12:00:00Z');
    const future = new Date('2024-01-15T12:01:00Z');
    expect(formatRelativeTime(future, now)).toBe('0s ago');
  });
});
