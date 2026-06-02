export interface Toast {
	id: string;
	type: 'error' | 'success' | 'info';
	message: string;
	requestId?: string;
	dismissible: boolean;
	persistent: boolean;
}

const MAX_VISIBLE = 5;
const SUCCESS_DISMISS_MS = 4000;
const ERROR_DISMISS_MS = 8000;

let toasts = $state<Toast[]>([]);
const timers = new Map<string, ReturnType<typeof setTimeout>>();

function generateId(): string {
	if (typeof crypto !== 'undefined' && crypto.randomUUID) {
		return crypto.randomUUID();
	}
	return `toast-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

function addToast(
	options: Omit<Toast, 'id' | 'dismissible'> & { dismissible?: boolean }
): string {
	const id = generateId();
	const toast: Toast = {
		id,
		type: options.type,
		message: options.message,
		requestId: options.requestId,
		dismissible: options.dismissible ?? true,
		persistent: options.persistent
	};

	toasts.push(toast);

	// If over max, remove oldest non-persistent first, then oldest overall
	while (toasts.length > MAX_VISIBLE) {
		const oldestIndex = toasts.findIndex((t) => t.id !== id && !t.persistent);
		if (oldestIndex !== -1) {
			const removed = toasts.splice(oldestIndex, 1)[0];
			clearTimer(removed.id);
		} else {
			// All are persistent or the new one — remove the oldest regardless
			const removed = toasts.splice(0, 1)[0];
			clearTimer(removed.id);
		}
	}

	// Start auto-dismiss timer unless persistent
	if (!toast.persistent) {
		const delay = toast.type === 'error' ? ERROR_DISMISS_MS : SUCCESS_DISMISS_MS;
		const timer = setTimeout(() => {
			dismissToast(id);
		}, delay);
		timers.set(id, timer);
	}

	return id;
}

function dismissToast(id: string): void {
	clearTimer(id);
	const index = toasts.findIndex((t) => t.id === id);
	if (index !== -1) {
		toasts.splice(index, 1);
	}
}

function clearTimer(id: string): void {
	const timer = timers.get(id);
	if (timer !== undefined) {
		clearTimeout(timer);
		timers.delete(id);
	}
}

function clearAll(): void {
	for (const timer of timers.values()) {
		clearTimeout(timer);
	}
	timers.clear();
	toasts.length = 0;
}

export const toastStore = {
	get toasts(): Toast[] {
		return toasts;
	},
	addToast,
	dismissToast,
	clearAll
};
