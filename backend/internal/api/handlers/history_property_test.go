package handlers

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: monitor-history-explorer, Property 5: Step parameter validation
//
// For any integer, the API accepts step in [60, 86400] and rejects values
// outside this range with 400.
//
// **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7**
func TestPropertyStepParameterValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate any integer in a wide range.
		step := rapid.IntRange(-100000, 200000).Draw(t, "step")

		err := validateStep(step)

		if step >= 60 && step <= 86400 {
			// Valid range: should accept.
			if err != nil {
				t.Fatalf("expected step=%d to be valid, got error: %v", step, err)
			}
		} else {
			// Invalid range: should reject.
			if err == nil {
				t.Fatalf("expected step=%d to be rejected, but got no error", step)
			}
		}
	})
}

// Feature: monitor-history-explorer, Property 6: Aggregation bucket correctness
//
// For any set of check results and valid step, each bucket has
// min <= avg <= max, correct check_count, and correct uptime_ratio.
//
// **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7**
func TestPropertyAggregationBucketCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a non-empty set of check results (1 to 100).
		n := rapid.IntRange(1, 100).Draw(t, "numResults")
		results := make([]checkResultInput, n)

		for i := 0; i < n; i++ {
			hasLatency := rapid.Bool().Draw(t, "hasLatency")
			var latency *int32
			if hasLatency {
				v := rapid.Int32Range(0, 10000).Draw(t, "latency")
				latency = &v
			}
			state := rapid.SampledFrom([]string{"up", "down"}).Draw(t, "state")
			results[i] = checkResultInput{
				LatencyMs: latency,
				State:     state,
			}
		}

		agg := computeAggregation(results)

		// Property: check_count matches row count.
		if agg.CheckCount != int32(n) {
			t.Fatalf("check_count=%d, expected %d", agg.CheckCount, n)
		}

		// Property: uptime_ratio = count(state="up") / total.
		upCount := 0
		for _, r := range results {
			if r.State == "up" {
				upCount++
			}
		}
		expectedRatio := float64(upCount) / float64(n)
		if math.Abs(agg.UptimeRatio-expectedRatio) > 1e-9 {
			t.Fatalf("uptime_ratio=%f, expected %f", agg.UptimeRatio, expectedRatio)
		}

		// Property: min <= avg <= max (only when latency data exists).
		if agg.MinLatency != nil && agg.MaxLatency != nil && agg.AvgLatency != nil {
			if *agg.MinLatency > *agg.AvgLatency {
				t.Fatalf("min_latency(%d) > avg_latency(%d)", *agg.MinLatency, *agg.AvgLatency)
			}
			if *agg.AvgLatency > *agg.MaxLatency {
				t.Fatalf("avg_latency(%d) > max_latency(%d)", *agg.AvgLatency, *agg.MaxLatency)
			}
		}

		// Property: if all latencies are nil, aggregated latencies should also be nil.
		hasAnyLatency := false
		for _, r := range results {
			if r.LatencyMs != nil {
				hasAnyLatency = true
				break
			}
		}
		if !hasAnyLatency {
			if agg.MinLatency != nil || agg.MaxLatency != nil || agg.AvgLatency != nil {
				t.Fatalf("expected nil latencies when no results have latency data")
			}
		}
	})
}

// Feature: monitor-history-explorer, Property 7: Auto-step calculation bounds response size
//
// For any range > 24h without explicit step, the auto-step = ceil(range_seconds / 1000)
// bounds response to ≤ 1000 points.
//
// **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7**
func TestPropertyAutoStepBoundsResponseSize(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a range > 24 hours (up to 365 days).
		// Minimum: 24h + 1s = 86401 seconds. Maximum: 365 days = 31536000 seconds.
		rangeSeconds := rapid.Float64Range(86401, 31536000).Draw(t, "rangeSeconds")

		step := autoStep(rangeSeconds)

		// Property: step == ceil(rangeSeconds / 1000).
		expectedStep := int(math.Ceil(rangeSeconds / 1000))
		if step != expectedStep {
			t.Fatalf("autoStep(%f) = %d, expected %d", rangeSeconds, step, expectedStep)
		}

		// Property: rangeSeconds / step <= 1000 (response bounded to ≤ 1000 points).
		points := rangeSeconds / float64(step)
		if points > 1000 {
			t.Fatalf("autoStep(%f) = %d yields %f points (> 1000)", rangeSeconds, step, points)
		}

		// Property: step must be at least 60 for ranges > 24h.
		// ceil(86401/1000) = 87 >= 60, so this is inherently satisfied for our range,
		// but let's verify anyway.
		if step < 1 {
			t.Fatalf("autoStep(%f) = %d, expected positive step", rangeSeconds, step)
		}
	})
}

// Feature: monitor-history-explorer, Property 8: Retention boundary enforcement
//
// For any history request, if from < now - retention_days, from is clamped to
// retention boundary and truncated=true; otherwise truncated is false.
//
// **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7**
func TestPropertyRetentionBoundaryEnforcement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate retention days [1, 365].
		retentionDays := rapid.Int32Range(1, 365).Draw(t, "retentionDays")

		// Use a fixed "now" for deterministic testing.
		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

		// Generate a "from" timestamp: anywhere from 730 days before now to now.
		offsetSeconds := rapid.Int64Range(0, 730*24*3600).Draw(t, "offsetSeconds")
		from := now.Add(-time.Duration(offsetSeconds) * time.Second)

		clampedFrom, truncated := retentionClamp(from, now, retentionDays)

		retentionBoundary := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

		if from.Before(retentionBoundary) {
			// from is before retention boundary: should be clamped and truncated.
			if !truncated {
				t.Fatalf("expected truncated=true when from(%v) < boundary(%v)", from, retentionBoundary)
			}
			if !clampedFrom.Equal(retentionBoundary) {
				t.Fatalf("expected clampedFrom=%v to equal boundary=%v", clampedFrom, retentionBoundary)
			}
		} else {
			// from is within retention: should NOT be clamped.
			if truncated {
				t.Fatalf("expected truncated=false when from(%v) >= boundary(%v)", from, retentionBoundary)
			}
			if !clampedFrom.Equal(from) {
				t.Fatalf("expected clampedFrom=%v to equal original from=%v", clampedFrom, from)
			}
		}
	})
}
