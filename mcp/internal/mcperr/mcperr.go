// Package mcperr maps Pulse API errors and connectivity failures to structured
// MCP error responses. Every MCPError carries a non-empty code and message so
// that MCP clients can programmatically classify failures.
package mcperr

import (
	"fmt"

	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// Error codes used by the MCP server.
const (
	CodeValidationError    = "VALIDATION_ERROR"
	CodeInvalidType        = "INVALID_TYPE"
	CodeAmbiguousName      = "AMBIGUOUS_NAME"
	CodeNotFound           = "NOT_FOUND"
	CodeInvalidRange       = "INVALID_RANGE"
	CodeInvalidIdentifier  = "INVALID_IDENTIFIER"
	CodeInvalidWindow      = "INVALID_WINDOW"
	CodeWriteDisabled      = "WRITE_DISABLED"
	CodePulseUnreachable   = "PULSE_UNREACHABLE"
	CodePulseTimeout       = "PULSE_TIMEOUT"
	CodePulseUnauthorized  = "PULSE_UNAUTHORIZED"
)

// MCPError represents an error returned to MCP clients. It always carries a
// non-empty Code and Message. RequestID is populated when the error originated
// from a Pulse API response that included an X-Request-ID header.
type MCPError struct {
	Code      string
	Message   string
	RequestID string // optional; set when error comes from Pulse
}

// Error implements the error interface.
func (e *MCPError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("%s: %s (request_id=%s)", e.Code, e.Message, e.RequestID)
	}
	return e.Code + ": " + e.Message
}

// --- Mapping constructors from Pulse errors ---

// FromPulseError maps a Pulse API error envelope to an MCPError, preserving
// the Pulse error code, message, and X-Request-ID verbatim (Req 12.1, 12.3).
// HTTP 401 from Pulse is mapped to PULSE_UNAUTHORIZED (Req 3.5).
func FromPulseError(pe *pulseapi.PulseError) *MCPError {
	if pe.HTTPStatus == 401 {
		return &MCPError{
			Code:      CodePulseUnauthorized,
			Message:   pe.Message,
			RequestID: pe.RequestID,
		}
	}
	return &MCPError{
		Code:      pe.Code,
		Message:   pe.Message,
		RequestID: pe.RequestID,
	}
}

// FromConnectivityError maps a connectivity failure to a PULSE_UNREACHABLE or
// PULSE_TIMEOUT error depending on the reason (Req 2.4, 12.4).
func FromConnectivityError(ce *pulseapi.ConnectivityError) *MCPError {
	if ce.Reason == "timeout" {
		return &MCPError{
			Code:    CodePulseTimeout,
			Message: "pulse api request timed out",
		}
	}
	return &MCPError{
		Code:    CodePulseUnreachable,
		Message: "pulse api is unreachable: " + ce.Reason,
	}
}

// --- Helper constructors for common MCP errors ---

// Validation returns a VALIDATION_ERROR with the given message.
func Validation(message string) *MCPError {
	return &MCPError{Code: CodeValidationError, Message: message}
}

// InvalidType returns an INVALID_TYPE error with the given message.
func InvalidType(message string) *MCPError {
	return &MCPError{Code: CodeInvalidType, Message: message}
}

// AmbiguousName returns an AMBIGUOUS_NAME error with the given message.
func AmbiguousName(message string) *MCPError {
	return &MCPError{Code: CodeAmbiguousName, Message: message}
}

// NotFound returns a NOT_FOUND error with the given message.
func NotFound(message string) *MCPError {
	return &MCPError{Code: CodeNotFound, Message: message}
}

// InvalidRange returns an INVALID_RANGE error with the given message.
func InvalidRange(message string) *MCPError {
	return &MCPError{Code: CodeInvalidRange, Message: message}
}

// InvalidIdentifier returns an INVALID_IDENTIFIER error with the given message.
func InvalidIdentifier(message string) *MCPError {
	return &MCPError{Code: CodeInvalidIdentifier, Message: message}
}

// InvalidWindow returns an INVALID_WINDOW error with the given message.
func InvalidWindow(message string) *MCPError {
	return &MCPError{Code: CodeInvalidWindow, Message: message}
}

// WriteDisabled returns a WRITE_DISABLED error with a fixed message.
func WriteDisabled() *MCPError {
	return &MCPError{Code: CodeWriteDisabled, Message: "write access is disabled"}
}
