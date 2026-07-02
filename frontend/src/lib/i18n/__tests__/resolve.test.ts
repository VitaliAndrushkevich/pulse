/**
 * Property-based tests for i18n resolution functions (resolveKey, interpolate).
 *
 * Uses fast-check to verify universal correctness properties
 * across randomly generated inputs.
 */
import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { resolveKey, interpolate } from '../resolve';
import type { TranslationDictionary } from '../types';

// Feature: i18n-localization, Property 1: Interpolation substitutes provided variables and preserves unmatched placeholders
describe('Property 1: Interpolation substitutes provided variables and preserves unmatched placeholders', () => {
  /**
   * Validates: Requirements 1.3, 1.6, 2.5
   *
   * For any template string containing {variable} placeholders and for any
   * Record<string, string | number> of parameters, the interpolate function SHALL
   * replace every placeholder whose key exists in the parameters with the
   * corresponding value, and SHALL leave every placeholder whose key is NOT in the
   * parameters as the literal {variable} text.
   */

  /** Generate valid variable names (alphanumeric + underscore, starting with a letter) */
  const arbitraryVarName = fc.stringMatching(/^[a-zA-Z]\w{0,9}$/);

  /** Generate a param value (string or number) */
  const arbitraryParamValue = fc.oneof(
    fc.string({ minLength: 0, maxLength: 20 }).filter((s) => !s.includes('{') && !s.includes('}')),
    fc.integer({ min: -10000, max: 10000 })
  );

  it('provided variables are replaced with their values', () => {
    fc.assert(
      fc.property(
        arbitraryVarName,
        arbitraryParamValue,
        fc.string({ minLength: 0, maxLength: 10 }).filter((s) => !s.includes('{') && !s.includes('}')),
        fc.string({ minLength: 0, maxLength: 10 }).filter((s) => !s.includes('{') && !s.includes('}')),
        (varName, value, prefix, suffix) => {
          const template = `${prefix}{${varName}}${suffix}`;
          const params = { [varName]: value };
          const result = interpolate(template, params);
          const expected = `${prefix}${String(value)}${suffix}`;
          expect(result).toBe(expected);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('unmatched placeholders remain as literal text', () => {
    fc.assert(
      fc.property(
        arbitraryVarName,
        arbitraryVarName,
        arbitraryParamValue,
        (providedVar, unmatchedVar, value) => {
          // Ensure the unmatched var is different from the provided one
          fc.pre(providedVar !== unmatchedVar);

          const template = `{${providedVar}} and {${unmatchedVar}}`;
          const params = { [providedVar]: value };
          const result = interpolate(template, params);

          // Provided var should be replaced
          expect(result).toContain(String(value));
          // Unmatched var should remain as literal placeholder
          expect(result).toContain(`{${unmatchedVar}}`);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('template with no placeholders returns unchanged', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 0, maxLength: 50 }).filter((s) => !/\{\w+\}/.test(s)),
        arbitraryParamValue,
        (template, value) => {
          const result = interpolate(template, { someKey: value });
          expect(result).toBe(template);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('interpolation with no params returns template unchanged', () => {
    fc.assert(
      fc.property(
        arbitraryVarName,
        fc.string({ minLength: 0, maxLength: 10 }).filter((s) => !s.includes('{') && !s.includes('}')),
        (varName, text) => {
          const template = `${text}{${varName}}${text}`;
          const result = interpolate(template);
          expect(result).toBe(template);
        }
      ),
      { numRuns: 100 }
    );
  });
});

// Feature: i18n-localization, Property 4: Broken path treated as missing
describe('Property 4: Broken path treated as missing', () => {
  /**
   * Validates: Requirements 3.3
   *
   * For any dictionary where an intermediate segment of a dot-notation key path
   * is a string (not an object), the resolveKey function SHALL return undefined,
   * treating the entire key as missing and triggering the fallback chain.
   */

  /** Generate a valid segment name (letters only, short) */
  const arbitrarySegment = fc.stringMatching(/^[a-z]{1,8}$/);

  /** Generate a leaf string value */
  const arbitraryLeafValue = fc.string({ minLength: 1, maxLength: 30 });

  it('returns undefined when intermediate segment is a string value', () => {
    fc.assert(
      fc.property(
        arbitrarySegment,
        arbitrarySegment,
        arbitraryLeafValue,
        (parentKey, childKey, leafValue) => {
          // Build a dict where parentKey is a string (not an object)
          const dictionary: TranslationDictionary = {
            [parentKey]: leafValue,
          };

          // Attempt to resolve parentKey.childKey — should return undefined
          // because parentKey is a string, not a nested object
          const result = resolveKey(dictionary, `${parentKey}.${childKey}`);
          expect(result).toBeUndefined();
        }
      ),
      { numRuns: 100 }
    );
  });

  it('returns undefined when deeply nested intermediate is a string', () => {
    fc.assert(
      fc.property(
        arbitrarySegment,
        arbitrarySegment,
        arbitrarySegment,
        arbitraryLeafValue,
        (level1, level2, level3, leafValue) => {
          // Ensure distinct segments to avoid collision
          fc.pre(level1 !== level2 && level2 !== level3);

          // Build a dict where level1.level2 is a string (blocks further traversal)
          const dictionary: TranslationDictionary = {
            [level1]: {
              [level2]: leafValue, // string, not an object
            },
          };

          // Attempt level1.level2.level3 — should return undefined
          const result = resolveKey(dictionary, `${level1}.${level2}.${level3}`);
          expect(result).toBeUndefined();
        }
      ),
      { numRuns: 100 }
    );
  });

  it('valid paths still resolve correctly (sanity check)', () => {
    fc.assert(
      fc.property(
        arbitrarySegment,
        arbitrarySegment,
        arbitraryLeafValue,
        (level1, level2, leafValue) => {
          fc.pre(level1 !== level2);

          // Build a properly nested dict
          const dictionary: TranslationDictionary = {
            [level1]: {
              [level2]: leafValue,
            },
          };

          const result = resolveKey(dictionary, `${level1}.${level2}`);
          expect(result).toBe(leafValue);
        }
      ),
      { numRuns: 100 }
    );
  });
});
