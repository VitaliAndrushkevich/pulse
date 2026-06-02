<script lang="ts" module>
  /**
   * VirtualList — a fixed-height row virtual scroller with DOM recycling.
   *
   * Renders only the visible rows plus a configurable buffer,
   * keeping total rendered DOM nodes ≤ 60 for large collections.
   *
   * Exposes scrollToIndex() and scrollToTop() via bind:this.
   */
  export interface VirtualListAPI {
    scrollToIndex: (index: number) => void;
    scrollToTop: () => void;
  }
</script>

<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';

  interface Props {
    items: T[];
    itemHeight: number;
    bufferCount?: number;
    containerHeight?: number;
    row: Snippet<[T, number]>;
  }

  let {
    items,
    itemHeight,
    bufferCount = 10,
    containerHeight,
    row,
  }: Props = $props();

  // Clamp buffer to [5, 20]
  let clampedBuffer = $derived(Math.max(5, Math.min(20, bufferCount)));

  // Container element ref
  let container: HTMLDivElement | undefined = $state(undefined);

  // Scroll state
  let scrollTop = $state(0);
  let rafPending = $state(false);

  // Effective container height
  let effectiveHeight = $derived(containerHeight ?? 600);

  // Total virtual height for scrollbar sizing
  let totalHeight = $derived(items.length * itemHeight);

  // Visible row count (how many fit in the viewport)
  let visibleCount = $derived(Math.ceil(effectiveHeight / itemHeight));

  // Compute start and end indices with buffer
  let startIndex = $derived(Math.max(0, Math.floor(scrollTop / itemHeight) - clampedBuffer));

  let rawEndIndex = $derived(Math.floor(scrollTop / itemHeight) + visibleCount + clampedBuffer);

  // Enforce max 60 rendered rows
  let endIndex = $derived.by(() => {
    const maxRendered = 60;
    const rawEnd = Math.min(rawEndIndex, items.length);
    const count = rawEnd - startIndex;
    if (count > maxRendered) {
      return startIndex + maxRendered;
    }
    return rawEnd;
  });

  // Visible slice of items
  let visibleItems = $derived(items.slice(startIndex, endIndex));

  // Spacer heights
  let topSpacer = $derived(startIndex * itemHeight);
  let bottomSpacer = $derived(Math.max(0, (items.length - endIndex) * itemHeight));

  // RAF-throttled scroll handler
  function handleScroll() {
    if (rafPending) return;
    rafPending = true;
    requestAnimationFrame(() => {
      if (container) {
        scrollTop = container.scrollTop;
      }
      rafPending = false;
    });
  }

  // Public API exposed via bind:this
  export function scrollToIndex(index: number): void {
    if (!container) return;
    const clampedIndex = Math.max(0, Math.min(index, items.length - 1));
    container.scrollTop = clampedIndex * itemHeight;
    scrollTop = container.scrollTop;
  }

  export function scrollToTop(): void {
    if (!container) return;
    container.scrollTop = 0;
    scrollTop = 0;
  }
</script>

<div
  bind:this={container}
  class="virtual-list-container"
  style="height: {effectiveHeight}px; overflow-y: auto; position: relative;"
  onscroll={handleScroll}
  role="list"
>
  <!-- Top spacer maintains scroll position -->
  <div style="height: {topSpacer}px;" aria-hidden="true"></div>

  <!-- Rendered visible rows -->
  {#each visibleItems as item, i (startIndex + i)}
    <div
      class="virtual-list-row"
      style="height: {itemHeight}px;"
      role="listitem"
    >
      {@render row(item, startIndex + i)}
    </div>
  {/each}

  <!-- Bottom spacer maintains scrollbar size -->
  <div style="height: {bottomSpacer}px;" aria-hidden="true"></div>
</div>
