package handlers

import (
	"testing"
	"time"
)

func TestRetentionClamp_HeartbeatHistory(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	t.Run("within retention is not clamped", func(t *testing.T) {
		from := now.Add(-10 * 24 * time.Hour) // 10 days ago, retention 30
		got, truncated := retentionClamp(from, now, 30)
		if truncated {
			t.Fatalf("expected not truncated")
		}
		if !got.Equal(from) {
			t.Fatalf("expected from unchanged, got %v", got)
		}
	})

	t.Run("before retention boundary is clamped and truncated", func(t *testing.T) {
		from := now.Add(-100 * 24 * time.Hour) // 100 days ago, retention 30
		got, truncated := retentionClamp(from, now, 30)
		if !truncated {
			t.Fatalf("expected truncated")
		}
		boundary := now.Add(-30 * 24 * time.Hour)
		if !got.Equal(boundary) {
			t.Fatalf("expected clamp to boundary %v, got %v", boundary, got)
		}
	})
}

func TestBuildHeartbeatSummary(t *testing.T) {
	t.Run("zero events yields null uptime and zero counts", func(t *testing.T) {
		s := buildHeartbeatSummary(map[string]int64{})
		if s.UptimePercent != nil {
			t.Fatalf("expected nil uptime, got %v", *s.UptimePercent)
		}
		if s.TotalEvents != 0 || s.SuccessCount != 0 || s.FailCount != 0 || s.MissedCount != 0 {
			t.Fatalf("expected all-zero counts, got %+v", s)
		}
	})

	t.Run("uptime percent and invariant", func(t *testing.T) {
		s := buildHeartbeatSummary(map[string]int64{
			"success": 142,
			"fail":    1,
			"missed":  2,
		})
		if s.SuccessCount != 142 || s.FailCount != 1 || s.MissedCount != 2 {
			t.Fatalf("unexpected counts: %+v", s)
		}
		if s.TotalEvents != 145 {
			t.Fatalf("expected total 145, got %d", s.TotalEvents)
		}
		// Invariant: success + fail + missed == total.
		if s.SuccessCount+s.FailCount+s.MissedCount != s.TotalEvents {
			t.Fatalf("invariant violated: %+v", s)
		}
		if s.UptimePercent == nil {
			t.Fatalf("expected non-nil uptime")
		}
		// 142/145*100 = 97.9310... -> rounded to one decimal = 97.9
		if *s.UptimePercent != 97.9 {
			t.Fatalf("expected 97.9, got %v", *s.UptimePercent)
		}
	})

	t.Run("fail and missed reduce uptime equally", func(t *testing.T) {
		// Same total, same success — one window has fails, the other missed.
		withFail := buildHeartbeatSummary(map[string]int64{"success": 8, "fail": 2})
		withMissed := buildHeartbeatSummary(map[string]int64{"success": 8, "missed": 2})
		if withFail.UptimePercent == nil || withMissed.UptimePercent == nil {
			t.Fatalf("expected non-nil uptime in both cases")
		}
		if *withFail.UptimePercent != *withMissed.UptimePercent {
			t.Fatalf("fail and missed should reduce uptime equally: fail=%v missed=%v",
				*withFail.UptimePercent, *withMissed.UptimePercent)
		}
		if *withFail.UptimePercent != 80.0 {
			t.Fatalf("expected 80.0, got %v", *withFail.UptimePercent)
		}
	})

	t.Run("all success is 100 percent", func(t *testing.T) {
		s := buildHeartbeatSummary(map[string]int64{"success": 5})
		if s.UptimePercent == nil || *s.UptimePercent != 100.0 {
			t.Fatalf("expected 100.0, got %v", s.UptimePercent)
		}
	})

	t.Run("no success is 0 percent not null", func(t *testing.T) {
		s := buildHeartbeatSummary(map[string]int64{"fail": 3, "missed": 1})
		if s.UptimePercent == nil {
			t.Fatalf("expected non-nil uptime (0.0) when events exist")
		}
		if *s.UptimePercent != 0.0 {
			t.Fatalf("expected 0.0, got %v", *s.UptimePercent)
		}
	})
}
