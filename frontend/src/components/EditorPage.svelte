<script lang="ts">
  import type { Snippet } from 'svelte';
  import PageHeader from './PageHeader.svelte';

  let { eyebrow, title, children }: { eyebrow: string; title: string; children: Snippet } = $props();
</script>

<section class="w-full max-w-5xl space-y-6">
  <PageHeader {eyebrow} {title} />
  {@render children()}
</section>
