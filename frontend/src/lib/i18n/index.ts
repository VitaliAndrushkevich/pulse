// Barrel export for i18n module

// Locale store API
export { t, setLocale, getLocale, initLocale, isLocaleLoading, getLoadError } from './locale.svelte';

// Configuration
export { SUPPORTED_LOCALES, FALLBACK_LOCALE, isSupportedLocale } from './config';
export type { LocaleCode } from './config';
