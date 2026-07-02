package monitor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/miekg/dns"
	"pgregory.net/rapid"
)

// Cross-protocol property tests verify invariants that apply to ALL checkers.
// These complement the per-protocol property tests with global guarantees.

// --- P-ALL-1: Context cancellation causes return within 100ms ---

func TestProperty_ALL_ContextCancellationReturnsQuickly_DNS(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		resolver := &mockDNSResolver{err: context.Canceled}
		checker := &DNSChecker{resolver: resolver}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		settings, _ := json.Marshal(DNSSettings{RecordType: "A"})

		start := time.Now()
		result := checker.Check(ctx, "example.com", settings)
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			rt.Fatalf("DNS checker took %v after context cancellation (expected <100ms)", elapsed)
		}
		if result.State != "down" {
			rt.Fatalf("DNS checker should return 'down' on cancellation, got %q", result.State)
		}
	})
}

func TestProperty_ALL_ContextCancellationReturnsQuickly_ICMP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		pinger := &mockPinger{err: context.Canceled}
		checker := &ICMPChecker{pinger: pinger}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		settings, _ := json.Marshal(ICMPSettings{PacketCount: 3})

		start := time.Now()
		result := checker.Check(ctx, "8.8.8.8", settings)
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			rt.Fatalf("ICMP checker took %v after context cancellation (expected <100ms)", elapsed)
		}
		if result.State != "down" {
			rt.Fatalf("ICMP checker should return 'down' on cancellation, got %q", result.State)
		}
	})
}

func TestProperty_ALL_ContextCancellationReturnsQuickly_SMTP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dialer := &mockSMTPDialer{err: context.Canceled}
		checker := &SMTPChecker{dialer: dialer}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		settings, _ := json.Marshal(SMTPSettings{Port: 25})

		start := time.Now()
		result := checker.Check(ctx, "mail.example.com", settings)
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			rt.Fatalf("SMTP checker took %v after context cancellation (expected <100ms)", elapsed)
		}
		if result.State != "down" {
			rt.Fatalf("SMTP checker should return 'down' on cancellation, got %q", result.State)
		}
	})
}

// --- P-ALL-2: Any settings JSON (nil, empty, random) never causes panic ---

func TestProperty_ALL_AnySettingsNeverPanics_DNS(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		resolver := &mockDNSResolver{
			response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
				newARecord("example.com.", "1.2.3.4"),
			}),
			rtt: 5 * time.Millisecond,
		}
		checker := &DNSChecker{resolver: resolver}

		settings := generateRandomSettings(rt)
		// Must not panic — that's the property
		_ = checker.Check(context.Background(), "example.com", settings)
	})
}

func TestProperty_ALL_AnySettingsNeverPanics_ICMP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		pinger := &mockPinger{
			sent:     3,
			received: 3,
			avgRTT:   10 * time.Millisecond,
		}
		checker := &ICMPChecker{pinger: pinger}

		settings := generateRandomSettings(rt)
		// Must not panic — that's the property
		_ = checker.Check(context.Background(), "8.8.8.8", settings)
	})
}

func TestProperty_ALL_AnySettingsNeverPanics_SMTP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dialer := &mockSMTPDialer{
			conn: &mockSMTPConn{
				conversation: []string{
					"220 mail.example.com ESMTP\r\n",
					"250-mail.example.com\r\n250 OK\r\n",
					"221 Bye\r\n",
				},
			},
		}
		checker := &SMTPChecker{dialer: dialer}

		settings := generateRandomSettings(rt)
		// Must not panic — that's the property
		_ = checker.Check(context.Background(), "mail.example.com", settings)
	})
}

// --- P-ALL-3: State is always exactly "up" or "down" ---

func TestProperty_ALL_StateAlwaysUpOrDown_DNS(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		hasError := rapid.Bool().Draw(rt, "hasError")
		var resolver *mockDNSResolver
		if hasError {
			resolver = &mockDNSResolver{err: context.DeadlineExceeded}
		} else {
			hasRecords := rapid.Bool().Draw(rt, "hasRecords")
			var answers []dns.RR
			if hasRecords {
				answers = []dns.RR{newARecord("example.com.", "1.2.3.4")}
			}
			resolver = &mockDNSResolver{
				response: buildDNSResponse(dns.RcodeSuccess, answers),
				rtt:      5 * time.Millisecond,
			}
		}

		checker := &DNSChecker{resolver: resolver}
		settings, _ := json.Marshal(DNSSettings{RecordType: "A"})
		result := checker.Check(context.Background(), "example.com", settings)

		if result.State != "up" && result.State != "down" {
			rt.Fatalf("DNS state must be 'up' or 'down', got %q", result.State)
		}
	})
}

func TestProperty_ALL_StateAlwaysUpOrDown_ICMP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		hasError := rapid.Bool().Draw(rt, "hasError")
		var pinger Pinger
		if hasError {
			pinger = &mockPinger{err: context.DeadlineExceeded}
		} else {
			sent := rapid.IntRange(1, 10).Draw(rt, "sent")
			received := rapid.IntRange(0, sent).Draw(rt, "received")
			pinger = &mockPinger{
				sent:     sent,
				received: received,
				avgRTT:   10 * time.Millisecond,
			}
		}

		checker := &ICMPChecker{pinger: pinger}
		settings, _ := json.Marshal(ICMPSettings{
			PacketCount:          3,
			LossThresholdPercent: rapid.IntRange(0, 100).Draw(rt, "threshold"),
		})
		result := checker.Check(context.Background(), "8.8.8.8", settings)

		if result.State != "up" && result.State != "down" {
			rt.Fatalf("ICMP state must be 'up' or 'down', got %q", result.State)
		}
	})
}

func TestProperty_ALL_StateAlwaysUpOrDown_SMTP(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		hasError := rapid.Bool().Draw(rt, "hasError")
		var dialer *mockSMTPDialer
		if hasError {
			dialer = &mockSMTPDialer{err: context.DeadlineExceeded}
		} else {
			dialer = &mockSMTPDialer{
				conn: &mockSMTPConn{
					conversation: []string{
						"220 mail.example.com ESMTP\r\n",
						"250-mail.example.com\r\n250 OK\r\n",
						"221 Bye\r\n",
					},
				},
			}
		}

		checker := &SMTPChecker{dialer: dialer}
		settings, _ := json.Marshal(SMTPSettings{
			Port:       rapid.IntRange(1, 65535).Draw(rt, "port"),
			StartTLS:   rapid.Bool().Draw(rt, "starttls"),
			EHLODomain: "pulse.local",
		})
		result := checker.Check(context.Background(), "mail.example.com", settings)

		if result.State != "up" && result.State != "down" {
			rt.Fatalf("SMTP state must be 'up' or 'down', got %q", result.State)
		}
	})
}

// --- Helpers ---

// generateRandomSettings produces nil, empty, or random JSON settings.
func generateRandomSettings(rt *rapid.T) json.RawMessage {
	choice := rapid.IntRange(0, 3).Draw(rt, "settingsChoice")
	switch choice {
	case 0:
		return nil
	case 1:
		return json.RawMessage(`{}`)
	case 2:
		return json.RawMessage(`{invalid json!!!`)
	default:
		// Random valid JSON with arbitrary fields
		numFields := rapid.IntRange(0, 5).Draw(rt, "numFields")
		fields := make(map[string]interface{})
		for i := 0; i < numFields; i++ {
			key := rapid.String().Draw(rt, "key")
			val := rapid.IntRange(-1000, 1000).Draw(rt, "val")
			fields[key] = val
		}
		data, _ := json.Marshal(fields)
		return data
	}
}
