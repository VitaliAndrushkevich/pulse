/**
 * Unit tests for the ThemeSwitcher component.
 *
 * Validates Requirements 6.1–6.7:
 * - Reads initial theme from document root data-theme attribute
 * - Toggles between light and dark on click
 * - Persists theme to localStorage under 'pulse-theme'
 * - Displays correct icons for each theme state
 * - Provides accessible aria-label values
 * - Handles localStorage unavailability gracefully
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import ThemeSwitcher from '../ThemeSwitcher.svelte';

describe('ThemeSwitcher', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute('data-theme');
  });

  afterEach(() => {
    cleanup();
    localStorage.clear();
    document.documentElement.removeAttribute('data-theme');
  });

  describe('initial render', () => {
    it('reads theme from document.documentElement.dataset.theme', () => {
      document.documentElement.dataset.theme = 'dark';

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      // When dark theme is active, aria-label should indicate switch to light
      expect(button?.getAttribute('aria-label')).toBe('Switch to light theme');
    });

    it('defaults to light theme when no data-theme attribute is set', () => {
      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      // Default is light, so aria-label should indicate switch to dark
      expect(button?.getAttribute('aria-label')).toBe('Switch to dark theme');
    });
  });

  describe('toggle behavior', () => {
    it('toggles data-theme attribute from light to dark on click', () => {
      document.documentElement.setAttribute('data-theme', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click();

      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });

    it('toggles data-theme attribute from dark to light on click', () => {
      document.documentElement.setAttribute('data-theme', 'dark');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click();

      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });

    it('persists theme to localStorage under key pulse-theme', () => {
      document.documentElement.setAttribute('data-theme', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click();

      expect(localStorage.getItem('pulse-theme')).toBe('dark');
    });

    it('persists correct value after multiple toggles', () => {
      document.documentElement.setAttribute('data-theme', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click();
      expect(localStorage.getItem('pulse-theme')).toBe('dark');

      button.click();
      expect(localStorage.getItem('pulse-theme')).toBe('light');
    });
  });

  describe('icon display', () => {
    it('displays sun icon when dark theme is active', () => {
      document.documentElement.dataset.theme = 'dark';

      const { container } = render(ThemeSwitcher);

      // Sun icon has a <circle> element
      const circle = container.querySelector('svg circle');
      expect(circle).not.toBeNull();

      // Should not have the moon crescent path
      const moonPath = container.querySelector('svg path[d*="12.79"]');
      expect(moonPath).toBeNull();
    });

    it('displays moon icon when light theme is active', () => {
      document.documentElement.dataset.theme = 'light';

      const { container } = render(ThemeSwitcher);

      // Moon icon has a <path> with the crescent d attribute
      const moonPath = container.querySelector('svg path[d*="12.79"]');
      expect(moonPath).not.toBeNull();

      // Should not have the sun circle
      const circle = container.querySelector('svg circle');
      expect(circle).toBeNull();
    });
  });

  describe('aria-label accessibility', () => {
    it('has aria-label "Switch to light theme" when dark theme is active', () => {
      document.documentElement.dataset.theme = 'dark';

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      expect(button?.getAttribute('aria-label')).toBe('Switch to light theme');
    });

    it('has aria-label "Switch to dark theme" when light theme is active', () => {
      document.documentElement.dataset.theme = 'light';

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      expect(button?.getAttribute('aria-label')).toBe('Switch to dark theme');
    });

    it('updates aria-label after toggle', async () => {
      document.documentElement.dataset.theme = 'light';

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click();

      // Svelte 5 reactivity flushes asynchronously in jsdom
      await new Promise((resolve) => setTimeout(resolve, 0));

      expect(button.getAttribute('aria-label')).toBe('Switch to light theme');
    });
  });

  describe('localStorage unavailable', () => {
    it('handles SecurityError gracefully when localStorage is unavailable', () => {
      document.documentElement.setAttribute('data-theme', 'light');

      // Mock localStorage.setItem to throw SecurityError
      const setItemSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new DOMException('Access denied', 'SecurityError');
      });

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      // Should not throw — toggle still works for the session
      expect(() => button.click()).not.toThrow();

      // data-theme should still be updated (session-only theming)
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');

      setItemSpy.mockRestore();
    });
  });
});
