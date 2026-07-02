package resolve_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
	"pgregory.net/rapid"
)

// --- Generators ---

// genMonitorName generates a non-empty monitor name that is NOT a UUID.
func genMonitorName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		// Generate names of length 1–50 from printable ASCII excluding whitespace-only.
		chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:"
		length := rapid.IntRange(1, 50).Draw(t, "nameLen")
		var b strings.Builder
		for i := 0; i < length; i++ {
			idx := rapid.IntRange(0, len(chars)-1).Draw(t, "charIdx")
			b.WriteByte(chars[idx])
		}
		return b.String()
	})
}

// genUUID generates a valid UUID string (8-4-4-4-12 hex format).
func genUUID() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		hexChars := "0123456789abcdef"
		var b strings.Builder
		lengths := []int{8, 4, 4, 4, 12}
		for i, l := range lengths {
			if i > 0 {
				b.WriteByte('-')
			}
			for j := 0; j < l; j++ {
				idx := rapid.IntRange(0, len(hexChars)-1).Draw(t, "hexIdx")
				b.WriteByte(hexChars[idx])
			}
		}
		return b.String()
	})
}

// --- Property 6: Case-sensitive exact name resolution ---
// Validates: Requirements 5.4
//
// For any monitor name that exists EXACTLY ONCE in the list and any case
// variation that differs from the exact name, Monitor() returns the correct
// ID for the exact case and NOT_FOUND for the case variation.
func TestProperty6_CaseSensitiveExactNameResolution(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a unique monitor name and its ID.
		name := genMonitorName().Draw(t, "targetName")
		id := genUUID().Draw(t, "targetID")

		// Generate some other monitors that do NOT share the target name.
		otherCount := rapid.IntRange(0, 5).Draw(t, "otherCount")
		monitors := make([]pulseapi.Monitor, 0, otherCount+1)
		for i := 0; i < otherCount; i++ {
			otherName := genMonitorName().Draw(t, "otherName")
			// Ensure the other name doesn't collide with the target.
			if otherName == name {
				otherName = otherName + "-other"
			}
			monitors = append(monitors, pulseapi.Monitor{
				ID:   genUUID().Draw(t, "otherID"),
				Name: otherName,
			})
		}
		// Insert the target monitor at a random position.
		insertIdx := rapid.IntRange(0, len(monitors)).Draw(t, "insertIdx")
		monitors = append(monitors, pulseapi.Monitor{})
		copy(monitors[insertIdx+1:], monitors[insertIdx:])
		monitors[insertIdx] = pulseapi.Monitor{ID: id, Name: name}

		client := &fakeClient{monitors: monitors}

		// Exact name match should return the correct ID.
		got, err := resolve.Monitor(context.Background(), client, name)
		if err != nil {
			t.Fatalf("expected no error for exact name %q, got: %v", name, err)
		}
		if got != id {
			t.Fatalf("expected ID %q for name %q, got %q", id, name, got)
		}

		// Generate a case variation that differs from the exact name.
		caseVariation := generateCaseVariation(t, name)
		if caseVariation == name {
			// If the name has no letters (so case variation is identical), skip this check.
			return
		}

		// Case variation should NOT match (case-sensitive) → NOT_FOUND.
		_, err = resolve.Monitor(context.Background(), client, caseVariation)
		if err == nil {
			t.Fatalf("expected NOT_FOUND error for case variation %q of name %q, got nil", caseVariation, name)
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeNotFound {
			t.Fatalf("expected code %q for case variation %q, got %q", mcperr.CodeNotFound, caseVariation, mcpErr.Code)
		}
	})
}

// generateCaseVariation creates a case-different version of name by flipping
// the case of at least one letter. Returns the original if no letters exist.
func generateCaseVariation(t *rapid.T, name string) string {
	// Find indices of letters in the name.
	var letterIdxs []int
	for i, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			letterIdxs = append(letterIdxs, i)
		}
	}
	if len(letterIdxs) == 0 {
		return name // no letters to flip
	}

	// Flip all letters to ensure the variation differs.
	runes := []rune(name)
	for _, idx := range letterIdxs {
		c := runes[idx]
		if c >= 'a' && c <= 'z' {
			runes[idx] = c - 32 // to uppercase
		} else {
			runes[idx] = c + 32 // to lowercase
		}
	}
	return string(runes)
}

