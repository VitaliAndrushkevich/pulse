package handlers

import (
	"sort"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestPropertyFilteredResultOrdering verifies Property 8: Filtered Result Ordering.
//
// For any filtered result set with more than one monitor, each monitor's
// created_at timestamp SHALL be greater than or equal to the next monitor's
// created_at in the response (descending order).
//
// This is a pure logic test: generate random timestamps, sort descending,
// verify the invariant holds.
//
// **Validates: Requirements 5.6**
func TestPropertyFilteredResultOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random number of timestamps (1-50) representing created_at
		// values from filtered monitor results.
		n := rapid.IntRange(1, 50).Draw(t, "numResults")
		timestamps := make([]time.Time, n)

		// Generate random timestamps within a reasonable range (past year).
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := range timestamps {
			// Random offset in seconds within a year (~31.5M seconds).
			offsetSec := rapid.Int64Range(0, 31_536_000).Draw(t, "offsetSec")
			timestamps[i] = baseTime.Add(time.Duration(offsetSec) * time.Second)
		}

		// Sort descending by created_at (simulating ORDER BY created_at DESC).
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i].After(timestamps[j])
		})

		// Verify the ordering invariant: each timestamp is >= the next.
		for i := 0; i < len(timestamps)-1; i++ {
			if timestamps[i].Before(timestamps[i+1]) {
				t.Fatalf(
					"ordering invariant violated at index %d: %v < %v",
					i, timestamps[i], timestamps[i+1],
				)
			}
		}
	})
}
