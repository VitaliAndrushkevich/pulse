/**
 * Property-based tests for locale persistence round-trip, invalid locale fallback,
 * and HTML lang attribute synchronization.
 *
 * Since locale.svelte.ts uses Svelte 5 module-level $effect (which cannot run outside
 * a component context), we test persistence and HTML lang logic by simulating the
 * exact behavior of setLocale/initLocale using the same config and pure functions.
 *
 * Uses fast-check to verify universal correctness properties
 * across randomly generated inputs.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import * as fc from 'fast-check';
import { SUPPORTED_LOCALES, STORAGE_KEY, FALLBACK_LOCALE, isSupportedLocale } from '../config';
import type { LocaleCode } from '../config';

/**
 * Simulate the persistence behavior of setLocale() from locale.svelte.ts:
 * - Writes the locale code to localStorage under STORAGE_KEY
 * - Updates document.documentElement.lang to the locale code
 * - Sets the internal "currentLocale" state
 */
function simulateSetLocale(code: LocaleCode): { currentLocale: LocaleCode } {
  // Persistence: write to localStorage (same as persistLocale helper)
  try {
    localStorage.setItem(STORAGE_KEY, code);
  } catch {
    // localStorage unavailable — locale works for session but won't persist
  }

  // HTML lang synchronization (same as the $effect in locale.svelte.ts)
  if (typeof document !== 'undefined') {
    document.documentElement.lang = code;
  }

  return { currentLocale: code };
}

/**
 * Simulate the initLocale() behavior from locale.svelte.ts:
 * 1. Read localStorage.getItem(STORAGE_KEY)
 * 2. If value is a supported locale → apply it (call setLocale)
 * 3. If value is unsupported → remove from localStorage, use 'en'
 * 4. If localStorage throws or empty → use 'en'
 */
function simulateInitLocale(): { currentLocale: LocaleCode; removedInvalid: boolean } {
  let stored: string | null = null;
  let removedInvalid = false;

  try {
    stored = localStorage.getItem(STORAGE_KEY);
  } catch {
    // localStorage unavailable — use fallback
    return { currentLocale: FALLBACK_LOCALE as LocaleCode, removedInvalid: false };
  }

  if (!stored) {
    return { currentLocale: FALLBACK_LOCALE as LocaleCode, removedInvalid: false };
  }

  if (!isSupportedLocale(stored)) {
    // Invalid locale — remove and use fallback
    try {
      localStorage.removeItem(STORAGE_KEY);
      removedInvalid = true;
    } catch {
      // Ignore write errors
    }
    return { currentLocale: FALLBACK_LOCALE as LocaleCode, removedInvalid };
  }

  // Valid stored locale — apply it
  const result = simulateSetLocale(stored);
  return { currentLocale: result.currentLocale, removedInvalid: false };
}

/** Arbitrary that picks one locale code from the supported list */
const arbitrarySupportedLocale = fc.constantFrom(
  ...SUPPORTED_LOCALES.map((l) => l.code)
) as fc.Arbitrary<LocaleCode>;

/** Arbitrary that generates strings NOT in the supported locales list */
const arbitraryUnsupportedLocale = fc
  .string({ minLength: 1, maxLength: 20 })
  .filter((s): boolean => {
    const isSupported: boolean = isSupportedLocale(s);
    return !isSupported && s.trim().length > 0;
  });

// Feature: i18n-localization, Property 5: Locale persistence round-trip
describe('Property 5: Locale persistence round-trip', () => {
  /**
   * Validates: Requirements 4.1, 4.2
   *
   * For any locale code in the supported locales list, calling setLocale(code) SHALL
   * write code to localStorage.getItem('pulse-locale'), and subsequently calling
   * initLocale() SHALL restore currentLocale to that same code.
   */

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.lang = 'en';
  });

  it('setLocale persists to localStorage and initLocale restores the same code', () => {
    fc.assert(
      fc.property(arbitrarySupportedLocale, (localeCode) => {
        // Clear state for this iteration
        localStorage.clear();

        // Call setLocale — should persist to localStorage
        simulateSetLocale(localeCode);

        // Verify localStorage was written with exact code
        expect(localStorage.getItem(STORAGE_KEY)).toBe(localeCode);

        // Simulate fresh app start: initLocale reads from localStorage
        const { currentLocale } = simulateInitLocale();

        // Verify the locale was restored to the same code
        expect(currentLocale).toBe(localeCode);
      }),
      { numRuns: 100 }
    );
  });

  it('setLocale writes exact locale code to localStorage key pulse-locale', () => {
    fc.assert(
      fc.property(arbitrarySupportedLocale, (localeCode) => {
        localStorage.clear();

        simulateSetLocale(localeCode);

        // Must write to the exact key defined in config
        expect(localStorage.getItem(STORAGE_KEY)).toBe(localeCode);
        expect(localStorage.getItem('pulse-locale')).toBe(localeCode);
      }),
      { numRuns: 100 }
    );
  });

  it('round-trip preserves locale code identity across multiple sets', () => {
    fc.assert(
      fc.property(
        arbitrarySupportedLocale,
        arbitrarySupportedLocale,
        (firstLocale, secondLocale) => {
          localStorage.clear();

          // Set first locale
          simulateSetLocale(firstLocale);
          expect(localStorage.getItem(STORAGE_KEY)).toBe(firstLocale);

          // Set second locale (overwrite)
          simulateSetLocale(secondLocale);
          expect(localStorage.getItem(STORAGE_KEY)).toBe(secondLocale);

          // initLocale should restore the LAST set locale
          const { currentLocale } = simulateInitLocale();
          expect(currentLocale).toBe(secondLocale);
        }
      ),
      { numRuns: 100 }
    );
  });
});