// --- Property 7: Ambiguous name reports all matching ids and no data ---
// Validates: Requirements 5.5
//
// For any monitor name that exists 2+ times in the list, Monitor() returns
// an AMBIGUOUS_NAME error AND the error message contains ALL matching IDs.
func TestProperty7_AmbiguousNameReportsAllMatchingIDs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a shared name and 2+ monitors with that name.
		sharedName := genMonitorName().Draw(t, "sharedName")
		matchCount := rapid.IntRange(2, 5).Draw(t, "matchCount")

		var matchIDs []string
		monitors := make([]pulseapi.Monitor, 0, matchCount+3)

		for i := 0; i < matchCount; i++ {
			id := genUUID().Draw(t, "matchID")
			matchIDs = append(matchIDs, id)
			monitors = append(monitors, pulseapi.Monitor{ID: id, Name: sharedName})
		}

		// Add some non-matching monitors to ensure filtering works.
		otherCount := rapid.IntRange(0, 3).Draw(t, "otherCount")
		for i := 0; i < otherCount; i++ {
			otherName := genMonitorName().Draw(t, "otherName")
			if otherName == sharedName {
				otherName = otherName + "-different"
			}
			monitors = append(monitors, pulseapi.Monitor{
				ID:   genUUID().Draw(t, "otherID"),
				Name: otherName,
			})
		}

		// Shuffle monitors so position doesn't matter.
		shuffled := rapid.Permutation(monitors).Draw(t, "perm")

		client := &fakeClient{monitors: shuffled}

		// Resolution should fail with AMBIGUOUS_NAME.
		_, err := resolve.Monitor(context.Background(), client, sharedName)
		if err == nil {
			t.Fatal("expected AMBIGUOUS_NAME error for duplicate name, got nil")
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeAmbiguousName {
			t.Fatalf("expected code %q, got %q", mcperr.CodeAmbiguousName, mcpErr.Code)
		}

		// The error message must contain ALL matching IDs.
		for _, id := range matchIDs {
			if !strings.Contains(mcpErr.Message, id) {
				t.Fatalf("AMBIGUOUS_NAME error message %q does not contain matching ID %q", mcpErr.Message, id)
			}
		}
	})
}

// --- Property 8: Unknown id or name yields not-found and no data ---
// Validates: Requirements 5.6, 5.7, 6.5
//
// For any name/id that does NOT exist in the monitor list (and is not a UUID),
// Monitor() returns a NOT_FOUND error. For a UUID that doesn't match any
// monitor, Monitor() returns the UUID directly (passthrough — the endpoint
// handles not-found).
func TestProperty8_UnknownNameOrIDYieldsNotFound(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Build a list of known monitors.
		monitorCount := rapid.IntRange(0, 5).Draw(t, "monitorCount")
		monitors := make([]pulseapi.Monitor, monitorCount)
		knownNames := make(map[string]bool)
		knownIDs := make(map[string]bool)
		for i := 0; i < monitorCount; i++ {
			monitors[i] = pulseapi.Monitor{
				ID:   genUUID().Draw(t, "knownID"),
				Name: genMonitorName().Draw(t, "knownName"),
			}
			knownNames[monitors[i].Name] = true
			knownIDs[monitors[i].ID] = true
		}

		client := &fakeClient{monitors: monitors}

		// --- Sub-property: unknown name → NOT_FOUND ---
		unknownName := genMonitorName().Draw(t, "unknownName")
		// Ensure it doesn't accidentally match a known name.
		for knownNames[unknownName] {
			unknownName = unknownName + "-x"
		}
		// Also ensure it's not a UUID (so it's treated as a name).
		if resolve.IsUUID(unknownName) {
			unknownName = "name-" + unknownName
		}

		_, err := resolve.Monitor(context.Background(), client, unknownName)
		if err == nil {
			t.Fatalf("expected NOT_FOUND for unknown name %q, got nil", unknownName)
		}
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected *mcperr.MCPError for unknown name, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeNotFound {
			t.Fatalf("expected code %q for unknown name, got %q", mcperr.CodeNotFound, mcpErr.Code)
		}

		// --- Sub-property: unknown UUID → passthrough (no error) ---
		unknownUUID := genUUID().Draw(t, "unknownUUID")
		// Ensure it doesn't match a known ID (extremely unlikely but be safe).
		for knownIDs[unknownUUID] {
			unknownUUID = genUUID().Draw(t, "retryUUID")
		}

		got, err := resolve.Monitor(context.Background(), client, unknownUUID)
		if err != nil {
			t.Fatalf("expected UUID passthrough for unknown UUID %q, got error: %v", unknownUUID, err)
		}
		if got != unknownUUID {
			t.Fatalf("expected passthrough to return UUID %q, got %q", unknownUUID, got)
		}
	})
}
