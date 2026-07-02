import type { TranslationDictionary } from './types';
import type { LocaleCode } from './config';
import { isSupportedLocale, FALLBACK_LOCALE, STORAGE_KEY, SUPPORTED_LOCALES } from './config';
import { resolveKey, interpolate } from './resolve';
import enDictionary from '../../locales/en.json';

// --- Reactive State ---
let currentLocale = $state<LocaleCode>(FALLBACK_LOCALE);
let activeDictionary = $state<TranslationDictionary>(enDictionary as TranslationDictionary);
let isLoading = $state<boolean>(false);
let loadError = $state<string | null>(null);

// --- Internal helpers ---

function syncHtmlLang(code: LocaleCode): void {
	if (typeof document !== 'undefined') {
		document.documentElement.lang = code;
		const entry = SUPPORTED_LOCALES.find((l) => l.code === code);
		document.documentElement.dir = entry?.dir ?? 'ltr';
	}
}

function persistLocale(code: LocaleCode): void {
	try {
		localStorage.setItem(STORAGE_KEY, code);
	} catch {
		// localStorage unavailable — locale works for session but won't persist
	}
}

// --- Public API ---

/** Current active locale code */
export function getLocale(): LocaleCode {
	return currentLocale;
}

/**
 * Translation function.
 * Fallback chain: activeDictionary → enDictionary → key string.
 * Applies interpolation on resolved value.
 */
export function t(key: string, params?: Record<string, string | number>): string {
	// Step 1: Try active dictionary
	let value = resolveKey(activeDictionary, key);

	// Step 2: Fallback to English dictionary
	if (value === undefined) {
		if (import.meta.env.DEV && currentLocale !== FALLBACK_LOCALE) {
			console.warn(`[i18n] Missing key "${key}" in locale "${currentLocale}"`);
		}
		value = resolveKey(enDictionary as TranslationDictionary, key);
	}

	// Step 3: Terminal fallback — return key itself
	if (value === undefined) {
		return key;
	}

	return interpolate(value, params);
}

/**
 * Change active locale. Triggers lazy load for non-en locales.
 * Persists choice to localStorage and syncs document.documentElement.lang.
 */
export async function setLocale(code: LocaleCode): Promise<void> {
	if (code === currentLocale) {
		return;
	}

	if (code === FALLBACK_LOCALE) {
		activeDictionary = enDictionary as TranslationDictionary;
		currentLocale = code;
		persistLocale(code);
		syncHtmlLang(code);
		return;
	}

	// Dynamic import for non-en locales
	isLoading = true;
	loadError = null;

	try {
		const module = await import(`../../locales/${code}.json`);
		activeDictionary = module.default as TranslationDictionary;
		currentLocale = code;
		persistLocale(code);
		syncHtmlLang(code);
	} catch (err) {
		loadError = err instanceof Error ? err.message : 'Failed to load locale';
	} finally {
		isLoading = false;
	}
}

/**
 * Initialize locale from localStorage.
 * Call once at app startup before first render.
 */
export function initLocale(): void {
	let stored: string | null = null;

	try {
		stored = localStorage.getItem(STORAGE_KEY);
	} catch {
		// localStorage unavailable — use fallback
		return;
	}

	if (!stored) {
		return;
	}

	if (!isSupportedLocale(stored)) {
		// Invalid locale — remove and use fallback
		try {
			localStorage.removeItem(STORAGE_KEY);
		} catch {
			// Ignore write errors
		}
		return;
	}

	// Apply the stored locale
	setLocale(stored);
}

/** Whether a locale chunk is currently loading */
export function isLocaleLoading(): boolean {
	return isLoading;
}

/** Current load error message, if any */
export function getLoadError(): string | null {
	return loadError;
}
