# Requirements Document

## Introduction

Internationalization (i18n) and localization support for the Pulse web UI. This feature introduces a translation system that allows the entire frontend to render in any of the supported languages, with a user-facing language selector in the Settings page. The language preference persists across sessions via localStorage.

## Glossary

- **I18n_System**: The internationalization module responsible for loading translation dictionaries, resolving translation keys, and providing localized strings to UI components.
- **Language_Selector**: A dropdown UI control in the Settings page that allows the user to choose their preferred display language.
- **Translation_Dictionary**: A structured key-value file containing all translatable strings for a single locale.
- **Locale_Code**: A BCP-47 language tag identifying a supported language (e.g., "en", "ru", "es").
- **Fallback_Locale**: The default locale ("en") used when a translation key is missing from the active Translation_Dictionary.
- **Locale_Store**: A Svelte 5 rune-based reactive store that holds the current locale selection and exposes a translation function to all components.

## Requirements

### Requirement 1: Translation Dictionary Structure

**User Story:** As a developer, I want translation strings organized in structured locale files, so that I can easily add or modify translations for any supported language.

#### Acceptance Criteria

1. THE I18n_System SHALL organize translations as one JSON file per Locale_Code, named `{Locale_Code}.json`, under a dedicated `locales/` directory.
2. THE I18n_System SHALL use dot-notation keys with a maximum depth of 4 levels to namespace translations by feature area (e.g., "settings.language.title").
3. THE I18n_System SHALL support interpolation of dynamic values within translation strings using a `{variable}` placeholder syntax.
4. WHEN a new language is added, THE I18n_System SHALL require only the creation of a new Translation_Dictionary file and the addition of the Locale_Code to the supported locales list.
5. IF a translation key is not found in the active locale's Translation_Dictionary, THEN THE I18n_System SHALL fall back to displaying the key itself as literal text.
6. IF an interpolation placeholder references a variable not provided at runtime, THEN THE I18n_System SHALL render the placeholder unchanged in the output string (e.g., "{variable}" remains visible).

### Requirement 2: Locale Store and Reactive Translation

**User Story:** As a developer, I want a reactive locale store using Svelte 5 runes, so that changing the language updates all visible translated strings without a page reload.

#### Acceptance Criteria

1. THE Locale_Store SHALL expose a reactive `locale` state (implemented as a Svelte 5 `$state` rune) holding the current Locale_Code.
2. THE Locale_Store SHALL expose a `t` function that accepts a dot-notation key and an optional `Record<string, string | number>` of interpolation parameters and returns the resolved localized string with all `{variable}` placeholders replaced by the corresponding parameter values.
3. WHEN the `locale` state changes, THE Locale_Store SHALL cause all components reading the `t` function to display updated translated strings corresponding to the new locale within the same page session, without requiring navigation or a full page reload.
4. THE Locale_Store SHALL load the Translation_Dictionary for the active locale via a static bundled import such that the `t` function returns the fully resolved string on first render without intermediate loading states, empty placeholders, or visible layout shift.
5. IF the `t` function is called with a translation string containing `{variable}` placeholders but the corresponding interpolation parameters are not provided, THEN THE Locale_Store SHALL return the resolved string with unmatched placeholders left as their literal `{variable}` text.

### Requirement 3: Fallback to Default Locale

**User Story:** As a user, I want to see English text for any untranslated strings, so that the UI is never broken regardless of translation completeness.

#### Acceptance Criteria

1. WHEN a translation key is missing from the active Translation_Dictionary, THE I18n_System SHALL resolve the key from the Fallback_Locale ("en") dictionary and apply any interpolation parameters to the resolved string before returning it.
2. IF a translation key is missing from both the active Translation_Dictionary and the Fallback_Locale dictionary, THEN THE I18n_System SHALL return the dot-notation key string unmodified (e.g., "settings.language.title") as the displayed text.
3. IF any intermediate segment of a dot-notation key path does not exist in a Translation_Dictionary (e.g., "settings.language" is not an object), THEN THE I18n_System SHALL treat the entire key as missing from that dictionary and proceed with the fallback chain.
4. WHILE the application is running in development mode, WHEN a translation key is missing from a non-fallback Translation_Dictionary, THE I18n_System SHALL emit a `console.warn` message that includes the missing key and the active Locale_Code.

### Requirement 4: Language Preference Persistence

**User Story:** As a user, I want my language choice to persist across browser sessions, so that I do not have to re-select my preferred language every time I open Pulse.

#### Acceptance Criteria

1. WHEN the user selects a language via the Language_Selector, THE Locale_Store SHALL persist the Locale_Code to localStorage under the key "pulse-locale".
2. WHEN the application initializes, THE Locale_Store SHALL read the persisted Locale_Code from localStorage and apply it as the active locale before the first UI render completes, so that no flash of an incorrect language is visible.
3. IF no persisted Locale_Code exists in localStorage or the stored value is empty, THEN THE Locale_Store SHALL default to the Fallback_Locale ("en").
4. IF the persisted Locale_Code is not in the supported locales list, THEN THE Locale_Store SHALL fall back to the Fallback_Locale ("en") and remove the invalid entry from localStorage.
5. IF localStorage is unavailable or throws an exception on read or write, THEN THE Locale_Store SHALL fall back to the Fallback_Locale ("en") and continue operating without persistence for that session.

### Requirement 5: Language Selector UI

**User Story:** As a user, I want a language selector in the Settings page, so that I can choose my preferred display language from a list of supported languages.

