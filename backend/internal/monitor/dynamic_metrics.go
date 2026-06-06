package monitor

import (
	"log"
	"os"
	"slices"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultMaxMetricTagKeys is the default maximum number of tag keys promoted to Prometheus labels.
	DefaultMaxMetricTagKeys = 10
)

// baseLabels are the fixed labels present on all monitor metric vectors.
var baseLabels = []string{"monitor_id", "monitor_name", "monitor_type", "monitor_url"}

// DynamicMetrics manages Prometheus gauge vectors whose label set changes
// when the set of distinct tag keys across monitors changes. It replaces
// static metrics for monitors to support tag-based label promotion.
//
// It implements prometheus.Collector with an empty Describe method so
// the registry treats it as an unchecked collector — this allows label
// dimensions to change at runtime without registry rejection.
type DynamicMetrics struct {
	registry      *prometheus.Registry
	monitorUp     *prometheus.GaugeVec
	responseTime  *prometheus.GaugeVec
	MonitorsTotal prometheus.Gauge
	labelNames    []string
	maxTagKeys    int
	mu            sync.RWMutex
}

// Verify DynamicMetrics implements prometheus.Collector.
var _ prometheus.Collector = (*DynamicMetrics)(nil)

// Describe implements prometheus.Collector. It intentionally yields no
// descriptors so the Prometheus registry treats DynamicMetrics as an
// unchecked collector, allowing label dimensions to change at runtime.
func (dm *DynamicMetrics) Describe(_ chan<- *prometheus.Desc) {
	// Intentionally empty — unchecked collector pattern.
}

// Collect implements prometheus.Collector. It delegates to the current
// gauge vectors and the monitorsTotal gauge.
func (dm *DynamicMetrics) Collect(ch chan<- prometheus.Metric) {
	dm.mu.RLock()
	up := dm.monitorUp
	rt := dm.responseTime
	total := dm.MonitorsTotal
	dm.mu.RUnlock()

	if up != nil {
		up.Collect(ch)
	}
	if rt != nil {
		rt.Collect(ch)
	}
	if total != nil {
		total.Collect(ch)
	}
}

// NewDynamicMetrics constructs a DynamicMetrics instance, reading the max
// promoted tag keys from the PULSE_MAX_METRIC_TAG_KEYS environment variable
// (default: 10). It registers itself as an unchecked collector with the
// provided registry and initializes metric vectors with base labels.
func NewDynamicMetrics(registry *prometheus.Registry) *DynamicMetrics {
	maxKeys := DefaultMaxMetricTagKeys
	if envVal := os.Getenv("PULSE_MAX_METRIC_TAG_KEYS"); envVal != "" {
		if parsed, err := strconv.Atoi(envVal); err == nil && parsed > 0 {
			maxKeys = parsed
		}
	}

	dm := &DynamicMetrics{
		registry:   registry,
		maxTagKeys: maxKeys,
		labelNames: make([]string, len(baseLabels)),
	}
	copy(dm.labelNames, baseLabels)

	// Create initial vectors with base labels.
	dm.monitorUp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pulse_monitor_up",
			Help: "Whether the monitor is up (1) or down (0).",
		},
		dm.labelNames,
	)
	dm.responseTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pulse_monitor_response_time_seconds",
			Help: "Last recorded response time in seconds.",
		},
		dm.labelNames,
	)
	dm.MonitorsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pulse_monitors_total",
			Help: "Total number of configured monitors.",
		},
	)

	// Register as unchecked collector (Describe is empty).
	registry.MustRegister(dm)

	return dm
}

