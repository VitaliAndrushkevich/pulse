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
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockSMTPConn simulates an SMTP connection with configurable responses.
type mockSMTPConn struct {
	net.Conn
	greeting      string
	ehloResponse  string
	starttlsOk   bool
	tlsCert       *tls.Certificate
	tlsErr        error
	closed        bool
	readIdx       int
	conversation  []string // lines to return on read
	writtenLines  []string
	quitReceived  bool
}

func (m *mockSMTPConn) Read(b []byte) (int, error) {
	if m.readIdx >= len(m.conversation) {
		return 0, fmt.Errorf("no more data")
	}
	line := m.conversation[m.readIdx]
	m.readIdx++
	n := copy(b, []byte(line))
	return n, nil
}

func (m *mockSMTPConn) Write(b []byte) (int, error) {
	line := strings.TrimSpace(string(b))
	m.writtenLines = append(m.writtenLines, line)
	if strings.HasPrefix(line, "QUIT") {
		m.quitReceived = true
	}
	return len(b), nil
}

func (m *mockSMTPConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockSMTPConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockSMTPConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockSMTPConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockSMTPConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockSMTPConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }

// mockSMTPDialer implements SMTPDialer for testing.
type mockSMTPDialer struct {
	conn net.Conn
	err  error
}

func (d *mockSMTPDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return d.conn, d.err
}

// smtpMockServer is a higher-level helper that simulates the full SMTP conversation flow.
type smtpMockServer struct {
	greeting       string           // "220 mail.example.com ESMTP"
	ehloExtensions []string         // e.g., "250-STARTTLS", "250-SIZE 52428800"
	starttlsAvail  bool             // advertise STARTTLS
	tlsCert        *tls.Certificate // cert to present after STARTTLS upgrade
	tlsHandshakeOk bool
	connErr        error // error on dial
}

// buildConversation builds the conversation lines for mock reading.
func (s *smtpMockServer) buildConversation(ehloRequested bool, starttlsRequested bool) []string {
	var lines []string

	// Greeting
	lines = append(lines, s.greeting+"\r\n")

	// EHLO response
	if ehloRequested {
		lines = append(lines, "250-mail.example.com Hello\r\n")
		for _, ext := range s.ehloExtensions {
			lines = append(lines, ext+"\r\n")
		}
		lines = append(lines, "250 OK\r\n")
	}

	// STARTTLS response
	if starttlsRequested && s.starttlsAvail {
		lines = append(lines, "220 Ready to start TLS\r\n")
	}

	// QUIT response
	lines = append(lines, "221 Bye\r\n")

	return lines
}

// --- Unit Tests ---

func TestSMTPChecker_SuccessfulHandshake_NoTLS(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n",
			"221 Bye\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false, EHLODomain: "pulse.local"})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining != nil {
		t.Fatalf("expected SSLDaysRemaining nil with no TLS, got %d", *result.SSLDaysRemaining)
	}
}

func TestSMTPChecker_SuccessfulHandshake_WithSTARTTLS(t *testing.T) {
	cert := generateTestCert(t, time.Now().Add(90*24*time.Hour))

	conn := newTLSMockConn(cert, true)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: true, EHLODomain: "pulse.local"})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining == nil {
		t.Fatal("expected SSLDaysRemaining to be populated after STARTTLS")
	}
	if *result.SSLDaysRemaining <= 0 {
		t.Fatalf("expected positive SSLDaysRemaining, got %d", *result.SSLDaysRemaining)
	}
}

func TestSMTPChecker_ConnectionRefused(t *testing.T) {
	dialer := &mockSMTPDialer{err: fmt.Errorf("connection refused")}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on connection refused, got %q", result.State)
	}
	if !strings.HasPrefix(result.Error, "smtp:") {
		t.Fatalf("expected error prefix 'smtp:', got: %s", result.Error)
	}
}

func TestSMTPChecker_BadGreeting(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"421 Service unavailable\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on bad greeting, got %q", result.State)
	}
	if !strings.Contains(result.Error, "greeting") {
		t.Fatalf("expected error to mention 'greeting', got: %s", result.Error)
	}
}

