package monitor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/version"
)

// HTTPSettings holds configuration for the HTTP/HTTPS checker.
// These fields are stored in the monitor's `settings` JSON column
// and configured at monitor creation time.
type HTTPSettings struct {
	// Method is the HTTP method to use (default: GET).
	Method string `json:"method,omitempty"`

	// ExpectedStatuses is an explicit list of acceptable HTTP status codes.
	// If set, it takes priority over ExpectedStatusMin/Max.
	// Example: [200, 201, 204, 301]
	ExpectedStatuses []int `json:"expected_statuses,omitempty"`

	// ExpectedStatusMin is the lower bound of acceptable status codes (default: 200).
	// Used only when ExpectedStatuses is empty.
	ExpectedStatusMin int `json:"expected_status_min,omitempty"`

	// ExpectedStatusMax is the upper bound of acceptable status codes (default: 399).
	// Used only when ExpectedStatuses is empty.
	ExpectedStatusMax int `json:"expected_status_max,omitempty"`

	// Headers are additional request headers to send.
	Headers map[string]string `json:"headers,omitempty"`

	// FollowRedirects controls whether HTTP redirects are followed.
	// When false (default), the checker reports the first response directly.
	FollowRedirects bool `json:"follow_redirects,omitempty"`

	// MaxRedirects limits the number of redirects to follow (default: 10).
	// Only applies when FollowRedirects is true.
	MaxRedirects int `json:"max_redirects,omitempty"`

	// SkipTLSVerify disables certificate chain and hostname verification (default: false).
	// When false, an invalid/expired cert causes the check to fail with state "down".
	SkipTLSVerify bool `json:"skip_tls_verify,omitempty"`

	// SSLExpiryThreshold is the minimum number of days before certificate expiry
	// that is considered acceptable. If the cert expires within this many days,
	// the check reports state "down" with an expiry warning.
	// Default: 0 (disabled — only reports days remaining without failing).
	SSLExpiryThreshold int `json:"ssl_expiry_threshold,omitempty"`

	// ValidateCertChain explicitly validates the full certificate chain
	// against system root CAs even when the HTTP request itself succeeds.
	// This catches certs that are technically valid but about to expire or
	// have chain issues. Default: true for HTTPS monitors.
	ValidateCertChain *bool `json:"validate_cert_chain,omitempty"`
}

// HTTPChecker implements the Checker interface for HTTP and HTTPS monitors.
type HTTPChecker struct{}

func (h *HTTPChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	return h.doCheck(ctx, target, settings, nil)
}

// CheckWithAuth performs an HTTP health check with credentials injected into
// the outbound request. All credentials are applied: bearer tokens and basic
// auth set the Authorization header, custom headers are added by name/value.
func (h *HTTPChecker) CheckWithAuth(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	return h.doCheck(ctx, target, settings, creds)
}

// doCheck is the shared implementation for Check and CheckWithAuth.
func (h *HTTPChecker) doCheck(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	s := parseHTTPSettings(settings)

	// Build transport with TLS configuration. We prefer to keep verification
	// enabled; when AllowSelfSigned is true we use a custom VerifyPeerCertificate
	// callback that accepts self-signed leaf certificates while still running
	// normal verification for other chains.
	tlsCfg := &tls.Config{
		InsecureSkipVerify: s.SkipTLSVerify,
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}

	client := &http.Client{Transport: transport}

	// Configure redirect policy.
	if !s.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else if s.MaxRedirects > 0 {
		maxRedirects := s.MaxRedirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		}
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, s.Method, target, nil)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("request build: %v", err)
		return result
	}

	// Set default User-Agent; allow per-monitor override via headers.
	req.Header.Set("User-Agent", version.UserAgent())
	for key, value := range s.Headers {
		req.Header.Set(key, value)
	}

	// Inject authentication credentials into the request.
	injectCredentials(req, creds)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	statusCode := int32(resp.StatusCode)
	result.StatusCode = &statusCode

	// --- TLS / Certificate validation ---
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		daysRemaining := int32(time.Until(leaf.NotAfter).Hours() / 24)
		result.SSLDaysRemaining = &daysRemaining

		// Check certificate chain validity if enabled.
		shouldValidateChain := s.ValidateCertChain == nil || *s.ValidateCertChain
		if shouldValidateChain && !s.SkipTLSVerify {
			if err := validateCertChain(resp.TLS.PeerCertificates, req.URL.Hostname()); err != nil {
				result.State = "down"
				result.Error = fmt.Sprintf("certificate validation: %v", err)
				return result
			}
		}

		// Check SSL expiry threshold.
		if s.SSLExpiryThreshold > 0 && int(daysRemaining) <= s.SSLExpiryThreshold {
			result.State = "down"
			result.Error = fmt.Sprintf("certificate expires in %d days (threshold: %d days)",
				daysRemaining, s.SSLExpiryThreshold)
			return result
		}
	}

	// --- Status code validation ---
	if isExpectedStatus(resp.StatusCode, s) {
		result.State = "up"
	} else {
		result.State = "down"
		result.Error = formatStatusError(resp.StatusCode, s)
	}

	return result
}

