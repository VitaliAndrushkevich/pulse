package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockPinger implements Pinger for testing without real ICMP.
type mockPinger struct {
	sent     int
	received int
	avgRTT   time.Duration
	err      error
}

func (m *mockPinger) Ping(ctx context.Context, addr string, count int, useIPv6 bool) (sent, received int, avgRTT time.Duration, err error) {
	if ctx.Err() != nil {
		return 0, 0, 0, ctx.Err()
	}
	return m.sent, m.received, m.avgRTT, m.err
}

// --- Unit Tests ---

func TestICMPChecker_AllPacketsReceived_Up(t *testing.T) {
	pinger := &mockPinger{
		sent:     3,
		received: 3,
		avgRTT:   20 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3, LossThresholdPercent: 100})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs != 20 {
		t.Fatalf("expected LatencyMs=20, got %d", result.LatencyMs)
	}
}

func TestICMPChecker_PartialLossBelowThreshold_Up(t *testing.T) {
	// 1 out of 3 lost = 33% loss, threshold is 50% → up
	pinger := &mockPinger{
		sent:     3,
		received: 2,
		avgRTT:   25 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3, LossThresholdPercent: 50})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' (loss 33%% < threshold 50%%), got %q (error: %s)", result.State, result.Error)
	}
}

func TestICMPChecker_LossAboveThreshold_Down(t *testing.T) {
	// 2 out of 3 lost = 66% loss, threshold is 50% → down
	pinger := &mockPinger{
		sent:     3,
		received: 1,
		avgRTT:   30 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3, LossThresholdPercent: 50})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' (loss 66%% > threshold 50%%), got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error when state is down")
	}
}

func TestICMPChecker_TotalLoss_Down(t *testing.T) {
	pinger := &mockPinger{
		sent:     3,
		received: 0,
		avgRTT:   0,
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3, LossThresholdPercent: 100})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on total loss, got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error on total loss")
	}
}

func TestICMPChecker_PingerError_Down(t *testing.T) {
	pinger := &mockPinger{
		err: fmt.Errorf("permission denied"),
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on pinger error, got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error on pinger failure")
	}
}

func TestICMPChecker_ContextCancellation(t *testing.T) {
	pinger := &mockPinger{
		err: context.Canceled,
	}

	checker := &ICMPChecker{pinger: pinger}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3})
	result := checker.Check(ctx, "8.8.8.8", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on context cancellation, got %q", result.State)
	}
}

func TestICMPChecker_MalformedSettings_NoPanic(t *testing.T) {
	pinger := &mockPinger{
		sent:     3,
		received: 3,
		avgRTT:   10 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	result := checker.Check(context.Background(), "8.8.8.8", json.RawMessage(`{invalid json`))

	if result.State != "up" {
		t.Fatalf("expected state 'up' with default settings on malformed JSON, got %q (error: %s)", result.State, result.Error)
	}
}

func TestICMPChecker_NilSettings_NoPanic(t *testing.T) {
	pinger := &mockPinger{
		sent:     3,
		received: 3,
		avgRTT:   10 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	result := checker.Check(context.Background(), "8.8.8.8", nil)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with nil settings, got %q (error: %s)", result.State, result.Error)
	}
}

func TestICMPChecker_DefaultThreshold100_PartialLoss_Up(t *testing.T) {
	// Default threshold is 100, so only 100% loss triggers down
	pinger := &mockPinger{
		sent:     3,
		received: 1,
		avgRTT:   50 * time.Millisecond,
	}

	checker := &ICMPChecker{pinger: pinger}
	// No threshold set → default 100
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3})
	result := checker.Check(context.Background(), "8.8.8.8", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with default threshold 100%% (loss 66%% < 100%%), got %q (error: %s)", result.State, result.Error)
	}
}

func TestICMPChecker_IPv6Flag(t *testing.T) {
	var capturedIPv6 bool
	pinger := &capturingPinger{
		result: pingResult{sent: 3, received: 3, avgRTT: 10 * time.Millisecond},
		ipv6:   &capturedIPv6,
	}

	checker := &ICMPChecker{pinger: pinger}
	settings, _ := json.Marshal(ICMPSettings{PacketCount: 3, UseIPv6: true})
	_ = checker.Check(context.Background(), "::1", settings)

	if !capturedIPv6 {
		t.Fatal("expected UseIPv6=true to be passed to pinger")
	}
}

// capturingPinger captures arguments for assertion.
type capturingPinger struct {
	result pingResult
	ipv6   *bool
}

type pingResult struct {
	sent     int
	received int
	avgRTT   time.Duration
	err      error
}

