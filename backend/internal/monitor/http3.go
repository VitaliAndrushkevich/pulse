package monitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/version"
	"github.com/quic-go/quic-go/http3"
)

// HTTP3Checker implements the Checker interface for HTTP/3 monitors.
// It reuses the same settings contract and auth injection logic as HTTPChecker.
type HTTP3Checker struct{}

func (h *HTTP3Checker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	return h.doCheck(ctx, target, settings, nil)
}

// CheckWithAuth performs an HTTP/3 health check with credentials injected into
// the outbound request.
func (h *HTTP3Checker) CheckWithAuth(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	return h.doCheck(ctx, target, settings, creds)
}

func (h *HTTP3Checker) doCheck(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	result := Result{CheckedAt: time.Now().UTC()}
	s := parseHTTPSettings(settings)

	tlsCfg := &tls.Config{InsecureSkipVerify: s.SkipTLSVerify}

	transport := &http3.Transport{TLSClientConfig: tlsCfg}
	defer func() { _ = transport.Close() }()
	client := &http.Client{Transport: transport}

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

	req, err := http.NewRequestWithContext(ctx, s.Method, target, nil)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("request build: %v", err)
		return result
	}

	req.Header.Set("User-Agent", version.UserAgent())
	for key, value := range s.Headers {
		req.Header.Set(key, value)
	}
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
	defer func() {
		_ = resp.Body.Close()
	}()

	statusCode := int32(resp.StatusCode)
	result.StatusCode = &statusCode

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		daysRemaining := int32(time.Until(leaf.NotAfter).Hours() / 24)
		result.SSLDaysRemaining = &daysRemaining

		shouldValidateChain := s.ValidateCertChain == nil || *s.ValidateCertChain
		if shouldValidateChain && !s.SkipTLSVerify {
			if err := validateCertChain(resp.TLS.PeerCertificates, req.URL.Hostname()); err != nil {
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

	if isExpectedStatus(resp.StatusCode, s) {
		result.State = "up"
	} else {
		result.State = "down"
		result.Error = formatStatusError(resp.StatusCode, s)
	}

	return result
}
