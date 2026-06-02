<script lang="ts">
  import '../app.css';
  import { isAuthenticated, clearToken } from '$lib/stores/auth.svelte';
  import Toast from '../components/Toast.svelte';
  import ConnectionBadge from '../components/ConnectionBadge.svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { createWsClient } from '$lib/ws';
  import { getMonitors } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { onDestroy } from 'svelte';

  let currentPath = $derived($page.url.pathname);
  let isPublicRoute = $derived(currentPath === '/login' || currentPath === '/setup');
  let authed = $derived(isAuthenticated());

  const navLinks = [
    { href: '/', label: 'Dashboard' },
    { href: '/monitors', label: 'Monitors' },
    { href: '/settings', label: 'Settings' }
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

<div class="min-h-screen bg-gradient-to-b from-sky-100 via-slate-50 to-slate-100">
  {#if isPublicRoute}
    <main class="mx-auto max-w-6xl px-6 py-8">
      <slot />
    </main>
  {:else if authed}
    <header class="border-b border-slate-200/70 bg-white/80 backdrop-blur">
      <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <a href="/" class="text-xl font-semibold tracking-tight text-brand-700">Pulse</a>
        <nav class="flex items-center gap-2">
          {#each navLinks as link}
            <a
              href={link.href}
              class="rounded-md px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-brand-50 hover:text-brand-700"
              class:bg-brand-50={currentPath === link.href}
              class:text-brand-700={currentPath === link.href}
            >
              {link.label}
            </a>
          {/each}
          <ConnectionBadge />
          <button
            onclick={logout}
            class="ml-2 rounded-md px-3 py-2 text-sm font-medium text-slate-500 transition hover:bg-red-50 hover:text-red-700"
          >
            Logout
          </button>
        </nav>
      </div>
    </header>

    <main class="mx-auto max-w-6xl px-6 py-8">
      <slot />
    </main>
  {/if}
</div>

<Toast />
