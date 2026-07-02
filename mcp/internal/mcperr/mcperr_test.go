package mcperr_test

import (
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

func TestFromPulseError_PreservesCodeMessageRequestID(t *testing.T) {
	pe := &pulseapi.PulseError{
		Code:       "MONITOR_NOT_FOUND",
		Message:    "monitor abc not found",
		RequestID:  "req-123",
		HTTPStatus: 404,
	}

	got := mcperr.FromPulseError(pe)

	if got.Code != "MONITOR_NOT_FOUND" {
		t.Errorf("Code = %q, want %q", got.Code, "MONITOR_NOT_FOUND")
	}
	if got.Message != "monitor abc not found" {
		t.Errorf("Message = %q, want %q", got.Message, "monitor abc not found")
	}
	if got.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want %q", got.RequestID, "req-123")
	}
}

func TestFromPulseError_401MapsToUnauthorized(t *testing.T) {
	pe := &pulseapi.PulseError{
		Code:       "UNAUTHORIZED",
		Message:    "invalid token",
		RequestID:  "req-456",
		HTTPStatus: 401,
	}

	got := mcperr.FromPulseError(pe)

	if got.Code != mcperr.CodePulseUnauthorized {
		t.Errorf("Code = %q, want %q", got.Code, mcperr.CodePulseUnauthorized)
	}
	if got.Message != "invalid token" {
		t.Errorf("Message = %q, want %q", got.Message, "invalid token")
	}
	if got.RequestID != "req-456" {
		t.Errorf("RequestID = %q, want %q", got.RequestID, "req-456")
	}
}

func TestFromConnectivityError_Timeout(t *testing.T) {
	ce := &pulseapi.ConnectivityError{Reason: "timeout"}

	got := mcperr.FromConnectivityError(ce)

	if got.Code != mcperr.CodePulseTimeout {
		t.Errorf("Code = %q, want %q", got.Code, mcperr.CodePulseTimeout)
	}
	if got.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestFromConnectivityError_ConnectionRefused(t *testing.T) {
	ce := &pulseapi.ConnectivityError{Reason: "connection_refused"}

	got := mcperr.FromConnectivityError(ce)

	if got.Code != mcperr.CodePulseUnreachable {
		t.Errorf("Code = %q, want %q", got.Code, mcperr.CodePulseUnreachable)
	}
	if got.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestFromConnectivityError_DialError(t *testing.T) {
	ce := &pulseapi.ConnectivityError{Reason: "dial_error"}

	got := mcperr.FromConnectivityError(ce)

	if got.Code != mcperr.CodePulseUnreachable {
		t.Errorf("Code = %q, want %q", got.Code, mcperr.CodePulseUnreachable)
	}
}

func TestHelperConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      *mcperr.MCPError
		wantCode string
	}{
		{"Validation", mcperr.Validation("bad input"), mcperr.CodeValidationError},
		{"InvalidType", mcperr.InvalidType("unknown type"), mcperr.CodeInvalidType},
		{"AmbiguousName", mcperr.AmbiguousName("multiple matches"), mcperr.CodeAmbiguousName},
		{"NotFound", mcperr.NotFound("not found"), mcperr.CodeNotFound},
		{"InvalidRange", mcperr.InvalidRange("from > to"), mcperr.CodeInvalidRange},
		{"InvalidIdentifier", mcperr.InvalidIdentifier("bad id"), mcperr.CodeInvalidIdentifier},
		{"InvalidWindow", mcperr.InvalidWindow("too short"), mcperr.CodeInvalidWindow},
		{"WriteDisabled", mcperr.WriteDisabled(), mcperr.CodeWriteDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", tt.err.Code, tt.wantCode)
			}
			if tt.err.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

func TestWriteDisabled_FixedMessage(t *testing.T) {
	got := mcperr.WriteDisabled()
	if got.Message != "write access is disabled" {
		t.Errorf("Message = %q, want %q", got.Message, "write access is disabled")
	}
}

func TestMCPError_ErrorString(t *testing.T) {
	t.Run("without request ID", func(t *testing.T) {
		e := &mcperr.MCPError{Code: "NOT_FOUND", Message: "monitor not found"}
		got := e.Error()
		want := "NOT_FOUND: monitor not found"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("with request ID", func(t *testing.T) {
		e := &mcperr.MCPError{Code: "NOT_FOUND", Message: "monitor not found", RequestID: "req-789"}
		got := e.Error()
		want := "NOT_FOUND: monitor not found (request_id=req-789)"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}
