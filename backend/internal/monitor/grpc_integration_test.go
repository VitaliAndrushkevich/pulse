//go:build integration

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
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// =============================================================================
// Helper functions for integration tests
// =============================================================================

// intTestGenerateSelfSignedCert creates a self-signed TLS certificate valid for "localhost".
func intTestGenerateSelfSignedCert(t *testing.T, notAfter time.Time) tls.Certificate {
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
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to load key pair: %v", err)
	}
	return cert
}

// intTestGenerateCA creates a CA certificate and key that can sign server certs.
func intTestGenerateCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(100),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	return caCert, caKey, caCertPEM
}

// intTestGenerateServerCert creates a server certificate signed by the given CA.
func intTestGenerateServerCert(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, notAfter time.Time) tls.Certificate {
	t.Helper()
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(200),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("failed to marshal server key: %v", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	cert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("failed to load server key pair: %v", err)
	}
	return cert
}

// intTestStartPlaintextServer starts a plaintext gRPC server that returns a given status code.
func intTestStartPlaintextServer(t *testing.T, returnCode codes.Code) (addr string, stop func()) {
	t.Helper()
	encoding.RegisterCodec(rawCodec{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return status.Error(returnCode, "mock response")
		}),
	)
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// intTestStartTLSServer starts a TLS gRPC server with the given certificate that returns OK.
func intTestStartTLSServer(t *testing.T, cert tls.Certificate) (addr string, stop func()) {
	t.Helper()
	encoding.RegisterCodec(rawCodec{})
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return nil // OK
		}),
	)
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// intTestStartDelayedServer starts a plaintext gRPC server that waits for the given
// duration before responding.
func intTestStartDelayedServer(t *testing.T, delay time.Duration) (addr string, stop func()) {
	t.Helper()
	encoding.RegisterCodec(rawCodec{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			select {
			case <-time.After(delay):
			case <-stream.Context().Done():
				return status.Error(codes.DeadlineExceeded, "deadline exceeded")
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return nil
		}),
	)
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// intTestStartMetadataEchoServer starts a plaintext gRPC server that captures incoming
// metadata and sends it back in the response trailer. It also stores the last received
// metadata in the provided channel.
func intTestStartMetadataEchoServer(t *testing.T, receivedMD chan<- metadata.MD) (addr string, stop func()) {
	t.Helper()
	encoding.RegisterCodec(rawCodec{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(func(srv interface{}, stream grpc.ServerStream) error {
			// Extract incoming metadata from the stream context.
			md, ok := metadata.FromIncomingContext(stream.Context())
			if ok {
				receivedMD <- md
			} else {
				receivedMD <- metadata.MD{}
			}
			var req []byte
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			if err := stream.SendMsg([]byte{}); err != nil {
				return err
			}
			return nil
		}),
	)
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestIntegration_GRPCChecker_HealthCheck verifies that the GRPCChecker can perform
// a health check against a mock gRPC server using the default Health/Check method.
//
// Validates: Requirements 2.1, 2.2, 2.4
func TestIntegration_GRPCChecker_HealthCheck(t *testing.T) {
	// Start a plaintext mock server returning OK.
	addr, stop := intTestStartPlaintextServer(t, codes.OK)
	defer stop()

	checker := &GRPCChecker{}

	// No service_method → falls back to grpc.health.v1.Health/Check
	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", result.LatencyMs)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

// TestIntegration_GRPCChecker_CustomMethod verifies that the GRPCChecker can invoke
// a custom service/method against a mock server.
//
// Validates: Requirements 3.1, 3.4
func TestIntegration_GRPCChecker_CustomMethod(t *testing.T) {
	// Start a plaintext mock server returning OK.
	addr, stop := intTestStartPlaintextServer(t, codes.OK)
	defer stop()

	checker := &GRPCChecker{}

	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ServiceMethod:    "my.package.Service/Method",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", result.LatencyMs)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

// TestIntegration_GRPCChecker_TLSConnection verifies that the GRPCChecker can connect
// via TLS using tls_skip_verify with a self-signed certificate and correctly reports
// SSLDaysRemaining.
//
// Validates: Requirements 4.1, 4.3, 5.1, 5.2
func TestIntegration_GRPCChecker_TLSConnection(t *testing.T) {
	// Generate a CA and a server certificate signed by it.
	caCert, caKey, _ := intTestGenerateCA(t)
	notAfter := time.Now().Add(90 * 24 * time.Hour)
	serverCert := intTestGenerateServerCert(t, caCert, caKey, notAfter)

	// Start a TLS server with the CA-signed server cert.
	addr, stop := intTestStartTLSServer(t, serverCert)
	defer stop()

	checker := &GRPCChecker{}

	// Use tls_skip_verify since we cannot easily inject a CA into the system pool in tests.
	settings := GRPCSettings{
		TLSMode:          "tls_skip_verify",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}

	// SSLDaysRemaining should be set when using TLS.
	if result.SSLDaysRemaining == nil {
		t.Fatalf("expected SSLDaysRemaining to be set for TLS connection, got nil")
	}

	// Should be approximately 90 days (allow ±1 day tolerance).
	expectedDays := int32(time.Until(notAfter).Hours() / 24)
	diff := *result.SSLDaysRemaining - expectedDays
	if diff < -1 || diff > 1 {
		t.Fatalf("SSLDaysRemaining mismatch: got %d, expected ~%d", *result.SSLDaysRemaining, expectedDays)
	}
}

// TestIntegration_GRPCChecker_PlaintextConnection verifies that the GRPCChecker can
// connect to a plaintext gRPC server and that SSLDaysRemaining is not set.
//
// Validates: Requirements 4.2, 5.5
func TestIntegration_GRPCChecker_PlaintextConnection(t *testing.T) {
	addr, stop := intTestStartPlaintextServer(t, codes.OK)
	defer stop()

	checker := &GRPCChecker{}

	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}

	// SSLDaysRemaining must be nil for plaintext connections.
	if result.SSLDaysRemaining != nil {
		t.Fatalf("expected SSLDaysRemaining=nil for plaintext, got %d", *result.SSLDaysRemaining)
	}
}

// TestIntegration_GRPCChecker_TimeoutHandling verifies that the GRPCChecker correctly
// handles timeout when the server is slow to respond.
//
// Validates: Requirements 12.1, 12.4
func TestIntegration_GRPCChecker_TimeoutHandling(t *testing.T) {
	// Start a server with 5s delay.
	addr, stop := intTestStartDelayedServer(t, 5*time.Second)
	defer stop()

	checker := &GRPCChecker{}

	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	// Use a 100ms timeout context — far less than the 5s server delay.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := checker.Check(ctx, addr, settingsJSON)
	elapsed := time.Since(start)

	if result.State != "down" {
		t.Fatalf("expected state 'down' for timeout, got %q", result.State)
	}

	// Error should mention deadline or timeout.
	errLower := strings.ToLower(result.Error)
	if !strings.Contains(errLower, "deadline") && !strings.Contains(errLower, "timeout") {
		t.Fatalf("expected timeout-related error, got: %s", result.Error)
	}

	// Latency should be reported.
	if result.LatencyMs <= 0 {
		t.Errorf("expected positive latency, got %d", result.LatencyMs)
	}

	// The check should have returned quickly (within ~500ms), not waited for the full 5s.
	if elapsed > 2*time.Second {
		t.Fatalf("expected check to return quickly after timeout, but took %v", elapsed)
	}
}

// TestIntegration_GRPCChecker_MetadataPropagation verifies that the GRPCChecker sends
// configured metadata to the server.
//
// Validates: Requirements 6.1
func TestIntegration_GRPCChecker_MetadataPropagation(t *testing.T) {
	receivedMD := make(chan metadata.MD, 1)

	addr, stop := intTestStartMetadataEchoServer(t, receivedMD)
	defer stop()

	checker := &GRPCChecker{}

	expectedMetadata := map[string]string{
		"authorization": "Bearer test-token-123",
		"x-request-id":  "integration-test-001",
		"x-custom":      "custom-value",
	}

	settings := GRPCSettings{
		TLSMode:          "plaintext",
		Metadata:         expectedMetadata,
		ExpectedStatuses: []int{0},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}

	// Read the metadata that the server captured.
	select {
	case md := <-receivedMD:
		// Verify all expected metadata keys are present.
		for key, expectedVal := range expectedMetadata {
			values := md.Get(key)
			if len(values) == 0 {
				t.Errorf("expected metadata key %q to be present, but it was not found", key)
				continue
			}
			if values[0] != expectedVal {
				t.Errorf("metadata key %q: expected %q, got %q", key, expectedVal, values[0])
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for server to report received metadata")
	}
}

// TestIntegration_GRPCChecker_RegistryDispatch verifies the full end-to-end flow:
// create a DefaultRegistry, get the "grpc" checker, and call Check against a live mock server.
//
// Validates: Requirements 2.1, 2.2, 2.3
func TestIntegration_GRPCChecker_RegistryDispatch(t *testing.T) {
	// Start a plaintext mock server.
	addr, stop := intTestStartPlaintextServer(t, codes.OK)
	defer stop()

	// Create a DefaultRegistry and resolve the "grpc" checker.
	registry := DefaultRegistry(nil)
	checker, err := registry.Get("grpc")
	if err != nil {
		t.Fatalf("failed to get grpc checker from registry: %v", err)
	}

	// Build settings simulating what the scheduler would pass.
	settings := GRPCSettings{
		TLSMode:          "plaintext",
		ServiceMethod:    "my.service.Health/Check",
		ExpectedStatuses: []int{0},
		Metadata: map[string]string{
			"x-monitor-id": "test-monitor-123",
		},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	// Simulate the scheduler calling the checker with a timeout context.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checker.Check(ctx, addr, settingsJSON)

	// Verify full round-trip.
	if result.State != "up" {
		t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
	}
	if result.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", result.LatencyMs)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}

	// SSLDaysRemaining should be nil for plaintext.
	if result.SSLDaysRemaining != nil {
		t.Errorf("expected SSLDaysRemaining=nil for plaintext, got %d", *result.SSLDaysRemaining)
	}
}
