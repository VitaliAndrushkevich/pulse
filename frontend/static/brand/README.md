# Pulse Brand Assets

Static brand files for use in documentation, README, external materials, and integrations.

## File Inventory

| File | Description | Dimensions |
|------|-------------|------------|
| `logo-mark.svg` | Standalone ECG peak icon (sky-500 stroke) | 32×32 viewBox |
| `brand-lockup.svg` | Full lockup (mark + wordmark) for light backgrounds | 140×32 viewBox |
| `brand-lockup-dark.svg` | Full lockup for dark backgrounds (white stroke/text) | 140×32 viewBox |
| `logo-mark-1x.png` | Logo mark raster export | 64×64 |
| `logo-mark-2x.png` | Logo mark raster export | 128×128 |
| `logo-mark-4x.png` | Logo mark raster export | 256×256 |

## Usage Guidelines

### When to use which file

- **`logo-mark.svg`** — Favicons, app icons, small UI placements, anywhere the mark stands alone.
- **`brand-lockup.svg`** — README headers, documentation, marketing on light backgrounds.
- **`brand-lockup-dark.svg`** — Same as above but for dark surfaces (dark docs themes, dark slides).
- **`logo-mark-{1x,2x,4x}.png`** — Where SVG is not supported (social previews, email, legacy systems).

### Minimum Clear Space

Maintain a clear space around the logo equal to **half the logo mark height** on all sides.

For the default 32px mark, this means 16px of padding on every side. Scale proportionally at other sizes.

```
┌──────────────────────────────┐
│        16px clear space      │
│   ┌──────────────────────┐   │
│   │   [Logo Mark] Pulse  │   │
│   └──────────────────────┘   │
│        16px clear space      │
└──────────────────────────────┘
```

### Do not

- Distort or stretch the logo non-proportionally
- Change the stroke color in the SVG files (use the provided variants instead)
- Place the light lockup on dark backgrounds or vice versa
- Reduce below 16px height (icon becomes unrecognizable)

## Color Specifications

| Token | Light Theme | Dark Theme |
|-------|-------------|------------|
| Brand primary (logo stroke) | `#0ea5e9` (sky-500) | `#22d3ee` (cyan-400) |
| Brand hover | `#0284c7` (sky-600) | `#06b6d4` (cyan-500) |
| Text (wordmark) | `#0f172a` (slate-900) | `#ffffff` |

### Static SVG Colors

- `logo-mark.svg`: stroke `#0ea5e9`
- `brand-lockup.svg`: stroke `#0ea5e9`, text `#0f172a`
- `brand-lockup-dark.svg`: stroke `#ffffff`, text `#ffffff`

## Regenerating PNGs

PNGs are generated from `logo-mark.svg` using the `sharp` library:

```bash
cd frontend
node scripts/generate-brand-pngs.mjs
```

This produces all three PNG sizes (64×64, 128×128, 256×256) from the source SVG.

## Naming Convention

All files follow the pattern `{asset}-{variant}.{ext}`:

- Asset: `logo-mark` or `brand-lockup`
- Variant: size multiplier (`1x`, `2x`, `4x`) or theme (`dark`)
- Extension: `svg` or `png`
