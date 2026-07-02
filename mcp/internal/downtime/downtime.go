// Package downtime derives downtime summaries from ordered check history points.
// All functions are pure — they take inputs and produce outputs without side effects.
package downtime

import (
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// Period represents a single contiguous downtime interval.
type Period struct {
	Start           time.Time
	End             time.Time
	DurationSeconds int64
}

// Summary is the computed downtime result for a time window.
type Summary struct {
	HadDowntime          bool
	DowntimePeriodCount  int
	TotalDowntimeSeconds int64
	WindowStart          time.Time
	WindowEnd            time.Time
	Truncated            bool
	Periods              []Period
}

// Summarize computes a downtime summary from ordered history points within a window.
//
// Points must be ordered ascending by CheckedAt. Only points with CheckedAt inside
// [windowStart, windowEnd] are considered. The "pending" state is treated as not-down
// (only "down" contributes to downtime).
//
// A downtime period is a contiguous run of "down" states bounded by:
//   - An up→down (or pending→down) transition, or the window start if the first point is down.
//   - A down→up (or down→pending) transition, or the window end if the last point is still down.
//
// Invariants guaranteed:
//   - TotalDowntimeSeconds ≥ 0 and ≤ windowEnd - windowStart (in seconds)
//   - Periods are non-overlapping and ordered by Start ascending
//   - DowntimePeriodCount == 0 iff TotalDowntimeSeconds == 0 iff HadDowntime == false
func Summarize(points []pulseapi.HistoryPoint, windowStart, windowEnd time.Time, truncated bool) Summary {
	summary := Summary{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Truncated:   truncated,
	}

	// Filter to points within the window and walk them in order.
	var inDown bool
	var periodStart time.Time
	var periods []Period

	for _, p := range points {
		// Skip points outside the window.
		if p.CheckedAt.Before(windowStart) || p.CheckedAt.After(windowEnd) {
			continue
		}

		isDown := p.State == "down"

		if isDown && !inDown {
			// Transition to down: open a new period.
			inDown = true
			periodStart = p.CheckedAt
		} else if !isDown && inDown {
			// Transition from down to not-down: close the period.
			inDown = false
			dur := int64(p.CheckedAt.Sub(periodStart).Seconds())
			// Only record periods with positive duration (same-timestamp transitions are noise).
			if dur > 0 {
				periods = append(periods, Period{
					Start:           periodStart,
					End:             p.CheckedAt,
					DurationSeconds: dur,
				})
			}
		}
	}

	// If still in a down state at the end of the walk, close the period at windowEnd.
	// Only record if the period has positive duration (a down point exactly at windowEnd
	// produces a zero-duration period which is noise, not a real downtime interval).
	if inDown {
		dur := int64(windowEnd.Sub(periodStart).Seconds())
		if dur > 0 {
			periods = append(periods, Period{
				Start:           periodStart,
				End:             windowEnd,
				DurationSeconds: dur,
			})
		}
	}

	// Compute aggregates.
	var totalSeconds int64
	for _, p := range periods {
		totalSeconds += p.DurationSeconds
	}

	// Enforce upper bound: total cannot exceed window length.
	windowSeconds := int64(windowEnd.Sub(windowStart).Seconds())
	if totalSeconds > windowSeconds {
		totalSeconds = windowSeconds
	}

	// Enforce non-negative.
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	summary.Periods = periods
	summary.DowntimePeriodCount = len(periods)
	summary.TotalDowntimeSeconds = totalSeconds
	summary.HadDowntime = len(periods) > 0

	return summary
}