func TestSMTPChecker_EHLOFailure(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"550 EHLO not accepted\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on EHLO failure, got %q", result.State)
	}
	if !strings.Contains(result.Error, "EHLO") {
		t.Fatalf("expected error to mention 'EHLO', got: %s", result.Error)
	}
}

func TestSMTPChecker_STARTTLSNotAdvertised(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n", // No STARTTLS extension
			"221 Bye\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: true})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' when STARTTLS not advertised, got %q", result.State)
	}
	if !strings.Contains(result.Error, "STARTTLS") {
		t.Fatalf("expected error to mention 'STARTTLS', got: %s", result.Error)
	}
}

func TestSMTPChecker_CertExpiryDetection(t *testing.T) {
	// Certificate expiring in 5 days, threshold is 10
	cert := generateTestCert(t, time.Now().Add(5*24*time.Hour))

	conn := newTLSMockConn(cert, true)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: true, SSLExpiryThreshold: 10})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' when cert expires within threshold, got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining == nil {
		t.Fatal("expected SSLDaysRemaining to be populated")
	}
}

func TestSMTPChecker_CertExpiryOK(t *testing.T) {
	// Certificate expiring in 60 days, threshold is 10
	cert := generateTestCert(t, time.Now().Add(60*24*time.Hour))

	conn := newTLSMockConn(cert, true)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: true, SSLExpiryThreshold: 10})
	result := checker.Check(context.Background(), "mail.example.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up' when cert not expiring, got %q (error: %s)", result.State, result.Error)
	}
}

func TestSMTPChecker_CustomEHLODomain(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n",
			"221 Bye\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false, EHLODomain: "custom.domain"})
	_ = checker.Check(context.Background(), "mail.example.com", settings)

	// Check that EHLO was sent with custom domain
	found := false
	for _, line := range conn.writtenLines {
		if strings.Contains(line, "EHLO custom.domain") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected EHLO with custom domain 'custom.domain', got writes: %v", conn.writtenLines)
	}
}

func TestSMTPChecker_ImplicitTLS_Success(t *testing.T) {
	cert := generateTestCert(t, time.Now().Add(90*24*time.Hour))

	conn := newImplicitTLSMockConn(cert)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 465, ImplicitTLS: true, EHLODomain: "pulse.local"})
	result := checker.Check(context.Background(), "smtp.gmail.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.SSLDaysRemaining == nil {
		t.Fatal("expected SSLDaysRemaining to be populated with implicit TLS")
	}
	if *result.SSLDaysRemaining <= 0 {
		t.Fatalf("expected positive SSLDaysRemaining, got %d", *result.SSLDaysRemaining)
	}
}

func TestSMTPChecker_ImplicitTLS_CertExpiry(t *testing.T) {
	cert := generateTestCert(t, time.Now().Add(3*24*time.Hour))

	conn := newImplicitTLSMockConn(cert)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 465, ImplicitTLS: true, SSLExpiryThreshold: 7})
	result := checker.Check(context.Background(), "smtp.gmail.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on cert expiry threshold, got %q", result.State)
	}
	if !strings.Contains(result.Error, "certificate expires") {
		t.Fatalf("expected cert expiry error, got: %s", result.Error)
	}
}

func TestSMTPChecker_ImplicitTLS_SkipsSTARTTLS(t *testing.T) {
	cert := generateTestCert(t, time.Now().Add(90*24*time.Hour))

	// Server does NOT advertise STARTTLS, but that's fine — implicit TLS skips it.
	conn := newImplicitTLSMockConn(cert)
	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	// Both StartTLS and ImplicitTLS set — ImplicitTLS takes precedence.
	settings, _ := json.Marshal(SMTPSettings{Port: 465, StartTLS: true, ImplicitTLS: true})
	result := checker.Check(context.Background(), "smtp.gmail.com", settings)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	// Verify STARTTLS command was never sent.
	for _, line := range conn.writtenLines {
		if strings.Contains(line, "STARTTLS") {
			t.Fatalf("STARTTLS should not be sent when implicit_tls is active, but found: %v", conn.writtenLines)
		}
	}
}

