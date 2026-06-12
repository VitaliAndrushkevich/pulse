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
func Normalize(monitorType, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}

	switch monitorType {
	case "http", "http3":
		return normalizeHTTP(target)
	case "tcp":
		return normalizeTCP(target)
	case "udp":
		return normalizeUDP(target)
	case "websocket":
		return normalizeWebSocket(target)
	case "grpc":
		return normalizeGRPC(target)
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
