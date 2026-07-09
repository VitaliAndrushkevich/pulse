# Performance Findings — Frontend Network and Real-Time

## PERF-010: No request deduplication for concurrent identical fetches

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Network Efficiency |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The API client (`api.ts`) does not implement request deduplication. When the WebSocket reconnects, the layout's `onStatusChange` callback fires `getMonitors(1, 500)` while the dashboard page's `$effect` watching `connectionStore.status` simultaneously calls `dashboardStore.load()` (which internally calls `getDashboardSummary()`). Additionally, if a user navigates quickly between pages, identical requests to the same endpoint (e.g., `getMonitors`) can be dispatched concurrently without any shared-promise coalescing.

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53`
```svelte
onStatusChange(status) {
  // On reconnect: re-fetch full monitor list to reconcile missed patches
  if (status === 'connected') {
    getMonitors(1, 500)
      .then((response) => {
        monitorStore.setMonitors(response.data);
      })
```

`frontend/src/routes/+page.svelte:50-56`
```svelte
$effect(() => {
  const currentStatus = connectionStore.status;
  if (initialLoadDone && previousConnectionStatus !== 'connected' && currentStatus === 'connected') {
    // WS reconnected — re-fetch all data
    dashboardStore.load();
```

**Impact:** On WebSocket reconnection, at minimum 2 parallel requests fire (monitors list + dashboard summary). Without deduplication, repeated rapid navigations or reconnect oscillations could generate duplicate in-flight requests to the same endpoint, wasting bandwidth and server resources. At 500+ monitors with multiple open tabs, this multiplies.

**Remediation:** Implement a request deduplication layer in `apiRequest()` that tracks in-flight GET requests by URL. If an identical GET is already pending, return the same Promise. This restores the network efficiency property of at-most-one-in-flight per unique URL.

---

## PERF-011: No AbortController cancellation on component unmount or navigation

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Network Efficiency |
| **Effort** | Medium (days) |
| **Priority** | 10 |

**Description:** The `apiRequest()` function creates an AbortController solely for the 15-second timeout. No page component passes an AbortController signal for unmount cancellation. When a user navigates away from the monitor detail page (which fires 4 parallel API requests via `Promise.all`), or leaves the monitors list page, those in-flight requests continue to completion, potentially updating state for a component that is already destroyed.

**Evidence:**
`frontend/src/routes/monitors/[id]/+page.svelte:78-89`
```svelte
async function fetchData() {
  loading = true;
  error = null;
  notFound = false;

  try {
    const id = monitorId;
    const to = new Date().toISOString();
    const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();

    const [monitorData, historyData, incidentsData, statsData] = await Promise.all([
      getMonitor(id),
```

No `onDestroy` or AbortController cleanup exists in any route page (verified via grep — only the layout uses `onDestroy` for WS disconnect).

**Impact:** Wasted bandwidth on completed-but-unused responses. Potential state corruption if a response arrives after the user navigated to a different monitor (the `.then` updates the now-obsolete local state). On slow connections with many monitors, abandoned requests consume connection slots that delay new page loads.

**Remediation:** Extend `apiRequest()` to accept an external `AbortSignal`. In each page component, create a per-fetch AbortController in `$effect` and abort it in the cleanup function (Svelte 5 effect cleanup). This restores the resource efficiency property that no abandoned request outlives its owning component.

---

## PERF-012: Reconnect refetch triggers simultaneous redundant monitor fetches

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | Network Efficiency |
| **Effort** | Small (hours) |
| **Priority** | 14 |

**Description:** On WebSocket reconnection, the layout unconditionally calls `getMonitors(1, 500)` to refresh the store. Simultaneously, the dashboard page's reconnect watcher calls `dashboardStore.load()` which fetches `/dashboard/summary`. If the user is on the monitors list page, that page may also have its own monitor fetch in flight. The monitor list refetch from the layout is correct (requirement 7.6 mandates full refetch on reconnect), but it overlaps with per-page data loading without coordination.

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53` — layout fires `getMonitors(1, 500)` on every `'connected'` status, including the **initial** connection (not just reconnections).

```typescript
onStatusChange(status) {
  if (status === 'connected') {
    getMonitors(1, 500)
```

This means on first page load, the layout fetches all monitors AND whichever page the user lands on also fetches its own data. The monitors list page fetches `getMonitors(page, 20)` separately.

**Impact:** Extra network request on each initial connection and reconnection. Moderate impact at scale (500+ monitors JSON payload fetched twice on monitor list page). Not a correctness issue but a bandwidth waste.

**Remediation:** Gate the layout's refetch behind a flag that tracks whether this is a *re*connection (not the first connection). Page-level initial loads handle the first fetch. Alternatively, implement the deduplication layer from PERF-010 which would coalesce these naturally. Restores the minimal-requests property.

---

## PERF-013: WebSocket reconnection strategy is sound but lacks cross-tab coordination

| Field | Value |
|-------|-------|
| **Severity** | Low |
| **Category** | WebSocket Performance |
| **Effort** | Medium (days) |
| **Priority** | 15 |

**Description:** The WebSocket reconnection implements exponential backoff (1s base, 2× multiplier, 30s cap) with ±25% jitter via `getBackoffDelay()`. This is a well-designed thundering herd mitigation for a single tab. However, there is no cross-tab coordination mechanism (e.g., BroadcastChannel or SharedWorker) to prevent 10+ open tabs from independently reconnecting after a server restart. Each tab computes its own jitter independently, and while the ±25% spread reduces exact overlap, with many tabs the reconnection window still clusters within ~15 seconds (30s ± 7.5s at max backoff).

**Evidence:**
`frontend/src/lib/ws.ts:80-89`
```typescript
export function getBackoffDelay(attempt: number): number {
  const base = 1000;
  const max = 30000;
  const multiplier = 2;
  const delay = Math.min(base * Math.pow(multiplier, attempt), max);
  const jitter = delay * 0.25 * (Math.random() * 2 - 1);
  return delay + jitter;
}
```

**Impact:** For a self-hosted platform typically used by a small team (1-5 users, maybe 2-3 tabs each), this is negligible. However, if deployed in a shared environment with many concurrent dashboards open, a server restart could cause a reconnection burst. The 500-monitor design target suggests this is an edge case.

**Remediation:** For future scale, consider a `BroadcastChannel`-based leader election where only one tab maintains the WebSocket connection and broadcasts patches to other tabs via `postMessage`. This is a significant architectural change — low priority given the self-hosted, small-team deployment model. Current implementation meets requirement 7.1 for reasonable thundering herd prevention.

---

## PERF-014: Static font assets served with no-cache instead of immutable Cache-Control

| Field | Value |
|-------|-------|
| **Severity** | Medium |
| **Category** | Static Asset Caching |
| **Effort** | Small (hours) |
| **Priority** | 11 |

**Description:** The `isHashedAsset()` function in the Go router only recognizes files under `_app/` as cacheable with `max-age=31536000, immutable`. However, self-hosted font files (`/fonts/inter-semibold.woff2`) and brand assets (`/brand/*.svg`, `/brand/*.png`) are stored in SvelteKit's `static/` directory (which outputs to the build root, not under `_app/`). These files receive `Cache-Control: no-cache`, forcing browsers to revalidate on every page navigation despite being immutable content.

**Evidence:**
`backend/internal/api/router.go:338-342`
```go
func isHashedAsset(filePath string) bool {
	// SvelteKit puts hashed assets under _app/immutable/
	if strings.HasPrefix(filePath, "_app/") {
		return true
	}
	return false
}
```

`frontend/src/app.css:1-8`
```css
@font-face {
  font-family: 'Inter';
  font-style: normal;
  font-weight: 600;
  font-display: swap;
  src: url('/fonts/inter-semibold.woff2') format('woff2');
}
```

**Impact:** The Inter WOFF2 font (~15-25 KB) is re-downloaded or revalidated on every page load in production. Brand assets (SVG, PNG) similarly miss the immutable cache. Over many page views this wastes bandwidth and increases perceived load time (FOIT/FOUT on font revalidation). Not critical since `font-display: swap` prevents blocking, but unnecessary network overhead.

**Remediation:** Extend `isHashedAsset()` to also match known-immutable static paths: `fonts/`, `brand/`, and files with explicit content hashes. Alternatively, rename font files with content hashes at build time (e.g., `inter-semibold.abc123.woff2`) and update the CSS reference, then they'd live under `_app/` naturally. Restores the cache-once-serve-forever property for immutable assets.

---

## PERF-015: Patch-merge update pattern is efficient and correctly implemented

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Real-Time Update Efficiency |
| **Effort** | — |
| **Priority** | — |

**Description:** The monitor store's patch-merge implementation meets all requirement 7.2 criteria:

1. **O(1) lookup**: Uses `Map<string, Monitor>` — `monitors.get(patch.monitor_id)` is constant-time.
2. **Field-level merge**: Only `state` and `last_checked_at` are merged from the patch (via `applyMonitorPatch`), preserving all other monitor fields.
3. **Unknown ID handling**: Patches for unknown `monitor_id` values are silently discarded without error or side effect.
4. **Deterministic**: Same monitor + same patch = same result (pure function `applyMonitorPatch` with spread operator).

The implementation creates a new Map on each patch (`new Map(monitors)`) which is O(n) in map size, but this is necessary for Svelte 5 reactivity (immutable reference change triggers re-render). With 500 monitors, Map copy is <1ms and negligible.

**Evidence:**
`frontend/src/lib/stores/monitors.svelte.ts:63-71`
```typescript
function applyPatch(patch: MonitorPatch): void {
  const existing = monitors.get(patch.monitor_id);
  if (!existing) {
    // Discard patch for unknown monitor_id
    return;
  }
  const updated = applyMonitorPatch(existing, patch);
  const next = new Map(monitors);
  next.set(patch.monitor_id, updated);
  monitors = next;
}
```

**Impact:** No negative impact. The pattern is sound and efficient for the 500+ monitor target.

**Remediation:** No action required. For extreme scale (10,000+ monitors), consider batch-applying multiple patches before creating the new Map reference, but this is unnecessary at current scale.

---

## PERF-016: Full monitor list refetch on WebSocket reconnection is correctly implemented

| Field | Value |
|-------|-------|
| **Severity** | Informational |
| **Category** | Real-Time State Reconciliation |
| **Effort** | — |
| **Priority** | — |

**Description:** Requirement 7.6 mandates that the client performs a full monitor list refetch upon successful WebSocket reconnection before resuming patch-merge processing. This is correctly implemented in the layout component's `onStatusChange` callback: when status transitions to `'connected'`, the client fetches `getMonitors(1, 500)` and replaces the entire store via `monitorStore.setMonitors()`.

The implementation also correctly handles the ordering guarantee: `setMonitors` is called from the `.then()` of the fetch promise, and the WebSocket `handleMessage` function has already reset `reconnectAttempt = 0` and set status to `'connected'` by the time patches resume. Since `setMonitors` does a full replace (new Map from scratch), any patches that arrive during the fetch in-flight are harmless — they'll either merge into the new state or be discarded (if the fetched list doesn't include that monitor).

**Evidence:**
`frontend/src/routes/+layout.svelte:46-53`
```svelte
onStatusChange(status) {
  if (status === 'connected') {
    getMonitors(1, 500)
      .then((response) => {
        monitorStore.setMonitors(response.data);
      })
```

`frontend/src/lib/stores/monitors.svelte.ts:52-57`
```typescript
function setMonitors(monitorList: Monitor[]): void {
  const next = new Map<string, Monitor>();
  for (const m of monitorList) {
    next.set(m.id, m);
  }
  monitors = next;
}
```

**Impact:** No negative impact. Correctly reconciles state after disconnection.

**Remediation:** No action required. One minor enhancement: if the fleet exceeds the 500-monitor page limit, the refetch should paginate or use a dedicated "all IDs" endpoint. Currently the `limit: 500` parameter could silently miss monitors if the fleet grows beyond 500.

---

## Summary

| Finding | Severity | Category | Priority |
|---------|----------|----------|----------|
| PERF-010 | Medium | Network Efficiency | 10 |
| PERF-011 | Medium | Network Efficiency | 10 |
| PERF-012 | Low | Network Efficiency | 14 |
| PERF-013 | Low | WebSocket Performance | 15 |
| PERF-014 | Medium | Static Asset Caching | 11 |
| PERF-015 | Informational | Real-Time Update Efficiency | — |
| PERF-016 | Informational | Real-Time State Reconciliation | — |

**Requirements Coverage:**
- 7.1 (Thundering herd prevention): ✅ PERF-013 — implemented correctly with backoff + jitter
- 7.2 (Patch-merge efficiency): ✅ PERF-015 — all criteria met
- 7.3 (API request patterns): ⚠️ PERF-010 — missing deduplication
- 7.4 (AbortController wiring): ⚠️ PERF-011 — timeout works, unmount cancellation missing
- 7.5 (Static asset caching): ⚠️ PERF-014 — hashed assets correct, static fonts/brand miss cache
- 7.6 (Full refetch on reconnect): ✅ PERF-016 — correctly implemented