func (c *capturingPinger) Ping(ctx context.Context, addr string, count int, useIPv6 bool) (int, int, time.Duration, error) {
	*c.ipv6 = useIPv6
	return c.result.sent, c.result.received, c.result.avgRTT, c.result.err
}

// --- Property Tests ---

// P-ICMP-1: packet_count always >= 1 (settings clamped).
func TestProperty_ICMP_PacketCountClamped(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		count := rapid.IntRange(-100, 100).Draw(rt, "packetCount")

		var capturedCount int
		pinger := &countCapturingPinger{
			count:  &capturedCount,
			result: pingResult{sent: 1, received: 1, avgRTT: 5 * time.Millisecond},
		}

		checker := &ICMPChecker{pinger: pinger}
		settings, _ := json.Marshal(ICMPSettings{PacketCount: count})
		_ = checker.Check(context.Background(), "8.8.8.8", settings)

		if capturedCount < 1 {
			rt.Fatalf("packet_count passed to pinger must be >= 1, got %d (input: %d)", capturedCount, count)
		}
	})
}

// countCapturingPinger captures the count argument.
type countCapturingPinger struct {
	count  *int
	result pingResult
}

func (c *countCapturingPinger) Ping(ctx context.Context, addr string, count int, useIPv6 bool) (int, int, time.Duration, error) {
	*c.count = count
	return c.result.sent, c.result.received, c.result.avgRTT, c.result.err
}

// P-ICMP-2: loss percentage always in [0, 100].
func TestProperty_ICMP_LossPercentageInRange(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		sent := rapid.IntRange(1, 100).Draw(rt, "sent")
		received := rapid.IntRange(0, sent).Draw(rt, "received")
		rttMs := rapid.IntRange(0, 1000).Draw(rt, "rttMs")

		pinger := &mockPinger{
			sent:     sent,
			received: received,
			avgRTT:   time.Duration(rttMs) * time.Millisecond,
		}

		checker := &ICMPChecker{pinger: pinger}
		settings, _ := json.Marshal(ICMPSettings{PacketCount: sent, LossThresholdPercent: 50})
		result := checker.Check(context.Background(), "8.8.8.8", settings)

		// We can't directly inspect loss_percent, but we can verify the result is consistent:
		// If received == sent → loss is 0% → state must be "up" (below 50%)
		// If received == 0 → loss is 100% → state must be "down" (above 50%)
		lossPct := (sent - received) * 100 / sent
		if lossPct < 0 || lossPct > 100 {
			rt.Fatalf("calculated loss %d%% is outside [0, 100]", lossPct)
		}

		// Verify state is consistent with loss vs threshold
		if lossPct >= 50 && result.State != "down" {
			rt.Fatalf("loss %d%% >= threshold 50%% but state is %q", lossPct, result.State)
		}
		if lossPct < 50 && result.State != "up" {
			rt.Fatalf("loss %d%% < threshold 50%% but state is %q (error: %s)", lossPct, result.State, result.Error)
		}
	})
}

// P-ICMP-3: LatencyMs >= 0.
func TestProperty_ICMP_LatencyNonNegative(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		rttMs := rapid.IntRange(0, 5000).Draw(rt, "rttMs")
		hasError := rapid.Bool().Draw(rt, "hasError")

		var pinger Pinger
		if hasError {
			pinger = &mockPinger{err: fmt.Errorf("network error")}
		} else {
			pinger = &mockPinger{
				sent:     3,
				received: 3,
				avgRTT:   time.Duration(rttMs) * time.Millisecond,
			}
		}

		checker := &ICMPChecker{pinger: pinger}
		settings, _ := json.Marshal(ICMPSettings{PacketCount: 3})
		result := checker.Check(context.Background(), "8.8.8.8", settings)

		if result.LatencyMs < 0 {
			rt.Fatalf("LatencyMs must be >= 0, got %d", result.LatencyMs)
		}
	})
}

// P-ICMP-4: 100% loss → state always "down".
func TestProperty_ICMP_TotalLossAlwaysDown(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		sent := rapid.IntRange(1, 100).Draw(rt, "sent")
		threshold := rapid.IntRange(0, 100).Draw(rt, "threshold")

		pinger := &mockPinger{
			sent:     sent,
			received: 0,
			avgRTT:   0,
		}

		checker := &ICMPChecker{pinger: pinger}
		settings, _ := json.Marshal(ICMPSettings{
			PacketCount:          sent,
			LossThresholdPercent: threshold,
		})
		result := checker.Check(context.Background(), "8.8.8.8", settings)

		if result.State != "down" {
			rt.Fatalf("100%% loss with threshold %d%% should always be 'down', got %q", threshold, result.State)
		}
	})
}
