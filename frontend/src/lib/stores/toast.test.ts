import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// We need to test the store module. Since it uses Svelte 5 runes ($state),
// we import and test via the exported store object.
import { toastStore, type Toast } from './toast.svelte';

describe('ToastStore', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		toastStore.clearAll();
	});

	afterEach(() => {
		toastStore.clearAll();
		vi.useRealTimers();
	});

	describe('addToast', () => {
		it('adds a toast to the store', () => {
			toastStore.addToast({ type: 'success', message: 'Done', persistent: false });
			expect(toastStore.toasts).toHaveLength(1);
			expect(toastStore.toasts[0].message).toBe('Done');
			expect(toastStore.toasts[0].type).toBe('success');
		});

		it('generates a unique id for each toast', () => {
			toastStore.addToast({ type: 'info', message: 'A', persistent: false });
			toastStore.addToast({ type: 'info', message: 'B', persistent: false });
			expect(toastStore.toasts[0].id).not.toBe(toastStore.toasts[1].id);
		});

		it('returns the toast id', () => {
			const id = toastStore.addToast({ type: 'success', message: 'Test', persistent: false });
			expect(typeof id).toBe('string');
			expect(id.length).toBeGreaterThan(0);
		});

		it('sets dismissible to true by default', () => {
			toastStore.addToast({ type: 'info', message: 'Test', persistent: false });
			expect(toastStore.toasts[0].dismissible).toBe(true);
		});

		it('respects explicit dismissible value', () => {
			toastStore.addToast({ type: 'info', message: 'Test', persistent: false, dismissible: false });
			expect(toastStore.toasts[0].dismissible).toBe(false);
		});

		it('includes requestId when provided', () => {
			toastStore.addToast({ type: 'error', message: 'Fail', persistent: false, requestId: 'req-123' });
			expect(toastStore.toasts[0].requestId).toBe('req-123');
		});
	});

	describe('max 5 visible toasts', () => {
		it('keeps at most 5 toasts, removing oldest when exceeded', () => {
			for (let i = 0; i < 6; i++) {
				toastStore.addToast({ type: 'info', message: `Toast ${i}`, persistent: false });
			}
			expect(toastStore.toasts).toHaveLength(5);
			// Oldest (Toast 0) should have been removed
			expect(toastStore.toasts[0].message).toBe('Toast 1');
		});

		it('prefers removing non-persistent toasts when over max', () => {
			// Add 4 persistent + 1 non-persistent
			for (let i = 0; i < 4; i++) {
				toastStore.addToast({ type: 'error', message: `Persistent ${i}`, persistent: true });
			}
			toastStore.addToast({ type: 'success', message: 'Non-persistent', persistent: false });

			// Now add a 6th — should remove the non-persistent one
			toastStore.addToast({ type: 'info', message: 'New one', persistent: false });
			expect(toastStore.toasts).toHaveLength(5);
			expect(toastStore.toasts.find((t) => t.message === 'Non-persistent')).toBeUndefined();
		});
	});

	describe('auto-dismiss timers', () => {
		it('auto-dismisses success toasts after 4 seconds', () => {
			toastStore.addToast({ type: 'success', message: 'Done', persistent: false });
			expect(toastStore.toasts).toHaveLength(1);

			vi.advanceTimersByTime(3999);
			expect(toastStore.toasts).toHaveLength(1);

			vi.advanceTimersByTime(1);
			expect(toastStore.toasts).toHaveLength(0);
		});

		it('auto-dismisses info toasts after 4 seconds', () => {
			toastStore.addToast({ type: 'info', message: 'Note', persistent: false });
			vi.advanceTimersByTime(4000);
			expect(toastStore.toasts).toHaveLength(0);
		});

		it('auto-dismisses error toasts after 8 seconds', () => {
			toastStore.addToast({ type: 'error', message: 'Error', persistent: false });
			expect(toastStore.toasts).toHaveLength(1);

			vi.advanceTimersByTime(7999);
			expect(toastStore.toasts).toHaveLength(1);

			vi.advanceTimersByTime(1);
			expect(toastStore.toasts).toHaveLength(0);
		});

		it('does not auto-dismiss persistent toasts', () => {
			toastStore.addToast({ type: 'error', message: 'Network error', persistent: true });
			vi.advanceTimersByTime(60000);
			expect(toastStore.toasts).toHaveLength(1);
		});
	});

	describe('dismissToast', () => {
		it('removes a toast by id', () => {
			const id = toastStore.addToast({ type: 'info', message: 'Test', persistent: false });
			expect(toastStore.toasts).toHaveLength(1);
			toastStore.dismissToast(id);
			expect(toastStore.toasts).toHaveLength(0);
		});

		it('clears the auto-dismiss timer when manually dismissed', () => {
			const id = toastStore.addToast({ type: 'success', message: 'Test', persistent: false });
			toastStore.dismissToast(id);
			vi.advanceTimersByTime(5000);
			// No error should occur from stale timer
			expect(toastStore.toasts).toHaveLength(0);
		});

		it('does nothing for unknown id', () => {
			toastStore.addToast({ type: 'info', message: 'Test', persistent: false });
			toastStore.dismissToast('nonexistent');
			expect(toastStore.toasts).toHaveLength(1);
		});
	});

	describe('clearAll', () => {
		it('removes all toasts and clears timers', () => {
			toastStore.addToast({ type: 'success', message: 'A', persistent: false });
			toastStore.addToast({ type: 'error', message: 'B', persistent: true });
			toastStore.addToast({ type: 'info', message: 'C', persistent: false });
			toastStore.clearAll();
			expect(toastStore.toasts).toHaveLength(0);
		});
	});
});
