/**
 * TDD tests for IcmpSettingsForm component.
 *
 * Property: packet_count output clamped to 1-10.
 * Unit: default packet_count is 3, IPv6 toggle works.
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup, fireEvent } from '@testing-library/svelte';
import * as fc from 'fast-check';

vi.mock('$lib/i18n/locale.svelte', () => ({
  getLocale: () => 'en',
  setLocale: vi.fn(),
  t: (key: string) => key,
}));

import IcmpSettingsForm from '../IcmpSettingsForm.svelte';

describe('IcmpSettingsForm', () => {
  afterEach(() => {
    cleanup();
  });

  it('property: packet_count input is clamped to 1-10', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 10 }),
        (count) => {
          cleanup();
          const { container } = render(IcmpSettingsForm, {
            props: {
              settings: { packet_count: count },
            },
          });

          const input = container.querySelector('[data-testid="icmp-packet-count"]') as HTMLInputElement;
          expect(input).not.toBeNull();
          expect(Number(input.min)).toBe(1);
          expect(Number(input.max)).toBe(10);
          expect(Number(input.value)).toBe(count);
        }
      ),
      { numRuns: 50 }
    );
  });

  it('default packet_count is 3', () => {
    const { container } = render(IcmpSettingsForm, {
      props: {
        settings: {},
      },
    });

    const input = container.querySelector('[data-testid="icmp-packet-count"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(Number(input.value)).toBe(3);
  });

  it('IPv6 toggle works', () => {
    const { container } = render(IcmpSettingsForm, {
      props: {
        settings: { use_ipv6: true },
      },
    });

    const checkbox = container.querySelector('[data-testid="icmp-use-ipv6"]') as HTMLInputElement;
    expect(checkbox).not.toBeNull();
    expect(checkbox.checked).toBe(true);
  });

  it('loss_threshold_percent input renders with correct bounds', () => {
    const { container } = render(IcmpSettingsForm, {
      props: {
        settings: {},
      },
    });

    const input = container.querySelector('[data-testid="icmp-loss-threshold"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(Number(input.min)).toBe(0);
    expect(Number(input.max)).toBe(100);
    expect(Number(input.value)).toBe(100);
  });
});
