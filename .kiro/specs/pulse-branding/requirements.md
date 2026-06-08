# Requirements Document

## Introduction

Pulse is a self-hosted uptime monitoring platform. This feature introduces a cohesive visual identity — logo, color system, typography, and theme switching — that communicates reliability, simplicity, and purpose. The branding uses an ECG heartbeat motif to reinforce the "pulse" concept and positions the product alongside technical monitoring tools like Datadog, Grafana, and New Relic while maintaining a clean, uncluttered aesthetic.

## Glossary

- **Logo_Mark**: The standalone ECG peak icon used as an identifiable symbol for Pulse at any size (favicon through splash screen).
- **Wordmark**: The styled "Pulse" text rendered in the brand typeface, displayed alongside the Logo_Mark.
- **Brand_Lockup**: The combined composition of Logo_Mark and Wordmark as a single unit with defined spacing.
- **Theme_Switcher**: A UI control that allows users to toggle between light and dark color schemes.
- **Light_Theme**: The color scheme using white/slate backgrounds with sky-blue accent colors.
- **Dark_Theme**: The color scheme using dark slate backgrounds with a lighter cyan/electric-blue accent that provides sufficient contrast.
- **Color_Token**: A named CSS custom property representing a semantic color value that changes between themes (e.g., `--color-brand-primary`).
- **Brand_Asset_Set**: The collection of all exported logo files in required formats and sizes.

## Requirements

### Requirement 1: Logo Mark Design

**User Story:** As a user, I want to see a recognizable heartbeat-inspired icon that immediately communicates "uptime monitoring," so that I can identify Pulse at a glance in browser tabs and navigation.

#### Acceptance Criteria

1. THE Logo_Mark SHALL depict a single sharp ECG peak (spike) rendered as an SVG path with a square viewBox (0 0 N N) to ensure uniform scaling.
2. THE Logo_Mark SHALL remain identifiable as a peaked waveform when rendered at 16x16 pixels (favicon size), with no SVG path details smaller than 1 pixel at that resolution.
3. THE Logo_Mark SHALL remain identifiable as a peaked waveform when rendered at 512x512 pixels (splash/marketing size) without visible aliasing or pixelation.
4. THE Logo_Mark SHALL use a single stroke whose width is between 8% and 12% of the viewBox dimension, scaling proportionally with the icon size.
5. THE Logo_Mark SHALL use the brand primary color (the value of Color_Token `--color-brand-primary`) for its stroke, adapting automatically between Light_Theme and Dark_Theme contexts.
6. THE Logo_Mark SHALL use `fill="none"` and render exclusively as a stroked path so that it remains visible on any background that provides sufficient contrast with the brand primary color.
7. IF the Logo_Mark SVG is rendered without CSS Color_Token support, THEN THE Logo_Mark SHALL fall back to the Light_Theme brand primary color (`#0ea5e9`) as the default stroke color.

### Requirement 2: Wordmark Typography

**User Story:** As a user, I want the "Pulse" name rendered in a clean, technical typeface, so that the brand feels professional and aligned with developer tooling.

#### Acceptance Criteria

1. THE Wordmark SHALL use the Inter typeface as the primary brand font.
2. THE Wordmark SHALL use a semi-bold (600) font weight for the logotype.
3. THE Wordmark SHALL use tight letter-spacing (`tracking-tight`, -0.025em) consistent with the existing header style.
4. THE Wordmark SHALL render using a self-hosted font file in WOFF2 format loaded from the `frontend/static/` directory.
5. IF the self-hosted font file fails to load, THEN THE Wordmark SHALL fall back to the system sans-serif font stack while preserving the specified font weight and letter-spacing.
6. THE Wordmark SHALL apply a `font-display: swap` strategy so that text remains visible during font loading.

### Requirement 3: Brand Lockup Composition

**User Story:** As a developer integrating the logo, I want a defined lockup with consistent spacing rules, so that the logo appears uniform across all placement contexts.

#### Acceptance Criteria

1. THE Brand_Lockup SHALL position the Logo_Mark to the left of the Wordmark, vertically center-aligned, with a gap of 8px when the Logo_Mark height is 32px, scaling proportionally at other sizes.
2. THE Brand_Lockup SHALL define a minimum clear space around the composition equal to half the Logo_Mark height on all four sides.
3. THE Brand_Lockup SHALL be provided as a single Svelte component that renders inline SVG and accepts a `variant` prop with values `full` (Logo_Mark + Wordmark) and `compact` (Logo_Mark only), defaulting to `full`.
4. THE Brand_Lockup SHALL accept a numeric `size` prop representing the Logo_Mark height in pixels, defaulting to 32px, and scale all internal dimensions (gap, Wordmark font size, clear space) proportionally to that value.
5. WHEN the `variant` prop is set to `compact`, THE Brand_Lockup SHALL render only the Logo_Mark without the Wordmark or the inter-element gap, preserving the same clear space rules.

### Requirement 4: Light Theme Color Palette

