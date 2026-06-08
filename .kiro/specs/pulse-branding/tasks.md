# Implementation Plan: Pulse Branding

## Overview

Implement a cohesive visual identity system for Pulse including an ECG-inspired logo mark, wordmark typography, CSS custom properties theme system, light/dark theme switcher, Tailwind integration, self-hosted Inter font, and static brand assets. All work is frontend-only — no backend changes required.

## Tasks

- [x] 1. Set up theme system foundation (CSS custom properties + Tailwind config)
  - [x] 1.1 Define CSS custom properties and theme tokens in `app.css`
    - Replace existing `:root` styles with full light/dark token definitions
    - Add `:root, [data-theme="light"]` block with all Color_Token properties: `--color-brand-primary`, `--color-brand-hover`, `--color-bg-page`, `--color-bg-surface`, `--color-text-primary`, `--color-text-secondary`, `--color-border`, `--color-success`, `--color-warning`, `--color-error`, and extended brand scale (50–900)
    - Add `[data-theme="dark"]` block overriding all tokens with dark palette values
    - Update `body` styles to use `var(--color-bg-page)` and `var(--color-text-primary)` instead of hardcoded Tailwind slate classes
    - _Requirements: 4.1–4.8, 5.1–5.7, 11.3_

  - [x] 1.2 Update Tailwind configuration with theme-aware color utilities
    - Set `darkMode: ['selector', '[data-theme="dark"]']` in `tailwind.config.cjs`
    - Extend `colors.brand` to use CSS variable references with fallbacks for shades 50–900
    - Add semantic color aliases (`success`, `warning`, `error`) referencing CSS custom properties
    - Add `fontFamily.brand` with Inter + system-ui fallback stack
    - _Requirements: 11.1, 11.2, 11.4, 11.5_

  - [x] 1.3 Add FOUC prevention inline script to `app.html`
    - Insert inline `<script>` in `<head>` before `%sveltekit.head%` that reads `localStorage('pulse-theme')`, validates value, falls back to `prefers-color-scheme`, and sets `data-theme` attribute on `<html>` element
    - _Requirements: 6.3, 6.4, 6.7_

- [x] 2. Implement brand components
  - [x] 2.1 Create `BrandLockup.svelte` component
    - Create `frontend/src/components/BrandLockup.svelte` with TypeScript props interface (`size?: number`, `variant?: 'full' | 'compact'`)
    - Render inline SVG Logo_Mark with `viewBox="0 0 32 32"`, single stroked ECG peak path, `stroke="currentColor"`, `fill="none"`, `stroke-width="3.2"`, rounded linecap/linejoin
    - Wrap SVG in container setting `color: var(--color-brand-primary, #0ea5e9)`
    - When `variant="full"`, render adjacent `<span>` wordmark "Pulse" with Inter semi-bold, tracking-tight, font-size = `size * 0.625`px
    - Gap between mark and wordmark: `size / 4` pixels
    - All dimensions scale proportionally from `size` prop
    - _Requirements: 1.1–1.7, 2.1–2.6, 3.1–3.5_

  - [x] 2.2 Write property test for Logo_Mark stroke width proportionality
    - **Property 1: Logo_Mark stroke width proportionality**
    - Generate random viewBox dimensions, verify stroke-width is between 0.08×N and 0.12×N
    - **Validates: Requirements 1.4**

  - [x] 2.3 Write property test for BrandLockup proportional scaling
    - **Property 2: BrandLockup proportional scaling**
    - Generate random positive `size` values, verify gap = S/4, wordmark font-size proportional to S, clear space = S/2
    - **Validates: Requirements 3.1, 3.2, 3.4**

  - [x] 2.4 Create `ThemeSwitcher.svelte` component
    - Create `frontend/src/components/ThemeSwitcher.svelte`
    - Read current theme from `document.documentElement.dataset.theme`
    - On click: toggle `light` ↔ `dark`, update `data-theme` attribute, write to `localStorage('pulse-theme')`
    - Display sun icon when dark theme active, moon icon when light theme active
    - Set `aria-label` describing action: "Switch to light theme" / "Switch to dark theme"
    - Wrap `localStorage.setItem` in try/catch for private browsing `SecurityError`
    - _Requirements: 6.1–6.7_

  - [x] 2.5 Write property test for theme toggle round-trip persistence
    - **Property 4: Theme toggle round-trip persistence**
    - Generate random starting states (light/dark), verify toggle sets opposite on `data-theme` and persists same value to localStorage
    - **Validates: Requirements 6.1, 6.2**

  - [x] 2.6 Write property test for theme icon indicates target theme
    - **Property 5: Theme icon indicates target theme**
    - Generate random active theme values, verify sun icon when dark is active, moon icon when light is active
    - **Validates: Requirements 6.6**

