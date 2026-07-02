# Implementation Plan: i18n-localization

## Overview

Implement a custom internationalization system for the Pulse frontend using Svelte 5 runes. The system provides reactive locale switching, lazy-loaded translation dictionaries (code-split per locale via Vite dynamic imports), a `t()` function with interpolation and fallback chain, and a LanguageSelector component in Settings. English is bundled inline; other 10 locales are loaded on demand from static chunks embedded in the Go binary.

## Tasks

- [x] 1. Set up i18n module structure and core types
  - [x] 1.1 Create `frontend/src/lib/i18n/` directory with `config.ts` and `types.ts`
    - Define `LocaleEntry` interface, `SUPPORTED_LOCALES` array (11 locales), `FALLBACK_LOCALE`, `STORAGE_KEY`, `LocaleCode` type, and `isSupportedLocale` guard function in `config.ts`
    - Define `TranslationDictionary` recursive type in `types.ts`
    - _Requirements: 1.1, 6.1, 6.2, 9.2_

  - [x] 1.2 Create `frontend/src/lib/i18n/resolve.ts` with pure resolution functions
    - Implement `resolveKey(dictionary, key)` — walk dot-notation path, return `undefined` if intermediate segment is not an object
    - Implement `interpolate(template, params?)` — replace `{variable}` placeholders using `/\{(\w+)\}/g` regex, leave unmatched placeholders as literal text
    - _Requirements: 1.2, 1.3, 1.5, 1.6, 3.3_

  - [x] 1.3 Write property tests for `resolveKey` and `interpolate`
    - **Property 1: Interpolation substitutes provided variables and preserves unmatched placeholders**
    - **Validates: Requirements 1.3, 1.6, 2.5**
    - **Property 4: Broken path treated as missing**
    - **Validates: Requirements 3.3**

- [x] 2. Implement locale store with fallback chain
  - [x] 2.1 Create `frontend/src/lib/i18n/locale.svelte.ts` reactive store
    - Implement module-level `$state` for `currentLocale`, `activeDictionary`, `isLoading`, `loadError`
    - Implement `t(key, params?)` with 3-step fallback: active dict → en dict → key string; apply `interpolate` on resolved value
    - Implement `setLocale(code)` with dynamic `import()` for non-en locales, localStorage persistence, `document.documentElement.lang` sync
    - Implement `initLocale()` — read localStorage, validate, apply, handle errors
    - Add `$effect` for `document.documentElement.lang` synchronization
    - Add `console.warn` in dev mode for missing keys in non-en locale
    - Export `getLocale`, `t`, `setLocale`, `initLocale`, `isLocaleLoading`, `getLoadError`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.2, 3.3, 3.4, 4.1, 4.2, 4.3, 4.4, 4.5, 8.1, 8.2, 8.3, 10.2, 10.4, 10.5_

  - [x] 2.2 Create `frontend/src/lib/i18n/index.ts` barrel export
    - Re-export public API: `t`, `setLocale`, `getLocale`, `initLocale`, `isLocaleLoading`, `getLoadError` from locale store
    - Re-export `SUPPORTED_LOCALES`, `FALLBACK_LOCALE`, `LocaleCode`, `isSupportedLocale` from config
    - _Requirements: 9.1_

  - [x] 2.3 Write property tests for locale store fallback chain
    - **Property 2: Fallback chain resolves to English for missing keys**
    - **Validates: Requirements 3.1, 6.3, 7.5**
    - **Property 3: Terminal fallback returns the key string**
    - **Validates: Requirements 1.5, 3.2**

  - [x] 2.4 Write property tests for locale persistence and HTML lang sync
    - **Property 5: Locale persistence round-trip**
    - **Validates: Requirements 4.1, 4.2**
    - **Property 6: Invalid stored locale falls back to English**
    - **Validates: Requirements 4.4, 6.4**
    - **Property 7: HTML lang attribute synchronization**
    - **Validates: Requirements 8.1, 8.2**

