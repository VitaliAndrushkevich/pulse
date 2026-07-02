package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"pgregory.net/rapid"
)

// mockDNSResolver implements DNSResolver for testing without real DNS.
type mockDNSResolver struct {
	response *dns.Msg
	rtt      time.Duration
	err      error
}

func (m *mockDNSResolver) Exchange(ctx context.Context, msg *dns.Msg, server string) (*dns.Msg, time.Duration, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	return m.response, m.rtt, m.err
}

// buildDNSResponse creates a mock DNS response with the given records.
func buildDNSResponse(rcode int, answers []dns.RR) *dns.Msg {
	msg := &dns.Msg{}
	msg.Rcode = rcode
	msg.Response = true
	msg.Answer = answers
	return msg
}

// --- Unit Tests: All record types ---

func TestDNSChecker_RecordTypeA(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "93.184.216.34"),
		}),
		rtt: 10 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A", ExpectedValue: "93.184.216.34"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeAAAA(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newAAAARecord("example.com.", "2606:2800:220:1:248:1893:25c8:1946"),
		}),
		rtt: 15 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "AAAA", ExpectedValue: "2606:2800:220:1:248:1893:25c8:1946"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeMX(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.MX{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeMX, Class: dns.ClassINET},
				Preference: 10, Mx: "mail.example.com."},
		}),
		rtt: 12 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "MX", ExpectedValue: "mail.example.com"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeCNAME(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.CNAME{Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET},
				Target: "example.com."},
		}),
		rtt: 8 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "CNAME", ExpectedValue: "example.com"})
	result := checker.Check(context.Background(), "www.example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeTXT(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.TXT{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeTXT, Class: dns.ClassINET},
				Txt: []string{"v=spf1 include:example.com ~all"}},
		}),
		rtt: 10 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "TXT", ExpectedValue: "v=spf1"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeNS(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.NS{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeNS, Class: dns.ClassINET},
				Ns: "ns1.example.com."},
		}),
		rtt: 11 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "NS", ExpectedValue: "ns1.example.com"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeSOA(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.SOA{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSOA, Class: dns.ClassINET},
				Ns: "ns1.example.com."},
		}),
		rtt: 10 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "SOA", ExpectedValue: "ns1.example.com"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypeSRV(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.SRV{Hdr: dns.RR_Header{Name: "_sip._tcp.example.com.", Rrtype: dns.TypeSRV, Class: dns.ClassINET},
				Target: "sip.example.com.", Port: 5060, Priority: 10, Weight: 100},
		}),
		rtt: 10 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "SRV", ExpectedValue: "sip.example.com:5060"})
	result := checker.Check(context.Background(), "_sip._tcp.example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_RecordTypePTR(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			&dns.PTR{Hdr: dns.RR_Header{Name: "34.216.184.93.in-addr.arpa.", Rrtype: dns.TypePTR, Class: dns.ClassINET},
				Ptr: "example.com."},
		}),
		rtt: 10 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "PTR", ExpectedValue: "example.com"})
	result := checker.Check(context.Background(), "34.216.184.93.in-addr.arpa", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

// --- Test: expected_value matching (found → up, not found → down) ---

func TestDNSChecker_ExpectedValueFound_Up(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
			newARecord("example.com.", "5.6.7.8"),
		}),
		rtt: 5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A", ExpectedValue: "1.2.3.4"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' when expected value found, got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_ExpectedValueNotFound_Down(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
			newARecord("example.com.", "5.6.7.8"),
		}),
		rtt: 5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A", ExpectedValue: "9.9.9.9"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' when expected value not found, got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error when state is down")
	}
}

// --- Test: no expected_value (any response → up) ---

func TestDNSChecker_NoExpectedValue_AnyResponse_Up(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
		}),
		rtt: 5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' when no expected_value set, got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_NoExpectedValue_EmptyResponse_Down(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, nil),
		rtt:      5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' when no records in response, got %q", result.State)
	}
}

// --- Test: custom resolver ---

func TestDNSChecker_CustomResolver(t *testing.T) {
	var usedServer string
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
		}),
		rtt: 5 * time.Millisecond,
	}
	// We can't capture in the mock easily, so we use a tracking resolver.
	trackingResolver := &trackingDNSResolver{
		inner:  resolver,
		server: &usedServer,
	}

	checker := &DNSChecker{resolver: trackingResolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "A", DNSServer: "8.8.8.8:53"})
	_ = checker.Check(context.Background(), "example.com", settings)

	if usedServer != "8.8.8.8:53" {
		t.Fatalf("expected custom resolver '8.8.8.8:53', got %q", usedServer)
	}
}

// trackingDNSResolver wraps a resolver and captures the server argument.
type trackingDNSResolver struct {
	inner  DNSResolver
	server *string
}

func (tr *trackingDNSResolver) Exchange(ctx context.Context, msg *dns.Msg, server string) (*dns.Msg, time.Duration, error) {
	*tr.server = server
	return tr.inner.Exchange(ctx, msg, server)
}

// --- Test: context cancellation ---

func TestDNSChecker_ContextCancellation(t *testing.T) {
	resolver := &mockDNSResolver{
		err: context.Canceled,
	}

	checker := &DNSChecker{resolver: resolver}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	settings, _ := json.Marshal(DNSSettings{RecordType: "A"})
	result := checker.Check(ctx, "example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on context cancellation, got %q", result.State)
	}
}

// --- Test: unsupported record type → down ---

func TestDNSChecker_UnsupportedRecordType(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, nil),
		rtt:      5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	settings, _ := json.Marshal(DNSSettings{RecordType: "INVALID"})
	result := checker.Check(context.Background(), "example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' for unsupported record type, got %q", result.State)
	}
	if !strings.Contains(result.Error, "unsupported") {
		t.Fatalf("expected error to mention 'unsupported', got: %s", result.Error)
	}
}

