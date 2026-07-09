# Performance Findings — Frontend Rendering and Bundle (Task 4.1)

Requirements referenced: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6

---

## PERF-001: VirtualList DOM node cap correctly enforced

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Rendering — Virtualization |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The VirtualList implementation correctly caps rendered DOM nodes at 60 via the `endIndex` derived computation. The buffer is clamped to [5, 20], and scroll handling is RAF-throttled to avoid excessive reflows. The implementation uses spacer divs for scroll position and proper ARIA `role="list"` / `role="listitem"` semantics.

**Evidence:**
`frontend/src/components/VirtualList.svelte:57-64`
```svelte
let endIndex = $derived.by(() => {
  const maxRendered = 60;
  const rawEnd = Math.min(rawEndIndex, items.length);
  const count = rawEnd - startIndex;
  if (count > maxRendered) {
    return startIndex + maxRendered;
  }
  return rawEnd;
});
```

**Impact:** No issue. The DOM cap is correctly implemented and would maintain smooth scrolling performance with 500+ monitors.

**Remediation:** None required. The implementation meets the 60-node ceiling requirement.

---

## PERF-002: VirtualList uses index-based keying instead of stable item identity

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Rendering — Virtualization |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The `{#each}` block uses `(startIndex + i)` as the key, which is a positional index rather than a stable item identifier. When the list scrolls, items at the same visual position get different keys, causing Svelte to destroy and recreate DOM nodes instead of recycling them. This defeats DOM recycling optimization and causes unnecessary GC pressure during rapid scrolling.

**Evidence:**
`frontend/src/components/VirtualList.svelte:97-104`
```svelte
{#each visibleItems as item, i (startIndex + i)}
  <div
    class="virtual-list-row"
    style="height: {itemHeight}px;"
    role="listitem"
  >
    {@render row(item, startIndex + i)}
  </div>
{/each}
```

**Impact:** During continuous scrolling of 500+ monitors, nodes are recreated instead of recycled. This increases GC pressure and can cause frame drops below 55 fps on lower-end devices, particularly when MonitorRow components contain complex DOM subtrees.

**Remediation:** Use a stable item identifier (e.g., `item.id`) as the each-block key. This requires the generic `T` to have an `id` field or accepting a `key` function prop. Target state: DOM nodes are recycled across scroll positions, maintaining consistent 60 fps. Restores the DOM recycling performance property.

---

## PERF-003: Initial JS bundle well under 200 KB gzipped target

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Bundle Size |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The initial page load preloads 22 JS modules totaling ~138 KB raw / ~50 KB gzipped. This is well below the 200 KB gzipped target. The SvelteKit static adapter produces content-hashed filenames for all chunks, enabling aggressive cache-control.

**Evidence:**
`frontend/build/index.html` — 22 `rel="modulepreload"` links totaling:
- Raw JS: 137,736 bytes
- Gzipped (combined): ~50,451 bytes
- CSS: 37,868 bytes raw / 7,156 bytes gzipped

**Impact:** No issue. The initial bundle is approximately 25% of the 200 KB budget, leaving substantial headroom.

**Remediation:** None required. Bundle size is healthy.

---

## PERF-004: CodeMirror dependency contributes 127 KB gzipped but is properly code-split

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Code Splitting |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The CodeMirror editor suite (`@codemirror/*` packages) produces a single chunk of 409 KB raw / 127 KB gzipped (`chunks/DbluOvrW.js`). This chunk is NOT in the initial bundle — it's loaded only by route nodes 6 and 7 (notification/webhook template editing pages). The code-splitting is effective: the dependency is isolated to routes that need it.

**Evidence:**
`frontend/build/_app/immutable/chunks/DbluOvrW.js` — 409,485 bytes raw, 126,841 bytes gzipped.
Loaded only by `nodes/6.DA41_QK8.js` and `nodes/7.BLgoDtZb.js` (not in `index.html` preloads).

`frontend/package.json:8-15` — 8 CodeMirror packages in dependencies.

**Impact:** Users navigating to webhook template editor will experience a noticeable load delay (~127 KB download). However, this only affects admin users configuring webhooks, not the primary monitoring dashboard flow.

**Remediation:** Consider dynamic `import()` of CodeMirror only when the user clicks into the template editor field, rather than at route-level. This would defer the 127 KB load until actually needed. Alternatively, evaluate lighter alternatives for JSON template editing if full CodeMirror features aren't required. Restores the property of sub-50 KB per lazy-loaded dependency.

---

## PERF-005: Route node 5 at 39 KB gzipped exceeds 20 KB route module threshold

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Code Splitting |
| **Effort** | Medium (days) |
| **Priority** | 14 |

