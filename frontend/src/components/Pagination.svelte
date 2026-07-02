<script lang="ts">
  import { t } from '$lib/i18n';

  interface Props {
    page: number;
    totalPages: number;
    onPageChange: (page: number) => void;
  }

  let { page, totalPages, onPageChange }: Props = $props();

  let isPrevDisabled = $derived(page === 1);
  let isNextDisabled = $derived(page === totalPages);

  function handlePrev() {
    if (!isPrevDisabled) {
      onPageChange(page - 1);
    }
  }

  function handleNext() {
    if (!isNextDisabled) {
      onPageChange(page + 1);
    }
  }
</script>

<nav aria-label="Pagination" class="flex items-center justify-between gap-4">
  <button
    type="button"
    onclick={handlePrev}
    disabled={isPrevDisabled}
    class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-surface"
  >
    {t('common.previous')}
  </button>

  <span class="text-sm text-secondary">
    {t('common.pageOf', { page, totalPages })}
  </span>

  <button
    type="button"
    onclick={handleNext}
    disabled={isNextDisabled}
    class="rounded-md border border-[var(--color-border)] bg-surface px-3 py-2 text-sm font-medium text-primary transition hover:bg-[var(--color-bg-surface-hover)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-surface"
  >
    {t('common.next')}
  </button>
</nav>
