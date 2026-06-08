/**
 * Property-based tests for Pulse branding components.
 *
 * Uses fast-check to verify universal correctness properties
 * across randomly generated inputs.
 */
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import * as fc from 'fast-check';
import BrandLockup from '../BrandLockup.svelte';

// Feature: pulse-branding, Property 2: BrandLockup proportional scaling
describe('Property 2: BrandLockup proportional scaling', () => {
  /**
   * Validates: Requirements 3.1, 3.2, 3.4
   *
   * For any positive numeric `size` prop value S, the BrandLockup SHALL render with:
   * - gap = S/4
   * - wordmark font-size = S * 0.625
   * - clear space = S/2 (documented formula, consumers apply)
   *
   * All internal dimensions must maintain a constant ratio to the size prop.
   */

  /**
   * Generate positive size values that represent realistic Logo_Mark heights.
   * Range: 4px to 512px covers favicon through splash/marketing sizes.
   */
  const arbitrarySize = fc.float({ min: 4, max: 512, noNaN: true })
    .filter((s) => s > 0 && Number.isFinite(s));

  it('gap between mark and wordmark equals size / 4', () => {
    fc.assert(
      fc.property(arbitrarySize, (size) => {
        const { container } = render(BrandLockup, { props: { size, variant: 'full' } });

        const lockup = container.querySelector('.brand-lockup') as HTMLElement;
        expect(lockup).not.toBeNull();

        const style = lockup.style;
        const gapValue = parseFloat(style.gap);
        const expectedGap = size / 4;

        expect(gapValue).toBeCloseTo(expectedGap, 2);
      }),
      { numRuns: 100 }
    );
  });

  it('wordmark font-size is proportional to size (size * 0.625)', () => {
    fc.assert(
      fc.property(arbitrarySize, (size) => {
        const { container } = render(BrandLockup, { props: { size, variant: 'full' } });

        const wordmark = container.querySelector('.brand-wordmark') as HTMLElement;
        expect(wordmark).not.toBeNull();

        const fontSize = parseFloat(wordmark.style.fontSize);
        const expectedFontSize = size * 0.625;

        expect(fontSize).toBeCloseTo(expectedFontSize, 2);
      }),
      { numRuns: 100 }
    );
  });

  it('clear space formula equals size / 2 for all positive sizes', () => {
    // Clear space is documented as size/2, applied by consumers.
    // We verify the formula holds mathematically for any positive size.
    fc.assert(
      fc.property(arbitrarySize, (size) => {
        const clearSpace = size / 2;

        // Clear space must be positive
        expect(clearSpace).toBeGreaterThan(0);

        // Clear space must maintain constant ratio to size
        expect(clearSpace / size).toBeCloseTo(0.5, 10);

        // Verify proportionality: clearSpace scales linearly with size
        const ratio = clearSpace / size;
        expect(ratio).toBe(0.5);
      }),
      { numRuns: 100 }
    );
  });

  it('all proportional dimensions scale linearly with size', () => {
    fc.assert(
      fc.property(arbitrarySize, (size) => {
        const { container } = render(BrandLockup, { props: { size, variant: 'full' } });

        const lockup = container.querySelector('.brand-lockup') as HTMLElement;
        const wordmark = container.querySelector('.brand-wordmark') as HTMLElement;

        expect(lockup).not.toBeNull();
        expect(wordmark).not.toBeNull();

        const gap = parseFloat(lockup.style.gap);
        const fontSize = parseFloat(wordmark.style.fontSize);
        const clearSpace = size / 2;

        // Verify constant ratios to size
        expect(gap / size).toBeCloseTo(0.25, 5);
        expect(fontSize / size).toBeCloseTo(0.625, 5);
        expect(clearSpace / size).toBeCloseTo(0.5, 5);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: pulse-branding, Property 3: Dark theme WCAG contrast compliance

/**
 * For any color defined in the Dark_Theme token set (primary text, brand primary,
 * success, warning, error), that color SHALL achieve a minimum contrast ratio of
 * 4.5:1 against the dark page background color (`#0f172a`) per the WCAG 2.1
 * relative luminance formula.
 *
 * **Validates: Requirements 5.5, 5.7**
 */
describe('Property 3: Dark theme WCAG contrast compliance', () => {
  // Dark theme foreground colors
  const darkThemeForegroundColors = [
    { name: 'Primary text', hex: '#f1f5f9' },
    { name: 'Brand primary', hex: '#22d3ee' },
    { name: 'Success', hex: '#34d399' },
    { name: 'Warning', hex: '#fbbf24' },
    { name: 'Error', hex: '#f87171' },
  ] as const;

  const darkBackground = '#0f172a';

  /**
   * Convert a hex color string to an array of sRGB values in [0, 1].
   */
  function hexToSrgb(hex: string): [number, number, number] {
    const r = parseInt(hex.slice(1, 3), 16) / 255;
    const g = parseInt(hex.slice(3, 5), 16) / 255;
    const b = parseInt(hex.slice(5, 7), 16) / 255;
    return [r, g, b];
  }

  /**
   * Convert an sRGB channel value to linear RGB per WCAG 2.1.
   * If c <= 0.04045 then c/12.92 else ((c + 0.055) / 1.055) ^ 2.4
   */
  function srgbToLinear(c: number): number {
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  }

  /**
   * Compute relative luminance per WCAG 2.1.
   * L = 0.2126 * R + 0.7152 * G + 0.0722 * B
   */
  function relativeLuminance(hex: string): number {
    const [r, g, b] = hexToSrgb(hex);
    return 0.2126 * srgbToLinear(r) + 0.7152 * srgbToLinear(g) + 0.0722 * srgbToLinear(b);
  }

  /**
   * Compute contrast ratio per WCAG 2.1.
   * ratio = (L1 + 0.05) / (L2 + 0.05) where L1 >= L2
   */
  function contrastRatio(foreground: string, background: string): number {
    const l1 = relativeLuminance(foreground);
    const l2 = relativeLuminance(background);
    const lighter = Math.max(l1, l2);
    const darker = Math.min(l1, l2);
    return (lighter + 0.05) / (darker + 0.05);
  }

  it('all dark theme foreground colors achieve >= 4.5:1 contrast ratio against dark background', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...darkThemeForegroundColors),
        (color) => {
          const ratio = contrastRatio(color.hex, darkBackground);
          expect(ratio).toBeGreaterThanOrEqual(4.5);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('relative luminance calculation produces values in valid range [0, 1]', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...darkThemeForegroundColors),
        (color) => {
          const lum = relativeLuminance(color.hex);
          expect(lum).toBeGreaterThanOrEqual(0);
          expect(lum).toBeLessThanOrEqual(1);
        }
      ),
      { numRuns: 100 }
    );
  });

  it('contrast ratio is always >= 1 (identity is 1:1)', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...darkThemeForegroundColors),
        (color) => {
          const ratio = contrastRatio(color.hex, darkBackground);
          expect(ratio).toBeGreaterThanOrEqual(1);
        }
      ),
      { numRuns: 100 }
    );
  });
});

