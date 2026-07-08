package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"pgregory.net/rapid"
)

// --- Helpers ---

// startQUICTestServer starts an HTTP/3 server with a self-signed cert for testing.
// Returns the server URL (https://host:port) and a cleanup function.
func startQUICTestServer(t *testing.T, handler http.Handler, certNotAfter time.Time) (string, func()) {
	t.Helper()

	cert := generateQUICTLSCert(t, certNotAfter)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	// Bind a UDP socket on a random port.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen UDP: %v", err)
	}
	addr := udpConn.LocalAddr().String()

	srv := &http3.Server{
		Handler:   handler,
		TLSConfig: tlsCfg,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(udpConn)
	}()

	// Give server a moment to start accepting connections.
	time.Sleep(20 * time.Millisecond)

	url := "https://" + addr
	cleanup := func() {
		_ = srv.Close()
		// Drain error channel.
		select {
		case <-errCh:
		case <-time.After(time.Second):
		}
	}

	return url, cleanup
}

func generateQUICTLSCert(t *testing.T, notAfter time.Time) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

// --- Unit Tests ---

func TestQUICChecker_Success_DefaultSettings(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("expected non-negative latency, got %d", result.LatencyMs)
	}
	if result.StatusCode == nil || *result.StatusCode != 200 {
		t.Fatalf("expected status code 200, got %v", result.StatusCode)
	}
}

func TestQUICChecker_Success_SSLDaysRemaining(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(45*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining == nil {
		t.Fatal("expected SSLDaysRemaining to be populated")
	}
	if *result.SSLDaysRemaining < 44 || *result.SSLDaysRemaining > 46 {
		t.Fatalf("expected ~45 SSL days remaining, got %d", *result.SSLDaysRemaining)
	}
}

func TestQUICChecker_SSLExpiryThreshold_Down(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Cert expires in 5 days, threshold is 10.
	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(5*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:             "GET",
		SkipTLSVerify:      true,
		SSLExpiryThreshold: 10,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on SSL expiry threshold, got %q", result.State)
	}
	if !strings.Contains(result.Error, "certificate expires") {
		t.Fatalf("expected cert expiry error, got: %s", result.Error)
	}
}

func TestQUICChecker_UnexpectedStatusCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on 503, got %q", result.State)
	}
	if result.StatusCode == nil || *result.StatusCode != 503 {
		t.Fatalf("expected status code 503, got %v", result.StatusCode)
	}
}

func TestQUICChecker_ExpectedStatuses_Custom(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted) // 202
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:           "GET",
		SkipTLSVerify:    true,
		ExpectedStatuses: []int{200, 202, 204},
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with custom expected statuses, got %q (error: %s)", result.State, result.Error)
	}
}

func TestQUICChecker_CustomHeaders(t *testing.T) {
	var capturedHeaders http.Header

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
		Headers: map[string]string{
			"X-Custom-Test": "quic-value",
			"Accept":        "application/json",
		},
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if capturedHeaders.Get("X-Custom-Test") != "quic-value" {
		t.Fatalf("expected custom header X-Custom-Test=quic-value, got %q", capturedHeaders.Get("X-Custom-Test"))
	}
	if capturedHeaders.Get("Accept") != "application/json" {
		t.Fatalf("expected Accept=application/json, got %q", capturedHeaders.Get("Accept"))
	}
}

func TestQUICChecker_UserAgentSet(t *testing.T) {
	var capturedUA string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if capturedUA == "" {
		t.Fatal("expected User-Agent header to be set")
	}
	if !strings.Contains(capturedUA, "Pulse") {
		t.Fatalf("expected User-Agent to contain 'Pulse', got %q", capturedUA)
	}
}

func TestQUICChecker_ContextTimeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, url, settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on timeout, got %q", result.State)
	}
}

func TestQUICChecker_InvalidURL(t *testing.T) {
	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{Method: "GET"})

	result := checker.Check(context.Background(), "://invalid-url", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on invalid URL, got %q", result.State)
	}
	if !strings.Contains(result.Error, "request build") {
		t.Fatalf("expected 'request build' error, got: %s", result.Error)
	}
}

func TestQUICChecker_UnreachableHost(t *testing.T) {
	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	// Use a non-routable address to trigger connection failure.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, "https://192.0.2.1:443/", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on unreachable host, got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error on unreachable host")
	}
}

func TestQUICChecker_NilSettings_NoPanic(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	// nil settings — should use defaults, no panic.
	result := checker.Check(context.Background(), url+"?skip_tls=1", nil)

	// Might be "down" because TLS verify fails with self-signed cert when no skip_tls,
	// but should not panic.
	_ = result
}

func TestQUICChecker_MalformedSettings_NoPanic(t *testing.T) {
	checker := &QUICChecker{}

	// Invalid JSON — should not panic.
	result := checker.Check(context.Background(), "https://127.0.0.1:1/test", json.RawMessage(`{invalid`))

	// Will be "down" (can't connect), but must not panic.
	_ = result
}

func TestQUICChecker_CheckedAtIsSet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	before := time.Now().UTC()
	result := checker.Check(context.Background(), url, settings)
	after := time.Now().UTC()

	if result.CheckedAt.Before(before) || result.CheckedAt.After(after) {
		t.Fatalf("CheckedAt %v not within [%v, %v]", result.CheckedAt, before, after)
	}
}

