package config

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty24_AccessModeParsingDefaultsToReadOnly validates that parseAccessMode
// returns ReadWrite only for "read-write" (case-insensitive, trimmed), and ReadOnly
// for all other inputs.
//
// **Validates: Requirements 10.1, 10.2**
func TestProperty24_AccessModeParsingDefaultsToReadOnly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")

		result := parseAccessMode(raw)

		trimmedLower := strings.TrimSpace(strings.ToLower(raw))
		if trimmedLower == "read-write" {
			if result != ReadWrite {
				t.Fatalf("parseAccessMode(%q) = %q, want %q (trimmed+lowered is 'read-write')",
					raw, result, ReadWrite)
			}
		} else {
			if result != ReadOnly {
				t.Fatalf("parseAccessMode(%q) = %q, want %q (trimmed+lowered is %q, not 'read-write')",
					raw, result, ReadOnly, trimmedLower)
			}
		}
	})
}