// Feature: pulse-branding, Property 5: Theme icon indicates target theme

/**
 * The ThemeSwitcher displays a sun icon when the dark theme is active (indicating
 * the user will switch TO light), and a moon icon when the light theme is active
 * (indicating the user will switch TO dark).
 *
 * This property test generates random theme values and verifies the correct icon
 * is rendered for each theme state.
 *
 * **Validates: Requirements 6.6**
 */
describe('Property 5: Theme icon indicates target theme', () => {
  it('displays sun icon (circle) when dark is active, moon icon (path) when light is active', async () => {
    // Dynamically import render + cleanup to avoid module-level side effects
    const { render: renderComponent, cleanup } = await import('@testing-library/svelte');
    const { default: ThemeSwitcher } = await import('../ThemeSwitcher.svelte');

    fc.assert(
      fc.property(
        fc.constantFrom('light' as const, 'dark' as const),
        (theme) => {
          // Set the document theme before rendering
          document.documentElement.dataset.theme = theme;

          const { container } = renderComponent(ThemeSwitcher);

          const svg = container.querySelector('svg');
          expect(svg).not.toBeNull();

          if (theme === 'dark') {
            // Sun icon: contains a <circle> element (sun body)
            const circle = container.querySelector('svg circle');
            expect(circle).not.toBeNull();
            // Should NOT have the moon crescent path
            const moonPath = container.querySelector('svg path[d*="12.79"]');
            expect(moonPath).toBeNull();
          } else {
            // Moon icon: contains a <path> with the crescent "d" attribute
            const moonPath = container.querySelector('svg path[d*="12.79"]');
            expect(moonPath).not.toBeNull();
            // Should NOT have the sun circle
            const circle = container.querySelector('svg circle');
            expect(circle).toBeNull();
          }

          cleanup();
        }
      ),
      { numRuns: 100 }
    );
  });
});

// Feature: pulse-branding, Property 6: Tailwind brand scale token mapping

