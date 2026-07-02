package downtime

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"pgregory.net/rapid"
)

// TestProperty14_DowntimeSummaryInvariantsHold validates that for ANY generated set of
// history points (with random states from {up, down, pending}) within a valid window
// [start, end], the computed Downtime_Summary satisfies all structural invariants:
//
// 1. total_downtime_seconds ≥ 0 and ≤ (windowEnd - windowStart) in seconds
// 2. downtime_period_count == len(periods)
// 3. count == 0 iff total == 0 iff had_downtime == false
// 4. Periods are non-overlapping: for consecutive periods, period[i].End ≤ period[i+1].Start
// 5. Periods are ordered by Start ascending
// 6. Each period has DurationSeconds == floor(End - Start) in seconds
// 7. Sum of all period durations == TotalDowntimeSeconds
//
// **Validates: Requirements 7.2, 7.4**
func TestProperty14_DowntimeSummaryInvariantsHold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random window start in a reasonable range (year 2020-2030).
		startUnix := rapid.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		).Draw(t, "startUnix")
		windowStart := time.Unix(startUnix, 0).UTC()

		// Window duration: 1 minute to 24 hours.
		durationSec := rapid.Int64Range(60, 24*3600).Draw(t, "durationSec")
		windowEnd := windowStart.Add(time.Duration(durationSec) * time.Second)

		// Generate 0 to 50 points within the window.
		numPoints := rapid.IntRange(0, 50).Draw(t, "numPoints")
		states := []string{"up", "down", "pending"}

		points := make([]pulseapi.HistoryPoint, numPoints)
		for i := range points {
			// Random offset within the window (inclusive of boundaries).
			offsetSec := rapid.Int64Range(0, durationSec).Draw(t, "offsetSec")
			state := rapid.SampledFrom(states).Draw(t, "state")
			points[i] = pulseapi.HistoryPoint{
				State:     state,
				CheckedAt: windowStart.Add(time.Duration(offsetSec) * time.Second),
			}
		}

		// Sort points by CheckedAt ascending (required input contract).
		sort.Slice(points, func(i, j int) bool {
			return points[i].CheckedAt.Before(points[j].CheckedAt)
		})

		// Call Summarize.
		s := Summarize(points, windowStart, windowEnd, false)

		windowSeconds := int64(windowEnd.Sub(windowStart).Seconds())

		// Invariant 1: total_downtime_seconds ≥ 0 and ≤ window length.
		if s.TotalDowntimeSeconds < 0 {
			t.Fatalf("TotalDowntimeSeconds = %d, want ≥ 0", s.TotalDowntimeSeconds)
		}
		if s.TotalDowntimeSeconds > windowSeconds {
			t.Fatalf("TotalDowntimeSeconds = %d exceeds window length %d",
				s.TotalDowntimeSeconds, windowSeconds)
		}

		// Invariant 2: downtime_period_count == len(periods).
		if s.DowntimePeriodCount != len(s.Periods) {
			t.Fatalf("DowntimePeriodCount = %d, but len(Periods) = %d",
				s.DowntimePeriodCount, len(s.Periods))
		}

		// Invariant 3: count == 0 iff total == 0 iff had_downtime == false.
		if s.DowntimePeriodCount == 0 && s.TotalDowntimeSeconds != 0 {
			t.Fatalf("PeriodCount=0 but TotalDowntimeSeconds=%d (should be 0)",
				s.TotalDowntimeSeconds)
		}
		if s.TotalDowntimeSeconds == 0 && s.DowntimePeriodCount != 0 {
			t.Fatalf("TotalDowntimeSeconds=0 but PeriodCount=%d (should be 0)",
				s.DowntimePeriodCount)
		}
		if s.HadDowntime != (s.DowntimePeriodCount > 0) {
			t.Fatalf("HadDowntime=%v but PeriodCount=%d (inconsistent)",
				s.HadDowntime, s.DowntimePeriodCount)
		}
		if s.HadDowntime != (s.TotalDowntimeSeconds > 0) {
			t.Fatalf("HadDowntime=%v but TotalDowntimeSeconds=%d (inconsistent)",
				s.HadDowntime, s.TotalDowntimeSeconds)
		}

		// Invariant 4 & 5: Periods are non-overlapping and ordered by Start ascending.
		for i := 1; i < len(s.Periods); i++ {
			if s.Periods[i].Start.Before(s.Periods[i-1].End) {
				t.Fatalf("Periods overlap: period[%d].End=%v > period[%d].Start=%v",
					i-1, s.Periods[i-1].End, i, s.Periods[i].Start)
			}
			if s.Periods[i].Start.Before(s.Periods[i-1].Start) {
				t.Fatalf("Periods not ordered: period[%d].Start=%v > period[%d].Start=%v",
					i-1, s.Periods[i-1].Start, i, s.Periods[i].Start)
			}
		}

		// Invariant 6: Each period has DurationSeconds == floor(End - Start) in seconds.
		for i, p := range s.Periods {
			expectedDuration := int64(math.Floor(p.End.Sub(p.Start).Seconds()))
			if p.DurationSeconds != expectedDuration {
				t.Fatalf("Period[%d].DurationSeconds=%d, want floor(End-Start)=%d (Start=%v, End=%v)",
					i, p.DurationSeconds, expectedDuration, p.Start, p.End)
			}
		}

		// Invariant 7: Sum of all period durations == TotalDowntimeSeconds.
		var sumDurations int64
		for _, p := range s.Periods {
			sumDurations += p.DurationSeconds
		}
		if sumDurations != s.TotalDowntimeSeconds {
			t.Fatalf("Sum of period durations (%d) != TotalDowntimeSeconds (%d)",
				sumDurations, s.TotalDowntimeSeconds)
		}
	})
}

