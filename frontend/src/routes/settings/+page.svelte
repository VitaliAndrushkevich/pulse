<script lang="ts">
  import ApiTokenSection from './ApiTokenSection.svelte';
</script>

<svelte:head>
  <title>Settings — Pulse</title>
</svelte:head>

<div class="mx-auto max-w-4xl space-y-8 px-4 py-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight text-[var(--color-text-primary)]">Settings</h1>
    <p class="mt-1 text-sm text-[var(--color-text-secondary)]">Manage your account and API access.</p>
  </div>

  <ApiTokenSection />
</div>