// --- Test: malformed settings JSON → down, no panic ---

func TestDNSChecker_MalformedSettings_NoPanic(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
		}),
		rtt: 5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}

	// Totally invalid JSON — should not panic, should use defaults.
	result := checker.Check(context.Background(), "example.com", json.RawMessage(`{invalid json`))

	// With defaults (record_type="A", no expected_value), if response has records → up
	if result.State != "up" {
		t.Fatalf("expected state 'up' with default settings on malformed JSON, got %q (error: %s)", result.State, result.Error)
	}
}

func TestDNSChecker_NilSettings_NoPanic(t *testing.T) {
	resolver := &mockDNSResolver{
		response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
			newARecord("example.com.", "1.2.3.4"),
		}),
		rtt: 5 * time.Millisecond,
	}

	checker := &DNSChecker{resolver: resolver}
	result := checker.Check(context.Background(), "example.com", nil)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with nil settings, got %q (error: %s)", result.State, result.Error)
	}
}

// --- Property Tests ---

// P-DNS-1: valid record_type always returns state "up" or "down" with non-empty Error when down.
func TestProperty_DNS_ValidRecordTypeReturnsValidState(t *testing.T) {
	validRecordTypes := []string{"A", "AAAA", "MX", "CNAME", "TXT", "NS", "SOA", "SRV", "PTR"}

	rapid.Check(t, func(rt *rapid.T) {
		recordType := rapid.SampledFrom(validRecordTypes).Draw(rt, "recordType")

		// Generate a response — may or may not have matching records.
		hasRecords := rapid.Bool().Draw(rt, "hasRecords")
		var answers []dns.RR
		if hasRecords {
			answers = []dns.RR{newARecord("example.com.", "1.2.3.4")}
		}

		resolver := &mockDNSResolver{
			response: buildDNSResponse(dns.RcodeSuccess, answers),
			rtt:      time.Duration(rapid.IntRange(1, 1000).Draw(rt, "rtt")) * time.Millisecond,
		}

		checker := &DNSChecker{resolver: resolver}
		settings, _ := json.Marshal(DNSSettings{RecordType: recordType})
		result := checker.Check(context.Background(), "example.com", settings)

		// State must be "up" or "down"
		if result.State != "up" && result.State != "down" {
			rt.Fatalf("expected state 'up' or 'down', got %q", result.State)
		}

		// When down, Error must be non-empty
		if result.State == "down" && result.Error == "" {
			rt.Fatalf("state is 'down' but Error is empty for record type %s", recordType)
		}
	})
}

// P-DNS-2: empty expected_value never fails on value mismatch.
func TestProperty_DNS_EmptyExpectedValueNeverMismatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random IP addresses for A records.
		numRecords := rapid.IntRange(1, 5).Draw(rt, "numRecords")
		answers := make([]dns.RR, numRecords)
		for i := range answers {
			ip := fmt.Sprintf("%d.%d.%d.%d",
				rapid.IntRange(1, 254).Draw(rt, "oct1"),
				rapid.IntRange(0, 255).Draw(rt, "oct2"),
				rapid.IntRange(0, 255).Draw(rt, "oct3"),
				rapid.IntRange(1, 254).Draw(rt, "oct4"),
			)
			answers[i] = newARecord("example.com.", ip)
		}

		resolver := &mockDNSResolver{
			response: buildDNSResponse(dns.RcodeSuccess, answers),
			rtt:      5 * time.Millisecond,
		}

		checker := &DNSChecker{resolver: resolver}
		// Empty expected_value — should ALWAYS be up as long as records exist.
		settings, _ := json.Marshal(DNSSettings{RecordType: "A", ExpectedValue: ""})
		result := checker.Check(context.Background(), "example.com", settings)

		if result.State != "up" {
			rt.Fatalf("expected state 'up' with empty expected_value and %d records, got %q (error: %s)",
				numRecords, result.State, result.Error)
		}

		// Error must NOT contain "mismatch" or "expected"
		if strings.Contains(strings.ToLower(result.Error), "mismatch") ||
			strings.Contains(strings.ToLower(result.Error), "expected") {
			rt.Fatalf("error mentions value mismatch when expected_value is empty: %s", result.Error)
		}
	})
}

// P-DNS-3: LatencyMs >= 0.
func TestProperty_DNS_LatencyNonNegative(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		rttMs := rapid.IntRange(0, 5000).Draw(rt, "rttMs")
		hasError := rapid.Bool().Draw(rt, "hasError")

		var resolver *mockDNSResolver
		if hasError {
			resolver = &mockDNSResolver{
				err: fmt.Errorf("network error"),
			}
		} else {
			resolver = &mockDNSResolver{
				response: buildDNSResponse(dns.RcodeSuccess, []dns.RR{
					newARecord("example.com.", "1.2.3.4"),
				}),
				rtt: time.Duration(rttMs) * time.Millisecond,
			}
		}

		checker := &DNSChecker{resolver: resolver}
		settings, _ := json.Marshal(DNSSettings{RecordType: "A"})
		result := checker.Check(context.Background(), "example.com", settings)

		if result.LatencyMs < 0 {
			rt.Fatalf("LatencyMs must be >= 0, got %d", result.LatencyMs)
		}
	})
}

// --- Helpers ---

func newARecord(name, ip string) *dns.A {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP(ip).To4(),
	}
}

func newAAAARecord(name, ip string) *dns.AAAA {
	return &dns.AAAA{
		Hdr:  dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300},
		AAAA: net.ParseIP(ip).To16(),
	}
}
