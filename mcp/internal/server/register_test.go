package server

import (
	"context"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pgregory.net/rapid"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/tools"
)

// noopClient is a minimal PulseClient that never gets called — we only need it
// to satisfy the Deps struct so Register can wire handler closures.
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

// readToolNames is the set of tools that must be registered in both modes.
var readToolNames = []string{
	"list-monitors",
	"get-monitor",
	"monitor-stats",
	"monitor-history",
	"downtime-summary",
	"list-incidents",
}

// writeToolNames is the set of tools registered only in read-write mode.
var writeToolNames = []string{
	"create-monitor",
}

// listRegisteredTools creates an MCP server, registers tools with the given mode,
// connects an in-memory client, and returns the advertised tool names.
func listRegisteredTools(t *testing.T, mode config.AccessMode) []string {
	t.Helper()

	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pulse-mcp-test",
		Version: "0.0.0-test",
	}, nil)

	deps := tools.Deps{
		Client:     noopClient{},
		AccessMode: mode,
	}
	Register(srv, deps, mode)

	// Connect an in-memory client to query the tool list.
	ct, st := mcp.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.0",
	}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// Feature: mcp-server, Property 25: Advertised tools match the Access_Mode exactly
//
// In read-only mode, the registered tools are exactly the 6 read tools
// (list-monitors, get-monitor, monitor-stats, monitor-history, downtime-summary,
// list-incidents). In read-write mode, the registered tools are the 6 read tools
// plus create-monitor (7 total).
//
// **Validates: Requirements 10.3, 10.5, 10.6**
func TestProperty25_AdvertisedToolsMatchAccessModeExactly(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Draw a random access mode.
		isReadWrite := rapid.Bool().Draw(rt, "isReadWrite")

		var mode config.AccessMode
		var expectedNames []string

		if isReadWrite {
			mode = config.ReadWrite
			expectedNames = append(append([]string{}, readToolNames...), writeToolNames...)
		} else {
			mode = config.ReadOnly
			expectedNames = append([]string{}, readToolNames...)
		}
		sort.Strings(expectedNames)

		actualNames := listRegisteredTools(t, mode)

		// Verify exact count.
		if len(actualNames) != len(expectedNames) {
			rt.Fatalf("mode=%s: expected %d tools, got %d\nexpected: %v\nactual: %v",
				mode, len(expectedNames), len(actualNames), expectedNames, actualNames)
		}

		// Verify exact set match.
		for i := range expectedNames {
			if actualNames[i] != expectedNames[i] {
				rt.Fatalf("mode=%s: tool mismatch at position %d: expected %q, got %q\nexpected: %v\nactual: %v",
					mode, i, expectedNames[i], actualNames[i], expectedNames, actualNames)
			}
		}
	})
}
