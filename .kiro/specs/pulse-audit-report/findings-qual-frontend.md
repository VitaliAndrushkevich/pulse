# Code Quality Findings — Frontend (TypeScript & Patterns)

Phase 4, Task 6.1: Audit TypeScript and frontend patterns

---

## QUAL-001: Missing SvelteKit Error Boundary Pages

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | error-handling |
| **Effort** | Small (hours) |
| **Priority** | 9 |

**Description:** No `+error.svelte` files exist anywhere in `frontend/src/routes/`. SvelteKit provides error boundary pages that catch unhandled exceptions during load/render and display a user-friendly fallback instead of a blank white screen. Without these, any uncaught runtime error during component rendering will bubble to the default SvelteKit error page with no custom UX or recovery action.

**Evidence:**
`frontend/src/routes/` (entire tree)
```
No +error.svelte files found at root, /monitors, /settings, /notifications, or any nested route.
```

**Impact:** An unhandled exception during page rendering (e.g., a null access on a malformed API response) results in a generic SvelteKit 500 page with no branding, retry button, or navigation path back to the app. Users perceive the app as crashed.

**Remediation:** Add at minimum a root-level `frontend/src/routes/+error.svelte` that displays the error message with a "Go Home" link, styled with the project's theme tokens. Consider per-section error pages for `/monitors` and `/notifications` with contextual retry actions. This restores the graceful degradation property.

---

## QUAL-002: Unchecked Type Assertions in WebSocket Message Handler

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | typescript-strictness |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The WebSocket message handler in `ws.ts` uses `as` casts to narrow the parsed JSON envelope payload to specific types (`MonitorPatch`, `MonitorTagsChangedPayload`, `WsMessage['type']`) without runtime validation. While the preceding `typeof envelope.type !== 'string'` guard is a minimal check, no schema validation (e.g., zod, runtime type check) confirms the payload structure matches the expected types. A malformed server message could assign incorrect field types to the cast objects.

**Evidence:**
`frontend/src/lib/ws.ts:222-243`
```typescript
const msg: WsMessage = {
  type: envelope.type as WsMessage['type'],
  payload: envelope.payload as WsMessage['payload']
};

switch (envelope.type) {
  case 'monitor_status': {
    const patch = envelope.payload as MonitorPatch;
    monitorStore.applyPatch(patch);
    patchBus.publish(patch);
    break;
  }

  case 'monitor_tags_changed': {
    const tagsPayload = envelope.payload as MonitorTagsChangedPayload;
    monitorStore.applyTagsChange(tagsPayload.monitor_id, tagsPayload.tags);
    break;
  }
}
```

**Impact:** If the server sends a `monitor_status` message with a missing `monitor_id` or non-string `state`, the patch is applied without error — potentially corrupting the monitor store with `undefined` values. This is a defense-in-depth gap; the server is trusted but a malformed message could propagate silently.

**Remediation:** Add lightweight runtime guards before applying patches (e.g., check `typeof patch.monitor_id === 'string'` and `typeof patch.state === 'string'`). This restores the fail-fast property for malformed messages without adding a heavy validation library.

---

## QUAL-003: Explicit `any` Types in Test Files

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | typescript-strictness |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** Several test files use explicit `any` type annotations, primarily in mock setups for CodeMirror and Proto source components. While production source code (`frontend/src/lib/`, component `.svelte` files) is free of `any` types, the test files bypass TypeScript's type safety for mock construction.

**Evidence:**
`frontend/src/components/PayloadEditor.test.ts:19-30`
```typescript
const EditorView = vi.fn().mockImplementation(function (this: any, config: any) {
  this.state = { doc: { toString: () => config.state?.doc ?? '', length: config.state?.doc?.length ?? 0 } };
  this.dispatch = vi.fn();
  return this;
});
(EditorView as any).updateListener = { of: vi.fn(() => ({})) };
(EditorView as any).theme = vi.fn(() => ({}));
```

Also in:
- `frontend/src/components/ProtoSourceUpload.test.ts:36-38` (`apiError: any`, `constructor(s: number, e: any, ...)`)
- `frontend/src/components/HistoryExplorer.test.ts:58` (`points: any[], aggregatedPoints: any[]`)
- `frontend/src/components/__tests__/ThemeSwitcher.test.ts:37` (`(mql as any).matches`)

**Impact:** No production impact. Reduces type safety within tests, making refactoring of mocked interfaces more error-prone. Tests may continue passing after an interface change that would break production code.

**Remediation:** Replace `any` with typed mock utilities or narrower type assertions. Use `vi.fn<Parameters, Return>()` generics for mock constructors. Low priority — informational only.

---

