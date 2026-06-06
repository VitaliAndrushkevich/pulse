package monitor

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"pgregory.net/rapid"
)

// validMetricTagKeyGen generates a random key matching ^[a-z][a-z0-9_-]{0,63}$
// that does not start with "__". Used by dynamic metrics property tests.
func validMetricTagKeyGen() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		first := rapid.ByteRange('a', 'z').Draw(t, "first")

		restLen := rapid.IntRange(0, 63).Draw(t, "restLen")
		rest := make([]byte, restLen)
		charset := "abcdefghijklmnopqrstuvwxyz0123456789_-"
		for i := range rest {
			rest[i] = charset[rapid.IntRange(0, len(charset)-1).Draw(t, "charIdx")]
		}

		key := string(first) + string(rest)
		if len(key) >= 2 && key[0] == '_' && key[1] == '_' {
			key = "a" + key[1:]
		}
		return key
	})
}

// TestPropertyLabelPrefixTransformation verifies Property 9: Prometheus Label
// Prefix Transformation.
//
// For any set of valid tag keys, after calling RebuildLabels, the label names
// at positions after the 4 base labels are exactly "tag_" + key for each key.
//
// **Validates: Requirements 6.1**
func TestPropertyLabelPrefixTransformation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-10 unique valid tag keys.
		numKeys := rapid.IntRange(1, 10).Draw(t, "numKeys")
		keySet := make(map[string]struct{}, numKeys)
		keys := make([]string, 0, numKeys)
		for len(keys) < numKeys {
			k := validMetricTagKeyGen().Draw(t, "tagKey")
			if _, exists := keySet[k]; !exists {
				keySet[k] = struct{}{}
				keys = append(keys, k)
			}
		}

		// Create a fresh DynamicMetrics with a new registry.
		reg := prometheus.NewRegistry()
		dm := NewDynamicMetrics(reg)

		// Rebuild labels with the generated keys.
		dm.RebuildLabels(keys)

		// Retrieve the resulting label names.
		labels := dm.LabelNames()

		// Verify base labels are intact (first 4).
		expectedBase := []string{"monitor_id", "monitor_name", "monitor_type", "monitor_url"}
		if len(labels) < len(expectedBase) {
			t.Fatalf("expected at least %d base labels, got %d", len(expectedBase), len(labels))
		}
		for i, base := range expectedBase {
			if labels[i] != base {
				t.Fatalf("base label[%d]: expected %q, got %q", i, base, labels[i])
			}
		}

		// Verify tag labels: positions after base labels should be "tag_" + key.
		tagLabels := labels[len(expectedBase):]
		if len(tagLabels) != len(keys) {
			t.Fatalf("expected %d tag labels, got %d", len(keys), len(tagLabels))
		}

		for i, key := range keys {
			expected := "tag_" + key
			if tagLabels[i] != expected {
				t.Fatalf("tag label[%d]: expected %q, got %q", i, expected, tagLabels[i])
			}
		}
	})
}

// TestPropertyLabelCardinalityCap verifies Property 12: Prometheus Label
// Cardinality Cap.
//
// Generate random tag key sets of varying size (0 to 30); call RebuildLabels
// with those keys; verify that the number of promoted tag labels never exceeds
// DefaultMaxMetricTagKeys (10).
//
// **Validates: Requirements 6.4**
func TestPropertyLabelCardinalityCap(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random set of tag keys (size 0 to 30).
		numKeys := rapid.IntRange(0, 30).Draw(t, "numKeys")
		tagKeys := make([]string, numKeys)
		for i := range tagKeys {
			tagKeys[i] = validMetricTagKeyGen().Draw(t, "tagKey")
		}

		// Create a fresh DynamicMetrics instance for each test iteration.
		registry := prometheus.NewRegistry()
		dm := NewDynamicMetrics(registry)

		// Call RebuildLabels with the generated tag keys.
		dm.RebuildLabels(tagKeys)

		// Count promoted labels: total labels minus the 4 base labels.
		labels := dm.LabelNames()
		promotedCount := len(labels) - len(baseLabels)

		// Property: promoted count must never exceed DefaultMaxMetricTagKeys (10).
		if promotedCount > DefaultMaxMetricTagKeys {
			t.Fatalf("cardinality cap violated: promoted %d tag labels (max %d) for %d input keys",
				promotedCount, DefaultMaxMetricTagKeys, numKeys)
		}

		// Additional invariant: promoted count should be min(numKeys, DefaultMaxMetricTagKeys).
		expected := numKeys
		if expected > DefaultMaxMetricTagKeys {
			expected = DefaultMaxMetricTagKeys
		}
		if promotedCount != expected {
			t.Fatalf("unexpected promoted count: got %d, expected min(%d, %d) = %d",
				promotedCount, numKeys, DefaultMaxMetricTagKeys, expected)
		}
	})
}
