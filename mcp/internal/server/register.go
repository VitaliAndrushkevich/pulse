// Package server wires the MCP server: it builds an mcp.Server, registers tools
// by Access_Mode, and runs the selected transport.
package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/tools"
)

// Tool metadata definitions — name + description for each tool.
var (
	listMonitorsTool = &mcp.Tool{
		Name:        "list-monitors",
		Description: "List Pulse monitors with optional type and tag filtering, paginated",
	}

	getMonitorTool = &mcp.Tool{
		Name:        "get-monitor",
		Description: "Get a single monitor's full configuration and current status",
	}

	monitorStatsTool = &mcp.Tool{
		Name:        "monitor-stats",
		Description: "Get a monitor's uptime statistics and optional SSL info",
	}

	monitorHistoryTool = &mcp.Tool{
		Name:        "monitor-history",
		Description: "Query a monitor's check history over a time range",
	}

	downtimeSummaryTool = &mcp.Tool{
		Name:        "downtime-summary",
		Description: "Get a downtime summary for a monitor over a recent window",
	}

	listIncidentsTool = &mcp.Tool{
		Name:        "list-incidents",
		Description: "List incidents globally or per monitor, with optional open-only filter",
	}

	createMonitorTool = &mcp.Tool{
		Name:        "create-monitor",
		Description: "Create a new health-check monitor (HTTP, TCP, UDP, or ICMP)",
	}
)

// Register adds all MCP tools to the server according to the Access_Mode.
// Read tools are always registered. Write tools (create-monitor) are registered
// only in read-write mode. This satisfies Requirements 10.3, 10.5, 10.6 —
// unregistered tools are neither advertised nor invocable.
//
// Defense-in-depth: the create-monitor handler also checks deps.AccessMode at
// call time (Requirement 10.4), so even if a client somehow invokes an
// unadvertised tool, the request is rejected before any Pulse API call.
func Register(s *mcp.Server, deps tools.Deps, mode config.AccessMode) {
	// Read tools — always available.
	mcp.AddTool(s, listMonitorsTool, wrapHandler(deps, tools.HandleListMonitors))
	mcp.AddTool(s, getMonitorTool, wrapHandler(deps, tools.HandleGetMonitor))
	mcp.AddTool(s, monitorStatsTool, wrapHandler(deps, tools.HandleMonitorStats))
	mcp.AddTool(s, monitorHistoryTool, wrapHandler(deps, tools.HandleMonitorHistory))
	mcp.AddTool(s, downtimeSummaryTool, wrapHandler(deps, tools.HandleDowntimeSummary))
	mcp.AddTool(s, listIncidentsTool, wrapHandler(deps, tools.HandleListIncidents))

	// Write tools — read-write mode only (Req 10.3, 10.5, 10.6).
	if mode == config.ReadWrite {
		mcp.AddTool(s, createMonitorTool, wrapHandler(deps, tools.HandleCreateMonitor))
	}
}

// wrapHandler adapts a tools package handler (func(ctx, Deps, Input) (*Output, error))
// into the MCP SDK's ToolHandlerFor signature. The SDK handles:
//   - Unmarshaling and validating input against the JSON Schema
//   - On success: marshaling output to JSON Content + StructuredContent
//   - On error: setting IsError=true and putting the error message in Content
func wrapHandler[In, Out any](
	deps tools.Deps,
	handler func(ctx context.Context, deps tools.Deps, input In) (*Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		result, err := handler(ctx, deps, input)
		if err != nil {
			var zero Out
			return nil, zero, err
		}
		return nil, *result, nil
	}
}
