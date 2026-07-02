import type { TranslationDictionary } from './types';

/**
 * Resolve a dot-notation key from a dictionary.
 * Splits on '.' and walks the nested object. If any intermediate segment
 * resolves to a non-object (string/number/null/undefined) before reaching
 * the final segment, returns undefined.
 */
export function resolveKey(
  dictionary: TranslationDictionary,
  key: string
): string | undefined {
  const segments = key.split('.');
  let current: string | TranslationDictionary = dictionary;

  for (let i = 0; i < segments.length; i++) {
    if (typeof current !== 'object' || current === null) {
      return undefined;
    }

    const segment = segments[i];
    const value: string | TranslationDictionary | undefined = (current as TranslationDictionary)[segment];

    if (i === segments.length - 1) {
      return typeof value === 'string' ? value : undefined;
    }

    if (typeof value !== 'object' || value === null) {
      return undefined;
    }

    current = value;
  }

  return undefined;
}

/**
 * Interpolate {variable} placeholders in a template string.
 * For each match, looks up the key in params. If found, substitutes the value.
 * Unmatched placeholders (no corresponding key in params) remain as literal text.
 */
export function interpolate(
  template: string,
  params?: Record<string, string | number>
): string {
  if (!params) {
    return template;
  }

  return template.replace(/\{(\w+)\}/g, (match, key) => {
    if (key in params) {
      return String(params[key]);
    }
    return match;
  });
}
