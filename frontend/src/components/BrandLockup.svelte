<script lang="ts">
  /**
   * BrandLockup — combined Logo_Mark + Wordmark composition.
   *
   * Renders an inline SVG ECG peak icon with optional "Pulse" wordmark.
   * All dimensions scale proportionally from the `size` prop.
   *
   * Requirements: 1.1–1.7, 2.1–2.6, 3.1–3.5
   */

  interface BrandLockupProps {
    /** Logo_Mark height in pixels. All dimensions scale proportionally. */
    size?: number;
    /** "full" = Logo_Mark + Wordmark, "compact" = Logo_Mark only */
    variant?: 'full' | 'compact';
  }

  let { size = 32, variant = 'full' }: BrandLockupProps = $props();

  let gap = $derived(size / 4);
  let wordmarkFontSize = $derived(size * 0.625);
</script>

<div
  class="brand-lockup"
  style="display: inline-flex; align-items: center; gap: {gap}px; color: var(--color-brand-primary, #0ea5e9);"
>
  <svg
    aria-hidden="true"
    width={size}
    height={size}
    viewBox="0 0 32 32"
    fill="none"
    stroke="currentColor"
    stroke-width="3.2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <path d="M2 20 L8 20 L11 24 L16 8 L21 24 L24 20 L30 20" />
  </svg>

  {#if variant === 'full'}
    <span
      class="brand-wordmark"
      style="font-family: 'Inter', system-ui, -apple-system, sans-serif; font-weight: 600; letter-spacing: -0.025em; font-size: {wordmarkFontSize}px; line-height: 1; white-space: nowrap;"
    >
      Pulse
    </span>
  {/if}
</div>
