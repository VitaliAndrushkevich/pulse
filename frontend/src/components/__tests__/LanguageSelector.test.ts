/**
 * Unit tests for the LanguageSelector component.
 *
 * Validates:
 * - Label element exists with correct `for` attribute (Req 5.5)
 * - Select has correct `aria-labelledby` attribute (Req 5.5)
 * - All 11 locale options are rendered with correct values (Req 5.3, 5.4)
 * - Selecting a different option calls setLocale with correct code (Req 5.4)
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import LanguageSelector from '../LanguageSelector.svelte';
import { SUPPORTED_LOCALES } from '$lib/i18n/config';

// Mock the locale store module
const mockGetLocale = vi.fn(() => 'en');
const mockSetLocale = vi.fn();
const mockT = vi.fn((key: string) => key);

vi.mock('$lib/i18n/locale.svelte', () => ({
  getLocale: () => mockGetLocale(),
  setLocale: (...args: unknown[]) => mockSetLocale(...args),
  t: (key: string) => mockT(key),
}));

describe('LanguageSelector', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetLocale.mockReturnValue('en');
    mockT.mockImplementation((key: string) => key);
  });

  afterEach(() => {
    cleanup();
  });

  describe('label and accessibility', () => {
    it('renders a label with for="language-select"', () => {
      const { container } = render(LanguageSelector);
      const label = container.querySelector('label');

      expect(label).not.toBeNull();
      expect(label?.getAttribute('for')).toBe('language-select');
    });

    it('renders a label with id="language-select-label"', () => {
      const { container } = render(LanguageSelector);
      const label = container.querySelector('label');

      expect(label?.getAttribute('id')).toBe('language-select-label');
    });

    it('renders select with aria-labelledby="language-select-label"', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('select');

      expect(select).not.toBeNull();
      expect(select?.getAttribute('aria-labelledby')).toBe('language-select-label');
    });

    it('renders select with id="language-select"', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('select');

      expect(select?.getAttribute('id')).toBe('language-select');
    });

    it('renders select with data-testid="language-select"', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('[data-testid="language-select"]');

      expect(select).not.toBeNull();
    });
  });

  describe('locale options', () => {
    it('renders all 11 locale options', () => {
      const { container } = render(LanguageSelector);
      const options = container.querySelectorAll('option');

      expect(options.length).toBe(11);
    });

    it('each option has the correct locale code as value', () => {
      const { container } = render(LanguageSelector);
      const options = container.querySelectorAll('option');

      SUPPORTED_LOCALES.forEach((locale, index) => {
        expect(options[index].getAttribute('value')).toBe(locale.code);
      });
    });

    it('each option displays the native language name', () => {
      const { container } = render(LanguageSelector);
      const options = container.querySelectorAll('option');

      SUPPORTED_LOCALES.forEach((locale, index) => {
        expect(options[index].textContent).toBe(locale.name);
      });
    });

    it('options appear in SUPPORTED_LOCALES config order', () => {
      const { container } = render(LanguageSelector);
      const options = container.querySelectorAll('option');
      const renderedValues = Array.from(options).map((opt) => opt.getAttribute('value'));
      const expectedValues = SUPPORTED_LOCALES.map((l) => l.code);

      expect(renderedValues).toEqual(expectedValues);
    });
  });

  describe('locale selection', () => {
    it('calls setLocale with the selected locale code on change', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('select')!;

      // Simulate selecting Russian
      select.value = 'ru';
      select.dispatchEvent(new Event('change', { bubbles: true }));

      expect(mockSetLocale).toHaveBeenCalledWith('ru');
    });

    it('calls setLocale with "es" when Spanish is selected', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('select')!;

      select.value = 'es';
      select.dispatchEvent(new Event('change', { bubbles: true }));

      expect(mockSetLocale).toHaveBeenCalledWith('es');
    });

    it('calls setLocale with "ja" when Japanese is selected', () => {
      const { container } = render(LanguageSelector);
      const select = container.querySelector('select')!;

      select.value = 'ja';
      select.dispatchEvent(new Event('change', { bubbles: true }));

      expect(mockSetLocale).toHaveBeenCalledWith('ja');
    });
  });
});
