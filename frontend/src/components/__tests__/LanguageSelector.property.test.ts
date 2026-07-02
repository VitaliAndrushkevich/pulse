/**
 * Property-based test for LanguageSelector option order.
 *
 * Feature: i18n-localization, Property 8: Language selector displays native names in config order
 *
 * Validates: Requirements 5.2, 5.6
 *
 * For any rendering of the LanguageSelector component, the dropdown options SHALL
 * appear in exactly the order defined by SUPPORTED_LOCALES, and each option's
 * visible text SHALL be the `name` field (native name) from the corresponding entry.
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import * as fc from 'fast-check';
import { SUPPORTED_LOCALES } from '$lib/i18n/config';

// Mock the locale.svelte module — the component imports getLocale, setLocale, t from it
vi.mock('$lib/i18n/locale.svelte', () => ({
  getLocale: () => 'en',
  setLocale: vi.fn(),
  t: (key: string) => key,
}));

import LanguageSelector from '../LanguageSelector.svelte';

describe('Property 8: Language selector displays native names in config order', () => {
  /**
   * Validates: Requirements 5.2, 5.6
   *
   * For any rendering of the LanguageSelector component, the dropdown options SHALL
   * appear in exactly the order defined by SUPPORTED_LOCALES, and each option's
   * visible text SHALL be the `name` field (native name) from the corresponding entry.
   */

  afterEach(() => {
    cleanup();
  });

  it('all options appear in exactly the SUPPORTED_LOCALES config order with native names', () => {
    const { container } = render(LanguageSelector);

    const select = container.querySelector('select');
    expect(select).not.toBeNull();

    const options = select!.querySelectorAll('option');

    // Number of options must match supported locales count
    expect(options.length).toBe(SUPPORTED_LOCALES.length);

    // Use fast-check to verify each position holds the correct entry
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: SUPPORTED_LOCALES.length - 1 }),
        (index) => {
          const option = options[index];
          const expected = SUPPORTED_LOCALES[index];

          // Option value must be the locale code at this position
          expect(option.getAttribute('value')).toBe(expected.code);

          // Option visible text must be the native name at this position
          expect(option.textContent).toBe(expected.name);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('option order is stable across multiple renders', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 5 }),
        (renderCount) => {
          // Render the component multiple times to verify order stability
          for (let r = 0; r < renderCount; r++) {
            cleanup();
            const { container } = render(LanguageSelector);
            const options = container.querySelectorAll('select option');

            expect(options.length).toBe(SUPPORTED_LOCALES.length);

            for (let i = 0; i < SUPPORTED_LOCALES.length; i++) {
              const option = options[i];
              const expected = SUPPORTED_LOCALES[i];

              expect(option.getAttribute('value')).toBe(expected.code);
              expect(option.textContent).toBe(expected.name);
            }
          }
        }
      ),
      { numRuns: 100 }
    );
  });

  it('every SUPPORTED_LOCALES entry has a corresponding option at its config index', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...SUPPORTED_LOCALES.map((_, i) => i)),
        (index) => {
          cleanup();
          const { container } = render(LanguageSelector);
          const options = container.querySelectorAll('select option');
          const expected = SUPPORTED_LOCALES[index];
          const option = options[index];

          // The option at position `index` must always match the config entry at `index`
          expect(option).toBeDefined();
          expect(option.getAttribute('value')).toBe(expected.code);
          expect(option.textContent).toBe(expected.name);
        }
      ),
      { numRuns: 100 }
    );
  });
});
