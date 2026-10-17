# Heartbeats i18n translation across 12 locales

The `heartbeats` i18n section (and `nav.heartbeats` where a natural equivalent exists) is now translated in all 12 non-English locale files under `frontend/src/locales/`. The section had previously been dropped in as verbatim English placeholder; this change replaces those placeholders with real translations while leaving `en.json` as the untouched source of truth. Key structure matches `en.json` exactly in every file, special characters are preserved, and the technical/product terms that are conventionally kept in Latin script (`Shell`, `Cron`, `CI`, `Ping URL`, product name `Heartbeats`) remain intentionally identical to English.

Watch for: nothing blocking. The only identical-to-English values left in the `heartbeats` section are legitimate (loanwords, product terms, and Latin-script technical terms) — **confirmed** by value-by-value comparison against `en.json`. (**possible**) Unrelated untranslated English in `nav.workspace`, `nav.signature`, and the whole `users.*` section appears in the branch diff, but that is outside this task's scope — the task is limited to `heartbeats` and `nav.heartbeats`.

**Verdict**: APPROVED

## High-level view

Every locale carries the full `heartbeats` key tree with the same nesting and no added, removed, or renamed keys — a flatten-and-diff against `en.json` reports zero missing and zero extra keys in all 12 files. The translation step produced pure additions: no line was removed or modified in any locale file, and `en.json` contains only additions (the new `heartbeats` block arriving with the feature), never edits.

The translatable full sentences that would be the real risk here — `subtitle`, `deleteConfirm`, `notFoundDescription`, `examples.note`, `token.showOnce`, `token.rotateConfirm`, the `form.*` validation strings — are genuinely translated in every file, none left equal to English. The residual identical-to-English values are confined to Latin-script technical terms (`examples.shell`/`cron`/`ci` everywhere), the product term `Heartbeats` used as `title` in Latin-script locales (de/es/fr/it/pt), `Ping URL` as `token.title` in be/ru/zh, and ordinary loanwords (de `Name`/`Status`/`Tags`, fr `Notifications`, pt `Status`/`Tags`). Each of these matches the convention already used elsewhere in the same file.

Special characters survived: the ellipsis U+2026 in `heartbeats.loading` is intact in all 12 files with no `...` substitution, and the em dash U+2014 in `token.showOnce` is intact in all 12. Russian reads as neutral professional prose and Arabic is proper Arabic with the technical `Ping URL` token kept Latin inline.

`nav.heartbeats` is translated where a natural rendering exists (ar نبضات الحياة, be Хартбіты, ru Хартбиты, ja ハートビート, ko 하트비트, zh 心跳监测) and kept as the product term `Heartbeats` for de/es/fr/it/pt/tr — a defensible per-language call, not a missed translation.

<details>
<summary>Issues (0 blocking)</summary>

No blocking findings. One informational, out-of-scope note:

1. **Unrelated English in other sections (out of scope)** — `nav.workspace`, `nav.signature`, `nav.overview`, `nav.collapseNavigation`, and the entire `users.*` section are still English across the non-English locales. These belong to other features on the same branch and are explicitly outside this task's scope (`heartbeats` + `nav.heartbeats` only). No action required for this task; may warrant a separate translation pass.

</details>

<details>
<summary>Details</summary>

### Key-structure parity

A recursive flatten of each locale's `heartbeats` object compared against `en.json` reports zero missing keys and zero extra keys in all 12 files. Nesting (`state`, `fields`, `form`, `token`, `ping`, `examples`, `bindings`) is identical. The git diff shows only added lines in the locale files — no removals or in-place modifications — so no pre-existing key was renamed or dropped. `en.json` likewise shows only additions and no removed lines, confirming the source of truth was not edited.

### No leftover English placeholder sentences

The high-risk translatable sentences were checked individually and are translated in every locale. Examples from the tone-sensitive files:

- ru `subtitle`: «Отслеживайте запланированные задания по ожидаемой отметке»
- ru `deleteConfirm`: «Удалить этот хартбит? Его ping URL перестанет работать.»
- ru `examples.note`: «Успешный пинг подтверждает, что команда выполнилась, но не то, что её вывод корректен или пригоден для восстановления.»
- ar `subtitle`: «راقب المهام المجدولة من خلال تسجيل الوصول المتوقع»
- ar `examples.note`: «تثبت نبضة النجاح أن الأمر قد تم تشغيله...»

The only values equal to English are:

- `examples.shell` = "Shell", `examples.cron` = "Cron", `examples.ci` = "CI" — in all 12 files, conventionally Latin.
- `title` = "Heartbeats" — product term in de/es/fr/it/pt (nav still localized where natural).
- `token.title` = "Ping URL" — Latin convention in be/ru/zh.
- de `fields.name`/`fields.status`/`fields.tags`, pt `fields.status`/`fields.tags`, fr `bindings.title` ("Notifications") — loanwords consistent with each file's existing translations.

None of these fall into the task's "blocking if identical" set (full sentences such as `subtitle`, `deleteConfirm`, `notFoundDescription`, `examples.note`).

### Special-character preservation

Across all 12 locales plus `en.json`: `heartbeats.loading` retains the U+2026 ellipsis with no `...` substitution, and `token.showOnce` retains the U+2014 em dash. (`rotateConfirm` contains no em dash in `en.json` either, so none is expected in the translations.)

### Russian tone and Arabic correctness

Russian uses neutral professional phrasing with no slang (e.g. «Обновить токен? Существующие ping URL перестанут работать.»). Arabic renders as proper Arabic with the `Ping URL` technical token kept Latin inline, which is standard practice. Both files parse as valid JSON (narrow spot-check).

### Scope confirmation

Diffing each locale against `main`, the added lines fall into `heartbeats.*` and `nav.heartbeats` as intended. The branch diff also surfaces untranslated English in `nav.workspace`, `nav.signature`, `nav.overview`, navigation a11y labels, and the `users.*` section — but these were introduced by other work on the same feature branch, not by this translation step, and lie outside the stated scope.

### Verification evidence

The recorded evidence at `.agents/tasks/heartbeats-i18n/verification.md` reports: JSON parse exit 0 for all 12 files (covering `ar`/`be` which `validate-locales` omits); a custom key-parity/leftover-English audit with no missing/extra keys; `pnpm validate-locales` passing with 713 keys across the 10 scripted locales; and `pnpm check` with 0 errors (57 pre-existing unrelated Svelte warnings). This matches my independent structural comparison, so the gated suites were not re-run.

</details>

<details>
<summary>File map</summary>

- `frontend/src/locales/{ar,be,de,es,fr,it,ja,ko,pt,ru,tr,zh}.json` — `heartbeats` section translated; `nav.heartbeats` localized where a natural term exists.
- `frontend/src/locales/en.json` — source of truth; additions only, no edits to the `heartbeats` values.

Full diff: `git diff main -- frontend/src/locales/`.

</details>
