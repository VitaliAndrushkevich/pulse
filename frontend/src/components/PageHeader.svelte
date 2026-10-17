<script lang="ts">
  import type { Snippet } from 'svelte';

  let { eyebrow, title, description, actions }: {
    eyebrow: string;
    title: string;
    description?: string;
    actions?: Snippet;
  } = $props();
</script>

<div class="page-header">
  <div class="min-w-0">
    <p class="pulse-eyebrow mb-2">{eyebrow}</p>
    <h1 class="text-3xl font-semibold tracking-tight text-primary">{title}</h1>
    {#if description}<p class="mt-2 text-sm text-secondary">{description}</p>{/if}
  </div>
  {#if actions}<div class="header-actions">{@render actions()}</div>{/if}
</div>

<style>
  .page-header { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: 1rem; padding-block-end: 1.5rem; border-block-end: 1px solid var(--color-border); }
  .header-actions { flex-shrink: 0; }
</style>