func TestSMTPChecker_ImplicitTLS_HandshakeFailure(t *testing.T) {
	// Connection that fails TLS handshake — use a mock that returns no cert.
	conn := &mockSMTPConn{
		conversation: []string{}, // Empty — handshake should fail before reading.
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	settings, _ := json.Marshal(SMTPSettings{Port: 465, ImplicitTLS: true})
	result := checker.Check(context.Background(), "smtp.gmail.com", settings)

	// Without GetPeerCert interface, it tries real TLS → fails on mock conn.
	// The mock conn doesn't implement GetPeerCert, so it attempts real TLS handshake,
	// which will fail. Let's verify the error path.
	if result.State != "down" {
		t.Fatalf("expected state 'down' on TLS handshake failure, got %q", result.State)
	}
	if !strings.Contains(result.Error, "implicit TLS handshake failed") {
		t.Fatalf("expected implicit TLS handshake error, got: %s", result.Error)
	}
}

func TestSMTPChecker_ContextCancellation(t *testing.T) {
	dialer := &mockSMTPDialer{err: context.Canceled}
	checker := &SMTPChecker{dialer: dialer}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false})
	result := checker.Check(ctx, "mail.example.com", settings)

	if result.State != "down" {
		t.Fatalf("expected state 'down' on context cancellation, got %q", result.State)
	}
}

func TestSMTPChecker_MalformedSettings_NoPanic(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n",
			"221 Bye\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	result := checker.Check(context.Background(), "mail.example.com", json.RawMessage(`{invalid`))

	// With defaults (starttls=true), but STARTTLS not advertised → down
	// OR defaults handle it gracefully. The important thing is no panic.
	_ = result
}

func TestSMTPChecker_NilSettings_NoPanic(t *testing.T) {
	conn := &mockSMTPConn{
		conversation: []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n",
			"221 Bye\r\n",
		},
	}

	dialer := &mockSMTPDialer{conn: conn}
	checker := &SMTPChecker{dialer: dialer}
	result := checker.Check(context.Background(), "mail.example.com", nil)

	// Should not panic regardless of outcome
	_ = result
}

// --- Property Tests ---

// P-SMTP-1: context deadline always respected.
func TestProperty_SMTP_DeadlineRespected(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		timeoutMs := rapid.IntRange(50, 500).Draw(rt, "timeoutMs")
		timeout := time.Duration(timeoutMs) * time.Millisecond

		// Dialer that blocks until context expires
		dialer := &blockingDialer{}

		checker := &SMTPChecker{dialer: dialer}
		settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: false})

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		start := time.Now()
		result := checker.Check(ctx, "mail.example.com", settings)
		elapsed := time.Since(start)

		if result.State != "down" {
			rt.Fatalf("expected 'down' on timeout, got %q", result.State)
		}

		// Allow 200ms grace for scheduling
		if elapsed > timeout+200*time.Millisecond {
			rt.Fatalf("check took %v, deadline was %v (exceeded by >200ms)", elapsed, timeout)
		}
	})
}

// blockingDialer blocks until context is done.
type blockingDialer struct{}

func (d *blockingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// P-SMTP-2: starttls=false → SSLDaysRemaining is never populated.
func TestProperty_SMTP_NoTLS_NoSSLDays(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		port := rapid.IntRange(1, 65535).Draw(rt, "port")

		conn := &mockSMTPConn{
			conversation: []string{
				"220 mail.example.com ESMTP\r\n",
				"250-mail.example.com\r\n250 OK\r\n",
				"221 Bye\r\n",
			},
		}

		dialer := &mockSMTPDialer{conn: conn}
		checker := &SMTPChecker{dialer: dialer}
		settings, _ := json.Marshal(SMTPSettings{Port: port, StartTLS: false})
		result := checker.Check(context.Background(), "mail.example.com", settings)

		if result.SSLDaysRemaining != nil {
			rt.Fatalf("SSLDaysRemaining must be nil when starttls=false, got %d", *result.SSLDaysRemaining)
		}
	})
}

