<script lang="ts">
  import '../app.css';

  const nav = [
    { href: '/', label: 'Dashboard' },
    { href: '/monitors', label: 'Monitors' },
    { href: '/settings', label: 'Settings' }
  ];
</script>

<div class="min-h-screen bg-gradient-to-b from-sky-100 via-slate-50 to-slate-100">
  <header class="border-b border-slate-200/70 bg-white/80 backdrop-blur">
    <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
      <a href="/" class="text-xl font-semibold tracking-tight text-brand-700">Pulse</a>
      <nav class="flex items-center gap-2">
        {#each nav as item}
          <a
            href={item.href}
            class="rounded-md px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-brand-50 hover:text-brand-700"
          >
            {item.label}
          </a>
        {/each}
      </nav>
    </div>
  </header>

  <main class="mx-auto max-w-6xl px-6 py-8">
    <slot />
  </main>
</div>
