/**
 * Property-based tests for locale store fallback chain logic.
 *
 * Since locale.svelte.ts uses Svelte 5 runes ($state, $effect) which cannot be
 * imported in test files, we test the fallback chain by directly exercising the
 * pure resolveKey and interpolate functions from resolve.ts — these implement
 * the exact same logic that t() uses internally.
 *
 * The fallback chain: activeDictionary → enDictionary → key string
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { resolveKey, interpolate } from '../resolve';
import type { TranslationDictionary } from '../types';
import enDictionary from '../../../locales/en.json';

/**
 * Simulate the t() fallback chain exactly as implemented in locale.svelte.ts:
 * 1. Look up key in activeDictionary
 * 2. If not found, look up key in enDictionary
 * 3. If not found in either, return the key string itself
 * 4. If found, apply interpolate(value, params)
 */
function simulateT(
  activeDictionary: TranslationDictionary,
  enDict: TranslationDictionary,
  key: string,
  params?: Record<string, string | number>
): string {
  let value = resolveKey(activeDictionary, key);

  if (value === undefined) {
    value = resolveKey(enDict, key);
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

// Collect all keys from the real en.json dictionary
const enKeys = collectKeys(enDictionary as TranslationDictionary);

// Feature: i18n-localization, Property 2: Fallback chain resolves to English for missing keys
describe('Property 2: Fallback chain resolves to English for missing keys', () => {
  /**
   * Validates: Requirements 3.1, 6.3, 7.5
   *
   * For any dot-notation key that exists in the English dictionary but does NOT
   * exist in the active locale's dictionary, the t() function SHALL return the
   * English translation string (with interpolation applied) rather than undefined,
   * an empty string, or the key itself.
   */

  /** Arbitrary generator that picks a random key from en.json */
  const arbitraryEnKey = fc.constantFrom(...enKeys);

  /** Generate a segment name for building partial dictionaries */
  const arbitrarySegment = fc.stringMatching(/^[a-z]{1,8}$/);

  it('returns the English value when key is missing from active dictionary', () => {
    fc.assert(
      fc.property(arbitraryEnKey, (key) => {
        // Use an empty dictionary as the "active" locale — key won't be found there
        const activeDictionary: TranslationDictionary = {};
        const enDict = enDictionary as TranslationDictionary;

        const result = simulateT(activeDictionary, enDict, key);
        const expectedValue = resolveKey(enDict, key);

        // The key exists in en.json, so resolveKey must return a string
        expect(expectedValue).toBeDefined();
        expect(typeof expectedValue).toBe('string');

        // The fallback must return the English value, not the key itself
        expect(result).toBe(expectedValue);
        expect(result).not.toBe(key); // Unless value === key by coincidence, check not undefined
        expect(result).not.toBe(undefined);
        expect(result).not.toBe('');
      }),
      { numRuns: 100 }
    );
  });

  it('applies interpolation to the English fallback value', () => {
    fc.assert(
      fc.property(
        fc.stringMatching(/^[a-z]{2,5}$/),
        fc.stringMatching(/^[a-z]{2,5}$/),
        fc.integer({ min: 0, max: 9999 }),
        (ns, key, count) => {
          fc.pre(ns !== key);
          const dotKey = `${ns}.${key}`;

          // Build an en dictionary with an interpolated value
          const enDict: TranslationDictionary = {
            [ns]: { [key]: '{count} items found' },
          };

          // Active dictionary is empty — key won't be found there
          const activeDictionary: TranslationDictionary = {};

          const result = simulateT(activeDictionary, enDict, dotKey, { count });

          // Should interpolate the English fallback value with the provided params
          expect(result).toBe(`${count} items found`);
          // Must NOT return the key itself
          expect(result).not.toBe(dotKey);
          // Must NOT return undefined or empty
          expect(result).not.toBe('');
        }
      ),
      { numRuns: 100 }
    );
  });

  it('returns the English value when active dictionary has unrelated keys only', () => {
    fc.assert(
      fc.property(
        arbitraryEnKey,
        arbitrarySegment,
        fc.string({ minLength: 1, maxLength: 20 }),
        (enKey, unrelatedKey, unrelatedValue) => {
          // Ensure the unrelated key doesn't accidentally match a real en.json path
          fc.pre(!enKeys.includes(unrelatedKey));

          // Build an active dictionary with only unrelated keys
          const activeDictionary: TranslationDictionary = {
            [unrelatedKey]: unrelatedValue,
          };
          const enDict = enDictionary as TranslationDictionary;

          const result = simulateT(activeDictionary, enDict, enKey);
          const expectedValue = resolveKey(enDict, enKey);

          expect(result).toBe(expectedValue);
        }
      ),
      { numRuns: 100 }
    );
  });
});

// Feature: i18n-localization, Property 3: Terminal fallback returns the key string
describe('Property 3: Terminal fallback returns the key string', () => {
  /**
   * Validates: Requirements 1.5, 3.2
   *
   * For any dot-notation key that does NOT exist in either the active locale's
   * dictionary or the English dictionary, the t() function SHALL return the key
   * string itself unmodified.
   */

  /** Generate dot-notation keys that definitely don't exist in en.json */
  const arbitraryNonExistentKey = fc
    .array(fc.stringMatching(/^[a-z]{2,6}$/), { minLength: 1, maxLength: 4 })
    .map((segments) => segments.join('.'))
    .filter((key) => {
      // Ensure the generated key doesn't exist in en.json
      return resolveKey(enDictionary as TranslationDictionary, key) === undefined;
    });

  it('returns the key string when key is missing from both active and English dictionaries', () => {
    fc.assert(
      fc.property(arbitraryNonExistentKey, (key) => {
        const activeDictionary: TranslationDictionary = {};
        const enDict = enDictionary as TranslationDictionary;

        const result = simulateT(activeDictionary, enDict, key);

        // Terminal fallback: return the key itself unmodified
        expect(result).toBe(key);
      }),
      { numRuns: 100 }
    );
  });

  it('returns the key string unmodified even when active dictionary has other keys', () => {
    fc.assert(
      fc.property(
        arbitraryNonExistentKey,
        fc.string({ minLength: 1, maxLength: 20 }),
        (key, someValue) => {
          // Active dictionary has content but not the key we're looking for
          const activeDictionary: TranslationDictionary = {
            unrelated: { stuff: someValue },
          };
          const enDict = enDictionary as TranslationDictionary;

          const result = simulateT(activeDictionary, enDict, key);

          // Terminal fallback: return key unchanged
          expect(result).toBe(key);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('returns the key string without applying interpolation patterns', () => {
    fc.assert(
      fc.property(
        arbitraryNonExistentKey,
        fc.integer({ min: 0, max: 9999 }),
        (key, paramValue) => {
          const activeDictionary: TranslationDictionary = {};
          const enDict = enDictionary as TranslationDictionary;

          // Even if params are provided, if the key is missing everywhere,
          // the raw key string is returned (no interpolation on the key itself)
          const result = simulateT(activeDictionary, enDict, key, {
            count: paramValue,
          });

          expect(result).toBe(key);
        }
      ),
      { numRuns: 100 }
    );
  });
});
