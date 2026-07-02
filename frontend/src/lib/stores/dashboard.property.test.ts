/**
 * Property-Based Tests: Widget Error Isolation (Property 19)
 *
 * Tests the fundamental property that for any subset of widgets that
 * experience data fetch failures, all remaining widgets continue to
 * render their data independently and do NOT enter error state due
 * to another widget's failure.
 *
 * The dashboardStore uses Promise.allSettled-style error isolation:
 * each widget's data population is independent, so a failure in one
 * widget does not propagate to siblings.
 *
 * **Validates: Requirements 8.4**
 *
 * Feature: dashboard-refactor, Property 19: Widget error isolation preserves other widgets
 */
import { describe, it, expect, beforeEach } from 'vitest';
import * as fc from 'fast-check';
import type { WidgetId } from '$lib/types';
import { dashboardStore } from '$lib/stores/dashboard.svelte';

// --- Constants ---

const ALL_WIDGETS: WidgetId[] = [
	'health-score',
	'status-ring',
	'incidents',
	'sparklines',
	'ssl-expiry',
	'heatmap',
	'events-feed'
];

// --- Arbitraries ---

/**
 * Generate an arbitrary subset of widget IDs that will "fail".
 * Uses fc.shuffledSubarray to produce any combination (including empty and full sets).
 */
const arbitraryFailedWidgetSubset: fc.Arbitrary<WidgetId[]> = fc.shuffledSubarray(ALL_WIDGETS);

// --- Property Tests ---

describe('Property 19: Widget error isolation preserves other widgets', () => {
	beforeEach(() => {
		dashboardStore.reset();
	});

	it('widgets NOT in the failed subset do not have errors set', () => {
		fc.assert(
			fc.property(arbitraryFailedWidgetSubset, (failedWidgets) => {
				dashboardStore.reset();

				// Simulate: set errors for the failed subset
				for (const widgetId of failedWidgets) {
					dashboardStore.setWidgetError(widgetId, `Error loading ${widgetId}`);
				}

				const errors = dashboardStore.widgetErrors;
				const remainingWidgets = ALL_WIDGETS.filter((w) => !failedWidgets.includes(w));

				// Property: remaining widgets must NOT have errors
				for (const widgetId of remainingWidgets) {
					expect(errors.has(widgetId)).toBe(false);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('failed widgets have their error recorded independently', () => {
		fc.assert(
			fc.property(arbitraryFailedWidgetSubset, (failedWidgets) => {
				dashboardStore.reset();

				// Simulate: set errors for the failed subset
				for (const widgetId of failedWidgets) {
					dashboardStore.setWidgetError(widgetId, `Error loading ${widgetId}`);
				}

				const errors = dashboardStore.widgetErrors;

				// Property: every failed widget has exactly its own error message
				for (const widgetId of failedWidgets) {
					expect(errors.has(widgetId)).toBe(true);
					expect(errors.get(widgetId)).toBe(`Error loading ${widgetId}`);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('error count equals exactly the number of failed widgets', () => {
		fc.assert(
			fc.property(arbitraryFailedWidgetSubset, (failedWidgets) => {
				dashboardStore.reset();

				// Simulate: set errors for the failed subset
				for (const widgetId of failedWidgets) {
					dashboardStore.setWidgetError(widgetId, `Error loading ${widgetId}`);
				}

				// Property: widgetErrors map size equals failed widget count
				expect(dashboardStore.widgetErrors.size).toBe(failedWidgets.length);
			}),
			{ numRuns: 100 }
		);
	});

	it('clearing one widget error does not affect other widget errors', () => {
		fc.assert(
			fc.property(
				arbitraryFailedWidgetSubset.filter((s) => s.length >= 2),
				(failedWidgets) => {
					dashboardStore.reset();

					// Set errors for all failed widgets
					for (const widgetId of failedWidgets) {
						dashboardStore.setWidgetError(widgetId, `Error loading ${widgetId}`);
					}

					// Clear the first widget's error (simulate retry success)
					const clearedWidget = failedWidgets[0];
					dashboardStore.setWidgetError(clearedWidget, null);

					const errors = dashboardStore.widgetErrors;

					// Property: cleared widget no longer has error
					expect(errors.has(clearedWidget)).toBe(false);

					// Property: remaining failed widgets still have their errors
					for (const widgetId of failedWidgets.slice(1)) {
						expect(errors.has(widgetId)).toBe(true);
						expect(errors.get(widgetId)).toBe(`Error loading ${widgetId}`);
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('widget loading states are independent from widget errors', () => {
		fc.assert(
			fc.property(
				arbitraryFailedWidgetSubset,
				fc.shuffledSubarray(ALL_WIDGETS),
				(failedWidgets, loadingWidgets) => {
					dashboardStore.reset();

					// Set loading for some widgets
					for (const widgetId of loadingWidgets) {
						dashboardStore.setWidgetLoading(widgetId, true);
					}

					// Set errors for failed widgets
					for (const widgetId of failedWidgets) {
						dashboardStore.setWidgetError(widgetId, `Error loading ${widgetId}`);
					}

					// Property: error state and loading state are orthogonal
					// Widgets not in failed set must not have errors regardless of loading state
					const remainingWidgets = ALL_WIDGETS.filter((w) => !failedWidgets.includes(w));
					for (const widgetId of remainingWidgets) {
						expect(dashboardStore.widgetErrors.has(widgetId)).toBe(false);
					}

					// Loading state should remain intact for all widgets that were set to loading
					for (const widgetId of loadingWidgets) {
						expect(dashboardStore.widgetLoading.has(widgetId)).toBe(true);
					}
				}
			),
			{ numRuns: 100 }
		);
	});
});
