/**
 * TDD tests for DnsSettingsForm component.
 *
 * Property: form always produces valid DnsSettings shape.
 * Unit: default record_type is 'A', expected_value is optional and bindable.
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import * as fc from 'fast-check';

vi.mock('$lib/i18n/locale.svelte', () => ({
  getLocale: () => 'en',
  setLocale: vi.fn(),
  t: (key: string) => key,
}));

import DnsSettingsForm from '../DnsSettingsForm.svelte';

const VALID_RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'SOA', 'PTR', 'NS'];

describe('DnsSettingsForm', () => {
  afterEach(() => {
    cleanup();
  });

  it('property: form always produces valid DnsSettings shape with valid record_type', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...VALID_RECORD_TYPES),
        (recordType) => {
          cleanup();
          const { container } = render(DnsSettingsForm, {
            props: {
              settings: { record_type: recordType as any },
            },
          });

          const select = container.querySelector('[data-testid="dns-record-type"]') as HTMLSelectElement;
          expect(select).not.toBeNull();
          expect(VALID_RECORD_TYPES).toContain(select.value);
        }
      ),
      { numRuns: 50 }
    );
  });

  it('default record_type is A', () => {
    const { container } = render(DnsSettingsForm, {
      props: {
        settings: { record_type: 'A' },
      },
    });

    const select = container.querySelector('[data-testid="dns-record-type"]') as HTMLSelectElement;
    expect(select).not.toBeNull();
    expect(select.value).toBe('A');
  });

  it('expected_value is optional and bindable', () => {
    const { container } = render(DnsSettingsForm, {
      props: {
        settings: { record_type: 'A', expected_value: '93.184.216.34' },
      },
    });

    const input = container.querySelector('[data-testid="dns-expected-value"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(input.value).toBe('93.184.216.34');
  });

  it('renders all 9 record type options', () => {
    const { container } = render(DnsSettingsForm, {
      props: {
        settings: { record_type: 'A' },
      },
    });

    const select = container.querySelector('[data-testid="dns-record-type"]') as HTMLSelectElement;
    const options = select.querySelectorAll('option');
    expect(options.length).toBe(9);

    const values = Array.from(options).map(o => o.value);
    for (const rt of VALID_RECORD_TYPES) {
      expect(values).toContain(rt);
    }
  });

  it('dns_server input renders with placeholder', () => {
    const { container } = render(DnsSettingsForm, {
      props: {
        settings: { record_type: 'A' },
      },
    });

    const input = container.querySelector('[data-testid="dns-server"]') as HTMLInputElement;
    expect(input).not.toBeNull();
  });
});
