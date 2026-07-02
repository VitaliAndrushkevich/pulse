package monitor

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// SMTPDialer abstracts TCP dialing for testability.
type SMTPDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// SMTPSettings holds configuration for the SMTP checker.
type SMTPSettings struct {
	Port               int    `json:"port"`                 // default: 25
	StartTLS           bool   `json:"starttls"`             // default: true
	EHLODomain         string `json:"ehlo_domain"`          // default: "pulse.local"
	SSLExpiryThreshold int    `json:"ssl_expiry_threshold"` // default: 0 (disabled)
}

// SMTPChecker implements the Checker interface for SMTP monitors.
type SMTPChecker struct {
	dialer SMTPDialer
}

// Check executes an SMTP handshake check against the given target.
func (c *SMTPChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	start := time.Now()
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	s := parseSMTPSettings(settings)

	// Build address with port.
	address := target
	if !strings.Contains(target, ":") {
		address = fmt.Sprintf("%s:%d", target, s.Port)
	}

	// Get or create dialer.
	dialer := c.dialer
	if dialer == nil {
		dialer = &defaultSMTPDialer{}
	}

	// Connect.
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: connection failed: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}
	defer conn.Close()

	// Create buffered reader for the connection.
	reader := bufio.NewReader(conn)

	// Read greeting (220).
	greeting, err := readSMTPResponse(reader)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: greeting read error: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	if !strings.HasPrefix(greeting, "220") {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: bad greeting: %s", strings.TrimSpace(greeting))
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	// Send EHLO.
	_, err = fmt.Fprintf(conn, "EHLO %s\r\n", s.EHLODomain)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: EHLO write error: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	ehloResponse, err := readSMTPResponse(reader)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: EHLO response error: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	if !strings.HasPrefix(ehloResponse, "250") {
		result.State = "down"
		result.Error = fmt.Sprintf("smtp: EHLO rejected: %s", strings.TrimSpace(ehloResponse))
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	// Check STARTTLS.
	if s.StartTLS {
		// Verify STARTTLS is advertised.
		if !strings.Contains(strings.ToUpper(ehloResponse), "STARTTLS") {
			result.State = "down"
			result.Error = "smtp: STARTTLS requested but not advertised by server"
			result.LatencyMs = int32(time.Since(start).Milliseconds())
			return result
		}

		// Send STARTTLS command.
		_, err = fmt.Fprintf(conn, "STARTTLS\r\n")
		if err != nil {
			result.State = "down"
			result.Error = fmt.Sprintf("smtp: STARTTLS write error: %v", err)
			result.LatencyMs = int32(time.Since(start).Milliseconds())
			return result
		}

		starttlsResp, err := readSMTPResponse(reader)
		if err != nil {
			result.State = "down"
			result.Error = fmt.Sprintf("smtp: STARTTLS response error: %v", err)
			result.LatencyMs = int32(time.Since(start).Milliseconds())
			return result
		}

		if !strings.HasPrefix(starttlsResp, "220") {
			result.State = "down"
			result.Error = fmt.Sprintf("smtp: STARTTLS rejected: %s", strings.TrimSpace(starttlsResp))
			result.LatencyMs = int32(time.Since(start).Milliseconds())
			return result
		}

		// Upgrade to TLS.
		var peerCert *x509.Certificate

		// Check if conn supports GetPeerCert (mock testing).
		if certProvider, ok := conn.(interface{ GetPeerCert() *x509.Certificate }); ok {
			peerCert = certProvider.GetPeerCert()
		} else {
			// Real TLS upgrade.
			tlsConn := tls.Client(conn, &tls.Config{
				ServerName:         extractHost(address),
				InsecureSkipVerify: true, //nolint:gosec // We inspect certs ourselves
			})

			if err := tlsConn.HandshakeContext(ctx); err != nil {
				result.State = "down"
				result.Error = fmt.Sprintf("smtp: TLS handshake failed: %v", err)
				result.LatencyMs = int32(time.Since(start).Milliseconds())
				return result
			}

			// Get peer certificate.
			state := tlsConn.ConnectionState()
			if len(state.PeerCertificates) > 0 {
				peerCert = state.PeerCertificates[0]
			}
		}

		// Inspect certificate.
		if peerCert != nil {
			daysRemaining := int32(time.Until(peerCert.NotAfter).Hours() / 24)
			result.SSLDaysRemaining = &daysRemaining

			// Check expiry threshold.
			if s.SSLExpiryThreshold > 0 && int(daysRemaining) <= s.SSLExpiryThreshold {
				result.State = "down"
				result.Error = fmt.Sprintf("smtp: certificate expires in %d days (threshold: %d days)", daysRemaining, s.SSLExpiryThreshold)
				result.LatencyMs = int32(time.Since(start).Milliseconds())
				return result
			}
		}
	}

	// Send QUIT.
	_, _ = fmt.Fprintf(conn, "QUIT\r\n")

	result.State = "up"
	result.LatencyMs = int32(time.Since(start).Milliseconds())
	return result
}

// parseSMTPSettings unmarshals settings JSON and applies defaults.
func parseSMTPSettings(settings json.RawMessage) SMTPSettings {
	s := SMTPSettings{}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}

	if s.Port <= 0 || s.Port > 65535 {
		s.Port = 25
	}
	if s.EHLODomain == "" {
		s.EHLODomain = "pulse.local"
	}
	// StartTLS defaults to true for zero-value (bool default false),
	// but we keep it explicit via JSON. If settings are nil/malformed,
	// the zero value false is fine — user must opt-in.

	return s
}

// readSMTPResponse reads a full SMTP response (may be multi-line).
func readSMTPResponse(reader *bufio.Reader) (string, error) {
	var response strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Return what we have if partial.
			if response.Len() > 0 {
				return response.String(), nil
			}
			return "", err
		}
		response.WriteString(line)

		// SMTP multi-line responses use "XXX-" continuation, "XXX " for final.
		if len(line) >= 4 && line[3] == ' ' {
			break
		}
		// Also break if it's a short line (malformed but we handle gracefully).
		if len(line) < 4 {
			break
		}
		// Continue if line[3] == '-' (multi-line).
	}
	return response.String(), nil
}

// extractHost extracts the host portion from a host:port address.
func extractHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}

// --- Production Dialer ---

type defaultSMTPDialer struct{}

func (d *defaultSMTPDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}
