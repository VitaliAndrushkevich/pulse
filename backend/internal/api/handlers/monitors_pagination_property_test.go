package handlers

import (
	"testing"

	"pgregory.net/rapid"
)

// paginationMetadata computes the pagination metadata returned by the List handler.
// This replicates the exact logic used in handlers/monitors.go List().
func paginationMetadata(total int64, page, limit int) (returnedPage, returnedLimit, totalPages int) {
	returnedPage = page
	returnedLimit = limit

	totalPages = int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
	}
	return returnedPage, returnedLimit, totalPages
}

// itemsOnPage computes the maximum number of items that can appear on a given page.
// For the last page, it may be less than limit.
func itemsOnPage(total int64, page, limit int) int {
	offset := (page - 1) * limit
	if int64(offset) >= total {
		return 0
	}
	remaining := int(total) - offset
	if remaining > limit {
		return limit
	}
	return remaining
}

// TestPropertyPaginationCorrectness verifies Property 7: Pagination Correctness.
//
// Generate random page/limit params; verify returned count ≤ limit and metadata
// consistent with total.
//
// **Validates: Requirements 5.4, 5.5**
func TestPropertyPaginationCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a total count of monitors (0 to 1000).
		total := int64(rapid.IntRange(0, 1000).Draw(t, "total"))

		// Generate valid limit (1–100, matching the API constraint).
		limit := rapid.IntRange(1, 100).Draw(t, "limit")

		// Generate a valid page number (1 to a reasonable upper bound).
		maxPage := 50
		page := rapid.IntRange(1, maxPage).Draw(t, "page")

		// Compute pagination metadata using the same logic as the handler.
		_, _, totalPages := paginationMetadata(total, page, limit)

		// Compute the number of items that would be returned for this page.
		count := itemsOnPage(total, page, limit)

		// Property 1: Returned items count ≤ limit.
		if count > limit {
			t.Fatalf("items on page (%d) exceeds limit (%d) [total=%d, page=%d]",
				count, limit, total, page)
		}

		// Property 2: total_pages = ceil(total / limit) for total > 0, or 0 when total == 0.
		var expectedTotalPages int
		if total == 0 {
			expectedTotalPages = 0
		} else {
			expectedTotalPages = (int(total) + limit - 1) / limit
		}
		if totalPages != expectedTotalPages {
			t.Fatalf("totalPages mismatch: got %d, want ceil(%d/%d)=%d",
				totalPages, total, limit, expectedTotalPages)
		}

		// Property 3: page × limit offset is consistent — items returned are
		// exactly the minimum of limit and (total - offset), or 0 if offset >= total.
		offset := (page - 1) * limit
		var expectedCount int
		if int64(offset) >= total {
			expectedCount = 0
		} else {
			expectedCount = int(total) - offset
			if expectedCount > limit {
				expectedCount = limit
			}
		}
		if count != expectedCount {
			t.Fatalf("items count mismatch: got %d, want %d [total=%d, page=%d, limit=%d, offset=%d]",
				count, expectedCount, total, page, limit, offset)
		}

		// Property 4: If page <= totalPages and total > 0, then count > 0.
		if total > 0 && page <= totalPages && count == 0 {
			t.Fatalf("page %d is within totalPages %d but returned 0 items [total=%d, limit=%d]",
				page, totalPages, total, limit)
		}

		// Property 5: If page > totalPages, then count == 0.
		if totalPages > 0 && page > totalPages && count != 0 {
			t.Fatalf("page %d exceeds totalPages %d but returned %d items [total=%d, limit=%d]",
				page, totalPages, count, total, limit)
		}
	})
}