**Description:** Route node 5 (`nodes/5.BUITZNHn.js`) is 108 KB raw / 39 KB gzipped, nearly double the 20 KB threshold for route-specific modules. This node contains the monitor detail page with chart rendering, history display, and notification binding management inlined into a single chunk.

**Evidence:**
`frontend/build/_app/immutable/nodes/5.BUITZNHn.js` — 107,984 bytes raw, 38,894 bytes gzipped.
Contains uPlot charting, history data formatting, and notification binding UI.

**Impact:** First navigation to monitor detail page downloads 39 KB of JS. On slow mobile networks (3G), this adds ~1.5 seconds to perceived page load time. The threshold exists to ensure snappy route transitions.

**Remediation:** Split the monitor detail page into sub-components loaded on demand: lazy-load the HistoryChart (uPlot) and notification bindings section independently. Target: each route segment < 20 KB gzipped. Restores the property of fast route transitions on all network conditions.

---

## PERF-006: HistoryChart uses Svelte 4 legacy API (export let) instead of Svelte 5 runes

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Reactivity — Svelte 5 Patterns |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The HistoryChart component uses Svelte 4's `export let data` pattern instead of Svelte 5's `$props()` rune. More critically, it does NOT react to prop changes — the chart is only created in `onMount` and never updated when `data` changes. If the parent updates the `data` prop (e.g., from real-time WS patches appending new points), the chart shows stale data until the component is unmounted and remounted.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:8-11`
```typescript
export let data: HistoryPoint[] = [];

let chartContainer: HTMLElement;
let chart: uPlot | null = null;
```

No `$:` reactive block, no `$effect`, and no `afterUpdate` to handle data changes.

**Impact:** The chart does not update when new history points arrive via WebSocket. Users see stale chart data until they navigate away and back. This contradicts the real-time update design goal.

**Remediation:** Migrate to `$props()` and add a `$effect` that calls `chart.setData(buildChartData(data))` when the data prop changes (without full chart recreation). Target state: chart updates in-place when new data points arrive, maintaining the real-time property.

---

## PERF-007: uPlot instance properly destroyed on unmount, no retained references

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Memory — Chart Lifecycle |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The HistoryChart component correctly destroys the uPlot instance in `onDestroy`, nulling the reference to allow GC. The `destroyChart()` function calls `chart.destroy()` which removes the canvas element and all internal event listeners, then sets `chart = null`.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:71-80`
```typescript
function destroyChart() {
  if (chart) {
    chart.destroy();
    chart = null;
  }
}

onDestroy(() => {
  destroyChart();
});
```

**Impact:** No retained references or detached canvas elements after unmount. uPlot's `destroy()` method removes the DOM element it created within the container.

**Remediation:** None required. The lifecycle management is correct. Note: A full verification via heap snapshot comparison is not possible in static analysis — runtime profiling would be needed to confirm zero growth under repeated mount/unmount cycles.

---

## PERF-008: HistoryChart reads computed styles on every createChart invocation

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Rendering — Layout Thrashing |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** Each time `createChart()` is called, it invokes `getComputedStyle(document.documentElement)` to read theme colors. This triggers a synchronous style recalculation. If createChart were called during a batch of DOM operations (e.g., multiple charts mounting simultaneously on dashboard), this could cause layout thrashing.

**Evidence:**
`frontend/src/components/HistoryChart.svelte:35-37`
```typescript
const styles = getComputedStyle(document.documentElement);
const axisStroke = styles.getPropertyValue('--color-text-muted').trim() || '#64748b';
const gridStroke = styles.getPropertyValue('--color-border').trim() || '#e2e8f0';
```

**Impact:** Minor — `getComputedStyle` is called once per chart mount, not in a loop. Impact is negligible for a single chart but could compound if the component is used in a list without virtualization.

**Remediation:** Cache theme colors at the module level or read them once per theme change (subscribe to a theme store). Alternatively, pass chart colors as props from the parent. Restores the property of zero forced style recalculations during component initialization.

---

## PERF-009: Svelte 5 reactivity patterns are clean — no deep derived chains

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Reactivity — Derived State Depth |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** All stores use Svelte 5 runes (`$state`, `$derived`) correctly with shallow derived chains. The maximum derived depth is 2 levels:
- `monitors.svelte.ts`: `$state(Map)` → `$derived(list)` → `$derived(healthyCount)` (2 levels)
- `dashboard.svelte.ts`: `$state(widgetLoading)` → `$derived(isLoading)` (1 level)
- `notifications.svelte.ts`: `$state(channels)` → `$derived(isEmpty)` (1 level)

