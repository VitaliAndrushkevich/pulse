<script lang="ts">
	import { toastStore } from '$lib/stores/toast.svelte';
	import { t } from '$lib/i18n';

	function getBgClass(type: 'error' | 'success' | 'info'): string {
		switch (type) {
			case 'error':
				return 'bg-red-50 border-red-300 text-red-900';
			case 'success':
				return 'bg-green-50 border-green-300 text-green-900';
			case 'info':
				return 'bg-blue-50 border-blue-300 text-blue-900';
		}
	}

	function getIconClass(type: 'error' | 'success' | 'info'): string {
		switch (type) {
			case 'error':
				return 'text-red-500';
			case 'success':
				return 'text-green-500';
			case 'info':
				return 'text-blue-500';
		}
	}
</script>

{#if toastStore.toasts.length > 0}
	<div
		class="fixed top-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2"
		aria-live="polite"
		aria-label={t('toast.notifications')}
	>
		{#each toastStore.toasts as toast (toast.id)}
			<div
				class="animate-slide-in rounded-lg border px-4 py-3 shadow-lg {getBgClass(toast.type)}"
				role="alert"
			>
				<div class="flex items-start gap-3">
					<span class="mt-0.5 flex-shrink-0 {getIconClass(toast.type)}">
						{#if toast.type === 'error'}
							<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<circle cx="12" cy="12" r="10" />
								<line x1="12" y1="8" x2="12" y2="12" />
								<line x1="12" y1="16" x2="12.01" y2="16" />
							</svg>
						{:else if toast.type === 'success'}
							<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
								<polyline points="22 4 12 14.01 9 11.01" />
							</svg>
						{:else}
							<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<circle cx="12" cy="12" r="10" />
								<line x1="12" y1="16" x2="12" y2="12" />
								<line x1="12" y1="8" x2="12.01" y2="8" />
							</svg>
						{/if}
					</span>

					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium">{toast.message}</p>
						{#if toast.type === 'error' && toast.requestId}
							<p class="mt-1 text-xs opacity-75">{t('toast.requestId', { id: toast.requestId })}</p>
						{/if}
					</div>

					{#if toast.dismissible}
						<button
							onclick={() => toastStore.dismissToast(toast.id)}
							class="flex-shrink-0 rounded p-1 opacity-70 transition hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-current"
							aria-label={t('toast.dismiss')}
						>
							<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<line x1="18" y1="6" x2="6" y2="18" />
								<line x1="6" y1="6" x2="18" y2="18" />
							</svg>
						</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	@keyframes slide-in {
		from {
			opacity: 0;
			transform: translateX(1rem);
		}
		to {
			opacity: 1;
			transform: translateX(0);
		}
	}

	:global(.animate-slide-in) {
		animation: slide-in 0.2s ease-out;
	}
</style>