- [x] 3. Checkpoint - Core i18n logic complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Create English translation dictionary and locale files
  - [x] 4.1 Create `frontend/src/locales/en.json` with full UI coverage
    - Provide translation keys for all routes: Login, Setup, Dashboard, Monitor list/detail/create/edit, Settings
    - Include nav items, buttons, form labels, placeholders, error messages, toast notifications, status text, time-relative descriptions, empty states, parameterized strings
    - Follow key naming convention: top-level feature area → page/sub-feature → element → variant (max 4 levels)
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [x] 4.2 Create stub translation dictionaries for remaining 10 locales
    - Create `frontend/src/locales/{ru,es,fr,pt,de,zh,ja,ko,tr,it}.json` — copy structure from `en.json` with translated strings for core keys (nav, common, settings) and English fallback values for the rest
    - _Requirements: 6.1, 6.3_

- [x] 5. Implement LanguageSelector component and Settings integration
  - [x] 5.1 Create `frontend/src/components/LanguageSelector.svelte`
    - Render a labeled `<select>` dropdown with `id`, `for`, `aria-labelledby` attributes
    - Display each locale option using native name from `SUPPORTED_LOCALES` in config order
    - Bind selected value to `getLocale()`, call `setLocale()` on change
    - Support full keyboard navigation (native select behavior: Tab, Arrow, Enter/Space)
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

  - [x] 5.2 Integrate LanguageSelector into Settings page
    - Add a "Language" section above the existing ApiTokenSection in `frontend/src/routes/settings/+page.svelte`
    - Use `t()` for the section heading and description text
    - _Requirements: 5.1_

  - [x] 5.3 Write property test for LanguageSelector option order
    - **Property 8: Language selector displays native names in config order**
    - **Validates: Requirements 5.2, 5.6**

  - [x] 5.4 Write unit tests for LanguageSelector component
    - Test rendering with correct label, aria attributes, and option values
    - Test that selecting a language calls `setLocale` with correct code
    - _Requirements: 5.3, 5.4, 5.5_

- [x] 6. Wire i18n into application layout and routes
  - [x] 6.1 Call `initLocale()` at application startup in root layout
    - Add initialization call in `frontend/src/routes/+layout.svelte` (or appropriate entry point) so locale is resolved before first render
    - _Requirements: 4.2, 8.2_

  - [x] 6.2 Replace hardcoded strings in navigation and layout components with `t()` calls
    - Update nav items, header text, and shared layout strings
    - _Requirements: 7.1, 7.2_

  - [x] 6.3 Replace hardcoded strings in page routes with `t()` calls
    - Update Login, Setup, Dashboard, Monitor list/detail/create/edit, and Settings pages
    - _Requirements: 7.1, 7.3_

  - [x] 6.4 Replace hardcoded strings in reusable components with `t()` calls
    - Update MonitorRow, MonitorForm, Pagination, Toast, ConnectionBadge, and other shared components
    - _Requirements: 7.2, 7.3_

  - [x] 6.5 Write property test for locale switch updating `t()` output
    - **Property 9: Locale switch updates `t()` output**
    - **Validates: Requirements 2.3**

- [x] 7. Checkpoint - UI integration complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Build-time validation script
  - [x] 8.1 Create `frontend/scripts/validate-locales.ts`
    - Verify each supported locale has a corresponding `.json` file in `src/locales/`
    - Verify all keys in `en.json` exist in other locale files (warn on missing)
    - Verify no key path exceeds 4 levels of nesting
    - Exit with non-zero code on critical failures (missing files, invalid structure)
    - _Requirements: 9.3, 1.2, 7.6_

  - [x] 8.2 Integrate validation into build pipeline
    - Add `"validate-locales": "tsx scripts/validate-locales.ts"` script to `package.json`
    - Update the `"build"` script to run `pnpm validate-locales` before `vite build`
    - _Requirements: 10.1, 9.3_

- [x] 9. Final checkpoint - All tests pass and build validates
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The design specifies TypeScript throughout — all implementation uses TypeScript strict mode
- English dictionary (`en.json`) is bundled inline via static import; other locales use Vite dynamic `import()` for code splitting
- `fast-check` is already available in devDependencies for property-based tests

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["1.3", "2.1"] },
    { "id": 3, "tasks": ["2.2", "2.3", "2.4"] },
    { "id": 4, "tasks": ["4.1"] },
    { "id": 5, "tasks": ["4.2", "5.1", "6.1"] },
    { "id": 6, "tasks": ["5.2", "5.3", "5.4", "6.2"] },
    { "id": 7, "tasks": ["6.3", "6.4"] },
    { "id": 8, "tasks": ["6.5", "8.1"] },
    { "id": 9, "tasks": ["8.2"] }
  ]
}
```