- [x] 3. Checkpoint — Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Self-hosted font and static brand assets
  - [x] 4.1 Add self-hosted Inter font
    - Create `frontend/static/fonts/` directory
    - Add `inter-semibold.woff2` font file
    - Add `@font-face` declaration in `app.css` with `font-display: swap`
    - _Requirements: 2.1, 2.4, 2.5, 2.6_

  - [x] 4.2 Create static brand asset files
    - Create `frontend/static/brand/` directory
    - Add `logo-mark.svg` — standalone ECG peak SVG with `stroke="#0ea5e9"`
    - Add `brand-lockup.svg` — full lockup SVG for light backgrounds
    - Add `brand-lockup-dark.svg` — lockup variant with white stroke/text for dark surfaces
    - Add `logo-mark-1x.png` (64×64), `logo-mark-2x.png` (128×128), `logo-mark-4x.png` (256×256)
    - Add `README.md` with usage guidelines
    - _Requirements: 10.1–10.6_

  - [x] 4.3 Add favicon and browser metadata
    - Add `frontend/static/favicon.png` (32×32 PNG)
    - Add `frontend/static/apple-touch-icon.png` (180×180 PNG)
    - Add `frontend/static/icon-192.png` and `frontend/static/icon-512.png` for PWA
    - Create `frontend/static/site.webmanifest` with name "Pulse", icons, theme_color, background_color
    - Update `app.html` `<head>` with `<link rel="icon">`, `<link rel="apple-touch-icon">`, and `<link rel="manifest">`
    - _Requirements: 9.1–9.6_

- [x] 5. Integrate brand into application layout
  - [x] 5.1 Replace header text with BrandLockup in `+layout.svelte`
    - Replace the plain-text "Pulse" `<a>` link with `<BrandLockup>` component
    - Set `size={32}`, link to `/`, add `aria-label="Pulse — Home"`
    - Show `variant="full"` when viewport > 640px, `variant="compact"` at ≤ 640px (use Tailwind responsive classes or media query)
    - _Requirements: 7.1–7.6_

  - [x] 5.2 Add ThemeSwitcher to navigation header
    - Place `ThemeSwitcher` in the header `<nav>` alongside ConnectionBadge and Logout button
    - _Requirements: 6.5_

  - [x] 5.3 Add BrandLockup to login and setup pages
    - Update `frontend/src/routes/login/+page.svelte` — add centered `<BrandLockup size={48} variant="full">` above the form with 24px bottom margin
    - Update `frontend/src/routes/setup/+page.svelte` — same treatment
    - Add `max-width: 100%` with `height: auto` for viewport overflow scaling
    - _Requirements: 8.1–8.4_

  - [x] 5.4 Update existing layout/page styles to use theme tokens
    - Replace hardcoded slate/sky color classes in `+layout.svelte` header with theme-aware utilities (`bg-[var(--color-bg-surface)]`, `border-[var(--color-border)]`, etc.)
    - Ensure gradient/background uses page background token
    - _Requirements: 4.3, 4.4, 4.6, 5.3, 5.6_

- [x] 6. Checkpoint — Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Testing and validation
  - [x] 7.1 Write unit tests for BrandLockup component
    - Test `variant="full"` renders SVG + wordmark span
    - Test `variant="compact"` renders SVG only, no wordmark
    - Test default `size=32` produces correct dimensions
    - Test custom size prop calculates correct gap and font-size
    - Test SVG has correct attributes (`viewBox`, `stroke`, `fill`, `stroke-width`)
    - Test accessible `aria-label` on wrapper link when used as link
    - _Requirements: 1.1–1.7, 3.1–3.5_

  - [x] 7.2 Write unit tests for ThemeSwitcher component
    - Test initial render reads theme from `document.documentElement.dataset.theme`
    - Test click toggles `data-theme` attribute between light and dark
    - Test click persists theme to localStorage under key `pulse-theme`
    - Test displays sun icon when dark theme active
    - Test displays moon icon when light theme active
    - Test `aria-label` values for both states
    - Test graceful handling when localStorage is unavailable
    - _Requirements: 6.1–6.7_

  - [x] 7.3 Write property test for dark theme WCAG contrast compliance
    - **Property 3: Dark theme WCAG contrast compliance**
    - Compute relative luminance for all dark theme foreground colors against `#0f172a` background
    - Verify each achieves minimum 4.5:1 contrast ratio per WCAG 2.1
    - **Validates: Requirements 5.5, 5.7**

  - [x] 7.4 Write property test for Tailwind brand scale token mapping
    - **Property 6: Tailwind brand scale token mapping**
    - For each shade in {50, 100, 200, 300, 400, 500, 600, 700, 800, 900}, verify the Tailwind config brand color resolves to `var(--color-brand-{shade})` with correct fallback hex
    - **Validates: Requirements 11.1, 11.3**

- [x] 8. Final checkpoint — Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The design specifies TypeScript (Svelte 5 + SvelteKit) — all code uses TypeScript strict mode
- Static brand asset PNG files will need to be generated from the SVG (can use a build script or manual export)
- The Inter font WOFF2 file must be obtained from the official Inter release

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3", "4.1"] },
    { "id": 2, "tasks": ["2.1", "2.4", "4.2", "4.3"] },
    { "id": 3, "tasks": ["2.2", "2.3", "2.5", "2.6", "5.1", "5.2", "5.3"] },
    { "id": 4, "tasks": ["5.4"] },
    { "id": 5, "tasks": ["7.1", "7.2", "7.3", "7.4"] }
  ]
}
```
