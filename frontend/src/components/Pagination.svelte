<script lang="ts">
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
    class="rounded-md border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-white"
  >
    Previous
  </button>

  <span class="text-sm text-slate-600">
    Page {page} of {totalPages}
  </span>

  <button
    type="button"
    onclick={handleNext}
    disabled={isNextDisabled}
    class="rounded-md border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-white"
  >
    Next
  </button>
</nav>
