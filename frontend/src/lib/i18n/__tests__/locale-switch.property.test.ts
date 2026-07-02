/**
 * Property-based test for locale switch updating t() output.
 *
 * Since locale.svelte.ts uses Svelte 5 module-level $state (which cannot run outside
 * a component context), we test the locale switch behavior by simulating the exact
 * logic of t() with different active dictionaries — same pattern used by
 * locale-persistence.test.ts and locale-fallback.test.ts.
 *
 * The property verifies: for any key that has different translations in two supported
 * locales (A and B), switching from locale A to locale B causes t(key) to return the
 * value from locale B's dictionary.
 *
 * Uses fast-check to verify this correctness property across randomly generated inputs.
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { resolveKey, interpolate } from '../resolve';
import type { TranslationDictionary } from '../types';
import { SUPPORTED_LOCALES } from '../config';
import type { LocaleCode } from '../config';

// --- Load all locale dictionaries statically for testing ---
import enDict from '../../../locales/en.json';
import ruDict from '../../../locales/ru.json';
import esDict from '../../../locales/es.json';
import frDict from '../../../locales/fr.json';
import ptDict from '../../../locales/pt.json';
import deDict from '../../../locales/de.json';
import zhDict from '../../../locales/zh.json';
import jaDict from '../../../locales/ja.json';
import koDict from '../../../locales/ko.json';
import trDict from '../../../locales/tr.json';
import itDict from '../../../locales/it.json';

/** Map of locale code to its dictionary */
const dictionaries: Record<string, TranslationDictionary> = {
  en: enDict as TranslationDictionary,
  ru: ruDict as TranslationDictionary,
  es: esDict as TranslationDictionary,
  fr: frDict as TranslationDictionary,
  pt: ptDict as TranslationDictionary,
  de: deDict as TranslationDictionary,
  zh: zhDict as TranslationDictionary,
  ja: jaDict as TranslationDictionary,
  ko: koDict as TranslationDictionary,
  tr: trDict as TranslationDictionary,
  it: itDict as TranslationDictionary,
};

/**
 * Simulate the t() function exactly as implemented in locale.svelte.ts:
 * 1. Look up key in activeDictionary
 * 2. If not found, look up key in enDictionary (fallback)
 * 3. If not found in either, return the key string itself
 * 4. If found, apply interpolate(value, params)
 */
function simulateT(
  activeDictionary: TranslationDictionary,
  key: string,
  params?: Record<string, string | number>
): string {
  let value = resolveKey(activeDictionary, key);

  if (value === undefined) {
    value = resolveKey(enDict as TranslationDictionary, key);
  }

  if (value === undefined) {
    return key;
  }

  return interpolate(value, params);
}

/**
 * Collect all dot-notation leaf keys from a dictionary recursively.
 */
function collectKeys(dict: TranslationDictionary, prefix = ''): string[] {
  const keys: string[] = [];
  for (const [key, value] of Object.entries(dict)) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    if (typeof value === 'string') {
      keys.push(fullKey);
    } else if (typeof value === 'object' && value !== null) {
      keys.push(...collectKeys(value as TranslationDictionary, fullKey));
    }
  }
  return keys;
}

/** All locale codes from the supported list */
const localeCodes = SUPPORTED_LOCALES.map((l) => l.code);

/**
 * Find all keys that have different translations between two locale dictionaries.
 * A key "differs" if both locales resolve it to a string, and those strings are not equal.
 */
function findDifferingKeys(
  dictA: TranslationDictionary,
  dictB: TranslationDictionary
): string[] {
  const keysA = collectKeys(dictA);
  return keysA.filter((key) => {
    const valueA = resolveKey(dictA, key);
    const valueB = resolveKey(dictB, key);
    return valueA !== undefined && valueB !== undefined && valueA !== valueB;
  });
}