No store exceeds 3 levels of derived state depth.

**Evidence:**
`frontend/src/lib/stores/monitors.svelte.ts:37-43`
```typescript
const list = $derived<Monitor[]>(Array.from(monitors.values()));
const totalCount = $derived<number>(monitors.size);
const healthyCount = $derived<number>(
  Array.from(monitors.values()).filter((m) => m.state === 'up').length
);
```

**Impact:** No issue. Derived chains are shallow and predictable, avoiding cascading re-computations.

**Remediation:** None required.

---

## PERF-010: Store subscriptions properly cleaned up via $effect return and onMount return

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Reactivity — Subscription Cleanup |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** All patchBus subscriptions are properly cleaned up:
- Dashboard (`+page.svelte`): subscribes in `onMount`, returns cleanup function that calls `unsubscribe()`.
- Monitor detail (`monitors/[id]/+page.svelte`): subscribes in `$effect`, returns cleanup function from the effect.

Module-level stores (`monitors.svelte.ts`, `dashboard.svelte.ts`) use singleton pattern with no subscriptions that need cleanup — they're module-scoped reactive state.

**Evidence:**
`frontend/src/routes/+page.svelte:67-76`
```typescript
const unsubscribe = patchBus.subscribe(handlePatch);
resetStalenessTimer();
return () => {
  unsubscribe();
  if (stalenessTimer !== null) { clearTimeout(stalenessTimer); }
};
```

`frontend/src/routes/monitors/[id]/+page.svelte:222-224`
```typescript
return () => {
  unsubscribe();
};
```

**Impact:** No memory leaks from orphaned subscriptions.

**Remediation:** None required.

---

## PERF-011: CSS strategy is efficient — Tailwind purging active, minimal render-blocking CSS

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | CSS — Strategy |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The CSS output is well-optimized:
- Total CSS: 37,868 bytes raw / 7,156 bytes gzipped (single render-blocking stylesheet)
- Tailwind content purging is configured to scan `./src/**/*.{html,js,svelte,ts}`
- Theme switching uses CSS custom properties on `:root` / `[data-theme="dark"]` which requires only a single attribute change on `<html>` — the browser applies new variable values via cascade without re-parsing rules
- Dark mode uses `selector` strategy (`[data-theme="dark"]`), not media query, enabling instant JS-controlled switching

The `precompress: false` setting in svelte.config.js means no pre-compressed `.gz`/`.br` files are generated — compression is expected to be handled by the Go embed server or reverse proxy at runtime.

**Evidence:**
`frontend/tailwind.config.cjs:2`
```javascript
content: ['./src/**/*.{html,js,svelte,ts}'],
```

`frontend/build/_app/immutable/assets/0.Cgtkzkix.css` — 37,868 bytes (only render-blocking CSS file)

**Impact:** Theme switching completes within a single frame (CSS variable cascade update). The 7 KB gzipped CSS payload is negligible. No unused rule bloat observed from Tailwind's purging.

**Remediation:** None required. The CSS strategy meets all performance criteria. Consider enabling `precompress: true` in svelte.config if the Go embed server doesn't handle dynamic gzip/brotli compression.

---

## PERF-012: i18n English locale statically imported — other 12 locales lazy-loaded via dynamic import

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | i18n — Lazy Loading |
| **Effort** | N/A |
| **Priority** | 16 |

**Description:** The i18n implementation correctly separates locale loading:
- English (`en.json`, 30.6 KB raw, ~8.4 KB gzipped) is statically imported in `locale.svelte.ts` and bundled into the initial chunk
- All 12 other locales use `await import(\`../../locales/${code}.json\`)` for on-demand loading
- Vite code-splits each locale into its own chunk (confirmed: `D1QIUBcK.js`, `DFAMjPfw.js`, `dqGjAJIM.js` contain non-English locale data and are NOT in index.html preloads)

The English locale contributes ~12 KB gzipped to the initial bundle (chunk `BtYoL1js.js` at 36 KB raw includes the i18n runtime + English dictionary).

**Evidence:**
`frontend/src/lib/i18n/locale.svelte.ts:5`
```typescript
import enDictionary from '../../locales/en.json';
```

`frontend/src/lib/i18n/locale.svelte.ts:73-76`
```typescript
const module = await import(`../../locales/${code}.json`);
activeDictionary = module.default as TranslationDictionary;
```

**Impact:** The initial i18n contribution is ~12 KB gzipped (English dictionary + i18n runtime), which is under the 15 KB target. Other locales add zero bytes to the initial bundle.