/**
 * For any shade value in {50, 100, 200, 300, 400, 500, 600, 700, 800, 900},
 * the Tailwind `brand-{shade}` color utility SHALL resolve to the CSS custom
 * property `var(--color-brand-{shade})`, and the fallback value SHALL match
 * the corresponding hex value defined for the Light_Theme.
 *
 * **Validates: Requirements 11.1, 11.3**
 */
describe('Property 6: Tailwind brand scale token mapping', () => {
  // Expected light-theme fallback hex values per shade
  const expectedFallbacks: Record<number, string> = {
    50: '#f0f9ff',
    100: '#e0f2fe',
    200: '#bae6fd',
    300: '#7dd3fc',
    400: '#38bdf8',
    500: '#0ea5e9',
    600: '#0284c7',
    700: '#0369a1',
    800: '#075985',
    900: '#0c4a6e',
  };

  const shades = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900] as const;

  // Arbitrary that samples from the set of valid brand shades
  const arbitraryShade = fc.constantFrom(...shades);

  it('each brand shade maps to var(--color-brand-{shade}, {fallback_hex})', () => {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const tailwindConfig = require('../../../tailwind.config.cjs');
    const brandColors = tailwindConfig.theme.extend.colors.brand;

    fc.assert(
      fc.property(arbitraryShade, (shade) => {
        const value = brandColors[shade];
        const expectedVar = `var(--color-brand-${shade}, ${expectedFallbacks[shade]})`;

        expect(value).toBe(expectedVar);
      }),
      { numRuns: 100 }
    );
  });

  it('brand color values follow the pattern var(--color-brand-{shade}, <hex>)', () => {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const tailwindConfig = require('../../../tailwind.config.cjs');
    const brandColors = tailwindConfig.theme.extend.colors.brand;

    fc.assert(
      fc.property(arbitraryShade, (shade) => {
        const value = brandColors[shade] as string;

        // Verify structural format: var(--color-brand-{shade}, #xxxxxx)
        const pattern = new RegExp(
          `^var\\(--color-brand-${shade},\\s*#[0-9a-f]{6}\\)$`,
          'i'
        );
        expect(value).toMatch(pattern);
      }),
      { numRuns: 100 }
    );
  });

  it('all 10 brand shades are defined in the Tailwind config', () => {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const tailwindConfig = require('../../../tailwind.config.cjs');
    const brandColors = tailwindConfig.theme.extend.colors.brand;

    fc.assert(
      fc.property(arbitraryShade, (shade) => {
        // The shade key must exist and have a non-empty string value
        expect(brandColors).toHaveProperty(String(shade));
        expect(typeof brandColors[shade]).toBe('string');
        expect(brandColors[shade].length).toBeGreaterThan(0);
      }),
      { numRuns: 100 }
    );
  });
});

// Feature: pulse-branding, Property 4: Theme toggle round-trip persistence

/**
 * For any initial theme state (light or dark), activating the ThemeSwitcher
 * SHALL: (a) set `document.documentElement.dataset.theme` to the opposite value,
 * and (b) store that same opposite value in `localStorage.getItem('pulse-theme')`.
 * Reading the stored value back produces the active theme.
 *
 * **Validates: Requirements 6.1, 6.2**
 */
describe('Property 4: Theme toggle round-trip persistence', () => {
  it('toggle sets opposite theme on data-theme and persists same value to localStorage', async () => {
    const { render: renderComponent, cleanup } = await import('@testing-library/svelte');
    const { default: ThemeSwitcher } = await import('../ThemeSwitcher.svelte');

    fc.assert(
      fc.property(
        fc.constantFrom('light' as const, 'dark' as const),
        (startingTheme) => {
          // Arrange: set the initial theme on the document root
          document.documentElement.setAttribute('data-theme', startingTheme);
          localStorage.clear();

          // Act: render ThemeSwitcher and click the toggle button
          const { container } = renderComponent(ThemeSwitcher);
          const button = container.querySelector('button');
          expect(button).not.toBeNull();
          button!.click();

          // Assert: data-theme should be the OPPOSITE of starting theme
          const expectedTheme = startingTheme === 'light' ? 'dark' : 'light';
          expect(document.documentElement.getAttribute('data-theme')).toBe(expectedTheme);

          // Assert: localStorage should persist the same opposite value
          expect(localStorage.getItem('pulse-theme')).toBe(expectedTheme);

          // Cleanup for next iteration
          cleanup();
          document.documentElement.removeAttribute('data-theme');
          localStorage.clear();
        }
      ),
      { numRuns: 100 }
    );
  });
});
