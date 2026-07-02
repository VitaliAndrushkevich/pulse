package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// DNSResolver abstracts DNS queries for testability.
type DNSResolver interface {
	Exchange(ctx context.Context, msg *dns.Msg, server string) (*dns.Msg, time.Duration, error)
}

// defaultDNSResolver wraps miekg/dns client for production use.
type defaultDNSResolver struct {
	client *dns.Client
}

func (d *defaultDNSResolver) Exchange(ctx context.Context, msg *dns.Msg, server string) (*dns.Msg, time.Duration, error) {
	return d.client.ExchangeContext(ctx, msg, server)
}

// DNSSettings holds configuration for the DNS checker.
type DNSSettings struct {
	// RecordType is the DNS record type to query (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, PTR).
	// Default: "A".
	RecordType string `json:"record_type,omitempty"`

	// ExpectedValue is an optional value to match against DNS results.
	// Uses "contains" semantics: if any record value contains expected_value, check passes.
	// If empty, any successful response with records → "up".
	ExpectedValue string `json:"expected_value,omitempty"`

	// DNSServer is the custom resolver address (host:port).
	// Default: "1.1.1.1:53" (Cloudflare DNS for determinism).
	DNSServer string `json:"dns_server,omitempty"`
}

// DNSChecker implements the Checker interface for DNS monitors.
type DNSChecker struct {
	resolver DNSResolver
}

// Check executes a DNS query against the given target domain.
func (d *DNSChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	start := time.Now()
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	s := parseDNSSettings(settings)

	// Validate record type and get the DNS type constant.
	qtype, err := recordTypeToQtype(s.RecordType)
	if err != nil {
		result.State = "down"
		result.Error = err.Error()
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	// Determine DNS server.
	server := s.DNSServer
	if server == "" {
		server = "1.1.1.1:53"
	}

	// Ensure server has port.
	if !strings.Contains(server, ":") {
		server = server + ":53"
	}

	// Build DNS query message.
	msg := new(dns.Msg)
	fqdn := dns.Fqdn(target)
	msg.SetQuestion(fqdn, qtype)
	msg.RecursionDesired = true

	// Get or create resolver.
	resolver := d.resolver
	if resolver == nil {
		resolver = &defaultDNSResolver{
			client: &dns.Client{
				Timeout: 10 * time.Second,
			},
		}
	}

	// Execute DNS query.
	resp, rtt, err := resolver.Exchange(ctx, msg, server)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("dns query: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	result.LatencyMs = int32(rtt.Milliseconds())
	if result.LatencyMs == 0 {
		// Fallback to wall-clock if rtt is zero.
		result.LatencyMs = int32(time.Since(start).Milliseconds())
	}

	// Check for NXDOMAIN or other error codes.
	if resp.Rcode != dns.RcodeSuccess {
		result.State = "down"
		result.Error = fmt.Sprintf("dns response code: %s", dns.RcodeToString[resp.Rcode])
		return result
	}

	// Extract record values from response.
	values := extractRecordValues(resp.Answer, qtype)

	// If no records found.
	if len(values) == 0 {
		result.State = "down"
		result.Error = fmt.Sprintf("no %s records found for %s", s.RecordType, target)
		return result
	}

	// If expected_value is set, check for match.
	if s.ExpectedValue != "" {
		found := false
		for _, v := range values {
			if strings.Contains(v, s.ExpectedValue) {
				found = true
				break
			}
		}
		if !found {
			result.State = "down"
			result.Error = fmt.Sprintf("expected value %q not found in %s records: %v",
				s.ExpectedValue, s.RecordType, values)
			return result
		}
	}

	result.State = "up"
	return result
}

// parseDNSSettings unmarshals settings JSON and applies defaults.
func parseDNSSettings(settings json.RawMessage) DNSSettings {
	s := DNSSettings{}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}
	if s.RecordType == "" {
		s.RecordType = "A"
	}
	return s
}

// recordTypeToQtype converts a record type string to a dns.Type constant.
func recordTypeToQtype(recordType string) (uint16, error) {
	switch strings.ToUpper(recordType) {
	case "A":
		return dns.TypeA, nil
	case "AAAA":
		return dns.TypeAAAA, nil
	case "CNAME":
		return dns.TypeCNAME, nil
	case "MX":
		return dns.TypeMX, nil
	case "TXT":
		return dns.TypeTXT, nil
	case "NS":
		return dns.TypeNS, nil
	case "SOA":
		return dns.TypeSOA, nil
	case "SRV":
		return dns.TypeSRV, nil
	case "PTR":
		return dns.TypePTR, nil
	default:
		return 0, fmt.Errorf("unsupported record type %q", recordType)
	}
}

// extractRecordValues extracts string values from DNS answer records.
func extractRecordValues(answers []dns.RR, qtype uint16) []string {
	var values []string
	for _, rr := range answers {
		if rr.Header().Rrtype != qtype {
			continue
		}
		switch v := rr.(type) {
		case *dns.A:
			values = append(values, v.A.String())
		case *dns.AAAA:
			values = append(values, v.AAAA.String())
		case *dns.CNAME:
			values = append(values, strings.TrimSuffix(v.Target, "."))
		case *dns.MX:
			values = append(values, strings.TrimSuffix(v.Mx, "."))
		case *dns.TXT:
			values = append(values, strings.Join(v.Txt, ""))
		case *dns.NS:
			values = append(values, strings.TrimSuffix(v.Ns, "."))
		case *dns.SOA:
			values = append(values, strings.TrimSuffix(v.Ns, "."))
		case *dns.SRV:
			values = append(values, fmt.Sprintf("%s:%d", strings.TrimSuffix(v.Target, "."), v.Port))
		case *dns.PTR:
			values = append(values, strings.TrimSuffix(v.Ptr, "."))
		}
	}
	return values
}
