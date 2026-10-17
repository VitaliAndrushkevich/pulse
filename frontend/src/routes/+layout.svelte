<script lang="ts">
  import '../app.css';
  import type { Snippet } from 'svelte';
  import { isAuthenticated, clearToken, getUser, getToken, setUser, isAdmin, mustChangePassword } from '$lib/stores/auth.svelte';
  import Toast from '../components/Toast.svelte';
  import ConnectionBadge from '../components/ConnectionBadge.svelte';
  import ThemeSwitcher from '../components/ThemeSwitcher.svelte';
  import BrandLockup from '../components/BrandLockup.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/stores';
  import { createWsClient } from '$lib/ws';
  import { canAccessRoute } from '$lib/access';
  import { getMonitors, getCurrentUser } from '$lib/api';
  import { monitorStore } from '$lib/stores/monitors.svelte';
  import { onMount, onDestroy } from 'svelte';
  import { initLocale, t } from '$lib/i18n';

  let { children }: { children: Snippet } = $props();

  // Initialize locale from localStorage before first render
  initLocale();

  let currentPath = $derived($page.url.pathname);
  let isPublicRoute = $derived(currentPath === '/login' || currentPath === '/setup');
  let authed = $derived(isAuthenticated());
  let sessionError = $state(false);
  let user = $derived(getUser());
  let allowed = $derived(canAccessRoute(user, currentPath));
  let passwordRequired = $derived(mustChangePassword());
  let verifiedToken: string | null = null;

  async function verifySession() {
    const token = getToken();
    if (!token) return;
    sessionError = false;
    try {
      const currentUser = await getCurrentUser();
      if (getToken() !== token) return;
      if (!currentUser.enabled) clearToken();
      else setUser(currentUser);
    } catch {
      if (getToken() === token) sessionError = true;
    }
  }
  $effect(() => {
    const token = getToken();
    if (token && token !== verifiedToken) {
      verifiedToken = token;
      void verifySession();
    }
    if (!token) verifiedToken = null;
  });

  let sidebarCollapsed = $state(false);
  const SIDEBAR_STORAGE_KEY = 'pulse-sidebar-collapsed';

  onMount(() => {
    try {
      sidebarCollapsed = localStorage.getItem(SIDEBAR_STORAGE_KEY) === 'true';
    } catch {
      // The sidebar remains usable when storage is unavailable.
    }
  });

  function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed;
    try {
      localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarCollapsed));
    } catch {
      // Keep the preference for this session when storage is unavailable.
    }
  }

  const navLinks = [
    { href: '/', key: 'nav.dashboard', icon: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z' },
    { href: '/monitors', key: 'nav.monitors', icon: 'M3 12h4l3-8 4 16 3-8h4' },
    { href: '/heartbeats', key: 'nav.heartbeats', icon: 'M12 3v4 M12 17v4 M3 12h4 M17 12h4 M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8' },
    { href: '/notifications', key: 'nav.notifications', icon: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9 M10 21h4' },
    { href: '/settings', key: 'nav.settings', icon: 'M4 7h16 M4 17h16 M9 4v6 M15 14v6' }
  ] as const;

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

  let connectedToken: string | null = null;

  // Reactively connect/disconnect WS based on auth state and route
  $effect(() => {
    const sessionToken = getToken();
    if (authed && user && !passwordRequired && !isPublicRoute && allowed) {
      if (connectedToken !== sessionToken) wsClient.disconnect();
      connectedToken = sessionToken;
      wsClient.connect();
    } else {
      connectedToken = null;
      wsClient.disconnect();
    }
  });

  // Redirect unauthenticated users to login
  $effect(() => {
    if (!isPublicRoute && !authed) {
      goto(resolve('/login', {}));
    }
  });

  $effect(() => {
    if (!authed || !user || isPublicRoute) return;
    if (passwordRequired && currentPath !== '/change-password') goto('/change-password');
    else if (!passwordRequired && currentPath === '/change-password') goto('/');
    else if (!allowed) goto('/');
  });

  // Ensure WS disconnects on component destroy (e.g. full page unload)
  onDestroy(() => {
    wsClient.disconnect();
  });

  function logout() {
    wsClient.disconnect();
    clearToken();
    goto(resolve('/login', {}));
  }
</script>

<div class={['app-shell', { 'is-collapsed': sidebarCollapsed }]}>
  {#if isPublicRoute}
    <main id="main-content" class="public-content">
      {@render children()}
    </main>
  {:else if authed && sessionError}
    <main class="public-content">
      <p role="alert">{t('users.sessionError')}</p>
      <button type="button" onclick={verifySession}>{t('common.retry')}</button>
      <button type="button" onclick={logout}>{t('nav.logout')}</button>
    </main>
  {:else if authed && user && passwordRequired && currentPath === '/change-password'}
    <main id="main-content" class="public-content">{@render children()}</main>
  {:else if authed && user && !passwordRequired && allowed}
    <a class="skip-link" href="#main-content">{t('shell.skipToContent')}</a>
    <aside class="navigation-rail">
      <div class="rail-top">
        <a href={resolve('/', {})} aria-label={t('nav.home')} class="rail-brand">
          <span class="full-brand"><BrandLockup size={36} variant="full" /></span>
          <span class="compact-brand"><BrandLockup size={32} variant="compact" /></span>
        </a>
        <button
          type="button"
          class="sidebar-toggle"
          onclick={toggleSidebar}
          aria-label={t(sidebarCollapsed ? 'shell.expandNavigation' : 'shell.collapseNavigation')}
          aria-expanded={!sidebarCollapsed}
          aria-controls="main-navigation"
          title={t(sidebarCollapsed ? 'shell.expandNavigation' : 'shell.collapseNavigation')}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <rect x="3" y="4" width="18" height="16" rx="2" />
            <path d="M9 4v16" />
            <path d={sidebarCollapsed ? 'm13 9 3 3-3 3' : 'm16 9-3 3 3 3'} />
          </svg>
        </button>
      </div>
      <div class="rail-caption">{t('shell.navigation')}</div>
      <nav id="main-navigation" aria-label={t('shell.navigation')} class="rail-links">
        {#each navLinks.filter((link) => link.href !== '/notifications' || isAdmin()) as link (link.href)}
          {@const active = link.href === '/' ? currentPath === '/' : currentPath === link.href || currentPath.startsWith(link.href + '/')}
          <a href={resolve(link.href, {})} class={['rail-link', { active }]} aria-label={t(link.key)} aria-current={active ? 'page' : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d={link.icon} />
            </svg>
            <span class="link-label">{t(link.key)}</span>
            <span class="rail-tooltip" aria-hidden="true">{t(link.key)}</span>
          </a>
        {/each}
      </nav>
      <div class="rail-footer">
        {#if user.role === 'read_only'}<p class="mb-2 px-2 text-xs">{t('users.readOnly')}</p>{/if}
        <div class="rail-status"><ConnectionBadge compact={sidebarCollapsed} /></div>
        <div class="rail-actions">
          <ThemeSwitcher />
          <button type="button" onclick={logout} class="logout-button" aria-label={t('nav.logout')} title={t('nav.logout')}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M9 4H4v16h5 M14 8l4 4-4 4 M8 12h13" />
            </svg>
            <span class="logout-label">{t('nav.logout')}</span>
          </button>
        </div>
        <span class="rail-signature">{t('shell.signature')}</span>
      </div>
    </aside>

    <div class="workspace">
      <main id="main-content" class="workspace-content">
        {@render children()}
      </main>
    </div>
  {/if}
</div>

<Toast />

<style>
  .app-shell { min-height: 100vh; --rail-width: 14rem; }
  .public-content { max-width: 80rem; margin-inline: auto; padding: 2rem 1rem; }
  .navigation-rail {
    position: fixed;
    inset-block: 0;
    inset-inline-start: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
    width: var(--rail-width);
    padding: 1.75rem 1rem 1.25rem;
    background: var(--color-rail);
    color: var(--color-rail-text);
    border-inline-end: 1px solid var(--color-rail-line);
  }
  .rail-top { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; margin-block-end: 3rem; }
  .rail-brand { display: inline-flex; align-items: center; margin-inline-start: 0.5rem; --color-brand-primary: var(--color-rail-accent); }
  .full-brand { display: inline-flex; }
  .compact-brand { display: none; }
  .sidebar-toggle { display: inline-flex; flex-shrink: 0; align-items: center; justify-content: center; width: 2rem; height: 2rem; border-radius: 0.25rem; }
  .sidebar-toggle:hover { background: var(--color-rail-line); color: var(--color-rail-active); }
  .sidebar-toggle svg { width: 1.1rem; height: 1.1rem; }
  :global([dir="rtl"]) .sidebar-toggle svg { transform: scaleX(-1); }
  .rail-caption, .rail-signature {
    font-family: var(--font-data);
    font-size: 0.625rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }
  .rail-caption { margin: 0 0.75rem 1rem; }
  .rail-links { display: flex; flex-direction: column; gap: 0.35rem; min-height: 0; overflow-y: auto; }
  .rail-link {
    display: flex;
    gap: 0.85rem;
    align-items: center;
    position: relative;
    flex-shrink: 0;
    min-height: 2.8rem;
    padding: 0.65rem 0.75rem;
    border-radius: 0.25rem;
    font-size: 0.8125rem;
    transition: background 120ms, color 120ms;
  }
  .rail-link svg { width: 1.1rem; height: 1.1rem; flex-shrink: 0; }
  .rail-link:hover, .rail-link.active { background: var(--color-rail-line); color: var(--color-rail-active); }
  .rail-link.active::before { content: ''; position: absolute; inset-inline-start: 0; width: 3px; height: 1.3rem; background: var(--color-rail-accent); }
  .rail-link.active svg { color: var(--color-rail-accent); }
  .rail-tooltip { display: none; }
  .rail-footer {
    margin-block-start: auto;
    padding-block-start: 1rem;
    flex-shrink: 0;
    border-block-start: 1px solid var(--color-rail-line);
    --color-text-primary: var(--color-rail-text);
    --color-border: var(--color-rail-line);
    --color-success: var(--color-rail-success);
    --color-warning: var(--color-rail-warning);
  }
  .rail-status { display: flex; min-height: 2rem; margin-block-end: 0.5rem; }
  .rail-actions { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; }
  .logout-button { display: inline-flex; align-items: center; justify-content: center; gap: 0.5rem; min-height: 2.5rem; padding: 0.5rem; font-size: 0.75rem; border-radius: 0.25rem; }
  .logout-button svg { width: 1.1rem; height: 1.1rem; flex-shrink: 0; }
  .logout-button:hover { color: var(--color-rail-active); background: var(--color-rail-line); }
  .rail-signature { display: block; margin-block-start: 1.25rem; padding-inline: 0.5rem; font-size: 0.55rem; line-height: 1.8; }
  .workspace { margin-inline-start: var(--rail-width); }
  .workspace-content { max-width: 90rem; padding: 2.5rem; margin-inline: auto; }
  .skip-link { position: fixed; inset-block-start: -5rem; inset-inline-start: 1rem; z-index: 50; padding: 0.75rem 1rem; background: var(--color-bg-surface); color: var(--color-text-primary); border: 1px solid var(--color-border); }
  .skip-link:focus { inset-block-start: 1rem; }
  @media (min-width: 1024px) {
    .is-collapsed { --rail-width: 4.5rem; }
    .is-collapsed .navigation-rail { padding-inline: 0.75rem; }
    .is-collapsed .rail-top { flex-direction: column; gap: 1rem; margin-block-end: 2rem; }
    .is-collapsed .rail-brand { margin-inline-start: 0; }
    .is-collapsed .full-brand, .is-collapsed .rail-caption, .is-collapsed .link-label,
    .is-collapsed .logout-label, .is-collapsed .rail-signature { display: none; }
    .is-collapsed .compact-brand { display: inline-flex; }
    .is-collapsed .rail-links { overflow: visible; }
    .is-collapsed .rail-link { justify-content: center; padding-inline: 0.5rem; }
    .is-collapsed .rail-link.active::before { inset-inline-start: 0; }
    .is-collapsed .rail-footer { padding-block-start: 1rem; }
    .is-collapsed .rail-status { justify-content: center; }
    .is-collapsed .rail-actions { flex-direction: column; }
    .is-collapsed .rail-link:hover .rail-tooltip,
    .is-collapsed .rail-link:focus-visible .rail-tooltip {
      display: block;
      position: absolute;
      inset-inline-start: calc(100% + 0.75rem);
      top: 50%;
      transform: translateY(-50%);
      width: max-content;
      max-width: 15rem;
      padding: 0.5rem 0.75rem;
      border: 1px solid var(--color-rail-line);
      border-radius: 0.25rem;
      background: var(--color-rail);
      color: var(--color-rail-active);
      font-size: 0.75rem;
      pointer-events: none;
    }
  }
  @media (max-width: 1023px) {
    .navigation-rail { position: relative; width: auto; padding: 1rem 1rem 0.75rem; border-inline-end: 0; }
    .rail-top { margin-block-end: 1rem; }
    .rail-brand { margin-inline-start: 0; }
    .rail-caption, .rail-signature, .sidebar-toggle { display: none; }
    .rail-links { flex-direction: row; overflow-x: auto; gap: 0.5rem; padding-block-end: 0.75rem; }
    .rail-link { gap: 0.5rem; min-height: 2.75rem; padding: 0.65rem; }
    .rail-link.active::before { inset-inline-start: 0.65rem; inset-block-end: 0; width: 1rem; height: 2px; }
    .rail-footer { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; padding-block-start: 0.5rem; }
    .rail-status { margin-block-end: 0; min-width: 0; }
    .workspace { margin-inline-start: 0; }
    .workspace-content { padding: 1.75rem 1.5rem; }
  }
  @media (max-width: 479px) {
    .workspace-content { padding: 1.5rem 1rem; }
    .rail-link { font-size: 0.75rem; }
    .rail-actions { gap: 0.15rem; }
    .logout-button { gap: 0.35rem; padding-inline: 0.35rem; }
  }
</style>