**User Story:** As a user on the light theme, I want a clean white/slate interface with sky-blue accents, so that the UI feels bright, focused, and trustworthy.

#### Acceptance Criteria

1. THE Light_Theme SHALL use `#0ea5e9` (sky-500) as the primary brand accent color for links, active navigation indicators, and primary action buttons.
2. THE Light_Theme SHALL use `#0284c7` (sky-600) as the hover and active state color for any element using the primary brand accent color.
3. THE Light_Theme SHALL use `#f8fafc` (slate-50) as the page background color.
4. THE Light_Theme SHALL use `#ffffff` as the surface background color for cards, panels, and elevated containers.
5. THE Light_Theme SHALL use `#0f172a` (slate-900) as the primary text color and `#475569` (slate-600) as the secondary text color for labels, timestamps, and supporting copy.
6. THE Light_Theme SHALL use `#e2e8f0` (slate-200) as the default border and divider color.
7. THE Light_Theme SHALL define success (`#10b981`), warning (`#f59e0b`), and error (`#ef4444`) semantic colors for monitor status indicators, each meeting a minimum WCAG AA contrast ratio of 3:1 against the surface background color.
8. THE Light_Theme SHALL expose color values through Color_Token CSS custom properties defined on the `:root` selector, including at minimum: `--color-brand-primary`, `--color-brand-hover`, `--color-bg-page`, `--color-bg-surface`, `--color-text-primary`, `--color-text-secondary`, `--color-border`, `--color-success`, `--color-warning`, and `--color-error`.

### Requirement 5: Dark Theme Color Palette

**User Story:** As a user who prefers dark interfaces, I want a dark slate background with a luminous cyan accent, so that I can comfortably monitor systems in low-light environments.

#### Acceptance Criteria

1. THE Dark_Theme SHALL use `#22d3ee` (cyan-400) as the primary brand accent color.
2. THE Dark_Theme SHALL use `#06b6d4` (cyan-500) as the hover/active state for interactive brand elements.
3. THE Dark_Theme SHALL use `#0f172a` (slate-900) as the page background color.
4. THE Dark_Theme SHALL use `#f1f5f9` (slate-100) as the primary text color.
5. THE Dark_Theme SHALL define success (`#34d399`), warning (`#fbbf24`), and error (`#f87171`) semantic colors, each achieving a minimum contrast ratio of 4.5:1 against the page background color (`#0f172a`) per WCAG 2.1 AA.
6. THE Dark_Theme SHALL expose all color values through the same Color_Token CSS custom property names as the Light_Theme, overriding values within a `[data-theme="dark"]` attribute selector.
7. THE Dark_Theme SHALL ensure that the primary text color and the primary brand accent color each achieve a minimum contrast ratio of 4.5:1 against the page background color per WCAG 2.1 AA.

### Requirement 6: Theme Switcher Control

**User Story:** As a user, I want to toggle between light and dark modes, so that I can choose the visual style that suits my environment.

#### Acceptance Criteria

1. WHEN the user activates the Theme_Switcher, THE Theme_Switcher SHALL toggle the active theme between Light_Theme and Dark_Theme by setting the `data-theme` attribute on the document root element to `light` or `dark`, without requiring a page reload.
2. THE Theme_Switcher SHALL persist the selected theme preference in browser localStorage under the key `pulse-theme` using the string value `light` or `dark`.
3. WHEN the application loads and no stored preference exists (or the stored value is neither `light` nor `dark`), THE Theme_Switcher SHALL default to the operating system preference via the `prefers-color-scheme` media query, falling back to Light_Theme if the media query is unsupported.
4. WHEN the application loads and a valid stored preference exists, THE Theme_Switcher SHALL apply the stored preference regardless of the operating system setting.
5. THE Theme_Switcher SHALL be placed in the navigation header alongside existing controls.
6. THE Theme_Switcher SHALL display a sun icon when Dark_Theme is active and a moon icon when Light_Theme is active, indicating the theme the user will switch to upon activation.
7. WHEN the application loads, THE Theme_Switcher SHALL apply the resolved theme before the first contentful paint to prevent a flash of incorrect theme colors.

### Requirement 7: Logo Placement — Navigation Header

**User Story:** As a user navigating the application, I want to see the Pulse logo in the header, so that the brand is consistently visible during use.

#### Acceptance Criteria

1. THE Brand_Lockup SHALL replace the current plain-text "Pulse" link in the navigation header.
2. THE Brand_Lockup SHALL link to the dashboard root path (`/`).
3. WHILE the viewport width is greater than 640px, THE Brand_Lockup SHALL display the full lockup variant (Logo_Mark + Wordmark).
4. WHILE the viewport width is 640px or narrower, THE Brand_Lockup SHALL display only the compact variant (Logo_Mark only).
5. THE Brand_Lockup link SHALL render with an accessible name of "Pulse — Home" so that screen readers identify the link regardless of which visual variant is displayed.
6. THE Brand_Lockup SHALL render at a height of 32px within the navigation header to maintain vertical alignment with adjacent navigation elements.