## QUAL-004: Tailwind `dark:` Prefix Violates Theme Convention

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | convention-adherence |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** The `ProtoSourceUpload.svelte` component uses Tailwind's `dark:` prefix for dark mode styling. Per AGENTS.md conventions, dark mode is controlled exclusively via the `data-theme` attribute on `<html>` and the `[data-theme="dark"]` CSS selector strategy. The `dark:` prefix relies on Tailwind's class-based dark mode detection, which may or may not align with the project's `data-theme` configuration depending on the Tailwind config setup.

**Evidence:**
`frontend/src/components/ProtoSourceUpload.svelte:205`
```html
<div
  class="rounded-md border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300"
  role="alert"
>
```

`frontend/src/components/ProtoSourceUpload.svelte:263`
```html
class="relative rounded-lg border-2 border-dashed p-6 text-center transition-colors {isDragging
  ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/20'
  : 'border-[var(--color-border)] hover:border-blue-400'}"
```

**Impact:** The `dark:` classes may not activate correctly if the Tailwind dark mode selector is configured differently from the `class` strategy. Even if they work due to config alignment (`darkMode: ['selector', '[data-theme="dark"]']`), they break the convention of centralizing theme logic in CSS custom properties and make this component inconsistent with the rest of the codebase.

**Remediation:** Replace `dark:` prefixed classes with CSS custom properties from `app.css` (e.g., `var(--color-error)`, `var(--color-bg-surface)`) or the `[data-theme="dark"]` selector pattern used elsewhere. This restores convention consistency.

---

## QUAL-005: Hardcoded User-Visible Strings Not Using `t()` Function

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | convention-adherence |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** Multiple components and route pages contain hardcoded English strings that bypass the i18n `t()` function. Per AGENTS.md: "All user-visible strings MUST use the `t()` function from `$lib/i18n`. Never hardcode display strings in Svelte templates or component scripts."

**Evidence:**
`frontend/src/components/HistoryExplorer.svelte:85-104`
```svelte
Data has been truncated to the monitor's retention period ({retentionDays} days).
...
Retry
...
No data available for the selected period.
```

`frontend/src/components/HistoryChartExplorer.svelte:427`
```svelte
<p class="text-sm text-[var(--color-text-secondary)]">No data available</p>
```

`frontend/src/routes/monitors/[id]/+page.svelte:132`
```typescript
error = err instanceof Error ? err.message : `Failed to ${newStatus === 'paused' ? 'pause' : 'resume'} monitor.`;
```

`frontend/src/routes/notifications/[id]/edit/+page.svelte:25`
```typescript
error = err instanceof Error ? err.message : 'Failed to load channel';
```

Additional occurrences in:
- `frontend/src/components/NotificationChannelForm.svelte:219` — "Failed to load template variables"
- `frontend/src/components/MonitorDeliveryLogs.svelte:31` — "Failed to load delivery logs"
- `frontend/src/components/MonitorNotificationBindings.svelte:123` — "Failed to load notification bindings"
- `frontend/src/components/PendingNotificationBindings.svelte:126` — "Failed to load notification channels"
- `frontend/src/lib/stores/notifications.svelte.ts:57` — "Failed to load notification channels"

**Impact:** These strings cannot be translated for non-English users. With 13 supported locales, untranslated error messages and empty-state text degrade the user experience for ~92% of potential non-English users.

**Remediation:** Add corresponding keys to `frontend/src/locales/en.json` and all other locale files, then replace hardcoded strings with `t('section.key')` calls. Target: zero hardcoded user-visible strings in component/route files.

---

## QUAL-006: Hardcoded Semantic Color Classes Instead of CSS Custom Properties

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | convention-adherence |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Many components use hardcoded Tailwind color classes (e.g., `text-rose-700`, `bg-rose-50`, `border-red-200`, `text-emerald-600`, `text-indigo-600`) for semantic states like "error", "success", and "interactive". Per AGENTS.md, the project should use CSS custom properties from `app.css` (e.g., `var(--color-error)`, `var(--color-success)`) to enable consistent theming across light and dark modes.

**Evidence:**
`frontend/src/routes/monitors/+page.svelte:138-145`
```html
<div class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center">
  <p class="text-sm text-rose-700">{error}</p>
```

`frontend/src/routes/settings/ApiTokenSection.svelte:135`
```html
class="mt-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
```

This pattern repeats across ~15 components for error states, success states, and interactive elements. The issue is widespread but low-severity because `app.css` only defines `--color-error`, `--color-success`, and `--color-warning` tokens without corresponding background/border variants.

**Impact:** Error, success, and interactive colors are not governed by the theme system. Switching to dark mode relies on Tailwind's color palette being legible on dark backgrounds rather than the centralized token system. Some rose/red backgrounds may have poor contrast in dark mode.

