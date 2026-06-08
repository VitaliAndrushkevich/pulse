<script lang="ts">
  /**
   * ThemeSwitcher — toggles between light and dark themes.
   *
   * Reads the active theme from `document.documentElement.dataset.theme`,
   * toggles on click, persists to localStorage under 'pulse-theme',
   * and updates the `data-theme` attribute on the document root.
   *
   * Displays a sun icon when dark theme is active (switch TO light),
   * and a moon icon when light theme is active (switch TO dark).
   *
   * Requirements: 6.1–6.7
   */
  import { onMount } from 'svelte';

  let currentTheme = $state<'light' | 'dark'>('light');

  onMount(() => {
    const stored = document.documentElement.dataset.theme;
    if (stored === 'dark' || stored === 'light') {
      currentTheme = stored;
    }
  });

  function toggle(): void {
    const next: 'light' | 'dark' = currentTheme === 'light' ? 'dark' : 'light';
    currentTheme = next;
    document.documentElement.setAttribute('data-theme', next);

    try {
      localStorage.setItem('pulse-theme', next);
    } catch {
      // SecurityError in private browsing — toggle still works for session
    }
  }

  let isDark = $derived(currentTheme === 'dark');
  let ariaLabel = $derived(isDark ? 'Switch to light theme' : 'Switch to dark theme');
</script>

<button
  type="button"
  onclick={toggle}
  aria-label={ariaLabel}
  class="inline-flex items-center justify-center rounded-md p-2 transition-colors hover:bg-[var(--color-border)]"
  style="color: var(--color-text-primary);"
>
  {#if isDark}
    <!-- Sun icon: shown when dark theme is active (click will switch TO light) -->
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="5" />
      <line x1="12" y1="1" x2="12" y2="3" />
      <line x1="12" y1="21" x2="12" y2="23" />
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
      <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
      <line x1="1" y1="12" x2="3" y2="12" />
      <line x1="21" y1="12" x2="23" y2="12" />
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
      <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
    </svg>
  {:else}
    <!-- Moon icon: shown when light theme is active (click will switch TO dark) -->
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  {/if}
</button>
