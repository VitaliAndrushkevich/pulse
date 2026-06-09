<script lang="ts">
  /**
   * ThemeSwitcher — cycles between light, dark, and system themes.
   *
   * Three modes:
   *   - 'light': forced light theme
   *   - 'dark': forced dark theme
   *   - 'system': follows OS preference via prefers-color-scheme
   *
   * Persists the chosen mode to localStorage under 'pulse-theme-mode'.
   * When mode is 'system', listens to matchMedia changes and updates
   * the `data-theme` attribute reactively.
   *
   * Icons:
   *   - Sun: light mode active
   *   - Moon: dark mode active
   *   - Monitor/Desktop: system mode active
   */
  import { onMount, onDestroy } from 'svelte';

  type ThemeMode = 'light' | 'dark' | 'system';

  let mode = $state<ThemeMode>('system');
  let mediaQuery: MediaQueryList | null = null;
  let mediaHandler: ((e: MediaQueryListEvent) => void) | null = null;

  function getSystemTheme(): 'light' | 'dark' {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  function applyTheme(m: ThemeMode): void {
    const resolved = m === 'system' ? getSystemTheme() : m;
    document.documentElement.setAttribute('data-theme', resolved);
  }

  function persist(m: ThemeMode): void {
    try {
      if (m === 'system') {
        localStorage.removeItem('pulse-theme');
        localStorage.setItem('pulse-theme-mode', 'system');
      } else {
        localStorage.setItem('pulse-theme', m);
        localStorage.setItem('pulse-theme-mode', m);
      }
    } catch {
      // SecurityError in private browsing — toggle still works for session
    }
  }

  function setupMediaListener(): void {
    cleanupMediaListener();
    if (mode === 'system') {
      mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      mediaHandler = () => applyTheme('system');
      mediaQuery.addEventListener('change', mediaHandler);
    }
  }

  function cleanupMediaListener(): void {
    if (mediaQuery && mediaHandler) {
      mediaQuery.removeEventListener('change', mediaHandler);
      mediaQuery = null;
      mediaHandler = null;
    }
  }

  function cycle(): void {
    const order: ThemeMode[] = ['light', 'dark', 'system'];
    const idx = order.indexOf(mode);
    const next = order[(idx + 1) % order.length];
    mode = next;
    applyTheme(next);
    persist(next);
    setupMediaListener();
  }

  onMount(() => {
    // Read stored mode — backwards compatible with old 'pulse-theme' key
    let stored: ThemeMode = 'system';
    try {
      const modeVal = localStorage.getItem('pulse-theme-mode');
      if (modeVal === 'light' || modeVal === 'dark' || modeVal === 'system') {
        stored = modeVal;
      } else {
        // Legacy: only had 'pulse-theme' with light/dark
        const legacy = localStorage.getItem('pulse-theme');
        if (legacy === 'light' || legacy === 'dark') {
          stored = legacy;
        }
      }
    } catch {
      // Private browsing
    }
    mode = stored;
    applyTheme(mode);
    setupMediaListener();
  });

  onDestroy(() => {
    cleanupMediaListener();
  });

  let ariaLabel = $derived(
    mode === 'light'
      ? 'Switch to dark theme'
      : mode === 'dark'
        ? 'Switch to system theme'
        : 'Switch to light theme'
  );

  let tooltip = $derived(
    mode === 'light' ? 'Light' : mode === 'dark' ? 'Dark' : 'System'
  );
</script>

<button
  type="button"
  onclick={cycle}
  aria-label={ariaLabel}
  title={tooltip}
  class="inline-flex items-center justify-center rounded-md p-2 transition-colors hover:bg-[var(--color-border)]"
  style="color: var(--color-text-primary);"
>
  {#if mode === 'dark'}
    <!-- Moon icon: dark mode forced -->
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
  {:else if mode === 'system'}
    <!-- Monitor icon: system/OS preference -->
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
      <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
      <line x1="8" y1="21" x2="16" y2="21" />
      <line x1="12" y1="17" x2="12" y2="21" />
    </svg>
  {:else}
    <!-- Sun icon: light mode forced -->
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
  {/if}
</button>
