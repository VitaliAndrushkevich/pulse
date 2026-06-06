package monitor

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewDynamicMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	if dm == nil {
		t.Fatal("expected non-nil DynamicMetrics")
	}

	// Verify base labels are set.
	labels := dm.LabelNames()
	expected := []string{"monitor_id", "monitor_name", "monitor_type", "monitor_url"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d", len(expected), len(labels))
	}
	for i, l := range labels {
		if l != expected[i] {
			t.Errorf("label[%d]: expected %q, got %q", i, expected[i], l)
		}
	}

	// Verify default max tag keys.
	if dm.MaxTagKeys() != DefaultMaxMetricTagKeys {
		t.Errorf("expected max tag keys %d, got %d", DefaultMaxMetricTagKeys, dm.MaxTagKeys())
	}

	// Verify vectors are accessible.
	if dm.MonitorUp() == nil {
		t.Error("expected non-nil MonitorUp gauge vector")
	}
	if dm.ResponseTime() == nil {
		t.Error("expected non-nil ResponseTime gauge vector")
	}
}

func TestRebuildLabels_AddsTagPrefix(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	dm.RebuildLabels([]string{"env", "team"})

	labels := dm.LabelNames()
	expected := []string{"monitor_id", "monitor_name", "monitor_type", "monitor_url", "tag_env", "tag_team"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(labels), labels)
	}
	for i, l := range labels {
		if l != expected[i] {
			t.Errorf("label[%d]: expected %q, got %q", i, expected[i], l)
		}
	}
}

func TestRebuildLabels_NoChangeSkipsRebuild(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	dm.RebuildLabels([]string{"env"})
	upBefore := dm.MonitorUp()

	// Calling again with same keys should be a no-op.
	dm.RebuildLabels([]string{"env"})
	upAfter := dm.MonitorUp()

	if upBefore != upAfter {
		t.Error("expected same GaugeVec pointer when labels unchanged")
	}
}

func TestRebuildLabels_CapsAtMaxTagKeys(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	// Generate more keys than the default max (10).
	keys := make([]string, 15)
	for i := range keys {
		keys[i] = "key" + string(rune('a'+i))
	}

	dm.RebuildLabels(keys)

	labels := dm.LabelNames()
	// Should have base (4) + max (10) = 14 labels.
	expectedLen := len(baseLabels) + DefaultMaxMetricTagKeys
	if len(labels) != expectedLen {
		t.Fatalf("expected %d labels (capped), got %d: %v", expectedLen, len(labels), labels)
	}
}

func TestRebuildLabels_EmptyTagKeys(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	// First add some tags.
	dm.RebuildLabels([]string{"env", "team"})

	// Then rebuild with empty keys — should revert to base labels.
	dm.RebuildLabels([]string{})

	labels := dm.LabelNames()
	if len(labels) != len(baseLabels) {
		t.Fatalf("expected %d base labels, got %d: %v", len(baseLabels), len(labels), labels)
	}
}

func TestRebuildLabels_MetricsStillUsable(t *testing.T) {
	reg := prometheus.NewRegistry()
	dm := NewDynamicMetrics(reg)

	dm.RebuildLabels([]string{"env"})

	// Should be able to set values without panic.
	up := dm.MonitorUp()
	up.WithLabelValues("id-1", "Test Monitor", "http", "https://example.com", "production").Set(1)

	rt := dm.ResponseTime()
	rt.WithLabelValues("id-1", "Test Monitor", "http", "https://example.com", "production").Set(0.042)
}