**Remediation:** None required. The lazy-loading implementation correctly meets the requirement that only the active locale is included in the initial bundle.

---

## PERF-013: No explicit manual chunking or bundle analysis configured in Vite

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Build Configuration |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** The `vite.config.ts` contains minimal configuration — only the SvelteKit plugin and dev server proxy. There is no `build.rollupOptions.output.manualChunks` configuration to control how dependencies are grouped. Vite/Rollup's automatic code-splitting is used exclusively. While the current output is acceptable, there's no guardrail preventing future dependency additions from landing in the initial bundle.

**Evidence:**
`frontend/vite.config.ts:1-14`
```typescript
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    proxy: {
      '/api': process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
      '/ws': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
        ws: true,
      },
    },
  },
});
```

**Impact:** No bundle size regression detection in CI. A new dependency could inadvertently inflate the initial bundle past the 200 KB target without automated detection. The lack of `rollup-plugin-visualizer` or similar tooling means bundle composition isn't easily auditable.

**Remediation:** Add `rollup-plugin-visualizer` for bundle analysis. Consider adding a CI check that asserts initial bundle gzipped size < 200 KB (e.g., `bundlesize` or a custom script comparing build output). Optionally add `manualChunks` to isolate known-large dependencies (CodeMirror, uPlot) into dedicated chunks with explicit boundaries.

---

## PERF-014: Static adapter `precompress: false` misses opportunity for pre-built compressed assets

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Bundle Size — Delivery |
| **Effort** | Small (hours) |
| **Priority** | 13 |

**Description:** The SvelteKit static adapter is configured with `precompress: false`, meaning no `.gz` or `.br` files are generated at build time. Since the frontend is embedded into the Go binary and served via Go's file server, compression depends entirely on the Go server's runtime compression middleware. If no compression middleware is configured, assets are served uncompressed (138 KB vs 50 KB for initial JS).

**Evidence:**
`frontend/svelte.config.js:5-10`
```javascript
adapter: adapter({
  pages: 'build',
  assets: 'build',
  fallback: 'index.html',
  precompress: false
})
```

No `.gz` files exist in `frontend/build/`.

**Impact:** If the Go embed server or reverse proxy doesn't apply on-the-fly gzip, users download 2.7x more bytes than necessary. Even with runtime compression, pre-compressed Brotli files would be ~15% smaller than runtime gzip.

**Remediation:** Enable `precompress: true` in svelte.config.js and configure the Go file server to serve pre-compressed files (check `Accept-Encoding` header, serve `.br` → `.gz` → raw). This eliminates runtime CPU cost of compression and enables Brotli (better compression than gzip). Restores the property of optimal transfer sizes for all clients.

---

## Summary

| Finding ID | Severity | Category | Title |
|------------|----------|----------|-------|
| PERF-001 | Informational | Virtualization | VirtualList DOM node cap correctly enforced |
| PERF-002 | Medium | Virtualization | VirtualList uses index-based keying (no DOM recycling) |
| PERF-003 | Informational | Bundle Size | Initial JS bundle 50 KB gzipped (well under 200 KB target) |
| PERF-004 | Low | Code Splitting | CodeMirror 127 KB gzipped, properly code-split |
| PERF-005 | Low | Code Splitting | Route node 5 at 39 KB gzipped exceeds 20 KB threshold |
| PERF-006 | Medium | Reactivity | HistoryChart uses legacy API, doesn't react to data changes |
| PERF-007 | Informational | Memory | uPlot properly destroyed on unmount |
| PERF-008 | Low | Layout Thrashing | getComputedStyle called on every chart mount |
| PERF-009 | Informational | Reactivity | Derived state depth ≤ 2, no cascading issues |
| PERF-010 | Informational | Subscriptions | All subscriptions properly cleaned up |
| PERF-011 | Informational | CSS | Efficient strategy, 7 KB gzipped, instant theme switch |
| PERF-012 | Informational | i18n | Only English in initial bundle, 12 locales lazy-loaded |
| PERF-013 | Low | Build Config | No manual chunking or bundle size CI guard |
| PERF-014 | Low | Delivery | precompress disabled, missing pre-built compressed assets |

### Findings by severity:
- **Critical:** 0
- **High:** 0
- **Medium:** 2 (PERF-002, PERF-006)
- **Low:** 4 (PERF-004, PERF-005, PERF-008, PERF-013, PERF-014)
- **Informational:** 7 (PERF-001, PERF-003, PERF-007, PERF-009, PERF-010, PERF-011, PERF-012)
