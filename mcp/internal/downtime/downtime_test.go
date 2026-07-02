package downtime

import (
	"testing"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

func TestSummarize_NoPoints(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	s := Summarize(nil, start, end, false)

	if s.HadDowntime {
		t.Error("expected HadDowntime=false with no points")
	}
	if s.DowntimePeriodCount != 0 {
		t.Errorf("expected period count 0, got %d", s.DowntimePeriodCount)
	}
	if s.TotalDowntimeSeconds != 0 {
		t.Errorf("expected total 0, got %d", s.TotalDowntimeSeconds)
	}
	if len(s.Periods) != 0 {
		t.Errorf("expected empty periods, got %d", len(s.Periods))
	}
}

func TestSummarize_AllUp(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "up", CheckedAt: start.Add(5 * time.Minute)},
		{State: "up", CheckedAt: start.Add(10 * time.Minute)},
		{State: "up", CheckedAt: start.Add(15 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if s.HadDowntime {
		t.Error("expected HadDowntime=false when all up")
	}
	if s.TotalDowntimeSeconds != 0 {
		t.Errorf("expected total 0, got %d", s.TotalDowntimeSeconds)
	}
}

func TestSummarize_SingleDownPeriod(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "up", CheckedAt: start.Add(5 * time.Minute)},
		{State: "down", CheckedAt: start.Add(10 * time.Minute)},
		{State: "down", CheckedAt: start.Add(15 * time.Minute)},
		{State: "up", CheckedAt: start.Add(20 * time.Minute)},
		{State: "up", CheckedAt: start.Add(25 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if !s.HadDowntime {
		t.Error("expected HadDowntime=true")
	}
	if s.DowntimePeriodCount != 1 {
		t.Errorf("expected 1 period, got %d", s.DowntimePeriodCount)
	}
	// Down from minute 10 to minute 20 = 600 seconds
	if s.TotalDowntimeSeconds != 600 {
		t.Errorf("expected 600s total, got %d", s.TotalDowntimeSeconds)
	}
	if len(s.Periods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(s.Periods))
	}
	if s.Periods[0].DurationSeconds != 600 {
		t.Errorf("expected period duration 600s, got %d", s.Periods[0].DurationSeconds)
	}
}

func TestSummarize_MultipleDownPeriods(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "down", CheckedAt: start.Add(5 * time.Minute)},
		{State: "up", CheckedAt: start.Add(10 * time.Minute)},
		{State: "down", CheckedAt: start.Add(30 * time.Minute)},
		{State: "up", CheckedAt: start.Add(40 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if !s.HadDowntime {
		t.Error("expected HadDowntime=true")
	}
	if s.DowntimePeriodCount != 2 {
		t.Errorf("expected 2 periods, got %d", s.DowntimePeriodCount)
	}
	// Period 1: min 5 → min 10 = 300s; Period 2: min 30 → min 40 = 600s; Total = 900s
	if s.TotalDowntimeSeconds != 900 {
		t.Errorf("expected 900s total, got %d", s.TotalDowntimeSeconds)
	}
}

func TestSummarize_DownAtEndClosesAtWindowEnd(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "up", CheckedAt: start.Add(10 * time.Minute)},
		{State: "down", CheckedAt: start.Add(50 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if !s.HadDowntime {
		t.Error("expected HadDowntime=true")
	}
	if s.DowntimePeriodCount != 1 {
		t.Errorf("expected 1 period, got %d", s.DowntimePeriodCount)
	}
	// Down from minute 50 to window end (minute 60) = 600s
	if s.TotalDowntimeSeconds != 600 {
		t.Errorf("expected 600s total, got %d", s.TotalDowntimeSeconds)
	}
	if !s.Periods[0].End.Equal(end) {
		t.Errorf("expected period end to be window end, got %v", s.Periods[0].End)
	}
}

func TestSummarize_PendingTreatedAsNotDown(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "pending", CheckedAt: start.Add(5 * time.Minute)},
		{State: "pending", CheckedAt: start.Add(10 * time.Minute)},
		{State: "pending", CheckedAt: start.Add(15 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if s.HadDowntime {
		t.Error("expected HadDowntime=false when all pending")
	}
	if s.TotalDowntimeSeconds != 0 {
		t.Errorf("expected total 0, got %d", s.TotalDowntimeSeconds)
	}
}

func TestSummarize_PendingBreaksDownPeriod(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "down", CheckedAt: start.Add(5 * time.Minute)},
		{State: "pending", CheckedAt: start.Add(10 * time.Minute)},
		{State: "down", CheckedAt: start.Add(15 * time.Minute)},
		{State: "up", CheckedAt: start.Add(20 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	// pending breaks the down run → two separate periods
	if s.DowntimePeriodCount != 2 {
		t.Errorf("expected 2 periods, got %d", s.DowntimePeriodCount)
	}
	// Period 1: min 5 → min 10 = 300s; Period 2: min 15 → min 20 = 300s; Total = 600s
	if s.TotalDowntimeSeconds != 600 {
		t.Errorf("expected 600s total, got %d", s.TotalDowntimeSeconds)
	}
}

func TestSummarize_PointsOutsideWindowIgnored(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "down", CheckedAt: start.Add(-10 * time.Minute)}, // before window
		{State: "up", CheckedAt: start.Add(30 * time.Minute)},   // inside
		{State: "down", CheckedAt: start.Add(2 * time.Hour)},    // after window
	}

	s := Summarize(points, start, end, false)

	if s.HadDowntime {
		t.Error("expected no downtime (down points are outside window)")
	}
}

func TestSummarize_TruncatedFlagPassedThrough(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	s := Summarize(nil, start, end, true)

	if !s.Truncated {
		t.Error("expected Truncated=true")
	}
	if !s.WindowStart.Equal(start) {
		t.Error("expected WindowStart preserved")
	}
	if !s.WindowEnd.Equal(end) {
		t.Error("expected WindowEnd preserved")
	}
}

func TestSummarize_PeriodsAreOrdered(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	points := []pulseapi.HistoryPoint{
		{State: "down", CheckedAt: start.Add(5 * time.Minute)},
		{State: "up", CheckedAt: start.Add(10 * time.Minute)},
		{State: "down", CheckedAt: start.Add(20 * time.Minute)},
		{State: "up", CheckedAt: start.Add(25 * time.Minute)},
		{State: "down", CheckedAt: start.Add(40 * time.Minute)},
		{State: "up", CheckedAt: start.Add(50 * time.Minute)},
	}

	s := Summarize(points, start, end, false)

	if s.DowntimePeriodCount != 3 {
		t.Fatalf("expected 3 periods, got %d", s.DowntimePeriodCount)
	}

	for i := 1; i < len(s.Periods); i++ {
		if !s.Periods[i].Start.After(s.Periods[i-1].End) && !s.Periods[i].Start.Equal(s.Periods[i-1].End) {
			t.Errorf("period %d start (%v) is not after period %d end (%v)",
				i, s.Periods[i].Start, i-1, s.Periods[i-1].End)
		}
	}
}
