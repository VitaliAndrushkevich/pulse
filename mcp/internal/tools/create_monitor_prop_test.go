package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// trackingFakeClient records whether CreateMonitor was called and captures the input.
type trackingFakeClient struct {
	fakePulseClient
	createCalled  bool
	capturedInput pulseapi.CreateMonitorInput
}

func (f *trackingFakeClient) CreateMonitor(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
	f.createCalled = true
	f.capturedInput = in
	return pulseapi.Monitor{
		ID:              "mon-generated-id",
		Name:            in.Name,
		Type:            in.Type,
		Target:          in.Target,
		Status:          "pending",
		State:           "active",
		IntervalSeconds: derefOr(in.IntervalSeconds, 60),
		TimeoutSeconds:  derefOr(in.TimeoutSeconds, 10),
	}, nil
}

func derefOr(p *int, def int) int {
	if p != nil {
		return *p
	}
	return def
}

// Feature: mcp-server, Property 20: create-monitor builds the correct payload and reflects defaults
//
// For any valid Simple_Monitor_Type, non-blank name (1–255), and type-valid target, the
// create-monitor tool (in read-write mode) issues exactly one POST whose payload matches the
// inputs, omits interval_seconds/timeout_seconds from the request when not supplied, and
// produces output that mirrors the monitor returned by Pulse (including Pulse-applied defaults).
//
// **Validates: Requirements 9.1, 9.2**
func TestProperty20_CreateMonitorBuildsCorrectPayloadAndReflectsDefaults(t *testing.T) {
	// Generator for valid create-monitor types and matching targets.
	type typeTarget struct {
		typ    string
		target string
	}

	genTypeTarget := rapid.Custom(func(t *rapid.T) typeTarget {
		types := []typeTarget{
			{"http", "https://example.com"},
			{"tcp", "example.com:5432"},
			{"udp", "example.com:53"},
			{"icmp", "192.168.1.1"},
		}
		idx := rapid.IntRange(0, len(types)-1).Draw(t, "typeIdx")
		return types[idx]
	})

	// Generator for valid names: 1–255 chars, not all whitespace.
	genValidName := rapid.Custom(func(t *rapid.T) string {
		// Generate a non-blank name of length 1–255.
		prefix := rapid.StringMatching(`[A-Za-z]`).Draw(t, "namePrefix")
		suffixLen := rapid.IntRange(0, 254).Draw(t, "nameLen")
		suffix := ""
		if suffixLen > 0 {
			suffix = rapid.StringMatching(`[A-Za-z0-9 _\-]{0,254}`).Draw(t, "nameSuffix")
			if len(suffix) > suffixLen {
				suffix = suffix[:suffixLen]
			}
		}
		name := prefix + suffix
		if len(name) > 255 {
			name = name[:255]
		}
		return name
	})

	rapid.Check(t, func(t *rapid.T) {
		tt := genTypeTarget.Draw(t, "typeTarget")
		name := genValidName.Draw(t, "name")

		// Randomly decide whether to supply optional fields.
		supplyInterval := rapid.Bool().Draw(t, "supplyInterval")
		supplyTimeout := rapid.Bool().Draw(t, "supplyTimeout")

		// Use a random case variation of the type input.
		typeInput := randomCase(t, tt.typ)

		client := &trackingFakeClient{}
		deps := Deps{Client: client, AccessMode: config.ReadWrite}

		input := CreateMonitorToolInput{
			Type:   typeInput,
			Name:   name,
			Target: tt.target,
		}

		var expectedInterval, expectedTimeout int
		if supplyInterval {
			v := rapid.IntRange(1, 3600).Draw(t, "interval")
			input.IntervalSeconds = &v
			expectedInterval = v
		} else {
			expectedInterval = 60 // Pulse default
		}
		if supplyTimeout {
			v := rapid.IntRange(1, 300).Draw(t, "timeout")
			input.TimeoutSeconds = &v
			expectedTimeout = v
		} else {
			expectedTimeout = 10 // Pulse default
		}

		out, err := HandleCreateMonitor(context.Background(), deps, input)
		if err != nil {
			t.Fatalf("unexpected error for valid input (type=%q, name=%q, target=%q): %v",
				typeInput, name, tt.target, err)
		}

		// Verify exactly one Pulse call was made.
		if !client.createCalled {
			t.Fatal("expected CreateMonitor to be called")
		}

		// Verify the payload matches the inputs.
		if client.capturedInput.Type != tt.typ {
			t.Fatalf("payload type: got %q, want %q", client.capturedInput.Type, tt.typ)
		}
		if client.capturedInput.Name != name {
			t.Fatalf("payload name: got %q, want %q", client.capturedInput.Name, name)
		}
		if client.capturedInput.Target != tt.target {
			t.Fatalf("payload target: got %q, want %q", client.capturedInput.Target, tt.target)
		}

		// Verify interval/timeout are omitted when not supplied.
		if !supplyInterval && client.capturedInput.IntervalSeconds != nil {
			t.Fatal("interval_seconds should be omitted from payload when not supplied")
		}
		if supplyInterval && (client.capturedInput.IntervalSeconds == nil || *client.capturedInput.IntervalSeconds != *input.IntervalSeconds) {
			t.Fatalf("interval_seconds mismatch in payload")
		}
		if !supplyTimeout && client.capturedInput.TimeoutSeconds != nil {
			t.Fatal("timeout_seconds should be omitted from payload when not supplied")
		}
		if supplyTimeout && (client.capturedInput.TimeoutSeconds == nil || *client.capturedInput.TimeoutSeconds != *input.TimeoutSeconds) {
			t.Fatalf("timeout_seconds mismatch in payload")
		}

		// Verify output mirrors the Pulse response (including defaults).
		if out.ID != "mon-generated-id" {
			t.Fatalf("output ID: got %q, want %q", out.ID, "mon-generated-id")
		}
		if out.Name != name {
			t.Fatalf("output name: got %q, want %q", out.Name, name)
		}
		if out.Type != tt.typ {
			t.Fatalf("output type: got %q, want %q", out.Type, tt.typ)
		}
		if out.Target != tt.target {
			t.Fatalf("output target: got %q, want %q", out.Target, tt.target)
		}
		if out.IntervalSeconds != expectedInterval {
			t.Fatalf("output interval: got %d, want %d", out.IntervalSeconds, expectedInterval)
		}
		if out.TimeoutSeconds != expectedTimeout {
			t.Fatalf("output timeout: got %d, want %d", out.TimeoutSeconds, expectedTimeout)
		}
	})
}