### Requirement 8: Logo Placement — Login and Setup Pages

**User Story:** As a user arriving at the login or setup page, I want to see the Pulse brand prominently, so that I know I am on the correct application.

#### Acceptance Criteria

1. THE Brand_Lockup SHALL appear horizontally centered above the login form, rendered at 48px height (1.5× the 32px navigation default), with 24px of vertical spacing between the lockup bottom edge and the form top edge.
2. THE Brand_Lockup SHALL appear horizontally centered above the setup form, rendered at 48px height (1.5× the 32px navigation default), with 24px of vertical spacing between the lockup bottom edge and the form top edge.
3. THE Brand_Lockup SHALL use the full lockup variant (Logo_Mark + Wordmark) on login and setup pages regardless of viewport width.
4. WHILE the viewport width is less than the Brand_Lockup rendered width plus the minimum clear space (as defined in Requirement 3 criterion 2), THE Brand_Lockup SHALL scale down proportionally to fit within the viewport while maintaining its aspect ratio.

### Requirement 9: Favicon and Browser Metadata

**User Story:** As a user with multiple browser tabs open, I want a distinctive favicon, so that I can quickly locate the Pulse tab.

#### Acceptance Criteria

1. THE Brand_Asset_Set SHALL include a 32x32 PNG favicon derived from the Logo_Mark.
2. THE Brand_Asset_Set SHALL include a 180x180 Apple Touch Icon derived from the Logo_Mark.
3. THE Brand_Asset_Set SHALL include a `site.webmanifest` file referencing 192x192 and 512x512 PNG icons with purpose "any", and containing `name` set to "Pulse", `short_name` set to "Pulse", `theme_color` set to the Light_Theme primary accent color, and `background_color` set to the Light_Theme page background color.
4. THE application HTML shell SHALL include in the `<head>` element a `<link rel="icon">` referencing the 32x32 favicon, a `<link rel="apple-touch-icon">` referencing the 180x180 icon, and a `<link rel="manifest">` referencing the web manifest file.
5. THE favicon and all PNG icons in the Brand_Asset_Set SHALL render the Logo_Mark in the Light_Theme primary accent color (`#0ea5e9`) on a transparent background.
6. WHEN the `site.webmanifest` file is validated against the W3C Web App Manifest schema, THE manifest SHALL produce no errors for required fields.

### Requirement 10: Brand Asset Export

**User Story:** As a developer or contributor, I want access to logo files in standard formats, so that I can use them in documentation, README, and external materials.

#### Acceptance Criteria

1. THE Brand_Asset_Set SHALL include the Logo_Mark as an SVG file in the `frontend/static/brand/` directory.
2. THE Brand_Asset_Set SHALL include the Brand_Lockup as an SVG file in the `frontend/static/brand/` directory.
3. THE Brand_Asset_Set SHALL include PNG exports of the Logo_Mark at 1x (64x64px), 2x (128x128px), and 4x (256x256px) resolutions.
4. THE Brand_Asset_Set SHALL include a dark-background variant of the Brand_Lockup SVG that uses white stroke and text colors instead of the default dark colors, suitable for placement on dark surfaces.
5. THE Brand_Asset_Set SHALL be organized in a `frontend/static/brand/` subdirectory.
6. THE Brand_Asset_Set SHALL use a consistent naming convention of `{asset}-{variant}.{ext}` (e.g., `logo-mark.svg`, `brand-lockup-dark.svg`, `logo-mark-2x.png`) for all exported files.

### Requirement 11: Tailwind Theme Integration

**User Story:** As a frontend developer, I want the brand colors available as Tailwind utility classes, so that I can use them consistently throughout the codebase without hardcoding hex values.

#### Acceptance Criteria

1. THE Tailwind configuration SHALL define an extended `brand` color scale with shades 50, 100, 200, 300, 400, 500, 600, 700, 800, and 900, where each shade value references the corresponding Color_Token CSS custom property (e.g., `var(--color-brand-50)` through `var(--color-brand-900)`), generating utility classes in the form `bg-brand-{shade}`, `text-brand-{shade}`, and `border-brand-{shade}`.
2. THE Tailwind configuration SHALL support dark mode via the `selector` strategy using `[data-theme="dark"]` as the selector, matching the attribute set by the Theme_Switcher.
3. WHEN a Color_Token CSS custom property value changes due to theme switching (the `data-theme` attribute is added or removed), THE Tailwind utilities referencing those tokens SHALL resolve to the updated color value within the same repaint cycle, without requiring a page reload or JavaScript-driven class swaps.
4. THE Tailwind configuration SHALL define semantic color aliases (`success`, `warning`, `error`) that reference theme-aware Color_Token CSS custom properties, generating utility classes in the form `bg-success`, `text-success`, `bg-warning`, `text-warning`, `bg-error`, and `text-error`.
5. IF a Color_Token CSS custom property referenced by a Tailwind utility is not defined on the current element's ancestry, THEN THE Tailwind utility SHALL fall back to the Light_Theme value for that token.
