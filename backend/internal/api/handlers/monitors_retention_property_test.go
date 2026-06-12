package handlers

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: monitor-history-explorer, Property 1: Retention period storage round-trip
//
// For any monitor creation/update request with history_retention_days in [1,365],
// the stored value equals the provided value; if omitted, effective value is 30.
//
// **Validates: Requirements 1.1, 1.2, 1.3**
func TestPropertyRetentionStorageRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Decide whether to provide a value or leave it nil (omitted).
		provided := rapid.Bool().Draw(t, "provided")

		if provided {
			// Generate a valid retention value in [1, 365].
			days := rapid.Int32Range(1, 365).Draw(t, "days")

			result, err := validateRetentionDays(&days)
			if err != nil {
				t.Fatalf("expected no error for valid days=%d, got: %v", days, err)
			}
			if result != days {
				t.Fatalf("round-trip failed: provided %d, got %d", days, result)
			}
		} else {
			// Omitted (nil) should default to 30.
			result, err := validateRetentionDays(nil)
			if err != nil {
				t.Fatalf("expected no error for nil input, got: %v", err)
			}
			if result != 30 {
				t.Fatalf("default mismatch: expected 30, got %d", result)
			}
		}
	})
}

// Feature: monitor-history-explorer, Property 2: Retention period validation rejects invalid values
//
// For any integer outside [1,365], the API returns INVALID_RETENTION_PERIOD error
// and monitor state is unchanged (validateRetentionDays returns an error).
//
// **Validates: Requirements 1.1, 1.2, 1.3, 1.4**
func TestPropertyRetentionValidationRejectsInvalid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate an invalid retention value: either <= 0 or > 365.
		var days int32
		if rapid.Bool().Draw(t, "belowRange") {
			// Values at or below 0: range [-1000, 0]
			days = rapid.Int32Range(-1000, 0).Draw(t, "daysBelowRange")
		} else {
			// Values above 365: range [366, 10000]
			days = rapid.Int32Range(366, 10000).Draw(t, "daysAboveRange")
		}

		result, err := validateRetentionDays(&days)
		if err == nil {
			t.Fatalf("expected error for invalid days=%d, but got result=%d with no error", days, result)
		}
		if result != 0 {
			t.Fatalf("expected zero-value result on error for days=%d, got %d", days, result)
		}
	})
}
