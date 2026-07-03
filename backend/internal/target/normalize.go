// Package target provides normalization of monitor target strings.
// It applies smart defaults so users can omit protocols when the type
// makes the intent unambiguous (e.g., "google.com" for HTTP → "https://google.com").
package target

import (
	"fmt"
	"net"
	"strings"
)

// Normalize applies protocol/port defaults based on monitor type.
// It allows users to be concise while ensuring checkers receive
// the format they expect.
//
// Rules by type:
//
//	http/http3: bare domain → "https://domain"; explicit http:// or https:// kept.
//	tcp:        must be host:port; no scheme allowed.
//	udp:        must be host:port; no scheme allowed.
//	websocket:  bare domain → "wss://domain"; explicit ws:// or wss:// kept.
//	grpc:       must be host:port; no scheme allowed.
//	dns:        bare domain; no scheme or port.
//	icmp:       hostname or IP; no scheme or port.
//	smtp:       bare hostname or host:port; no scheme.
//	quic:       bare domain → "https://domain"; explicit http:// or https:// kept.
func Normalize(monitorType, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	switch monitorType {
	case "http", "http3":
		return normalizeHTTP(target)
	case "quic":
		return normalizeQUIC(target)
	case "tcp":
		return normalizeTCP(target)
	case "udp":
		return normalizeUDP(target)
	case "websocket":
		return normalizeWebSocket(target)
	case "grpc":
		return normalizeGRPC(target)
	case "dns":
		return normalizeDNS(target)
	case "icmp":
		return normalizeICMP(target)
	case "smtp":
		return normalizeSMTP(target)
	default:
		return target, nil
	}
}

// normalizeHTTP ensures an HTTP(S) target has a scheme.
// - "example.com" → "https://example.com"
// - "example.com:8080" → "https://example.com:8080"
// - "http://example.com" → kept as-is
// - "https://example.com" → kept as-is
func normalizeHTTP(target string) (string, error) {
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return target, nil
	}
	// Reject other explicit schemes.
	if idx := strings.Index(target, "://"); idx > 0 {
		scheme := target[:idx]
		return "", fmt.Errorf("unsupported scheme %q for HTTP monitor; use http:// or https://", scheme)
	}
	return "https://" + target, nil
}

// normalizeQUIC ensures a QUIC target has an http:// or https:// scheme.
// QUIC operates over UDP but uses the same URL format as HTTPS.
// - "example.com" → "https://example.com"
// - "example.com:4433" → "https://example.com:4433"
// - "https://example.com" → kept as-is
func normalizeQUIC(target string) (string, error) {
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return target, nil
	}
	// Reject other explicit schemes.
	if idx := strings.Index(target, "://"); idx > 0 {
		scheme := target[:idx]
		return "", fmt.Errorf("unsupported scheme %q for QUIC monitor; use https:// or http://", scheme)
	}
	return "https://" + target, nil
}

// normalizeTCP ensures a TCP target is host:port without a scheme.
func normalizeTCP(target string) (string, error) {
	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("TCP target must be host:port without a scheme (e.g., example.com:443)")
	}
	_, _, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("TCP target must be host:port (e.g., example.com:443): %w", err)
	}
	return target, nil
}

// normalizeUDP ensures a UDP target is host:port without a scheme.
func normalizeUDP(target string) (string, error) {
	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("UDP target must be host:port without a scheme (e.g., example.com:53)")
	}
	_, _, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("UDP target must be host:port (e.g., example.com:53): %w", err)
	}
	return target, nil
}

// normalizeWebSocket ensures a WebSocket target has a ws:// or wss:// scheme.
// - "example.com/ws" → "wss://example.com/ws"
// - "example.com:8080/ws" → "wss://example.com:8080/ws"
// - "ws://example.com/ws" → kept
// - "wss://example.com/ws" → kept
func normalizeWebSocket(target string) (string, error) {
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		return target, nil
	}
	// Reject other explicit schemes.
	if idx := strings.Index(target, "://"); idx > 0 {
		scheme := target[:idx]
		return "", fmt.Errorf("unsupported scheme %q for WebSocket monitor; use ws:// or wss://", scheme)
	}
	return "wss://" + target, nil
}

// normalizeGRPC ensures a gRPC target is host:port without a scheme.
func normalizeGRPC(target string) (string, error) {
	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("gRPC target must be host:port without a scheme (e.g., example.com:443)")
	}
	_, _, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("gRPC target must be host:port (e.g., example.com:443): %w", err)
	}
	return target, nil
}

// normalizeICMP validates that the target is a hostname or IP without port or scheme.
// ICMP targets are raw IPs (v4/v6) or hostnames — no port, no scheme.
func normalizeICMP(target string) (string, error) {
	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("ICMP target must be a hostname or IP without a scheme (e.g., 8.8.8.8)")
	}

	// Reject targets with port (but allow IPv6 addresses like ::1).
	if _, _, err := net.SplitHostPort(target); err == nil {
		return "", fmt.Errorf("ICMP target must not include a port (e.g., 8.8.8.8 or example.com)")
	}

	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	return target, nil
}

// normalizeSMTP accepts a bare hostname or host:port format.
// If no port is present, it's kept bare (port comes from settings).
func normalizeSMTP(target string) (string, error) {
	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("SMTP target must be a hostname or host:port without a scheme (e.g., mail.example.com)")
	}

	// If it has a port, validate the format.
	if _, _, err := net.SplitHostPort(target); err == nil {
		// Valid host:port format — verify host is not empty.
		host, _, _ := net.SplitHostPort(target)
		if host == "" {
			return "", fmt.Errorf("SMTP target host must not be empty")
		}
		return target, nil
	}

	// Bare hostname — validate not empty.
	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	return target, nil
}

// normalizeDNS strips trailing dots and validates domain format.
// DNS targets must be bare domain names without scheme or port.
func normalizeDNS(target string) (string, error) {
	// Strip trailing dot (FQDN notation).
	target = strings.TrimSuffix(target, ".")

	// Reject any scheme prefix.
	if idx := strings.Index(target, "://"); idx > 0 {
		return "", fmt.Errorf("DNS target must be a domain name without a scheme (e.g., example.com)")
	}

	// Reject targets with port.
	if _, _, err := net.SplitHostPort(target); err == nil {
		return "", fmt.Errorf("DNS target must be a domain name without a port (e.g., example.com)")
	}

	// Basic domain validation: must have at least one dot or be a valid single-label.
	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	return target, nil
}
