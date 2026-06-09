/**
 * Unit tests for the ThemeSwitcher component (tri-state: light / dark / system).
 *
 * Validates:
 * - Defaults to system mode (follows OS preference)
 * - Cycles through light → dark → system on click
 * - Persists mode to localStorage under 'pulse-theme-mode'
 * - Backwards compatible with legacy 'pulse-theme' key
 * - Displays correct icons for each mode
 * - Provides accessible aria-label values
 * - Listens to matchMedia changes in system mode
 * - Handles localStorage unavailability gracefully
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import ThemeSwitcher from '../ThemeSwitcher.svelte';

// -- matchMedia mock --------------------------------------------------------

function createMatchMediaMock(prefersDark: boolean) {
  const listeners: Array<(e: MediaQueryListEvent) => void> = [];
  const mql: MediaQueryList = {
    matches: prefersDark,
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addEventListener: (_event: string, handler: any) => {
      listeners.push(handler);
    },
    removeEventListener: (_event: string, handler: any) => {
      const idx = listeners.indexOf(handler);
      if (idx !== -1) listeners.splice(idx, 1);
    },
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => true,
  };
  return { mql, listeners, setPrefersDark: (v: boolean) => { (mql as any).matches = v; } };
}

let matchMediaState: ReturnType<typeof createMatchMediaMock>;

beforeEach(() => {
  matchMediaState = createMatchMediaMock(false);
  vi.stubGlobal('matchMedia', () => matchMediaState.mql);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ---------------------------------------------------------------------------

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

  describe('initial render — system mode (default)', () => {
    it('defaults to system mode when no localStorage keys exist', () => {
      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      // System mode → aria-label says "Switch to light theme" (next in cycle)
      expect(button?.getAttribute('aria-label')).toBe('Switch to light theme');
    });

    it('applies OS light theme when prefers-color-scheme is light', () => {
      matchMediaState.setPrefersDark(false);

      render(ThemeSwitcher);

      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });

    it('applies OS dark theme when prefers-color-scheme is dark', () => {
      matchMediaState.setPrefersDark(true);

      render(ThemeSwitcher);

      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });

    it('shows monitor icon in system mode', () => {
      const { container } = render(ThemeSwitcher);

      // Monitor icon has a <rect> element
      const rect = container.querySelector('svg rect');
      expect(rect).not.toBeNull();
    });
  });

  describe('initial render — from stored mode', () => {
    it('reads pulse-theme-mode = light from localStorage', () => {
      localStorage.setItem('pulse-theme-mode', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      expect(button?.getAttribute('aria-label')).toBe('Switch to dark theme');
      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });

    it('reads pulse-theme-mode = dark from localStorage', () => {
      localStorage.setItem('pulse-theme-mode', 'dark');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      expect(button?.getAttribute('aria-label')).toBe('Switch to system theme');
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });

    it('reads legacy pulse-theme key when pulse-theme-mode is absent', () => {
      localStorage.setItem('pulse-theme', 'dark');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button');

      expect(button?.getAttribute('aria-label')).toBe('Switch to system theme');
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });
  });

  describe('cycle behavior', () => {
    it('cycles light → dark → system → light', async () => {
      localStorage.setItem('pulse-theme-mode', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      // Start: light. Click → dark
      button.click();
      await tick();
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
      expect(button.getAttribute('aria-label')).toBe('Switch to system theme');

      // Click → system
      button.click();
      await tick();
      // System resolves to OS (mocked as light)
      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
      expect(button.getAttribute('aria-label')).toBe('Switch to light theme');

      // Click → light
      button.click();
      await tick();
      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
      expect(button.getAttribute('aria-label')).toBe('Switch to dark theme');
    });

    it('persists mode to pulse-theme-mode in localStorage', () => {
      localStorage.setItem('pulse-theme-mode', 'light');

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      button.click(); // → dark
      expect(localStorage.getItem('pulse-theme-mode')).toBe('dark');
      expect(localStorage.getItem('pulse-theme')).toBe('dark');

      button.click(); // → system
      expect(localStorage.getItem('pulse-theme-mode')).toBe('system');
      // When system, pulse-theme is removed
      expect(localStorage.getItem('pulse-theme')).toBeNull();
    });
  });

  describe('icon display', () => {
    it('displays sun icon when mode is light', () => {
      localStorage.setItem('pulse-theme-mode', 'light');

      const { container } = render(ThemeSwitcher);

      const circle = container.querySelector('svg circle');
      expect(circle).not.toBeNull();
    });

    it('displays moon icon when mode is dark', () => {
      localStorage.setItem('pulse-theme-mode', 'dark');

      const { container } = render(ThemeSwitcher);

      const moonPath = container.querySelector('svg path[d*="12.79"]');
      expect(moonPath).not.toBeNull();
    });

    it('displays monitor icon when mode is system', () => {
      // Default (no stored mode) = system
      const { container } = render(ThemeSwitcher);

      const rect = container.querySelector('svg rect');
      expect(rect).not.toBeNull();
    });
  });

  describe('system mode reactivity', () => {
    it('updates theme when OS preference changes in system mode', async () => {
      // Start in system mode, OS = light
      matchMediaState.setPrefersDark(false);

      render(ThemeSwitcher);
      expect(document.documentElement.getAttribute('data-theme')).toBe('light');

      // Simulate OS switching to dark
      matchMediaState.setPrefersDark(true);
      // Fire the listener
      for (const handler of matchMediaState.listeners) {
        handler({ matches: true } as MediaQueryListEvent);
      }

      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });
  });

  describe('localStorage unavailable', () => {
    it('handles SecurityError gracefully when localStorage is unavailable', () => {
      localStorage.setItem('pulse-theme-mode', 'light');

      const setItemSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new DOMException('Access denied', 'SecurityError');
      });

      const { container } = render(ThemeSwitcher);
      const button = container.querySelector('button')!;

      // Should not throw — toggle still works for the session
      expect(() => button.click()).not.toThrow();

      // data-theme should still be updated
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');

      setItemSpy.mockRestore();
    });
  });
});

// Helper: wait for Svelte reactivity flush
function tick(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}