#### Acceptance Criteria

1. THE Language_Selector SHALL appear as a labeled dropdown in the Settings page, grouped in a "Language" section.
2. THE Language_Selector SHALL display each supported language using its native name (e.g., "Русский" for Russian, "日本語" for Japanese).
3. THE Language_Selector SHALL visually indicate the currently active language as the selected option in the dropdown.
4. WHEN the user selects a different language from the Language_Selector, THE Locale_Store SHALL update the active locale within 100ms without requiring a page reload or confirmation dialog.
5. THE Language_Selector SHALL associate a visible `<label>` element with the dropdown control, support full keyboard navigation (Tab to focus, Arrow keys to browse options, Enter/Space to select), and include an `aria-label` or `aria-labelledby` attribute identifying the control's purpose.
6. THE Language_Selector SHALL display language options in the fixed order defined by the supported locales configuration array.

### Requirement 6: Supported Languages

**User Story:** As a user, I want access to multiple languages, so that I can use Pulse in my preferred language.

#### Acceptance Criteria

1. THE I18n_System SHALL provide a Translation_Dictionary for each of the following eleven Locale_Codes: "en", "ru", "es", "fr", "pt", "de", "zh", "ja", "ko", "tr", "it".
2. THE I18n_System SHALL treat "en" (English) as the Fallback_Locale and the default when no preference is stored.
3. WHEN a Translation_Dictionary is incomplete for a given Locale_Code, THE I18n_System SHALL display the Fallback_Locale ("en") value for any key not present in the selected locale's dictionary, while still allowing that locale to be selected.
4. IF the stored locale preference references a Locale_Code not present in the supported list, THEN THE I18n_System SHALL fall back to "en" and allow the user to select a new locale.

### Requirement 7: Full UI Coverage

**User Story:** As a user, I want all visible text in the UI to be translated, so that I have a consistent localized experience throughout the application.

#### Acceptance Criteria

1. THE I18n_System SHALL provide translation keys for all user-visible strings across all page routes: Login, Setup, Dashboard, Monitor list, Monitor detail, Monitor create/edit, and Settings.
2. THE I18n_System SHALL provide translation keys for all reusable component labels including navigation items, buttons, form labels, placeholders, error messages, and toast notifications.
3. THE I18n_System SHALL provide translation keys for status text, time-relative descriptions, empty-state messages, and parameterized strings containing interpolated values such as counts or resource names.
4. THE I18n_System SHALL NOT translate user-generated content such as monitor names, URLs, or secret labels.
5. IF a translation key has no value defined for the active locale, THEN THE I18n_System SHALL render the default-locale value for that key so that no empty string or raw key identifier is displayed to the user.
6. WHEN a build or lint check is executed, THE I18n_System SHALL report any hardcoded user-visible string literal found in Svelte template markup or component script blocks across all routes and reusable components as a coverage violation.

### Requirement 8: HTML Document Language Attribute

**User Story:** As a user relying on assistive technology, I want the HTML document language to reflect my chosen locale, so that screen readers announce content in the correct language.

#### Acceptance Criteria

1. WHEN the active locale changes, THE I18n_System SHALL synchronously update the `lang` attribute on the `<html>` element to the new active Locale_Code before any re-rendered translation content is painted to the screen.
2. WHEN the application initializes, THE I18n_System SHALL set the `lang` attribute on the `<html>` element to the resolved active locale before the first contentful paint, so that screen readers never announce content against a stale or missing language tag.
3. IF the resolved active locale at initialization is the Fallback_Locale ("en"), THEN THE I18n_System SHALL set the `lang` attribute to "en".

### Requirement 9: Extensibility for New Languages

**User Story:** As a contributor, I want a simple process to add a new language, so that the translation system can grow without code changes to the core i18n logic.

#### Acceptance Criteria

1. WHEN a new Translation_Dictionary file is added and its Locale_Code is appended to the supported locales list, THE I18n_System SHALL make the new language available in the Language_Selector without any other code modifications.
2. THE I18n_System SHALL define supported locales in a single configuration array that maps each Locale_Code to its native display name.
3. THE I18n_System SHALL validate at build time that each supported Locale_Code has a corresponding Translation_Dictionary file.

### Requirement 10: Static Build Compatibility

**User Story:** As a developer, I want the i18n system to work within the static SPA build, so that translations are bundled into the output without requiring a runtime server or dynamic imports.

#### Acceptance Criteria

1. THE I18n_System SHALL include all Translation_Dictionary files in the static build output as part of the embedded asset set, so that no runtime fetch requests to an external server are needed to load translations.
2. THE I18n_System SHALL include only the Fallback_Locale ("en") dictionary in the initial JavaScript entry chunk; all other Translation_Dictionary files SHALL be emitted as separate static chunks that are loaded on demand when their Locale_Code is selected.
3. WHILE the application is running offline or embedded in the Go binary, THE I18n_System SHALL resolve all translations — including on-demand locale chunks — from the locally served static build assets without requiring network connectivity to an external host.
4. WHEN the user switches to a locale whose Translation_Dictionary has not yet been loaded, THE I18n_System SHALL load the corresponding static chunk and activate the new locale within 100 milliseconds on a warm cache, displaying the Fallback_Locale strings until the chunk is ready.
5. IF a locale static chunk fails to load, THEN THE I18n_System SHALL continue displaying Fallback_Locale strings for all keys in the requested locale and SHALL present an error indication to the user.
