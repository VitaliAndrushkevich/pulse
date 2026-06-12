/**
 * Unit tests for the TimeRangePicker component.
 *
 * Validates:
 * - Preset buttons render and respond to clicks (Req 4.1)
 * - Custom mode shows date inputs (Req 4.2)
 * - Validation error shown for start >= end (Req 4.5)
 * - Retention notice displayed when range exceeds retention (Req 4.4)
 *
 * Requirements: 3.2, 4.1, 4.2, 4.3, 4.4, 4.5
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TimeRangePicker from './TimeRangePicker.svelte';

function defaultProps(overrides: Partial<{
  retentionDays: number;
  onchange: (range: { from: string; to: string }) => void;
  selected: string;
}> = {}) {
  return {
    retentionDays: 30,
    onchange: vi.fn(),
    ...overrides,
  };
}

describe('TimeRangePicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('preset buttons', () => {
    it('renders all preset buttons (1h, 6h, 24h, 7d, 30d) and a Custom button', () => {
      const props = defaultProps();
      render(TimeRangePicker, { props });

      expect(screen.getByTestId('preset-1h')).toBeTruthy();
      expect(screen.getByTestId('preset-6h')).toBeTruthy();
      expect(screen.getByTestId('preset-24h')).toBeTruthy();
      expect(screen.getByTestId('preset-7d')).toBeTruthy();
      expect(screen.getByTestId('preset-30d')).toBeTruthy();
      expect(screen.getByTestId('preset-custom')).toBeTruthy();
    });

    it('calls onchange with computed from/to when a preset is clicked', async () => {
      const onchange = vi.fn();
      const props = defaultProps({ onchange });
      render(TimeRangePicker, { props });

      const before = Date.now();
      await fireEvent.click(screen.getByTestId('preset-1h'));
      const after = Date.now();

      expect(onchange).toHaveBeenCalledTimes(1);
      const call = onchange.mock.calls[0][0];
      expect(call).toHaveProperty('from');
      expect(call).toHaveProperty('to');

      // Verify the range is ~1 hour
      const fromMs = new Date(call.from).getTime();
      const toMs = new Date(call.to).getTime();
      const durationMs = toMs - fromMs;

      // Should be 1 hour (3600000ms)
      expect(durationMs).toBe(60 * 60 * 1000);
      // End time should be close to now
      expect(toMs).toBeGreaterThanOrEqual(before);
      expect(toMs).toBeLessThanOrEqual(after + 100);
    });

    it('marks the selected preset with aria-pressed=true', async () => {
      const props = defaultProps({ selected: '24h' });
      render(TimeRangePicker, { props });

      expect(screen.getByTestId('preset-24h').getAttribute('aria-pressed')).toBe('true');
      expect(screen.getByTestId('preset-1h').getAttribute('aria-pressed')).toBe('false');
    });

    it('clicking a different preset switches the active button', async () => {
      const onchange = vi.fn();
      const props = defaultProps({ onchange, selected: '24h' });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-7d'));

      expect(screen.getByTestId('preset-7d').getAttribute('aria-pressed')).toBe('true');
      expect(screen.getByTestId('preset-24h').getAttribute('aria-pressed')).toBe('false');
    });
  });

  describe('custom mode', () => {
    it('shows custom range inputs when Custom button is clicked', async () => {
      const props = defaultProps();
      render(TimeRangePicker, { props });

      // Custom inputs should not be visible initially
      expect(screen.queryByTestId('custom-range-inputs')).toBeNull();

      await fireEvent.click(screen.getByTestId('preset-custom'));

      expect(screen.getByTestId('custom-range-inputs')).toBeTruthy();
      expect(screen.getByTestId('custom-start')).toBeTruthy();
      expect(screen.getByTestId('custom-end')).toBeTruthy();
      expect(screen.getByTestId('custom-apply')).toBeTruthy();
    });

    it('marks Custom button as aria-pressed=true in custom mode', async () => {
      const props = defaultProps();
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-custom'));

      expect(screen.getByTestId('preset-custom').getAttribute('aria-pressed')).toBe('true');
    });

    it('shows validation error when start/end are empty and Apply is clicked', async () => {
      const onchange = vi.fn();
      const props = defaultProps({ onchange });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-custom'));
      await fireEvent.click(screen.getByTestId('custom-apply'));

      expect(screen.getByTestId('validation-error')).toBeTruthy();
      expect(screen.getByTestId('validation-error').textContent).toContain('Please select both start and end times');
      expect(onchange).not.toHaveBeenCalled();
    });

    it('shows validation error when start >= end', async () => {
      const onchange = vi.fn();
      const props = defaultProps({ onchange });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-custom'));

      const startInput = screen.getByTestId('custom-start') as HTMLInputElement;
      const endInput = screen.getByTestId('custom-end') as HTMLInputElement;

      // Set start after end
      await fireEvent.input(startInput, { target: { value: '2024-06-15T14:00' } });
      await fireEvent.input(endInput, { target: { value: '2024-06-15T10:00' } });
      await fireEvent.click(screen.getByTestId('custom-apply'));

      expect(screen.getByTestId('validation-error')).toBeTruthy();
      expect(screen.getByTestId('validation-error').textContent).toContain('Start time must be before end time');
      expect(onchange).not.toHaveBeenCalled();
    });

    it('calls onchange with valid custom range', async () => {
      const onchange = vi.fn();
      const props = defaultProps({ onchange });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-custom'));

      const startInput = screen.getByTestId('custom-start') as HTMLInputElement;
      const endInput = screen.getByTestId('custom-end') as HTMLInputElement;

      // Set valid range in the past
      await fireEvent.input(startInput, { target: { value: '2024-01-01T10:00' } });
      await fireEvent.input(endInput, { target: { value: '2024-01-01T12:00' } });
      await fireEvent.click(screen.getByTestId('custom-apply'));

      expect(onchange).toHaveBeenCalledTimes(1);
      const call = onchange.mock.calls[0][0];
      expect(new Date(call.from).getTime()).toBe(new Date('2024-01-01T10:00').getTime());
    });
  });

  describe('retention notice', () => {
    it('shows retention notice when selected preset exceeds retention days', async () => {
      // retentionDays = 5 means 30d preset exceeds retention
      const props = defaultProps({ retentionDays: 5 });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-30d'));

      expect(screen.getByTestId('retention-notice')).toBeTruthy();
    });

    it('does not show retention notice when preset is within retention', async () => {
      const props = defaultProps({ retentionDays: 30 });
      render(TimeRangePicker, { props });

      await fireEvent.click(screen.getByTestId('preset-24h'));

      expect(screen.queryByTestId('retention-notice')).toBeNull();
    });
  });
});