func TestQUICChecker_CheckWithAuth_BearerToken(t *testing.T) {
	var capturedAuth string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	creds := []AuthCredential{
		{AuthType: "bearer", Token: "test-token-123"},
	}

	result := checker.CheckWithAuth(context.Background(), url, settings, creds)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if capturedAuth != "Bearer test-token-123" {
		t.Fatalf("expected 'Bearer test-token-123', got %q", capturedAuth)
	}
}

func TestQUICChecker_CheckWithAuth_BasicAuth(t *testing.T) {
	var capturedAuth string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	creds := []AuthCredential{
		{AuthType: "basic", Username: "admin", Password: "secret"},
	}

	result := checker.CheckWithAuth(context.Background(), url, settings, creds)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if !strings.HasPrefix(capturedAuth, "Basic ") {
		t.Fatalf("expected Basic auth prefix, got %q", capturedAuth)
	}
}

func TestQUICChecker_CheckWithAuth_CustomHeader(t *testing.T) {
	var capturedAPIKey string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	creds := []AuthCredential{
		{AuthType: "header", HeaderName: "X-Api-Key", HeaderValue: "key-abc-123"},
	}

	result := checker.CheckWithAuth(context.Background(), url, settings, creds)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if capturedAPIKey != "key-abc-123" {
		t.Fatalf("expected X-Api-Key='key-abc-123', got %q", capturedAPIKey)
	}
}

func TestQUICChecker_MethodHonored(t *testing.T) {
	var capturedMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "HEAD",
		SkipTLSVerify: true,
	})

	result := checker.Check(context.Background(), url, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if capturedMethod != "HEAD" {
		t.Fatalf("expected method HEAD, got %q", capturedMethod)
	}
}

// --- Property Tests ---

// P-QUIC-1: Successful response with expected status always reports "up".
func TestProperty_QUIC_SuccessInvariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		statusCode := rapid.SampledFrom([]int{200, 201, 202, 204, 301, 302}).Draw(rt, "statusCode")

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(statusCode)
		})

		url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
		defer cleanup()

		checker := &QUICChecker{}
		settings, _ := json.Marshal(HTTPSettings{
			Method:            "GET",
			SkipTLSVerify:     true,
			ExpectedStatusMin: 200,
			ExpectedStatusMax: 399,
		})

		result := checker.Check(context.Background(), url, settings)

		if result.State != "up" {
			rt.Fatalf("expected 'up' for status %d, got %q (error: %s)", statusCode, result.State, result.Error)
		}
		if result.LatencyMs < 0 {
			rt.Fatalf("expected non-negative latency, got %d", result.LatencyMs)
		}
		if result.StatusCode == nil {
			rt.Fatal("expected StatusCode to be set")
		}
		if *result.StatusCode != int32(statusCode) {
			rt.Fatalf("expected status %d, got %d", statusCode, *result.StatusCode)
		}
		if result.Error != "" {
			rt.Fatalf("expected empty error on success, got: %s", result.Error)
		}
	})
}

// P-QUIC-2: Error status codes outside expected range always report "down".
func TestProperty_QUIC_UnexpectedStatus_Down(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		statusCode := rapid.SampledFrom([]int{400, 401, 403, 404, 500, 502, 503}).Draw(rt, "statusCode")

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(statusCode)
		})

		url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
		defer cleanup()

		checker := &QUICChecker{}
		settings, _ := json.Marshal(HTTPSettings{
			Method:        "GET",
			SkipTLSVerify: true,
			// Default range: 200-399
		})

		result := checker.Check(context.Background(), url, settings)

		if result.State != "down" {
			rt.Fatalf("expected 'down' for status %d, got %q", statusCode, result.State)
		}
		if result.StatusCode == nil || *result.StatusCode != int32(statusCode) {
			rt.Fatalf("expected status %d in result", statusCode)
		}
		if result.Error == "" {
			rt.Fatal("expected non-empty error for unexpected status")
		}
	})
}

// P-QUIC-3: Context deadline is always respected.
func TestProperty_QUIC_DeadlineRespected(t *testing.T) {
	// This property is expensive (real QUIC handshake + timeout per iteration).
	// Test with a few targeted timeout values instead of full property-based sweep.
	timeouts := []time.Duration{
		150 * time.Millisecond,
		250 * time.Millisecond,
		400 * time.Millisecond,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	url, cleanup := startQUICTestServer(t, handler, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(HTTPSettings{
		Method:        "GET",
		SkipTLSVerify: true,
	})

	for _, timeout := range timeouts {
		t.Run(timeout.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()
			result := checker.Check(ctx, url, settings)
			elapsed := time.Since(start)

			if result.State != "down" {
				t.Fatalf("expected 'down' on timeout, got %q", result.State)
			}

			// Allow 500ms grace for QUIC handshake overhead + scheduling.
			if elapsed > timeout+500*time.Millisecond {
				t.Fatalf("check took %v, deadline was %v (exceeded by >500ms)", elapsed, timeout)
			}
		})
	}
}