// injectCredentials applies all authentication credentials to an HTTP request.
// Bearer and Basic credentials set the Authorization header; custom header
// credentials add arbitrary headers by name and value.
func injectCredentials(req *http.Request, creds []AuthCredential) {
	for _, cred := range creds {
		switch cred.AuthType {
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+cred.Token)
		case "basic":
			encoded := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password))
			req.Header.Set("Authorization", "Basic "+encoded)
		case "header":
			req.Header.Set(cred.HeaderName, cred.HeaderValue)
		}
	}
}

// parseHTTPSettings unmarshals settings JSON and applies defaults.
func parseHTTPSettings(settings json.RawMessage) HTTPSettings {
	s := HTTPSettings{
		Method:            "GET",
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 399,
		MaxRedirects:      10,
	}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}
	if s.Method == "" {
		s.Method = "GET"
	}
	if s.ExpectedStatusMin == 0 && len(s.ExpectedStatuses) == 0 {
		s.ExpectedStatusMin = 200
	}
	if s.ExpectedStatusMax == 0 && len(s.ExpectedStatuses) == 0 {
		s.ExpectedStatusMax = 399
	}
	if s.MaxRedirects == 0 {
		s.MaxRedirects = 10
	}
	return s
}

// isExpectedStatus checks whether the response code is acceptable.
func isExpectedStatus(code int, s HTTPSettings) bool {
	// Explicit list takes priority.
	if len(s.ExpectedStatuses) > 0 {
		for _, expected := range s.ExpectedStatuses {
			if code == expected {
				return true
			}
		}
		return false
	}
	// Fallback to range.
	return code >= s.ExpectedStatusMin && code <= s.ExpectedStatusMax
}

// formatStatusError produces a human-readable error for unexpected status codes.
func formatStatusError(code int, s HTTPSettings) string {
	if len(s.ExpectedStatuses) > 0 {
		return fmt.Sprintf("unexpected status %d (expected one of %v)", code, s.ExpectedStatuses)
	}
	return fmt.Sprintf("unexpected status %d (expected %d-%d)",
		code, s.ExpectedStatusMin, s.ExpectedStatusMax)
}

// validateCertChain performs explicit certificate chain validation against system roots.
func validateCertChain(certs []*x509.Certificate, hostname string) error {
	if len(certs) == 0 {
		return fmt.Errorf("no certificates presented")
	}

	// Build intermediate pool from the chain (everything except the leaf).
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	opts := x509.VerifyOptions{
		DNSName:       hostname,
		Intermediates: intermediates,
		// Uses system root CAs by default when Roots is nil.
	}

	_, err := certs[0].Verify(opts)
	if err != nil {
		return fmt.Errorf("chain verification failed: %w", err)
	}
	return nil
}

// isSelfSigned heuristically checks whether a certificate is self-signed.
// (removed isSelfSigned helper — no longer accepting self-signed certs as a special case)
