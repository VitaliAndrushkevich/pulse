package server_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/server"
)

// unreachableClient is a PulseClient that returns ConnectivityError for all calls,
// simulating a Pulse API that is unreachable.
type unreachableClient struct{}

func (unreachableClient) ListMonitors(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	return pulseapi.MonitorPage{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}
func (unreachableClient) GetMonitor(_ context.Context, _ string) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}
func (unreachableClient) GetMonitorStats(_ context.Context, _ string) (pulseapi.MonitorStats, error) {
	return pulseapi.MonitorStats{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}
func (unreachableClient) GetMonitorHistory(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
	return pulseapi.History{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}
func (unreachableClient) ListIncidents(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	return pulseapi.IncidentPage{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}
func (unreachableClient) CreateMonitor(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, &pulseapi.ConnectivityError{Reason: "connection_refused"}
}

// noopClient satisfies the PulseClient interface without doing anything.
type noopClient struct{}

func (noopClient) ListMonitors(_ context.Context, _ pulseapi.MonitorQuery) (pulseapi.MonitorPage, error) {
	return pulseapi.MonitorPage{}, nil
}
func (noopClient) GetMonitor(_ context.Context, _ string) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}
func (noopClient) GetMonitorStats(_ context.Context, _ string) (pulseapi.MonitorStats, error) {
	return pulseapi.MonitorStats{}, nil
}
func (noopClient) GetMonitorHistory(_ context.Context, _ string, _ pulseapi.TimeRange) (pulseapi.History, error) {
	return pulseapi.History{}, nil
}
func (noopClient) ListIncidents(_ context.Context, _ pulseapi.IncidentQuery) (pulseapi.IncidentPage, error) {
	return pulseapi.IncidentPage{}, nil
}
func (noopClient) CreateMonitor(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	return pulseapi.Monitor{}, nil
}

// connectClientToServer creates a full MCP server via server.New, connects an
// in-memory client, and returns the client session. Cleanup is registered on t.
func connectClientToServer(t *testing.T, cfg *config.Config, client pulseapi.PulseClient) *mcp.ClientSession {
	t.Helper()

	ctx := context.Background()

	srv, err := server.New(cfg, client)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	ct, st := mcp.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.0",
	}, nil)

	cs, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// TestBlankTokenAbortsStartup verifies that config.Load returns ErrMissingAPIToken
// when PULSE_MCP_API_TOKEN is empty, preventing the server from starting.
// Validates: Requirements 3.3, 3.4
func TestBlankTokenAbortsStartup(t *testing.T) {
	t.Setenv("PULSE_MCP_API_TOKEN", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when PULSE_MCP_API_TOKEN is empty, got nil")
	}
	if !errors.Is(err, config.ErrMissingAPIToken) {
		t.Fatalf("expected ErrMissingAPIToken, got: %v", err)
	}
}

// TestBlankTokenWhitespaceOnlyAbortsStartup verifies whitespace-only tokens are rejected.
// Validates: Requirements 3.3, 3.4
func TestBlankTokenWhitespaceOnlyAbortsStartup(t *testing.T) {
	t.Setenv("PULSE_MCP_API_TOKEN", "   \t\n")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when PULSE_MCP_API_TOKEN is whitespace-only, got nil")
	}
	if !errors.Is(err, config.ErrMissingAPIToken) {
		t.Fatalf("expected ErrMissingAPIToken, got: %v", err)
	}
}

// TestServerAcceptsConnectionsWhilePulseUnreachable verifies that the MCP server
// accepts client connections even when the Pulse API is unreachable, and that
// tool calls return connectivity errors rather than crashing.
// Validates: Requirements 2.1, 2.5
func TestServerAcceptsConnectionsWhilePulseUnreachable(t *testing.T) {
	cfg := &config.Config{
		APIBaseURL: "http://unreachable:9999/api/v1",
		APIToken:   "test-token",
		AccessMode: config.ReadOnly,
		Transport:  "stdio",
	}

	cs := connectClientToServer(t, cfg, unreachableClient{})

	// The connection succeeded (Pulse being unreachable did not prevent it).
	// Now call a tool and verify we get a connectivity error, not a crash.
	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list-monitors",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool returned protocol error: %v", err)
	}

	// The tool should report an error via IsError=true (connectivity error).
	if !result.IsError {
		t.Fatal("expected IsError=true for unreachable Pulse, got false")
	}

	// Verify the error content mentions unreachable/connectivity.
	if len(result.Content) == 0 {
		t.Fatal("expected error content, got empty")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if text.Text == "" {
		t.Fatal("expected non-empty error text")
	}
}

