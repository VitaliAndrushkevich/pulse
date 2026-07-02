export interface LocaleEntry {
  code: string; // BCP-47 tag: "en", "ru", "es", etc.
  name: string; // Native name: "English", "Русский", "Español"
  dir?: 'ltr' | 'rtl'; // Text direction (default: 'ltr')
}

export const SUPPORTED_LOCALES: readonly LocaleEntry[] = [
  { code: 'en', name: 'English' },
  { code: 'ar', name: 'العربية', dir: 'rtl' },
  { code: 'be', name: 'Беларуская' },
  { code: 'de', name: 'Deutsch' },
  { code: 'es', name: 'Español' },
  { code: 'fr', name: 'Français' },
  { code: 'it', name: 'Italiano' },
  { code: 'ja', name: '日本語' },
  { code: 'ko', name: '한국어' },
  { code: 'pt', name: 'Português' },
  { code: 'ru', name: 'Русский' },
  { code: 'tr', name: 'Türkçe' },
  { code: 'zh', name: '中文' },
] as const;

export const FALLBACK_LOCALE = 'en';
export const STORAGE_KEY = 'pulse-locale';

export type LocaleCode = (typeof SUPPORTED_LOCALES)[number]['code'];

export function isSupportedLocale(code: string): code is LocaleCode {
  return SUPPORTED_LOCALES.some((l) => l.code === code);
}