// Feature: mcp-server, Property 21: Non-simple monitor type is rejected without calling Pulse
//
// For any monitor type not in {HTTP, TCP, UDP, ICMP, QUIC} (including otherwise-valid Pulse types
// such as gRPC or DNS), the create-monitor tool returns an MCP error listing the supported
// types and makes zero Pulse calls.
//
// **Validates: Requirements 9.3**
func TestProperty21_NonSimpleMonitorTypeRejectedWithoutCallingPulse(t *testing.T) {
	// Set of supported types (lowercase) for create-monitor.
	supported := map[string]bool{
		"http": true, "tcp": true, "udp": true, "icmp": true, "quic": true,
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate an arbitrary type string NOT in the supported set.
		typeStr := rapid.StringMatching(`[A-Za-z0-9/_\-]{1,30}`).Filter(func(s string) bool {
			return !supported[strings.ToLower(strings.TrimSpace(s))]
		}).Draw(t, "type")

		client := &trackingFakeClient{}
		deps := Deps{Client: client, AccessMode: config.ReadWrite}

		_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
			Type:   typeStr,
			Name:   "Valid Name",
			Target: "example.com:80",
		})

		// Must return an error.
		if err == nil {
			t.Fatalf("expected error for unsupported type %q but got nil", typeStr)
		}

		// Must be INVALID_TYPE.
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeInvalidType {
			t.Fatalf("expected code %q, got %q", mcperr.CodeInvalidType, mcpErr.Code)
		}

		// Error message should list supported types.
		if !strings.Contains(mcpErr.Message, "HTTP, TCP, UDP, ICMP, QUIC") {
			t.Fatalf("error message should list supported types, got: %s", mcpErr.Message)
		}

		// Zero Pulse calls.
		if client.createCalled {
			t.Fatal("CreateMonitor was called despite unsupported type")
		}
	})
}

// Feature: mcp-server, Property 22: Access-mode gate blocks writes in read-only mode
//
// For any valid create-monitor input, when the server is in read-only mode the tool returns
// a write-disabled MCP error and makes zero Pulse calls.
//
// **Validates: Requirements 9.5, 10.4**
func TestProperty22_AccessModeGateBlocksWritesInReadOnlyMode(t *testing.T) {
	// Generator for valid create-monitor inputs.
	type validInput struct {
		typ    string
		name   string
		target string
	}

	genValidInput := rapid.Custom(func(t *rapid.T) validInput {
		types := []validInput{
			{"HTTP", "My Monitor", "https://example.com"},
			{"TCP", "TCP Check", "db.example.com:5432"},
			{"UDP", "DNS Check", "ns1.example.com:53"},
			{"ICMP", "Ping Check", "192.168.1.1"},
		}
		idx := rapid.IntRange(0, len(types)-1).Draw(t, "inputIdx")
		return types[idx]
	})

	rapid.Check(t, func(t *rapid.T) {
		vi := genValidInput.Draw(t, "input")

		client := &trackingFakeClient{}
		deps := Deps{Client: client, AccessMode: config.ReadOnly}

		_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
			Type:   vi.typ,
			Name:   vi.name,
			Target: vi.target,
		})

		// Must return an error.
		if err == nil {
			t.Fatal("expected error in read-only mode but got nil")
		}

		// Must be WRITE_DISABLED.
		var mcpErr *mcperr.MCPError
		if !errors.As(err, &mcpErr) {
			t.Fatalf("expected MCPError, got %T: %v", err, err)
		}
		if mcpErr.Code != mcperr.CodeWriteDisabled {
			t.Fatalf("expected code %q, got %q", mcperr.CodeWriteDisabled, mcpErr.Code)
		}

		// Zero Pulse calls.
		if client.createCalled {
			t.Fatal("CreateMonitor was called despite read-only mode")
		}
	})
}