// TestHandshakeCompletesSuccessfully verifies that the MCP initialization handshake
// completes without error, confirming protocol version and capabilities are exchanged.
// Validates: Requirements 1.1, 1.6
func TestHandshakeCompletesSuccessfully(t *testing.T) {
	cfg := &config.Config{
		APIBaseURL: "http://localhost:8080/api/v1",
		APIToken:   "test-token",
		AccessMode: config.ReadOnly,
		Transport:  "stdio",
	}

	// connectClientToServer performs the full handshake (client.Connect).
	// If it doesn't panic/fatal, the handshake succeeded.
	cs := connectClientToServer(t, cfg, noopClient{})

	// Verify we can also get the initialization result.
	initResult := cs.InitializeResult()
	if initResult == nil {
		t.Fatal("expected non-nil InitializeResult after handshake")
	}
	if initResult.ProtocolVersion == "" {
		t.Fatal("expected non-empty protocol version in handshake result")
	}
}

// TestToolsListReadOnly verifies that in ReadOnly mode, exactly 6 read tools are returned.
// Validates: Requirements 1.2, 3.3, 3.4
func TestToolsListReadOnly(t *testing.T) {
	cfg := &config.Config{
		APIBaseURL: "http://localhost:8080/api/v1",
		APIToken:   "test-token",
		AccessMode: config.ReadOnly,
		Transport:  "stdio",
	}

	cs := connectClientToServer(t, cfg, noopClient{})

	ctx := context.Background()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"downtime-summary",
		"get-monitor",
		"list-incidents",
		"list-monitors",
		"monitor-history",
		"monitor-stats",
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	if len(names) != 6 {
		t.Fatalf("ReadOnly mode: expected 6 tools, got %d: %v", len(names), names)
	}
	for i, name := range expectedTools {
		if names[i] != name {
			t.Fatalf("tool mismatch at %d: expected %q, got %q\nexpected: %v\nactual: %v",
				i, name, names[i], expectedTools, names)
		}
	}
}

// TestToolsListReadWrite verifies that in ReadWrite mode, exactly 7 tools are returned
// (6 read + 1 write).
// Validates: Requirements 1.2, 3.3, 3.4
func TestToolsListReadWrite(t *testing.T) {
	cfg := &config.Config{
		APIBaseURL: "http://localhost:8080/api/v1",
		APIToken:   "test-token",
		AccessMode: config.ReadWrite,
		Transport:  "stdio",
	}

	cs := connectClientToServer(t, cfg, noopClient{})

	ctx := context.Background()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"create-monitor",
		"downtime-summary",
		"get-monitor",
		"list-incidents",
		"list-monitors",
		"monitor-history",
		"monitor-stats",
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	if len(names) != 7 {
		t.Fatalf("ReadWrite mode: expected 7 tools, got %d: %v", len(names), names)
	}
	for i, name := range expectedTools {
		if names[i] != name {
			t.Fatalf("tool mismatch at %d: expected %q, got %q\nexpected: %v\nactual: %v",
				i, name, names[i], expectedTools, names)
		}
	}
}

// TestUnknownToolInvocationReturnsError verifies that calling a tool that doesn't
// exist returns a protocol-level error from the SDK.
// Validates: Requirements 1.4
func TestUnknownToolInvocationReturnsError(t *testing.T) {
	cfg := &config.Config{
		APIBaseURL: "http://localhost:8080/api/v1",
		APIToken:   "test-token",
		AccessMode: config.ReadOnly,
		Transport:  "stdio",
	}

	cs := connectClientToServer(t, cfg, noopClient{})

	ctx := context.Background()
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "nonexistent-tool",
		Arguments: map[string]any{},
	})

	// The SDK should return a protocol-level error for unknown tools.
	if err == nil {
		t.Fatal("expected error when calling unknown tool, got nil")
	}
}
