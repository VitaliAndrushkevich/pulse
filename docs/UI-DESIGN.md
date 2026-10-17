# Pulse UI design

Pulse is an operations tool for people running their own infrastructure. The UI
uses an instrument-panel vocabulary: a fixed blue navigation rail, a quiet
workspace, compact labels, and a clear distinction between actions and status.
The existing ECG mark remains the product's signature.

## Review and direction

The previous interface combined a blue/slate palette, rounded cards, colored
protocol pills, and a horizontal menu. Those choices made the screens feel
interchangeable with other dashboard templates. The updated foundation adds:

- Pulse blue accents for actions and selection, with green/amber/red reserved for
  operational state. Airy blue-white light surfaces and brighter navy dark surfaces
  use the same hierarchy.
- A persistent desktop navigation rail with section icons and a visible active
  marker. The desktop rail can collapse to icons with hover/focus labels; its
  preference is saved locally. Live status, theme, and logout controls sit in the
  rail footer, leaving the workspace free of a separate utility bar. Narrow screens
  keep labeled, horizontally scrollable navigation and visible footer controls.
  Detail and edit routes retain their parent section's active state.
- Flatter panels with smaller corners, quieter borders, and fewer shadows.
- Monospace values, time axes, protocol labels, and small section labels. Page
  headings use the locally hosted Inter face; body copy uses system fonts.
- Neutral tag and protocol labels, letting failures stand out without competing
  with arbitrary protocol colors. Rows retain their fixed height for virtualization.
- A skip link, keyboard focus styles, reduced-motion support, and logical CSS
  properties for the navigation rail in RTL layouts.

## Extending the UI

Define colors in `frontend/src/app.css` and use semantic tokens or `brand-*`
utilities. Text and filled brand actions have separate token mappings so both
remain readable in dark mode. Existing `indigo-*` form utilities temporarily map
to the brand scale; new components should use `brand-*` directly.

Use `pulse-panel` for simple panels, `pulse-eyebrow` for short section labels,
`pulse-tag` for metadata, and `pulse-protocol` for compact protocol/state labels.
Reserve fully rounded shapes for status dots rather than every piece of metadata.
Keep operational values tabular and retain fixed row heights in virtual lists.
Use `PageHeader.svelte` for a short eyebrow above the main title, with optional
description and action snippet. The dashboard, monitor list, and settings use
this shared header. The workspace owns outer padding; pages should not add an
extra centered/padded wrapper. Settings sections use one panel each rather than
nesting cards inside an existing section panel.

Monitor and heartbeat create/edit pages share `EditorPage.svelte`: a left-aligned
editor with a 64rem maximum width and the same `PageHeader` typography as the
dashboard. Keep the header, panel, and action footer on the same width and start
edge. The workspace supplies outer padding; the editor must not center a narrow
form inside a full-width page. Both resource forms use the shared editor panel,
field, timing-row, and action styles. Timing fields stack on narrow screens.

Use Inter semibold for page and section headings, system sans-serif for labels
and help text, and the existing monospace data font for operational values such
as durations. Preserve the same control heights, panel padding, semantic theme
colors, and Cancel/Save order in both editors. Selection controls already draw
their own border and padding; their wrapper should only receive layout classes.

Use `SelectField.svelte` for all selection fields. Pass the existing option
snippet and typed `bind:value`; give it an id and an external label. The component
preserves a native select for typed bindings, form participation, and fallback,
while exposing a styled combobox/listbox. Options appear in a viewport-positioned
popover above other panels. Arrow keys, Home/End, Enter/Space, Escape, Tab, and
typeahead work without moving focus away from the field. Disabled options are
skipped and clicks outside dismiss the menu. Localized option labels update while
the field is mounted. Adjacent input, select, and submit controls should align at
40px height, with form rows stacking on narrow screens.

All new display text goes through `t()` with entries in every locale.

The branding property tests read the actual theme tokens to verify dark foreground
contrast and alignment between Tailwind fallbacks and the light theme.

## Frontend toolchain

The frontend uses Svelte 5.57, SvelteKit 2, Vite 8, and Svelte's Vite plugin 7.
Use Node 22.12+ (or a newer supported LTS) and the pnpm version pinned in
`frontend/package.json`. Tailwind remains on version 3; SvelteKit 3 and other
major framework migrations are separate work. Exact versions and reproducible
resolutions live in `frontend/package.json` and `frontend/pnpm-lock.yaml`.

After UI changes run `pnpm install`, `pnpm check`, `pnpm validate-locales`,
`pnpm test`, and `pnpm build` from `frontend/`. Inspect populated and empty
screens in both themes, narrow viewports, and Arabic RTL. Browser fixtures are
useful for visual checks without changing a running instance's monitoring data.
