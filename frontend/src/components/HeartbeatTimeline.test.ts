/**
 * Unit tests for the HeartbeatTimeline component.
 *
 * Validates (design §5.3–5.5):
 * - Mandatory empty state at zero events
 * - Loading and error states
 * - success / missed / fail marker rendering (one marker per event below threshold)
 * - Bucketing above the maxMarkers threshold picks the WORST state colour
 *   (missed > fail > success) so problems are never hidden
 * - Clicking a problem marker opens a popover listing the failures
 * - Every visible string resolves through t() (no literal strings leak)
 */
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import HeartbeatTimeline from './HeartbeatTimeline.svelte';
import { t } from '$lib/i18n';
import type { HeartbeatEvent, HeartbeatHistorySummary } from '$lib/types';

afterEach(() => cleanup());

const FROM = '2026-07-01T00:00:00Z';
const TO = '2026-07-02T00:00:00Z';

function summaryFrom(events: HeartbeatEvent[]): HeartbeatHistorySummary {
  const success = events.filter((e) => e.event_type === 'success').length;
  const fail = events.filter((e) => e.event_type === 'fail').length;
  const missed = events.filter((e) => e.event_type === 'missed').length;
  const total = events.length;
  return {
    uptime_percent: total === 0 ? null : Number(((success / total) * 100).toFixed(1)),
    missed_count: missed,
    fail_count: fail,
    success_count: success,
    total_events: total,
  };
}

function baseProps(events: HeartbeatEvent[], overrides: Partial<Record<string, unknown>> = {}) {
  return {
    events,
    summary: summaryFrom(events),
    loading: false,
    error: null as string | null,
    from: FROM,
    to: TO,
    ...overrides,
  };
}

/** Spread events evenly across the [FROM, TO] window so bucketing is deterministic. */
function makeEvents(types: HeartbeatEvent['event_type'][]): HeartbeatEvent[] {
  const start = new Date(FROM).getTime();
  const end = new Date(TO).getTime();
  const span = end - start;
  return types.map((event_type, i) => ({
    event_type,
    occurred_at: new Date(start + (span * i) / Math.max(types.length, 1)).toISOString(),
  }));
}