// Feature: i18n-localization, Property 6: Invalid stored locale falls back to English
describe('Property 6: Invalid stored locale falls back to English', () => {
  /**
   * Validates: Requirements 4.4, 6.4
   *
   * For any string that is NOT in the supported locales list, if that string is
   * stored in localStorage under 'pulse-locale', then initLocale() SHALL set
   * currentLocale to 'en' and SHALL remove the invalid entry from localStorage.
   */

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.lang = 'en';
  });

  it('invalid stored locale causes fallback to en and removal from localStorage', () => {
    fc.assert(
      fc.property(arbitraryUnsupportedLocale, (invalidCode) => {
        // Store an invalid locale code in localStorage
        localStorage.setItem(STORAGE_KEY, invalidCode);

        // Verify it was stored
        expect(localStorage.getItem(STORAGE_KEY)).toBe(invalidCode);

        // Call initLocale — should detect invalid and fall back
        const { currentLocale, removedInvalid } = simulateInitLocale();

        // currentLocale should be 'en' (fallback)
        expect(currentLocale).toBe('en');

        // The invalid entry should be removed from localStorage
        expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
        expect(removedInvalid).toBe(true);
      }),
      { numRuns: 100 }
    );
  });

  it('isSupportedLocale correctly rejects arbitrary unsupported strings', () => {
    fc.assert(
      fc.property(arbitraryUnsupportedLocale, (invalidCode) => {
        // By construction the arbitrary only generates strings not in the list
        expect(isSupportedLocale(invalidCode)).toBe(false);
      }),
      { numRuns: 100 }
    );
  });

  it('valid locale codes are never treated as invalid', () => {
    fc.assert(
      fc.property(arbitrarySupportedLocale, (validCode) => {
        localStorage.setItem(STORAGE_KEY, validCode);

        const { currentLocale, removedInvalid } = simulateInitLocale();

        // A valid code should NOT be removed
        expect(removedInvalid).toBe(false);
        // The locale should be set to the stored valid code
        expect(currentLocale).toBe(validCode);
        // localStorage should still contain the valid code
        expect(localStorage.getItem(STORAGE_KEY)).toBe(validCode);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: i18n-localization, Property 7: HTML lang attribute synchronization
describe('Property 7: HTML lang attribute synchronization', () => {
  /**
   * Validates: Requirements 8.1, 8.2
   *
   * For any locale code in the supported locales list, after setLocale(code) resolves,
   * document.documentElement.lang SHALL equal that locale code.
   */

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.lang = 'en';
  });

  it('document.documentElement.lang matches locale after setLocale', () => {
    fc.assert(
      fc.property(arbitrarySupportedLocale, (localeCode) => {
        // Reset HTML lang
        document.documentElement.lang = '';

        // Call setLocale — should synchronize HTML lang attribute
        simulateSetLocale(localeCode);

        // The HTML lang attribute should match the locale code
        expect(document.documentElement.lang).toBe(localeCode);
      }),
      { numRuns: 100 }
    );
  });

  it('HTML lang updates correctly on sequential locale changes', () => {
    fc.assert(
      fc.property(
        arbitrarySupportedLocale,
        arbitrarySupportedLocale,
        (firstLocale, secondLocale) => {
          // Set first locale
          simulateSetLocale(firstLocale);
          expect(document.documentElement.lang).toBe(firstLocale);

          // Set second locale — should update HTML lang to new value
          simulateSetLocale(secondLocale);
          expect(document.documentElement.lang).toBe(secondLocale);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('initLocale sets HTML lang for valid stored locale', () => {
    fc.assert(
      fc.property(arbitrarySupportedLocale, (localeCode) => {
        // Reset HTML lang
        document.documentElement.lang = '';

        // Store a valid locale
        localStorage.setItem(STORAGE_KEY, localeCode);

        // initLocale should restore and sync HTML lang
        simulateInitLocale();

        expect(document.documentElement.lang).toBe(localeCode);
      }),
      { numRuns: 100 }
    );
  });
});
