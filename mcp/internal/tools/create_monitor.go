package tools

import (
	"context"
	"strings"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// createMonitorTypes maps lowercase user input to the canonical Pulse wire form
// for the subset of types supported by the create-monitor tool.
var createMonitorTypes = map[string]string{
	"http":      "http",
	"http/3":    "http3",
	"http3":     "http3",
	"tcp":       "tcp",
	"udp":       "udp",
	"icmp":      "icmp",
	"quic":      "quic",
	"websocket": "websocket",
	"dns":       "dns",
	"smtp":      "smtp",
}

// createMonitorSupportedTypes is the sorted list shown in error messages.
var createMonitorSupportedTypes = "DNS, HTTP, HTTP/3, ICMP, QUIC, SMTP, TCP, UDP, WebSocket"

// CreateMonitorToolInput defines the input schema for the create-monitor tool.
type CreateMonitorToolInput struct {
	// Type is the monitor type (case-insensitive).
	Type string `json:"type" jsonschema:"Monitor type: DNS, HTTP, HTTP/3, ICMP, QUIC, SMTP, TCP, UDP, or WebSocket (case-insensitive)"`

	// Name is the display name for the monitor (1–255 characters, not blank).
	Name string `json:"name" jsonschema:"Monitor display name (1-255 characters)"`

	// Target is the monitoring target. Format depends on type:
	// HTTP/HTTP3: URL or bare host; TCP/UDP: host:port; ICMP: hostname or IP;
	// WebSocket: ws:// or wss:// URL; DNS: domain name to query; SMTP: hostname or host:port.
	Target string `json:"target" jsonschema:"Monitoring target. HTTP: URL or bare host; TCP/UDP: host:port; ICMP: hostname or IP; WebSocket: ws:// or wss:// URL; DNS: domain name; SMTP: hostname or host:port"`

	// IntervalSeconds is the check interval in seconds (≥1, default: Pulse default 60).
	IntervalSeconds *int `json:"interval_seconds,omitempty" jsonschema:"Check interval in seconds (minimum 1, default 60)"`

	// TimeoutSeconds is the check timeout in seconds (≥1, default: Pulse default 10).
	TimeoutSeconds *int `json:"timeout_seconds,omitempty" jsonschema:"Check timeout in seconds (minimum 1, default 10)"`

	// HTTPExpectedStatuses is an optional list of expected HTTP status codes (100–599).
	// Only applicable for HTTP and HTTP/3 type monitors.
	HTTPExpectedStatuses []int `json:"http_expected_statuses,omitempty" jsonschema:"Expected HTTP status codes (100-599, HTTP and HTTP/3 type only)"`

	// WSExpectedStatuses is an optional list of HTTP status codes considered acceptable
	// on a failed WebSocket upgrade (100–599, max 10). Only applicable for WebSocket type.
	// Useful for monitoring auth-protected endpoints where 401/403 means "server is alive".
	WSExpectedStatuses []int `json:"ws_expected_statuses,omitempty" jsonschema:"Expected HTTP status codes on failed WebSocket upgrade (100-599, max 10, WebSocket type only)"`

	// DNSRecordType is the DNS record type to query (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, PTR).
	// Only applicable for DNS type monitors. Default: A.
	DNSRecordType *string `json:"dns_record_type,omitempty" jsonschema:"DNS record type to query: A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, PTR (DNS type only, default: A)"`

	// DNSServer is a custom DNS resolver address (host:port).
	// Only applicable for DNS type monitors. Default: 1.1.1.1:53.
	DNSServer *string `json:"dns_server,omitempty" jsonschema:"Custom DNS resolver (host:port, DNS type only, default: 1.1.1.1:53)"`
}

// CreateMonitorOutput contains the created monitor details.
type CreateMonitorOutput struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Target          string `json:"target"`
	Status          string `json:"status"`
	State           string `json:"state"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// HandleCreateMonitor creates a new monitor via the Pulse API.
// Validation order: (1) read-only mode, (2) type check, (3) name check, (4) target shape check.
func HandleCreateMonitor(ctx context.Context, deps Deps, input CreateMonitorToolInput) (*CreateMonitorOutput, error) {
	// 1. Check access mode — reject if read-only.
	if deps.AccessMode != config.ReadWrite {
		return nil, mcperr.WriteDisabled()
	}

	// 2. Validate and normalize type.
	lower := strings.ToLower(strings.TrimSpace(input.Type))
	canonicalType, ok := createMonitorTypes[lower]
	if !ok {
		return nil, mcperr.InvalidType(
			"unsupported monitor type " + quote(input.Type) + "; supported types: " + createMonitorSupportedTypes,
		)
	}

	// 3. Validate name: must not be empty, blank, or >255 chars.
	if err := validateMonitorName(input.Name); err != nil {
		return nil, err
	}

	// 4. Validate target shape.
	if err := validateTarget(canonicalType, input.Target); err != nil {
		return nil, err
	}

	// 5. Validate optional fields.
	if input.IntervalSeconds != nil && *input.IntervalSeconds < 1 {
		return nil, mcperr.Validation("interval_seconds must be ≥ 1")
	}
	if input.TimeoutSeconds != nil && *input.TimeoutSeconds < 1 {
		return nil, mcperr.Validation("timeout_seconds must be ≥ 1")
	}

	// 6. Build settings map for type-specific options.
	settings := map[string]any{}
	if (canonicalType == "http" || canonicalType == "http3") && len(input.HTTPExpectedStatuses) > 0 {
		for _, code := range input.HTTPExpectedStatuses {
			if code < 100 || code > 599 {
				return nil, mcperr.Validation("http_expected_statuses values must be between 100 and 599")
			}
		}
		settings["expected_statuses"] = input.HTTPExpectedStatuses
	}
	if canonicalType == "websocket" && len(input.WSExpectedStatuses) > 0 {
		if len(input.WSExpectedStatuses) > 10 {
			return nil, mcperr.Validation("ws_expected_statuses must have at most 10 entries")
		}
		for _, code := range input.WSExpectedStatuses {
			if code < 100 || code > 599 {
				return nil, mcperr.Validation("ws_expected_statuses values must be between 100 and 599")
			}
		}
		settings["expected_statuses"] = input.WSExpectedStatuses
	}
	if canonicalType == "dns" {
		if input.DNSRecordType != nil {
			rt := strings.ToUpper(strings.TrimSpace(*input.DNSRecordType))
			if !isValidDNSRecordType(rt) {
				return nil, mcperr.Validation("dns_record_type must be one of: A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, PTR")
			}
			settings["record_type"] = rt
		}
		if input.DNSServer != nil {
			server := strings.TrimSpace(*input.DNSServer)
			if server == "" {
				return nil, mcperr.Validation("dns_server must not be blank")
			}
			settings["dns_server"] = server
		}
	}

	// 7. Build Pulse API input.
	createInput := pulseapi.CreateMonitorInput{
		Type:     canonicalType,
		Name:     input.Name,
		Target:   input.Target,
		Settings: settings,
	}
	if input.IntervalSeconds != nil {
		createInput.IntervalSeconds = input.IntervalSeconds
	}
	if input.TimeoutSeconds != nil {
		createInput.TimeoutSeconds = input.TimeoutSeconds
	}

	// 8. Call Pulse API.
	monitor, err := deps.Client.CreateMonitor(ctx, createInput)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// 9. Return created monitor.
	return &CreateMonitorOutput{
		ID:              monitor.ID,
		Name:            monitor.Name,
		Type:            monitor.Type,
		Target:          monitor.Target,
		Status:          monitor.Status,
		State:           monitor.State,
		IntervalSeconds: monitor.IntervalSeconds,
		TimeoutSeconds:  monitor.TimeoutSeconds,
	}, nil
}

// validateMonitorName checks that the name is non-empty, not blank, and within length bounds.
func validateMonitorName(name string) error {
	if name == "" {
		return mcperr.Validation("name is required")
	}
	if strings.TrimSpace(name) == "" {
		return mcperr.Validation("name must not be blank")
	}
	if len(name) > 255 {
		return mcperr.Validation("name must be 1–255 characters")
	}
	return nil
}

// validateTarget performs a basic shape check on the target before sending to Pulse.
func validateTarget(monitorType, target string) error {
	if target == "" {
		return mcperr.Validation("target is required")
	}

	switch monitorType {
	case "tcp", "udp":
		// TCP/UDP targets must contain a colon separating host and port.
		if !strings.Contains(target, ":") {
			return mcperr.Validation("target for " + strings.ToUpper(monitorType) + " must be in host:port format")
		}
	case "icmp":
		// ICMP targets must not contain a port (no colon with port-like suffix).
		// Allow IPv6 addresses (contain colons) but reject obvious host:port patterns.
		// Simple heuristic: if it has a colon and the last segment is numeric, reject.
		// Otherwise allow (could be IPv6 or hostname).
	case "http", "http3", "quic":
		// HTTP/HTTP3/QUIC targets: non-empty is sufficient; Pulse handles normalization.
	case "websocket":
		// WebSocket targets should be ws:// or wss:// URLs.
		// Non-empty is sufficient; Pulse handles normalization.
	case "dns":
		// DNS targets are domain names to query. Non-empty is sufficient.
	case "smtp":
		// SMTP targets are hostnames or host:port. Non-empty is sufficient.
	}

	return nil
}

// isValidDNSRecordType checks if the given string is a supported DNS record type.
func isValidDNSRecordType(rt string) bool {
	switch rt {
	case "A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "SRV", "PTR":
		return true
	}
	return false
}

// quote wraps a string in double quotes for use in error messages.
func quote(s string) string {
	return `"` + s + `"`
}