describe('HeartbeatTimeline', () => {
  describe('empty state', () => {
    it('renders the mandatory empty state when total_events is zero', () => {
      render(HeartbeatTimeline, { props: baseProps([]) });

      const empty = screen.getByTestId('hb-timeline-empty');
      expect(empty).toBeTruthy();
      expect(empty.textContent).toContain(t('heartbeats.timeline.empty'));
      expect(empty.textContent).toContain(t('heartbeats.timeline.emptyHint'));
      // No bar is drawn when empty.
      expect(screen.queryByTestId('hb-timeline-bar')).toBeNull();
    });
  });

  describe('loading and error states', () => {
    it('renders the loading state via t()', () => {
      render(HeartbeatTimeline, { props: baseProps([], { loading: true }) });
      const el = screen.getByTestId('hb-timeline-loading');
      expect(el.textContent).toContain(t('heartbeats.timeline.loading'));
    });

    it('renders the error message when error is set', () => {
      render(HeartbeatTimeline, {
        props: baseProps([], { error: 'boom' }),
      });
      const el = screen.getByTestId('hb-timeline-error');
      expect(el.textContent).toContain('boom');
      // Error takes precedence over the empty state.
      expect(screen.queryByTestId('hb-timeline-empty')).toBeNull();
    });
  });

  describe('marker rendering (below threshold)', () => {
    it('renders one marker per event with the matching state colour', () => {
      const events = makeEvents(['success', 'missed', 'fail']);
      render(HeartbeatTimeline, { props: baseProps(events) });

      const markers = screen.getAllByTestId('hb-timeline-bar')[0].querySelectorAll('[data-hb-marker]');
      expect(markers.length).toBe(3);

      const states = Array.from(markers).map((m) => m.getAttribute('data-state'));
      expect(states).toEqual(['success', 'missed', 'fail']);

      const classes = Array.from(markers).map((m) => m.className);
      expect(classes[0]).toContain('bg-success');
      expect(classes[1]).toContain('bg-warning');
      expect(classes[2]).toContain('bg-error');
    });
  });

  describe('bucketing (above threshold)', () => {
    it('buckets dense events and colours each bucket by its worst state', () => {
      // 300 events in one small time slice: mostly success with a single missed.
      // With maxMarkers tiny, they collapse into one bucket whose worst state is missed.
      const types: HeartbeatEvent['event_type'][] = Array.from({ length: 300 }, (_, i) =>
        i === 150 ? 'missed' : 'success'
      );
      const start = new Date(FROM).getTime();
      const events: HeartbeatEvent[] = types.map((event_type, i) => ({
        event_type,
        // Pack all events into the first minute so they land in bucket 0.
        occurred_at: new Date(start + i).toISOString(),
      }));

      render(HeartbeatTimeline, { props: baseProps(events, { maxMarkers: 2 }) });

      const markers = screen
        .getAllByTestId('hb-timeline-bar')[0]
        .querySelectorAll('[data-hb-marker]');
      // Problems are never hidden: a bucket containing a missed event is coloured missed.
      const states = Array.from(markers).map((m) => m.getAttribute('data-state'));
      expect(states).toContain('missed');
      const missedMarker = Array.from(markers).find(
        (m) => m.getAttribute('data-state') === 'missed'
      );
      expect(missedMarker!.className).toContain('bg-warning');
    });

    it('ranks missed above fail above success when picking a bucket colour', () => {
      // All three in the same slice -> worst (missed) wins.
      const start = new Date(FROM).getTime();
      const events: HeartbeatEvent[] = (['success', 'fail', 'missed'] as const).flatMap((type, i) =>
        Array.from({ length: 50 }, (_, j) => ({
          event_type: type,
          occurred_at: new Date(start + i + j).toISOString(),
        }))
      );

      render(HeartbeatTimeline, { props: baseProps(events, { maxMarkers: 1 }) });

      const marker = screen
        .getAllByTestId('hb-timeline-bar')[0]
        .querySelector('[data-hb-marker]');
      expect(marker!.getAttribute('data-state')).toBe('missed');
    });
  });

  describe('popover', () => {
    it('opens a popover listing failures when a problem marker is clicked', async () => {
      const events = makeEvents(['success', 'fail']);
      render(HeartbeatTimeline, { props: baseProps(events) });

      const markers = screen
        .getAllByTestId('hb-timeline-bar')[0]
        .querySelectorAll('[data-hb-marker]');
      const failMarker = Array.from(markers).find(
        (m) => m.getAttribute('data-state') === 'fail'
      )!;

      await fireEvent.click(failMarker);

      const popover = screen.getByTestId('hb-event-popover');
      expect(popover).toBeTruthy();
      // Lists the explicit failure through the t() event label.
      expect(popover.textContent).toContain(t('heartbeats.timeline.legendFail'));
    });

    it('does not open a popover for success markers', async () => {
      const events = makeEvents(['success']);
      render(HeartbeatTimeline, { props: baseProps(events) });

      const marker = screen
        .getAllByTestId('hb-timeline-bar')[0]
        .querySelector('[data-hb-marker]')!;
      await fireEvent.click(marker);

      expect(screen.queryByTestId('hb-event-popover')).toBeNull();
    });
  });

  describe('summary + i18n', () => {
    it('shows uptime, missed count, and the three-state legend from t()', () => {
      const events = makeEvents(['success', 'success', 'missed', 'fail']);
      const { container } = render(HeartbeatTimeline, { props: baseProps(events) });

      const text = container.textContent ?? '';
      expect(text).toContain(t('heartbeats.timeline.uptime'));
      expect(text).toContain(t('heartbeats.timeline.missed'));
      expect(text).toContain(t('heartbeats.timeline.legendSuccess'));
      expect(text).toContain(t('heartbeats.timeline.legendMissed'));
      expect(text).toContain(t('heartbeats.timeline.legendFail'));
      // 2 success of 4 events -> 50.0%
      expect(text).toContain('50.0%');
    });

    it('renders an em dash for null uptime (guarded by the empty state otherwise)', () => {
      // A summary with events but null uptime should not crash; render a single event.
      const events = makeEvents(['success']);
      const summary = { ...summaryFrom(events), uptime_percent: null };
      const { container } = render(HeartbeatTimeline, {
        props: { ...baseProps(events), summary },
      });
      expect(container.textContent).toContain('—');
    });

    it('does not leak raw translation keys into the DOM', () => {
      const events = makeEvents(['success', 'missed', 'fail']);
      const { container } = render(HeartbeatTimeline, { props: baseProps(events) });
      // If a key failed to resolve, t() returns the key itself (e.g. "heartbeats.timeline.*").
      expect(container.textContent).not.toContain('heartbeats.timeline.');
    });
  });
});
