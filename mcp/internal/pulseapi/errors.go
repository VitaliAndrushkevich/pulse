package pulseapi

import "fmt"

// PulseError represents an error returned by the Pulse REST API.
// It preserves the error code, message, request ID, and HTTP status
// from the Pulse error envelope.
type PulseError struct {
	Code       string
	Message    string
	RequestID  string
	HTTPStatus int
}

// Error implements the error interface.
func (e *PulseError) Error() string {
	return e.Code + ": " + e.Message
}

// ConnectivityError represents a failure to reach the Pulse API.
// It is distinct from PulseError so that the MCP error mapper can
// emit a separate error code for unreachable/timeout conditions.
type ConnectivityError struct {
	Reason string // "timeout" | "connection_refused" | "dial_error"
}

// Error implements the error interface.
func (e *ConnectivityError) Error() string {
	return fmt.Sprintf("pulse api unreachable: %s", e.Reason)
}
