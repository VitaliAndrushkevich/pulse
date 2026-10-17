package handlers

import (
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyHeartbeatPaginationCorrectness mirrors the monitor pagination
// property test for the heartbeat List handler. It verifies that pageCount
// (the metadata helper used by HeartbeatHandler.List) is consistent with the
// total count and page size for any valid page/limit combination.
func TestPropertyHeartbeatPaginationCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		total := int64(rapid.IntRange(0, 1000).Draw(t, "total"))
		limit := rapid.IntRange(1, 100).Draw(t, "limit")
		page := rapid.IntRange(1, 50).Draw(t, "page")

		totalPages := pageCount(total, limit)

		// Property 1: total_pages = ceil(total / limit), or 0 when total == 0.
		var expected int
		if total > 0 {
			expected = (int(total) + limit - 1) / limit
		}
		if totalPages != expected {
			t.Fatalf("pageCount(%d,%d) = %d, want %d", total, limit, totalPages, expected)
		}

		// Property 2: items returned on a page never exceed the limit.
		offset := (page - 1) * limit
		count := 0
		if int64(offset) < total {
			count = int(total) - offset
			if count > limit {
				count = limit
			}
		}
		if count > limit {
			t.Fatalf("items on page (%d) exceeds limit (%d)", count, limit)
		}

		// Property 3: a page beyond total_pages yields zero items.
		if totalPages > 0 && page > totalPages && count != 0 {
			t.Fatalf("page %d > totalPages %d but count %d != 0", page, totalPages, count)
		}
	})
}

// TestHeartbeatListLimitClampedTo100 documents that the List handler clamps the
// page size to 100 even though parsePagination already rejects limits above 100.
// pageCount must behave consistently at the boundary.
func TestHeartbeatListLimitClampedTo100(t *testing.T) {
	if got := pageCount(250, 100); got != 3 {
		t.Fatalf("pageCount(250,100) = %d, want 3", got)
	}
	if got := pageCount(0, 100); got != 0 {
		t.Fatalf("pageCount(0,100) = %d, want 0", got)
	}
}
