// Package tools implements MCP tool handlers for the Pulse MCP server.
// Each tool handler is a standalone function that receives a Deps struct
// providing access to shared dependencies (Pulse API client, access mode).
package tools

import (
	"errors"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// Deps holds shared dependencies injected into all tool handlers.
type Deps struct {
	Client     pulseapi.PulseClient
	AccessMode config.AccessMode
}

// mapPulseError maps Pulse API errors and connectivity errors to MCP errors.
func mapPulseError(err error) error {
	var pe *pulseapi.PulseError
	if errors.As(err, &pe) {
		return mcperr.FromPulseError(pe)
	}

	var ce *pulseapi.ConnectivityError
	if errors.As(err, &ce) {
		return mcperr.FromConnectivityError(ce)
	}

	// Fallback: unknown error type.
	return mcperr.Validation(err.Error())
}

// mapResolveError converts errors from resolve.Monitor to appropriate MCP errors.
// If the error is already an MCPError (from the resolve package), it passes through.
// Otherwise it falls back to mapPulseError for connectivity/Pulse errors.
func mapResolveError(err error) error {
	var mcpErr *mcperr.MCPError
	if errors.As(err, &mcpErr) {
		return mcpErr
	}
	return mapPulseError(err)
}