// P-SMTP-3: starttls=true → SSLDaysRemaining populated OR Error non-empty.
func TestProperty_SMTP_TLS_DaysOrError(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		advertiseStartTLS := rapid.Bool().Draw(rt, "advertiseStartTLS")

		var conversation []string
		if advertiseStartTLS {
			conversation = []string{
				"220 mail.example.com ESMTP\r\n",
				"250-mail.example.com\r\n250-STARTTLS\r\n250 OK\r\n",
				"220 Ready to start TLS\r\n",
				"221 Bye\r\n",
			}
		} else {
			conversation = []string{
				"220 mail.example.com ESMTP\r\n",
				"250-mail.example.com\r\n250 OK\r\n", // No STARTTLS
				"221 Bye\r\n",
			}
		}

		conn := &mockSMTPConn{conversation: conversation}
		dialer := &mockSMTPDialer{conn: conn}
		checker := &SMTPChecker{dialer: dialer}
		settings, _ := json.Marshal(SMTPSettings{Port: 25, StartTLS: true})
		result := checker.Check(context.Background(), "mail.example.com", settings)

		// When STARTTLS requested: either SSLDaysRemaining is set OR error is non-empty
		if result.SSLDaysRemaining == nil && result.Error == "" {
			rt.Fatalf("starttls=true: expected SSLDaysRemaining or Error, got neither (state: %s)", result.State)
		}
	})
}

// --- Test Helpers ---

// generateTestCert creates a self-signed certificate expiring at the given time.
func generateTestCert(t *testing.T, notAfter time.Time) *tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"test.example.com", "mail.example.com"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

// tlsMockConn simulates a connection that supports STARTTLS upgrade.
type tlsMockConn struct {
	net.Conn
	conversation []string
	readIdx      int
	writtenLines []string
	cert         *tls.Certificate
	tlsUpgraded bool
	closed       bool
}

func newTLSMockConn(cert *tls.Certificate, starttlsAvail bool) *tlsMockConn {
	var conv []string
	if starttlsAvail {
		conv = []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250-STARTTLS\r\n250 OK\r\n",
			"220 Ready to start TLS\r\n",
			"221 Bye\r\n",
		}
	} else {
		conv = []string{
			"220 mail.example.com ESMTP\r\n",
			"250-mail.example.com\r\n250 OK\r\n",
			"221 Bye\r\n",
		}
	}
	return &tlsMockConn{
		conversation: conv,
		cert:         cert,
	}
}

func (c *tlsMockConn) Read(b []byte) (int, error) {
	if c.readIdx >= len(c.conversation) {
		return 0, fmt.Errorf("no more data")
	}
	line := c.conversation[c.readIdx]
	c.readIdx++
	n := copy(b, []byte(line))
	return n, nil
}

func (c *tlsMockConn) Write(b []byte) (int, error) {
	line := strings.TrimSpace(string(b))
	c.writtenLines = append(c.writtenLines, line)
	return len(b), nil
}

func (c *tlsMockConn) Close() error {
	c.closed = true
	return nil
}

func (c *tlsMockConn) SetDeadline(t time.Time) error      { return nil }
func (c *tlsMockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *tlsMockConn) SetWriteDeadline(t time.Time) error { return nil }
func (c *tlsMockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *tlsMockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }

// GetPeerCert returns the certificate for TLS inspection.
func (c *tlsMockConn) GetPeerCert() *x509.Certificate {
	if c.cert != nil && c.cert.Leaf != nil {
		return c.cert.Leaf
	}
	return nil
}

// newImplicitTLSMockConn simulates a connection where TLS is established before
// the SMTP greeting (implicit TLS / SMTPS on port 465).
// It implements GetPeerCert so the checker skips real TLS handshake in tests.
func newImplicitTLSMockConn(cert *tls.Certificate) *tlsMockConn {
	// After implicit TLS handshake, server sends greeting over encrypted channel.
	conv := []string{
		"220 smtp.gmail.com ESMTP ready\r\n",
		"250-smtp.gmail.com\r\n250 OK\r\n",
		"221 Bye\r\n",
	}
	return &tlsMockConn{
		conversation: conv,
		cert:         cert,
	}
}