// Feature: mcp-server, Property 23: Invalid monitor name is rejected without calling Pulse
//
// For any name that is empty, whitespace-only, or longer than 255 characters, the
// create-monitor tool returns a name-validation MCP error and makes zero Pulse calls;
// any non-blank name of length 1–255 passes this check.
//
// **Validates: Requirements 9.6**
func TestProperty23_InvalidMonitorNameRejectedWithoutCallingPulse(t *testing.T) {
	// Sub-property: invalid names are rejected.
	t.Run("invalid_names_rejected", func(t *testing.T) {
		genInvalidName := rapid.Custom(func(t *rapid.T) string {
			kind := rapid.IntRange(0, 2).Draw(t, "nameKind")
			switch kind {
			case 0:
				// Empty string
				return ""
			case 1:
				// Whitespace-only (spaces, tabs, newlines)
				length := rapid.IntRange(1, 50).Draw(t, "wsLen")
				ws := rapid.SampledFrom([]rune{' ', '\t', '\n', '\r'}).Draw(t, "wsChar")
				return strings.Repeat(string(ws), length)
			case 2:
				// Longer than 255 characters
				length := rapid.IntRange(256, 1000).Draw(t, "longLen")
				return strings.Repeat("x", length)
			}
			return ""
		})

		rapid.Check(t, func(t *rapid.T) {
			name := genInvalidName.Draw(t, "name")

			client := &trackingFakeClient{}
			deps := Deps{Client: client, AccessMode: config.ReadWrite}

			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   "HTTP",
				Name:   name,
				Target: "https://example.com",
			})

			// Must return an error.
			if err == nil {
				t.Fatalf("expected error for invalid name %q but got nil", name)
			}

			// Must be VALIDATION_ERROR.
			var mcpErr *mcperr.MCPError
			if !errors.As(err, &mcpErr) {
				t.Fatalf("expected MCPError, got %T: %v", err, err)
			}
			if mcpErr.Code != mcperr.CodeValidationError {
				t.Fatalf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
			}

			// Zero Pulse calls.
			if client.createCalled {
				t.Fatal("CreateMonitor was called despite invalid name")
			}
		})
	})

	// Sub-property: valid names pass the check (Pulse is called).
	t.Run("valid_names_pass", func(t *testing.T) {
		genValidName := rapid.Custom(func(t *rapid.T) string {
			// Non-blank name of 1–255 characters.
			prefix := rapid.StringMatching(`[A-Za-z]`).Draw(t, "prefix")
			extraLen := rapid.IntRange(0, 254).Draw(t, "extraLen")
			extra := ""
			if extraLen > 0 {
				extra = rapid.StringMatching(`[A-Za-z0-9 _\-]{0,254}`).Draw(t, "extra")
				if len(extra) > extraLen {
					extra = extra[:extraLen]
				}
			}
			name := prefix + extra
			if len(name) > 255 {
				name = name[:255]
			}
			return name
		})

		rapid.Check(t, func(t *rapid.T) {
			name := genValidName.Draw(t, "name")

			client := &trackingFakeClient{}
			deps := Deps{Client: client, AccessMode: config.ReadWrite}

			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   "HTTP",
				Name:   name,
				Target: "https://example.com",
			})

			// Should NOT return a name-validation error.
			if err != nil {
				var mcpErr *mcperr.MCPError
				if errors.As(err, &mcpErr) && mcpErr.Code == mcperr.CodeValidationError &&
					strings.Contains(mcpErr.Message, "name") {
					t.Fatalf("valid name %q was rejected: %v", name, err)
				}
				// Other errors (e.g., target validation) are not name-related, so are acceptable.
			}

			// Pulse must have been called (name validation passed).
			if !client.createCalled {
				// Only a problem if there was no error at all or the error was name-related.
				if err == nil {
					t.Fatalf("expected CreateMonitor to be called for valid name %q", name)
				}
			}
		})
	})
}
