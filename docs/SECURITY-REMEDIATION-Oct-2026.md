# Security dependency remediation — October 17, 2026

## Review scope

Reviewed all open GitHub code-scanning and Dependabot alerts using the GitHub
API, and checked the local frontend lockfile against the npm advisory registry
using `pnpm audit --json`. GitHub reported three open code-scanning alerts and
14 open Dependabot alerts. All open dependency alerts concern the frontend;
none concern the backend or MCP Go modules.

## Code-scanning fixes

Alerts #1, #3, and #4 report missing `GITHUB_TOKEN` permissions in the PR checks
workflow. Set the workflow default to `contents: read`. The MCP job additionally
gets `pull-requests: read` because `dorny/paths-filter` reads changed PR files.
No job in this workflow needs write access.

## Prepared fixes

- Upgrade Vitest from 3.2.7 to 4.1.12 to fix the mock redirect file-read advisory
  [GHSA-82fw-gwwq-j7x9](https://github.com/advisories/GHSA-82fw-gwwq-j7x9).
  This also removes the vulnerable `tinypool` dependency, including its two
  critical prototype-pollution advisories.
- Upgrade Sharp from 0.35.4 to 0.35.5 for the bundled librsvg fix
  [GHSA-wq5f-xc86-pv6w](https://github.com/advisories/GHSA-wq5f-xc86-pv6w).
- Add targeted pnpm overrides for vulnerable transitive versions of `cookie`,
  `devalue`, `undici`, `source-map-js`, and `postcss-selector-parser`.
  These keep the existing SvelteKit and Tailwind major versions while selecting
  patched dependencies. The overrides live in `frontend/pnpm-workspace.yaml`.

These changes cover all 14 currently open Dependabot alerts: #20, #38–40, #42,
#44–45, #47, #49–50, #52–53, and #55–56. The independent audit also identified
the Sharp, source-map-js, selector-parser, and tinypool advisories addressed here.
The Vitest upgrade requires constructor mocks to use ordinary functions rather
than arrow functions; the two history component test files are updated accordingly.

After installation, the audit reports one high-severity advisory, down from
26 advisories (two critical, nine high, ten moderate, five low).

## Remaining advisory

`tailwindcss@3.4.19 → chokidar@3.6.0 → braces@3.0.3` is affected by
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), a
stack-exhaustion denial of service through deeply nested patterns. The registry
reports no patched version.

This dependency is part of frontend build tooling. Pulse ships static frontend
assets embedded in its Go executable; this Node dependency is not executed by
the deployed Pulse server. The advisory remains relevant when processing
untrusted patterns during development or builds. Do not override Chokidar to
major version 4 just to remove the alert: it removes glob support used by
Tailwind's CLI watcher. Revisit when an upstream fix or a separately validated
Tailwind migration is available.

## Reproduction

From `frontend/`, run:

```sh
pnpm install --frozen-lockfile
pnpm audit
pnpm check
pnpm validate-locales
pnpm test
pnpm build
```

`pnpm audit` still exits nonzero for the remaining `braces` advisory.

## Validation results

- Frozen-lockfile installation passed.
- Type checking passed with zero errors and 52 warnings in unchanged Svelte
  components.
- Locale validation passed.
- All 538 tests in 48 files passed with Vitest 4.1.12.
- The static production build passed.
- PR workflow YAML parsing and effective permissions checks passed for all
  three jobs. Execution of the workflow and GitHub code scanning still need CI.

The fixes are prepared locally; GitHub alerts have not been dismissed or closed.
