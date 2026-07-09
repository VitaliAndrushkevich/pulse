package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
)

// QUICSettings holds configuration for a QUIC connection check.
type QUICSettings struct {
	// SkipTLSVerify disables certificate verification (user-controlled).
	SkipTLSVerify bool `json:"skip_tls_verify"`
	// SSLExpiryThreshold marks the monitor as down when the certificate
	// expires within this many days (0 = disabled).
	SSLExpiryThreshold int `json:"ssl_expiry_threshold"`
	// ValidateCertChain enables full chain validation (default: true).
	ValidateCertChain *bool `json:"validate_cert_chain"`
	// ALPN protocols to advertise during the TLS handshake.
	// Defaults to ["h3"] if empty.
	NextProtos []string `json:"next_protos"`
}

func parseQUICSettings(raw json.RawMessage) QUICSettings {
	var s QUICSettings
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	if len(s.NextProtos) == 0 {
		s.NextProtos = []string{"h3"}
	}
	return s
}

// QUICChecker implements the Checker interface for QUIC monitors.
// It establishes a raw QUIC connection (TLS + QUIC handshake) using quic-go's
// DialAddr to verify that the target is reachable over QUIC/UDP.
type QUICChecker struct{}

// Check executes a single QUIC health check against the given target.
func (q *QUICChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	return q.doCheck(ctx, target, settings)
}

// CheckWithAuth performs a QUIC health check. Credentials are not applicable
// to raw QUIC connections, so they are ignored.
func (q *QUICChecker) CheckWithAuth(ctx context.Context, target string, settings json.RawMessage, _ []AuthCredential) Result {
	return q.doCheck(ctx, target, settings)
}

func (q *QUICChecker) doCheck(ctx context.Context, target string, settings json.RawMessage) Result {
	result := Result{CheckedAt: time.Now().UTC()}
	s := parseQUICSettings(settings)

	addr, serverName, err := resolveQUICAddr(target)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("invalid target: %v", err)
		return result
	}

	tlsCfg := &tls.Config{ //nolint:gosec // skip_tls_verify is user-controlled
		InsecureSkipVerify: s.SkipTLSVerify,
		NextProtos:         s.NextProtos,
		ServerName:         serverName,
	}

	quicCfg := &quic.Config{
		// HandshakeIdleTimeout is controlled by the context deadline.
	}

	start := time.Now()
	conn, err := quic.DialAddr(ctx, addr, tlsCfg, quicCfg)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("quic dial: %v", err)
		return result
	}
	defer func() { _ = conn.CloseWithError(0, "check done") }()

	// Extract TLS state for certificate checks.
	tlsState := conn.ConnectionState().TLS
	if len(tlsState.PeerCertificates) > 0 {
		leaf := tlsState.PeerCertificates[0]
		daysRemaining := int32(time.Until(leaf.NotAfter).Hours() / 24)
		result.SSLDaysRemaining = &daysRemaining

		shouldValidateChain := s.ValidateCertChain == nil || *s.ValidateCertChain
		if shouldValidateChain && !s.SkipTLSVerify {
			if err := validateCertChain(tlsState.PeerCertificates, serverName); err != nil {
				result.State = "down"
				result.Error = fmt.Sprintf("certificate validation: %v", err)
				return result
			}
		}

		if s.SSLExpiryThreshold > 0 && int(daysRemaining) <= s.SSLExpiryThreshold {
			result.State = "down"
			result.Error = fmt.Sprintf("certificate expires in %d days (threshold: %d days)",
				daysRemaining, s.SSLExpiryThreshold)
			return result
		}
	}

	result.State = "up"
	return result
}

// resolveQUICAddr parses the target into a host:port address and extracts the
// server name for TLS SNI. Accepts formats:
//   - host:port           (e.g. "example.com:443")
//   - https://host[:port] (port defaults to 443)
//   - host                (port defaults to 443)
func resolveQUICAddr(target string) (addr, serverName string, err error) {
	// Try parsing as URL first.
	if u, parseErr := url.Parse(target); parseErr == nil && u.Host != "" {
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return net.JoinHostPort(host, port), host, nil
	}

	// Try as host:port.
	host, port, splitErr := net.SplitHostPort(target)
	if splitErr == nil {
		return net.JoinHostPort(host, port), host, nil
	}

	// Bare hostname — default to port 443.
	if target == "" {
		return "", "", fmt.Errorf("empty target")
	}
	return net.JoinHostPort(target, "443"), target, nil
}