**Remediation:** Extend `app.css` token system with background and border semantic variants (e.g., `--color-error-bg`, `--color-error-border`, `--color-error-text`) for both themes. Replace hardcoded Tailwind color classes with these tokens. This is a medium effort due to the number of affected components but restores theme centralization.

---

## QUAL-007: No Test Coverage for Notification Store and Routes

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | test-coverage |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The notification subsystem (store, route pages, channel form) has zero test coverage. No test file exists for `notifications.svelte.ts`, the notification channel form, notification bindings component, or the notifications route pages. This represents a significant feature area (J milestone) with CRUD operations, async state management, and error handling that is completely untested.

**Evidence:**
```
frontend/src/lib/stores/notifications.svelte.ts — no corresponding .test.ts file
frontend/src/routes/notifications/ — no __tests__/ directory
frontend/src/components/NotificationChannelForm.svelte — no test file
frontend/src/components/MonitorNotificationBindings.svelte — no test file
frontend/src/components/MonitorDeliveryLogs.svelte — no test file
frontend/src/components/PendingNotificationBindings.svelte — no test file
```

Auth flows are tested (login page, auth store), WS reconnection is tested (ws.test.ts with 15+ test cases), and monitor store is tested. But the notifications category — representing a full milestone of functionality — has 0% coverage.

**Impact:** Regressions in notification channel CRUD, binding management, or delivery log display would go undetected. The store's error handling, pagination logic, and state cleanup could silently break during refactoring.

**Remediation:** Add unit tests for `notifications.svelte.ts` (fetchChannels, create, update, remove, error states, pagination). Add integration tests for the channel form validation and notification route pages. Target: at least 50% module coverage for the notification subsystem.

---

## QUAL-008: Component Data Fetching Mixed with Presentation

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | component-architecture |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Several route-level page components combine data fetching logic, state management, and rendering in the same file. While SvelteKit pages naturally serve as the integration point, some components (`HistoryExplorer.svelte`, `MonitorNotificationBindings.svelte`, `MonitorDeliveryLogs.svelte`) are reusable components that also perform their own API calls, manage loading/error state, and render UI. This mixes concerns within "presentational" components.

**Evidence:**
`frontend/src/components/HistoryExplorer.svelte:30-48`
```typescript
async function fetchHistory() {
  loading = true;
  error = null;
  try {
    const response = await getMonitorHistoryExtended(
      monitorId, selectedRange.from, selectedRange.to, step
    );
    points = response.points ?? [];
    aggregatedPoints = response.aggregated_points ?? [];
    step = response.step;
    truncated = response.truncated ?? false;
  } catch (err: unknown) {
    error = err instanceof Error ? err.message : 'Failed to load history data.';
  } finally {
    loading = false;
  }
}
```

Similarly: `MonitorNotificationBindings.svelte`, `MonitorDeliveryLogs.svelte`, `PendingNotificationBindings.svelte`.

**Impact:** These components are harder to test in isolation (require mocking API calls), harder to reuse with pre-fetched data, and couple presentation to the specific API client. However, this is a pragmatic SvelteKit pattern and not a correctness issue.

**Remediation:** Consider extracting data-fetching logic into composable functions or stores, passing data as props from the parent route. This is a low-priority architectural improvement that restores the container-presentational separation pattern. Given Svelte's design philosophy favoring collocated logic, this is acceptable as-is for most cases.

---

## QUAL-009: Silent `catch` Blocks in Auth Store

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | error-handling |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** The auth store's localStorage operations (`readTokenFromStorage`, `writeTokenToStorage`, `removeTokenFromStorage`) catch errors silently without any logging or user feedback. While the comment says "Silently fail — storage quota or access issue," this means a user in private browsing mode (where localStorage may throw) will have no indication that their session cannot persist.

**Evidence:**
`frontend/src/lib/stores/auth.svelte.ts:29-33`
```typescript
function writeTokenToStorage(token: string): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Silently fail — storage quota or access issue
  }
}
```

**Impact:** If localStorage is unavailable, the user can log in (token is kept in reactive state) but loses their session on page refresh without any warning. This is a rare edge case affecting private browsing users on restrictive browsers.

**Remediation:** Log a warning to the console (non-user-facing) when localStorage operations fail, and consider showing a one-time informational toast: "Session will not persist across page refreshes in this browser mode." This restores the user-awareness property for edge-case storage failures.

---

