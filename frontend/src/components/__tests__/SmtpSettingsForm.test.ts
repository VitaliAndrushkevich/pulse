/**
 * TDD tests for SmtpSettingsForm component.
 *
 * Property: port defaults to 25 when empty.
 * Unit: starttls defaults correctly, ehlo_domain defaults to "pulse.local".
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import * as fc from 'fast-check';

vi.mock('$lib/i18n/locale.svelte', () => ({
  getLocale: () => 'en',
  setLocale: vi.fn(),
  t: (key: string) => key,
}));

import SmtpSettingsForm from '../SmtpSettingsForm.svelte';

describe('SmtpSettingsForm', () => {
  afterEach(() => {
    cleanup();
  });

  it('property: port defaults to 25 when no value provided', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 10 }),
        (_iteration) => {
          cleanup();
          const { container } = render(SmtpSettingsForm, {
            props: {
              settings: {},
            },
          });

          const input = container.querySelector('[data-testid="smtp-port"]') as HTMLInputElement;
          expect(input).not.toBeNull();
          expect(Number(input.value)).toBe(25);
        }
      ),
      { numRuns: 10 }
    );
  });

  it('starttls defaults to checked', () => {
    const { container } = render(SmtpSettingsForm, {
      props: {
        settings: {},
      },
    });

    const checkbox = container.querySelector('[data-testid="smtp-starttls"]') as HTMLInputElement;
    expect(checkbox).not.toBeNull();
    expect(checkbox.checked).toBe(true);
  });

  it('ehlo_domain defaults to pulse.local placeholder', () => {
    const { container } = render(SmtpSettingsForm, {
      props: {
        settings: {},
      },
    });

    const input = container.querySelector('[data-testid="smtp-ehlo-domain"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    // The placeholder text is the i18n key in tests
    expect(input.placeholder).toBe('smtp.ehloDomainPlaceholder');
  });

  it('ssl_expiry_threshold input is conditional on starttls', () => {
    const { container } = render(SmtpSettingsForm, {
      props: {
        settings: { starttls: true },
      },
    });

    const input = container.querySelector('[data-testid="smtp-ssl-expiry"]') as HTMLInputElement;
    expect(input).not.toBeNull();
  });

  it('ssl_expiry_threshold hidden when starttls is false', () => {
    const { container } = render(SmtpSettingsForm, {
      props: {
        settings: { starttls: false },
      },
    });

    const input = container.querySelector('[data-testid="smtp-ssl-expiry"]') as HTMLInputElement;
    expect(input).toBeNull();
  });

  it('port input has correct bounds', () => {
    const { container } = render(SmtpSettingsForm, {
      props: {
        settings: { port: 587 },
      },
    });

    const input = container.querySelector('[data-testid="smtp-port"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(Number(input.min)).toBe(1);
    expect(Number(input.max)).toBe(65535);
    expect(Number(input.value)).toBe(587);
  });
});