// TestProperty15_WindowClampingSetsTrincationAndReportsEffectiveWindow validates that
// when truncated is true, the summary's Truncated field is true and WindowStart/WindowEnd
// match the provided window boundaries.
//
// **Validates: Requirements 7.3, 7.5**
func TestProperty15_WindowClampingSetsTrincationAndReportsEffectiveWindow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random window start in a reasonable range.
		startUnix := rapid.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		).Draw(t, "startUnix")
		windowStart := time.Unix(startUnix, 0).UTC()

		// Window duration: 1 minute to 24 hours.
		durationSec := rapid.Int64Range(60, 24*3600).Draw(t, "durationSec")
		windowEnd := windowStart.Add(time.Duration(durationSec) * time.Second)

		// Randomly decide whether the window was truncated.
		truncated := rapid.Bool().Draw(t, "truncated")

		// Generate 0 to 20 points within the window.
		numPoints := rapid.IntRange(0, 20).Draw(t, "numPoints")
		states := []string{"up", "down", "pending"}

		points := make([]pulseapi.HistoryPoint, numPoints)
		for i := range points {
			offsetSec := rapid.Int64Range(0, durationSec).Draw(t, "offsetSec")
			state := rapid.SampledFrom(states).Draw(t, "state")
			points[i] = pulseapi.HistoryPoint{
				State:     state,
				CheckedAt: windowStart.Add(time.Duration(offsetSec) * time.Second),
			}
		}

		sort.Slice(points, func(i, j int) bool {
			return points[i].CheckedAt.Before(points[j].CheckedAt)
		})

		// Call Summarize with the truncated flag.
		s := Summarize(points, windowStart, windowEnd, truncated)

		// Property: when truncated is true, the summary reports it.
		if s.Truncated != truncated {
			t.Fatalf("Summary.Truncated=%v, want %v (passed truncated=%v)",
				s.Truncated, truncated, truncated)
		}

		// Property: WindowStart and WindowEnd always match the provided boundaries.
		if !s.WindowStart.Equal(windowStart) {
			t.Fatalf("Summary.WindowStart=%v, want %v", s.WindowStart, windowStart)
		}
		if !s.WindowEnd.Equal(windowEnd) {
			t.Fatalf("Summary.WindowEnd=%v, want %v", s.WindowEnd, windowEnd)
		}
	})
}
