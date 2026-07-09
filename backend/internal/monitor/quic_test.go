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
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// --- Helpers ---

// startQUICTestServer starts a raw QUIC listener with a self-signed cert for testing.
// Returns the address (host:port) and a cleanup function.
func startQUICTestServer(t *testing.T, certNotAfter time.Time) (string, func()) {
	t.Helper()

	cert := generateQUICTLSCert(t, certNotAfter)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	listener, err := quic.ListenAddr("127.0.0.1:0", tlsCfg, nil)
	if err != nil {
		t.Fatalf("failed to listen QUIC: %v", err)
	}
	addr := listener.Addr().String()

	// Accept connections in the background (just accept and hold them open).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				return
			}
			// Hold connection open until cleanup.
			go func(c *quic.Conn) {
				<-c.Context().Done()
			}(conn)
		}
	}()

	cleanup := func() {
		_ = listener.Close()
		<-done
	}

	return addr, cleanup
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
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("expected non-negative latency, got %d", result.LatencyMs)
	}
}

func TestQUICChecker_Success_SSLDaysRemaining(t *testing.T) {
	addr, cleanup := startQUICTestServer(t, time.Now().Add(45*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settings)

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
	// Cert expires in 5 days, threshold is 10.
	addr, cleanup := startQUICTestServer(t, time.Now().Add(5*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify:      true,
		SSLExpiryThreshold: 10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on SSL expiry threshold, got %q", result.State)
	}
	if !strings.Contains(result.Error, "certificate expires") {
		t.Fatalf("expected cert expiry error, got: %s", result.Error)
	}
}

func TestQUICChecker_ContextTimeout(t *testing.T) {
	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	// Use a non-routable address to force timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, "192.0.2.1:443", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on timeout, got %q", result.State)
	}
}

func TestQUICChecker_InvalidTarget(t *testing.T) {
	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{})

	result := checker.Check(context.Background(), "", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on empty target, got %q", result.State)
	}
	if !strings.Contains(result.Error, "invalid target") {
		t.Fatalf("expected 'invalid target' error, got: %s", result.Error)
	}
}

func TestQUICChecker_UnreachableHost(t *testing.T) {
	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := checker.Check(ctx, "192.0.2.1:443", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on unreachable host, got %q", result.State)
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error on unreachable host")
	}
}

func TestQUICChecker_NilSettings_NoPanic(t *testing.T) {
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// nil settings — should use defaults (NextProtos=["h3"]), no panic.
	result := checker.Check(ctx, addr, nil)
	// Might be "down" because TLS verify fails with self-signed cert,
	// but should not panic.
	_ = result
}

func TestQUICChecker_MalformedSettings_NoPanic(t *testing.T) {
	checker := &QUICChecker{}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Invalid JSON — should not panic.
	result := checker.Check(ctx, "127.0.0.1:1", json.RawMessage(`{invalid`))

	// Will be "down" (can't connect), but must not panic.
	_ = result
}

func TestQUICChecker_CheckedAtIsSet(t *testing.T) {
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := time.Now().UTC()
	result := checker.Check(ctx, addr, settings)
	after := time.Now().UTC()

	if result.CheckedAt.Before(before) || result.CheckedAt.After(after) {
		t.Fatalf("CheckedAt %v not within [%v, %v]", result.CheckedAt, before, after)
	}
}

func TestQUICChecker_CheckWithAuth_Ignored(t *testing.T) {
	// Auth credentials are not applicable to raw QUIC; verify they don't cause errors.
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	creds := []AuthCredential{
		{AuthType: "bearer", Token: "test-token-123"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.CheckWithAuth(ctx, addr, settings, creds)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
}

func TestQUICChecker_URLTarget(t *testing.T) {
	// Verify that https://host:port URL format is correctly parsed.
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pass as URL instead of bare host:port.
	result := checker.Check(ctx, "https://"+addr, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with URL target, got %q (error: %s)", result.State, result.Error)
	}
}

func TestQUICChecker_CustomALPN(t *testing.T) {
	// Server advertises custom ALPN.
	cert := generateQUICTLSCert(t, time.Now().Add(90*24*time.Hour))
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"custom-proto"},
	}

	listener, err := quic.ListenAddr("127.0.0.1:0", tlsCfg, nil)
	if err != nil {
		t.Fatalf("failed to listen QUIC: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				return
			}
			go func(c *quic.Conn) { <-c.Context().Done() }(conn)
		}
	}()

	addr := listener.Addr().String()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
		NextProtos:    []string{"custom-proto"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' with custom ALPN, got %q (error: %s)", result.State, result.Error)
	}
}

// --- Property Tests ---

// P-QUIC-1: Successful QUIC dial always reports "up" with non-negative latency.
func TestProperty_QUIC_SuccessInvariants(t *testing.T) {
	addr, cleanup := startQUICTestServer(t, time.Now().Add(90*24*time.Hour))
	defer cleanup()

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	// Run multiple iterations to confirm invariants hold.
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := checker.Check(ctx, addr, settings)
		cancel()

		if result.State != "up" {
			t.Fatalf("iteration %d: expected 'up', got %q (error: %s)", i, result.State, result.Error)
		}
		if result.LatencyMs < 0 {
			t.Fatalf("iteration %d: expected non-negative latency, got %d", i, result.LatencyMs)
		}
		if result.Error != "" {
			t.Fatalf("iteration %d: expected empty error on success, got: %s", i, result.Error)
		}
	}
}

// P-QUIC-2: Context deadline is always respected.
func TestProperty_QUIC_DeadlineRespected(t *testing.T) {
	timeouts := []time.Duration{
		200 * time.Millisecond,
		300 * time.Millisecond,
		500 * time.Millisecond,
	}

	checker := &QUICChecker{}
	settings, _ := json.Marshal(QUICSettings{
		SkipTLSVerify: true,
	})

	for _, timeout := range timeouts {
		t.Run(timeout.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()
			// Non-routable address ensures we always hit the deadline.
			result := checker.Check(ctx, "192.0.2.1:443", settings)
			elapsed := time.Since(start)

			if result.State != "down" {
				t.Fatalf("expected 'down' on timeout, got %q", result.State)
			}

			// Allow 500ms grace for scheduling jitter.
			if elapsed > timeout+500*time.Millisecond {
				t.Fatalf("check took %v, deadline was %v (exceeded by >500ms)", elapsed, timeout)
			}
		})
	}
}