## QUAL-010: Monitor Detail Page Uses Hardcoded Color Utility Classes for State

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | accessibility |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** The monitor detail page uses hardcoded Tailwind color classes for state indicators (`bg-emerald-500` for up, `bg-rose-500` for down) and uptime percentages (`text-emerald-600`, `text-amber-600`, `text-rose-600`). These colors are not verified against WCAG 2.1 AA contrast requirements in both light and dark themes. Additionally, color is the only differentiator for state — there is no accompanying text or icon that distinguishes "up" from "down" for color-blind users in the small status dot indicator.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:36-40`
```typescript
const stateColors: Record<string, string> = {
  up: 'bg-emerald-500',
  down: 'bg-rose-500',
  unknown: 'bg-slate-400'
};
```

The 3×3 pixel state indicator dot (line ~220 in template) uses only color:
```html
<span class="h-3 w-3 rounded-full {stateColors[monitor.state] ?? 'bg-slate-400'}"></span>
```

However, the adjacent text label ("Up"/"Down"/"Unknown") provides a text alternative, so this specific instance is compliant. The concern is limited to contexts where only the dot is visible (e.g., MonitorRow in the list view).

**Impact:** The state dot alone does not convey state to color-blind users if they cannot read the adjacent label. In the current UI, labels are always present next to indicators, making this a minor accessibility gap rather than a violation.

**Remediation:** Confirm that all instances of the state dot are accompanied by text labels. Consider adding `aria-label` to the dot span for screen readers. Verify contrast ratios of emerald-500 and rose-500 against both light (`#ffffff`) and dark (`#1e293b`) surface backgrounds.

---

## QUAL-011: Tab Navigation Lacks `tablist` Role and Arrow Key Support

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | accessibility |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The monitor detail page implements a tab UI (Overview, History, Notifications) using `role="tab"` and `aria-selected` on individual buttons, but the container `<nav>` does not have `role="tablist"`. Additionally, the ARIA Tab pattern requires arrow-key navigation between tabs (left/right arrows move focus), which is not implemented. Users can only navigate between tabs via the Tab key, which is non-standard for ARIA tab widgets.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:187-206`
```html
<div class="border-b border-[var(--color-border)]" data-testid="tab-bar">
  <nav class="-mb-px flex gap-6" aria-label="Monitor tabs">
    <button
      type="button"
      onclick={() => activeTab = 'overview'}
      class="..."
      aria-selected={activeTab === 'overview'}
      role="tab"
      data-testid="tab-overview"
    >
```

Missing: `role="tablist"` on the `<nav>`, `tabindex="-1"` on non-selected tabs, `aria-controls` pointing to panels, and keyboard event handler for arrow key navigation.

**Impact:** Screen readers may not announce the widget as a proper tab list. Keyboard-only users must use Tab to move between tabs rather than the expected arrow keys. This is a WCAG 2.1 AA guideline gap (4.1.2 Name, Role, Value) but not a strict failure since the buttons are still focusable and operable.

**Remediation:** Add `role="tablist"` to the `<nav>` container. Add `aria-controls` attributes linking tabs to their panels. Implement arrow-key navigation handler. Set `tabindex="0"` on the active tab and `tabindex="-1"` on inactive tabs. This restores full ARIA Tabs pattern compliance.

---

## QUAL-012: Uncaught Promise in Monitor Create Page Submit Flow

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | error-handling |
| **Effort** | Small (hours) |
| **Priority** | 16 |

**Description:** The `handleSubmit` function in the monitor create page (`+page.svelte`) is an async function passed as a callback to `MonitorForm`. While `MonitorForm` wraps the `onSubmit` call in a try/catch (confirmed in the component), the `handleSubmit` function itself chains multiple `await` calls (createMonitor, createCredential, createNotificationBinding) without individual error handling for the secondary operations.

**Evidence:**
`frontend/src/routes/monitors/create/+page.svelte:8-35`
```typescript
async function handleSubmit(values, pendingCredential?, pendingBindings?) {
  const created = await createMonitor(values);

  if (pendingCredential) {
    await createCredential(created.id, pendingCredential);
  }

  if (pendingBindings && pendingBindings.length > 0) {
    await Promise.all(
      pendingBindings.map((binding) => { ... })
    );
  }

  await goto(`/monitors/${created.id}`);
}
```

If `createMonitor` succeeds but `createCredential` or `createNotificationBinding` fails, the error is caught by MonitorForm's try/catch and displayed as a generic error. The monitor has already been created, but the user sees an error without knowing the monitor exists. The `goto` never fires.

**Impact:** Partial creation state: the monitor exists server-side but credentials/bindings were not attached. The user sees a generic error and may retry, potentially creating a duplicate monitor. This is mitigated by the idempotent PUT design but still a confusing UX.

**Remediation:** Handle secondary operation failures independently — e.g., catch credential/binding errors, show a warning toast ("Monitor created, but credentials failed to attach"), and still navigate to the new monitor. This restores the atomic-feedback property for compound operations.

