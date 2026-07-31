<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    description?: string;
    defaultOpen?: boolean;
    children: Snippet;
  }

  let { title, description, defaultOpen = false, children }: Props = $props();
</script>

<details class="group rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-surface)] shadow-sm" open={defaultOpen}>
  <summary class="flex cursor-pointer items-center justify-between px-6 py-4 select-none list-none [&::-webkit-details-marker]:hidden">
    <div>
      <h2 class="text-lg font-medium text-[var(--color-text-primary)]">{title}</h2>
      {#if description}
        <p class="mt-0.5 text-sm text-[var(--color-text-secondary)]">{description}</p>
      {/if}
    </div>
    <svg
      class="h-5 w-5 shrink-0 text-[var(--color-text-muted)] transition-transform duration-200 group-open:rotate-180"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="2"
    >
      <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
    </svg>
  </summary>
  <div class="border-t border-[var(--color-border)] px-6 py-6">
    {@render children()}
  </div>
</details>
