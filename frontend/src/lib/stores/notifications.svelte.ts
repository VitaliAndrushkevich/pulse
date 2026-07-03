/**
 * NotificationStore — reactive notification channel collection with CRUD operations.
 *
 * Uses Svelte 5 runes ($state, $derived) for fine-grained reactivity.
 * Manages paginated channel list, loading/error states, and API interactions.
 *
 * Requirements: 10.1, 10.2
 */

import type { NotificationChannel, PaginatedList } from '$lib/types';
import {
	listNotificationChannels,
	createNotificationChannel,
	updateNotificationChannel,
	deleteNotificationChannel,
	testNotificationChannel,
	getNotificationChannel,
	type CreateNotificationChannelRequest,
	type UpdateNotificationChannelRequest,
} from '$lib/api';

// --- Reactive state (Svelte 5 runes) ---

let channels = $state<NotificationChannel[]>([]);
let total = $state<number>(0);
let page = $state<number>(1);
let limit = $state<number>(20);
let totalPages = $state<number>(0);
let loading = $state<boolean>(false);
let error = $state<string | null>(null);

// --- Derived values ---

const isEmpty = $derived<boolean>(channels.length === 0 && !loading);
const hasNextPage = $derived<boolean>(page < totalPages);
const hasPreviousPage = $derived<boolean>(page > 1);

// --- Actions ---

/**
 * Fetch channels for the current page and limit.
 * Resets error state on each call.
 */
async function fetchChannels(p?: number, l?: number): Promise<void> {
	if (p !== undefined) page = p;
	if (l !== undefined) limit = l;

	loading = true;
	error = null;

	try {
		const result: PaginatedList<NotificationChannel> = await listNotificationChannels(page, limit);
		channels = result.data;
		total = result.total;
		totalPages = result.total_pages;
	} catch (err: unknown) {
		error = err instanceof Error ? err.message : 'Failed to load notification channels';
	} finally {
		loading = false;
	}
}

/**
 * Create a new notification channel and refresh the list.
 * Returns the created channel on success, throws on failure.
 */
async function create(data: CreateNotificationChannelRequest): Promise<NotificationChannel> {
	const channel = await createNotificationChannel(data);
	// Refresh current page to reflect new channel
	await fetchChannels();
	return channel;
}

/**
 * Update an existing notification channel and refresh the list.
 * Returns the updated channel on success, throws on failure.
 */
async function update(
	id: string,
	data: UpdateNotificationChannelRequest
): Promise<NotificationChannel> {
	const channel = await updateNotificationChannel(id, data);
	// Update local state immediately for responsiveness
	channels = channels.map((c) => (c.id === id ? channel : c));
	return channel;
}

/**
 * Delete a notification channel and refresh the list.
 * Throws on failure.
 */
async function remove(id: string): Promise<void> {
	await deleteNotificationChannel(id);
	// Remove from local state immediately
	channels = channels.filter((c) => c.id !== id);
	total = Math.max(0, total - 1);
	// If the page is now empty and not the first page, go back one page
	if (channels.length === 0 && page > 1) {
		await fetchChannels(page - 1);
	}
}

/**
 * Send a test notification through a channel.
 * Returns the test result.
 */
async function test(id: string) {
	return testNotificationChannel(id);
}

/**
 * Fetch a single channel by ID.
 */
async function getById(id: string): Promise<NotificationChannel> {
	return getNotificationChannel(id);
}

/**
 * Navigate to next page.
 */
async function nextPage(): Promise<void> {
	if (hasNextPage) {
		await fetchChannels(page + 1);
	}
}

/**
 * Navigate to previous page.
 */
async function previousPage(): Promise<void> {
	if (hasPreviousPage) {
		await fetchChannels(page - 1);
	}
}

/**
 * Go to a specific page.
 */
async function goToPage(p: number): Promise<void> {
	if (p >= 1 && p <= totalPages) {
		await fetchChannels(p);
	}
}

/**
 * Clear all local state.
 */
function clear(): void {
	channels = [];
	total = 0;
	page = 1;
	totalPages = 0;
	loading = false;
	error = null;
}

// --- Exported singleton store ---

export const notificationStore = {
	get channels(): NotificationChannel[] {
		return channels;
	},
	get total(): number {
		return total;
	},
	get page(): number {
		return page;
	},
	get limit(): number {
		return limit;
	},
	get totalPages(): number {
		return totalPages;
	},
	get loading(): boolean {
		return loading;
	},
	get error(): string | null {
		return error;
	},
	get isEmpty(): boolean {
		return isEmpty;
	},
	get hasNextPage(): boolean {
		return hasNextPage;
	},
	get hasPreviousPage(): boolean {
		return hasPreviousPage;
	},
	fetchChannels,
	create,
	update,
	remove,
	test,
	getById,
	nextPage,
	previousPage,
	goToPage,
	clear,
};