// Feature: i18n-localization, Property 9: Locale switch updates t() output
describe('Property 9: Locale switch updates t() output', () => {
  /**
   * Validates: Requirements 2.3
   *
   * For any key that has different translations in two supported locales (A and B),
   * switching from locale A to locale B SHALL cause t(key) to return the value from
   * locale B's dictionary.
   */

  /** Arbitrary that picks a pair of distinct locale codes */
  const arbitraryLocalePair = fc
    .tuple(
      fc.constantFrom(...localeCodes),
      fc.constantFrom(...localeCodes)
    )
    .filter(([a, b]) => a !== b) as fc.Arbitrary<[LocaleCode, LocaleCode]>;

  it('switching locale causes t(key) to return the new locale value for keys with different translations', () => {
    fc.assert(
      fc.property(arbitraryLocalePair, ([localeA, localeB]) => {
        const dictA = dictionaries[localeA];
        const dictB = dictionaries[localeB];

        // Find keys that have different values between locale A and locale B
        const differingKeys = findDifferingKeys(dictA, dictB);

        // Precondition: there must be at least one differing key between the two locales
        fc.pre(differingKeys.length > 0);

        // Pick a random differing key (deterministic per run via index)
        const keyIndex = Math.floor(Math.random() * differingKeys.length);
        const key = differingKeys[keyIndex];

        // Simulate being on locale A — t(key) returns locale A's value
        const resultA = simulateT(dictA, key);
        const expectedA = resolveKey(dictA, key)!;
        expect(resultA).toBe(expectedA);

        // Switch to locale B — t(key) now returns locale B's value
        const resultB = simulateT(dictB, key);
        const expectedB = resolveKey(dictB, key)!;
        expect(resultB).toBe(expectedB);

        // The two results must be different (since we filtered for differing keys)
        expect(resultA).not.toBe(resultB);
      }),
      { numRuns: 100 }
    );
  });

  it('after switching locale, t() uses the new dictionary for ALL differing keys', () => {
    fc.assert(
      fc.property(
        arbitraryLocalePair,
        fc.integer({ min: 0, max: 999 }),
        ([localeA, localeB], seed) => {
          const dictA = dictionaries[localeA];
          const dictB = dictionaries[localeB];

          const differingKeys = findDifferingKeys(dictA, dictB);
          fc.pre(differingKeys.length > 0);

          // Use seed to pick a subset of keys to test (up to 5)
          const sampleSize = Math.min(5, differingKeys.length);
          const startIdx = seed % differingKeys.length;
          const sampledKeys: string[] = [];
          for (let i = 0; i < sampleSize; i++) {
            sampledKeys.push(differingKeys[(startIdx + i) % differingKeys.length]);
          }

          for (const key of sampledKeys) {
            // While on locale A, t returns A's value
            const resultOnA = simulateT(dictA, key);
            expect(resultOnA).toBe(resolveKey(dictA, key));

            // After switching to locale B, t returns B's value
            const resultOnB = simulateT(dictB, key);
            expect(resultOnB).toBe(resolveKey(dictB, key));

            // Values differ
            expect(resultOnA).not.toBe(resultOnB);
          }
        }
      ),
      { numRuns: 100 }
    );
  });

  it('locale switch preserves interpolation behavior with the new dictionary value', () => {
    fc.assert(
      fc.property(
        arbitraryLocalePair,
        fc.integer({ min: 0, max: 9999 }),
        ([localeA, localeB], paramValue) => {
          const dictA = dictionaries[localeA];
          const dictB = dictionaries[localeB];

          // Find keys with interpolation placeholders that differ between locales
          const differingKeys = findDifferingKeys(dictA, dictB);
          const interpolatedKeys = differingKeys.filter((key) => {
            const valA = resolveKey(dictA, key);
            const valB = resolveKey(dictB, key);
            return (
              valA !== undefined &&
              valB !== undefined &&
              (valA.includes('{') || valB.includes('{'))
            );
          });

          // If no interpolated differing keys exist, test with a regular key instead
          if (interpolatedKeys.length === 0) {
            fc.pre(differingKeys.length > 0);
            const key = differingKeys[0];

            const resultA = simulateT(dictA, key, { count: paramValue });
            const resultB = simulateT(dictB, key, { count: paramValue });

            expect(resultA).toBe(resolveKey(dictA, key));
            expect(resultB).toBe(resolveKey(dictB, key));
            expect(resultA).not.toBe(resultB);
            return;
          }

          const key = interpolatedKeys[paramValue % interpolatedKeys.length];
          const params = { count: paramValue, name: 'test' };

          // On locale A with params
          const resultA = simulateT(dictA, key, params);
          // On locale B with params
          const resultB = simulateT(dictB, key, params);

          // Both results should have interpolation applied
          const rawA = resolveKey(dictA, key)!;
          const rawB = resolveKey(dictB, key)!;
          expect(resultA).toBe(interpolate(rawA, params));
          expect(resultB).toBe(interpolate(rawB, params));

          // The raw templates differ, so results should differ
          // (unless interpolation coincidentally produces the same output)
          expect(rawA).not.toBe(rawB);
        }
      ),
      { numRuns: 100 }
    );
  });
});