// RebuildLabels updates the metric vector label set when the set of distinct
// tag keys changes. It caps promoted tag keys at maxTagKeys.
//
// Since DynamicMetrics uses the unchecked collector pattern (empty Describe),
// label dimension changes are handled by swapping internal vectors. The
// Collect method delegates to whatever vectors are current at scrape time.
//
// If vector creation fails for any reason, the error is logged and previous
// vectors are retained (requirement 6.7).
func (dm *DynamicMetrics) RebuildLabels(allTagKeys []string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Cap promoted tag keys.
	promoted := allTagKeys
	if len(promoted) > dm.maxTagKeys {
		promoted = promoted[:dm.maxTagKeys]
	}

	// Build new label set: base labels + tag_ prefixed keys.
	newLabels := make([]string, 0, len(baseLabels)+len(promoted))
	newLabels = append(newLabels, baseLabels...)
	for _, key := range promoted {
		newLabels = append(newLabels, "tag_"+key)
	}

	// Skip if unchanged.
	if slices.Equal(dm.labelNames, newLabels) {
		return
	}

	// Create new vectors with updated label set.
	// NewGaugeVec panics on invalid label names — recover and retain old vectors.
	var newUp, newRT *prometheus.GaugeVec
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("dynamic_metrics: failed to create new vectors: %v — retaining previous", r)
			}
		}()
		newUp = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_monitor_up",
				Help: "Whether the monitor is up (1) or down (0).",
			},
			newLabels,
		)
		newRT = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pulse_monitor_response_time_seconds",
				Help: "Last recorded response time in seconds.",
			},
			newLabels,
		)
	}()

	// If creation failed (panic recovered), retain previous vectors.
	if newUp == nil || newRT == nil {
		return
	}

	// Swap vectors — old ones become unreferenced and will be GC'd.
	// The Collect method will serve from the new vectors on next scrape.
	dm.monitorUp = newUp
	dm.responseTime = newRT
	dm.labelNames = newLabels

	log.Printf("dynamic_metrics: rebuilt labels with %d tag keys (total %d labels)",
		len(promoted), len(newLabels))
}

// RecordCheck updates Prometheus metrics for a monitor check result.
// It builds label values from the base labels plus tag_ prefixed keys,
// filling missing tag values with empty string. Both pulse_monitor_up and
// pulse_monitor_response_time_seconds are set with identical label sets.
func (dm *DynamicMetrics) RecordCheck(monitorID, name, mType, url string, tags map[string]string, up bool, latencySeconds float64) {
	dm.mu.RLock()
	labels := dm.labelNames
	upVec := dm.monitorUp
	rtVec := dm.responseTime
	dm.mu.RUnlock()

	if upVec == nil || rtVec == nil {
		return
	}

	// Build label values: [monitorID, name, mType, url, tag_key1_val, tag_key2_val, ...]
	values := make([]string, len(labels))
	values[0] = monitorID
	values[1] = name
	values[2] = mType
	values[3] = url

	// Fill tag label values (positions after base labels).
	for i := len(baseLabels); i < len(labels); i++ {
		// labelNames[i] is "tag_<key>", extract the key portion.
		key := labels[i][len("tag_"):]
		if v, ok := tags[key]; ok {
			values[i] = v
		} else {
			values[i] = ""
		}
	}

	// Set monitor up gauge.
	var upVal float64
	if up {
		upVal = 1.0
	}
	upVec.WithLabelValues(values...).Set(upVal)

	// Set response time gauge.
	rtVec.WithLabelValues(values...).Set(latencySeconds)
}

// CleanupMonitor removes stale time series for a monitor from both gauge
// vectors. Call this when a monitor is deleted or its tags change (before
// re-recording with updated labels). labelValues must match the exact label
// values previously used to record metrics for the monitor.
func (dm *DynamicMetrics) CleanupMonitor(labelValues []string) {
	dm.mu.RLock()
	upVec := dm.monitorUp
	rtVec := dm.responseTime
	dm.mu.RUnlock()

	if upVec != nil {
		upVec.DeleteLabelValues(labelValues...)
	}
	if rtVec != nil {
		rtVec.DeleteLabelValues(labelValues...)
	}
}

// LabelNames returns a copy of the current label names (thread-safe).
func (dm *DynamicMetrics) LabelNames() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	result := make([]string, len(dm.labelNames))
	copy(result, dm.labelNames)
	return result
}

// MaxTagKeys returns the configured maximum number of promoted tag keys.
func (dm *DynamicMetrics) MaxTagKeys() int {
	return dm.maxTagKeys
}

// MonitorUp returns the current monitorUp gauge vector (thread-safe).
func (dm *DynamicMetrics) MonitorUp() *prometheus.GaugeVec {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.monitorUp
}

// ResponseTime returns the current responseTime gauge vector (thread-safe).
func (dm *DynamicMetrics) ResponseTime() *prometheus.GaugeVec {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.responseTime
}
