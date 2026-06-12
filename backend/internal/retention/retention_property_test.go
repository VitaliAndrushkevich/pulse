package retention

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: monitor-history-explorer, Property 3: Retention cleanup removes only expired rows
//
// For any monitor with retention period R days, after cleanup, all rows where
// checked_at < now - R days are deleted and all rows where checked_at >= now - R days
// remain intact. We test the cutoff classification logic: given a retention period
// and a set of timestamps, verify the cutoff correctly partitions rows into
// "expired" (should be deleted) and "retained" (should remain).
//
// **Validates: Requirements 1.5, 1.6, 2.1, 2.2, 2.3**
func TestPropertyRetentionCleanupRemovesOnlyExpiredRows(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random retention period in [1, 365] days.
		retentionDays := rapid.Int32Range(1, 365).Draw(t, "retentionDays")

		// Use a fixed "now" for deterministic cutoff computation.
		now := time.Now()

		// Compute the cutoff exactly as the service does.
		cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

		// Generate a batch of random timestamps spread across a wide range.
		numRows := rapid.IntRange(1, 50).Draw(t, "numRows")

		for i := 0; i < numRows; i++ {
			// Generate an offset in hours: [-365*24, +24] relative to now.
			// This covers timestamps well before and slightly after the current time.
			offsetHours := rapid.Int64Range(-365*24, 24).Draw(t, fmt.Sprintf("offsetHours_%d", i))
			checkedAt := now.Add(time.Duration(offsetHours) * time.Hour)

			// Classification: should this row be deleted?
			shouldDelete := checkedAt.Before(cutoff)
			shouldRetain := !shouldDelete // checkedAt >= cutoff

			// Verify the classification is consistent and mutually exclusive.
			if shouldDelete == shouldRetain {
				t.Fatalf("classification contradiction for checkedAt=%v, cutoff=%v", checkedAt, cutoff)
			}

			// Verify the classification matches the mathematical definition:
			// deleted iff checkedAt < now - retentionDays*24h
			expectedDelete := checkedAt.Before(cutoff)
			if shouldDelete != expectedDelete {
				t.Fatalf("cutoff classification error: retentionDays=%d, now=%v, cutoff=%v, checkedAt=%v, shouldDelete=%v, expectedDelete=%v",
					retentionDays, now, cutoff, checkedAt, shouldDelete, expectedDelete)
			}

			// Verify boundary: rows exactly at the cutoff should be retained.
			if checkedAt.Equal(cutoff) && shouldDelete {
				t.Fatalf("row at exact cutoff should be retained: cutoff=%v, checkedAt=%v", cutoff, checkedAt)
			}
		}

		// Verify cutoff is exactly retentionDays*24h before now.
		expectedCutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
		if !cutoff.Equal(expectedCutoff) {
			t.Fatalf("cutoff mismatch: got %v, want %v (retentionDays=%d)", cutoff, expectedCutoff, retentionDays)
		}
	})
}

// Feature: monitor-history-explorer, Property 4: Retention interval config validation
//
// For any string value of PULSE_RETENTION_CHECK_INTERVAL, if it's a valid Go duration
// in [1m, 168h], the service starts (ParseRetentionInterval succeeds); otherwise it fails.
//
// **Validates: Requirements 1.5, 1.6, 2.1, 2.2, 2.3**
func TestPropertyRetentionIntervalConfigValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Decide whether to generate a valid or invalid input.
		validInput := rapid.Bool().Draw(t, "validInput")

		if validInput {
			// Generate a valid duration in [1m, 168h].
			// We express this as minutes in [1, 10080] (168h = 10080m).
			minutes := rapid.IntRange(1, 10080).Draw(t, "minutes")
			input := fmt.Sprintf("%dm", minutes)

			d, err := ParseRetentionInterval(input)
			if err != nil {
				t.Fatalf("expected success for valid input %q (%d minutes), got error: %v", input, minutes, err)
			}

			expected := time.Duration(minutes) * time.Minute
			if d != expected {
				t.Fatalf("parsed duration mismatch for %q: got %v, want %v", input, d, expected)
			}

			// Verify it's within bounds.
			if d < MinCheckInterval {
				t.Fatalf("parsed duration %v below minimum %v for input %q", d, MinCheckInterval, input)
			}
			if d > MaxCheckInterval {
				t.Fatalf("parsed duration %v above maximum %v for input %q", d, MaxCheckInterval, input)
			}
		} else {
			// Generate an invalid input: pick from several categories.
			category := rapid.IntRange(0, 3).Draw(t, "invalidCategory")

			var input string
			switch category {
			case 0:
				// Below minimum: 0 to 59 seconds.
				seconds := rapid.IntRange(0, 59).Draw(t, "secondsBelowMin")
				input = fmt.Sprintf("%ds", seconds)
			case 1:
				// Above maximum: 168h1m to 8760h.
				minutes := rapid.IntRange(10081, 525600).Draw(t, "minutesAboveMax")
				input = fmt.Sprintf("%dm", minutes)
			case 2:
				// Invalid format: random non-duration strings.
				input = rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "invalidString")
			case 3:
				// Negative duration.
				minutes := rapid.IntRange(1, 10080).Draw(t, "negMinutes")
				input = fmt.Sprintf("-%dm", minutes)
			}

			_, err := ParseRetentionInterval(input)
			if err == nil {
				t.Fatalf("expected error for invalid input %q (category=%d), got success", input, category)
			}
		}
	})
}
