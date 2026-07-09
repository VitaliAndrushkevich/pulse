<script lang="ts">
  import '../app.css';
  import type { Snippet } from 'svelte';
  import { isAuthenticated, clearToken } from '$lib/stores/auth.svelte';
  import Toast from '../components/Toast.svelte';
  import ConnectionBadge from '../components/ConnectionBadge.svelte';
  import ThemeSwitcher from '../components/ThemeSwitcher.svelte';
  import BrandLockup from '../components/BrandLockup.svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createWsClient } from '$lib/ws';
  import { getMonitors } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { onDestroy } from 'svelte';
  import { initLocale, t } from '$lib/i18n';

  let { children }: { children: Snippet } = $props();

  // Initialize locale from localStorage before first render
  initLocale();

  let currentPath = $derived($page.url.pathname);
  let isPublicRoute = $derived(currentPath === '/login' || currentPath === '/setup');
  let authed = $derived(isAuthenticated());

  const navLinks = [
    { href: '/', key: 'nav.dashboard' },
    { href: '/monitors', key: 'nav.monitors' },
    { href: '/notifications', key: 'nav.notifications' },
    { href: '/settings', key: 'nav.settings' }
  ];

  // Create the WS client at module level (once per layout lifecycle)
  const wsClient = createWsClient({
    url: '/ws',
    onStatusChange(status) {
      // On reconnect: re-fetch full monitor list to reconcile missed patches
      if (status === 'connected') {
        getMonitors(1, 500)
          .then((response) => {
            monitorStore.setMonitors(response.data);
          })
          .catch(() => {
            // Errors are handled by the API client (toast notifications)
          });
      }
    }
  });

  // Reactively connect/disconnect WS based on auth state and route
  $effect(() => {
    if (authed && !isPublicRoute) {
      wsClient.connect();
    } else {
      wsClient.disconnect();
    }
  });

  // Redirect unauthenticated users to login
  $effect(() => {
    if (!isPublicRoute && !authed) {
      goto('/login');
    }
  });

  // Ensure WS disconnects on component destroy (e.g. full page unload)
  onDestroy(() => {
    wsClient.disconnect();
  });

  function logout() {
    wsClient.disconnect();
    clearToken();
    goto('/login');
  }
</script>

<div class="min-h-screen bg-[var(--color-bg-page)]">
  {#if isPublicRoute}
    <main class="mx-auto max-w-6xl px-6 py-8">
      {@render children()}
    </main>
  {:else if authed}
    <header class="border-b border-[var(--color-border)] backdrop-blur" style="background-color: color-mix(in srgb, var(--color-bg-surface) 80%, transparent)">
      <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <a href="/" aria-label="Pulse — Home" class="inline-flex items-center">
          <span class="hidden sm:inline-flex">
            <BrandLockup size={32} variant="full" />
          </span>
          <span class="inline-flex sm:hidden">
            <BrandLockup size={32} variant="compact" />
          </span>
        </a>
        <nav class="flex items-center gap-2">
          {#each navLinks as link}
            <a
              href={link.href}
              class="rounded-md px-3 py-2 text-sm font-medium text-[var(--color-text-primary)] transition hover:bg-brand-50 hover:text-brand-700"
              class:bg-brand-50={currentPath === link.href}
              class:text-brand-700={currentPath === link.href}
            >
              {t(link.key)}
            </a>
          {/each}
          <ConnectionBadge />
          <ThemeSwitcher />
          <button
            onclick={logout}
            class="ml-2 rounded-md px-3 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition hover:bg-red-50 hover:text-red-700"
          >
            {t('nav.logout')}
          </button>
        </nav>
      </div>
    </header>

    <main class="mx-auto max-w-6xl px-6 py-8">
      {@render children()}
    </main>
  {/if}
</div>

<Toast />
