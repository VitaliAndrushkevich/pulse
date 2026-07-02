/**
 * Nested string dictionary with max 4 levels of nesting.
 * Leaf values are translation strings, possibly containing {variable} placeholders.
 */
export type TranslationDictionary = {
  [key: string]: string | TranslationDictionary;
};
