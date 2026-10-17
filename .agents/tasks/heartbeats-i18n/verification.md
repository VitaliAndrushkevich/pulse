# Heartbeats i18n — Verification Note

Iteration: first (no prior `review.json` existed).

## What changed
Translated the `heartbeats` i18n section (and `nav.heartbeats` where appropriate)
in all 12 non-English locale files under `frontend/src/locales/`:
`ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh`.

- `en.json` left unchanged (source of truth).
- Only the `heartbeats` object values and the `nav.heartbeats` value were edited.
  No keys renamed, no structure changed, no other section touched.
- `nav.heartbeats` translated for: ar (نبضات الحياة), be (Хартбіты), ru (Хартбиты),
  ja (ハートビート), ko (하트비트), zh (心跳监测). Kept as "Heartbeats" (product term,
  natural Latin-script spelling) for de, es, fr, it, pt, tr.
- Technical terms kept in Latin form per convention: `examples.shell` = "Shell",
  `examples.cron` = "Cron", `examples.ci` = "CI" in every locale; "Ping URL" / "Token"
  kept Latin where that is the local convention.
- Special characters preserved exactly: ellipsis "…" (U+2026) in `heartbeats.loading`
  and em dash "—" (U+2014) in `heartbeats.token.showOnce` / `rotateConfirm` etc.
  Verified no "…"→"..." or "—"→"-" conversion.

## Commands run (from frontend/)
`node_modules` already present — `pnpm install` NOT needed.

1. JSON parse per file (all 12):
   `python3 -c "import json; json.load(open('src/locales/<code>.json'))"`
   Result: exit 0 for ALL 12 (ar, be, de, es, fr, it, ja, ko, pt, ru, tr, zh).
   This also covers ar.json and be.json, which `validate-locales` omits.

2. Key-parity + leftover-English audit (custom python flatten vs en.json):
   - missing keys: none in any locale
   - extra keys: none in any locale
   - "identical to English" values are all legitimate (de "Name"/"Status"/"Tags",
     product term "Heartbeats" as `title` in de/es/fr/it/pt, FR "Notifications",
     Latin "Ping URL" in be/ru/zh). Confirmed consistent with each file's existing
     translations (e.g. de monitors.status uses "Status"; fr nav.notifications is
     "Notifications").

3. `pnpm validate-locales`
   Result: PASSED. 713 keys, all 10 checked locales (ru, es, fr, pt, de, zh, ja, ko,
   tr, it) report "all 713 keys present" and within 4 nesting levels.
   NOTE: this script's locale list omits `ar` and `be` — both verified via step 1.

4. `pnpm check` (svelte-check)
   Result: 0 errors, 57 warnings in 4 files. All warnings are pre-existing
   `state_referenced_locally` warnings inside `.svelte` component files
   (e.g. NotificationChannelForm.svelte) — unrelated to locale JSON. No NEW errors.

Not run (per instructions): `pnpm dev`, any watch/server command, full `pnpm build`.
