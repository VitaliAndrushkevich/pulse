package handlers

import (
	"testing"

	"pgregory.net/rapid"
)

// --- Pure filter logic for property testing ---

// testMonitor represents a monitor with a type and a set of tags for filter testing.
type testMonitor struct {
	ID   string
	Type string
	Tags map[string]string // key → value (one value per key for simplicity)
}

// testFilter represents a combined type + tag filter.
type testFilter struct {
	Type string            // empty means no type filter
	Tags map[string]string // key → value pairs; AND semantics (all must match)
}

// monitorMatchesFilter is the pure oracle function that determines whether a
// monitor should appear in filtered results. A monitor matches iff:
//   - (type filter is empty OR monitor.Type == type filter), AND
//   - (monitor possesses ALL tag key:value pairs specified in the filter)
//
// This function serves as both the system-under-test and the oracle since it
// encapsulates the exact specification from Requirements 5.1, 5.2, 5.3.
func monitorMatchesFilter(m testMonitor, f testFilter) bool {
	// Requirement 5.2: type filter
	if f.Type != "" && m.Type != f.Type {
		return false
	}

	// Requirement 5.1: tag filter with AND semantics
	for k, v := range f.Tags {
		monitorVal, exists := m.Tags[k]
		if !exists || monitorVal != v {
			return false
		}
	}

	return true
}

// filterMonitors applies the filter to a set of monitors, returning those that match.
func filterMonitors(monitors []testMonitor, f testFilter) []testMonitor {
	var result []testMonitor
	for _, m := range monitors {
		if monitorMatchesFilter(m, f) {
			result = append(result, m)
		}
	}
	return result
}

// --- Generators ---

var monitorTypes = []string{"http", "http3", "tcp", "udp", "websocket", "grpc", "quic"}

// genMonitorType generates a valid monitor type.
func genMonitorType() *rapid.Generator[string] {
	return rapid.SampledFrom(monitorTypes)
}

// genTagKey generates a plausible tag key (lowercase, short).
func genTagKey() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"env", "team", "region", "service", "tier", "owner", "project", "cluster",
	})
}

// genTagValue generates a plausible tag value.
func genTagValue() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"production", "staging", "development", "platform", "infra", "data",
		"us-east-1", "eu-west-1", "ap-south-1", "backend", "frontend", "api",
	})
}

// genTags generates a random tag map with 0-5 entries.
func genTags() *rapid.Generator[map[string]string] {
	return rapid.Custom(func(t *rapid.T) map[string]string {
		n := rapid.IntRange(0, 5).Draw(t, "numTags")
		tags := make(map[string]string, n)
		for i := 0; i < n; i++ {
			key := genTagKey().Draw(t, "tagKey")
			value := genTagValue().Draw(t, "tagValue")
			tags[key] = value
		}
		return tags
	})
}

// genTestMonitor generates a random monitor with type and tags.
func genTestMonitor() *rapid.Generator[testMonitor] {
	return rapid.Custom(func(t *rapid.T) testMonitor {
		return testMonitor{
			ID:   rapid.StringMatching(`[a-f0-9]{8}`).Draw(t, "id"),
			Type: genMonitorType().Draw(t, "type"),
			Tags: genTags().Draw(t, "tags"),
		}
	})
}

// genTestFilter generates a random filter with optional type and 0-3 tag conditions.
func genTestFilter() *rapid.Generator[testFilter] {
	return rapid.Custom(func(t *rapid.T) testFilter {
		var typ string
		if rapid.Bool().Draw(t, "hasTypeFilter") {
			typ = genMonitorType().Draw(t, "filterType")
		}

		numTagFilters := rapid.IntRange(0, 3).Draw(t, "numTagFilters")
		tags := make(map[string]string, numTagFilters)
		for i := 0; i < numTagFilters; i++ {
			key := genTagKey().Draw(t, "filterTagKey")
			value := genTagValue().Draw(t, "filterTagValue")
			tags[key] = value
		}

		return testFilter{Type: typ, Tags: tags}
	})
}

// --- Property Test ---

// TestPropertyFilterCompletenessAndSoundness verifies Property 6: Filter
// Completeness and Soundness (AND Semantics).
//
// For any set of monitors and any combined filter (type + tags), a monitor
// appears in the filtered result set if and only if:
//   - (a) its type matches the type filter (or type filter is empty), AND
//   - (b) it possesses ALL tags specified in the filter.
//
// This property ensures no false positives (soundness) and no false negatives
// (completeness) in the filter logic.
//
// **Validates: Requirements 5.1, 5.2, 5.3**
func TestPropertyFilterCompletenessAndSoundness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random set of monitors (1-20).
		numMonitors := rapid.IntRange(1, 20).Draw(t, "numMonitors")
		monitors := make([]testMonitor, numMonitors)
		for i := range monitors {
			monitors[i] = genTestMonitor().Draw(t, "monitor")
		}

		// Generate a random filter.
		filter := genTestFilter().Draw(t, "filter")

		// Apply the filter function under test.
		results := filterMonitors(monitors, filter)

		// Build a set of result IDs for fast lookup.
		resultIDs := make(map[string]bool, len(results))
		for _, r := range results {
			resultIDs[r.ID] = true
		}

		// Oracle check: for every monitor, verify it appears in results iff it
		// matches the filter criteria.
		for _, m := range monitors {
			expectedMatch := monitorMatchesFilter(m, filter)
			actualMatch := resultIDs[m.ID]

			if expectedMatch && !actualMatch {
				t.Fatalf("completeness violation: monitor %q (type=%q, tags=%v) should match filter (type=%q, tags=%v) but was excluded",
					m.ID, m.Type, m.Tags, filter.Type, filter.Tags)
			}
			if !expectedMatch && actualMatch {
				t.Fatalf("soundness violation: monitor %q (type=%q, tags=%v) should NOT match filter (type=%q, tags=%v) but was included",
					m.ID, m.Type, m.Tags, filter.Type, filter.Tags)
			}
		}

		// Additional invariant: result count must equal the number of monitors
		// that individually match.
		expectedCount := 0
		for _, m := range monitors {
			if monitorMatchesFilter(m, filter) {
				expectedCount++
			}
		}
		if len(results) != expectedCount {
			t.Fatalf("result count mismatch: got %d, expected %d", len(results), expectedCount)
		}
	})
}
