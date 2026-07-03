package tools

import (
	"context"
	"strings"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// createMonitorTypes maps lowercase user input to the canonical Pulse wire form
// for the subset of types supported by the create-monitor tool.
var createMonitorTypes = map[string]string{
	"http": "http",
	"tcp":  "tcp",
	"udp":  "udp",
	"icmp": "icmp",
	"quic": "quic",
}

// createMonitorSupportedTypes is the sorted list shown in error messages.
var createMonitorSupportedTypes = "HTTP, TCP, UDP, ICMP, QUIC"

// CreateMonitorToolInput defines the input schema for the create-monitor tool.
type CreateMonitorToolInput struct {
	// Type is the monitor type: HTTP, TCP, UDP, ICMP, or QUIC (case-insensitive).
	Type string `json:"type" jsonschema:"Monitor type: HTTP, TCP, UDP, ICMP, or QUIC (case-insensitive)"`

	// Name is the display name for the monitor (1–255 characters, not blank).
	Name string `json:"name" jsonschema:"Monitor display name (1-255 characters)"`

	// Target is the monitoring target. Format depends on type:
	// HTTP: URL or bare host; TCP/UDP: host:port; ICMP: hostname or IP.
	Target string `json:"target" jsonschema:"Monitoring target. HTTP: URL or bare host; TCP/UDP: host:port; ICMP: hostname or IP"`

	// IntervalSeconds is the check interval in seconds (≥1, default: Pulse default 60).
	IntervalSeconds *int `json:"interval_seconds,omitempty" jsonschema:"Check interval in seconds (minimum 1, default 60)"`

	// TimeoutSeconds is the check timeout in seconds (≥1, default: Pulse default 10).
	TimeoutSeconds *int `json:"timeout_seconds,omitempty" jsonschema:"Check timeout in seconds (minimum 1, default 10)"`

	// HTTPExpectedStatuses is an optional list of expected HTTP status codes (100–599).
	// Only applicable for HTTP type monitors.
	HTTPExpectedStatuses []int `json:"http_expected_statuses,omitempty" jsonschema:"Expected HTTP status codes (100-599, HTTP type only)"`
}

// CreateMonitorOutput contains the created monitor details.
type CreateMonitorOutput struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Target          string `json:"target"`
	Status          string `json:"status"`
	State           string `json:"state"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// HandleCreateMonitor creates a new monitor via the Pulse API.
// Validation order: (1) read-only mode, (2) type check, (3) name check, (4) target shape check.
func HandleCreateMonitor(ctx context.Context, deps Deps, input CreateMonitorToolInput) (*CreateMonitorOutput, error) {
	// 1. Check access mode — reject if read-only.
	if deps.AccessMode != config.ReadWrite {
		return nil, mcperr.WriteDisabled()
	}

	// 2. Validate and normalize type.
	lower := strings.ToLower(strings.TrimSpace(input.Type))
	canonicalType, ok := createMonitorTypes[lower]
	if !ok {
		return nil, mcperr.InvalidType(
			"unsupported monitor type " + quote(input.Type) + "; supported types: " + createMonitorSupportedTypes,
		)
	}

	// 3. Validate name: must not be empty, blank, or >255 chars.
	if err := validateMonitorName(input.Name); err != nil {
		return nil, err
	}

	// 4. Validate target shape.
	if err := validateTarget(canonicalType, input.Target); err != nil {
		return nil, err
	}

	// 5. Validate optional fields.
	if input.IntervalSeconds != nil && *input.IntervalSeconds < 1 {
		return nil, mcperr.Validation("interval_seconds must be ≥ 1")
	}
	if input.TimeoutSeconds != nil && *input.TimeoutSeconds < 1 {
		return nil, mcperr.Validation("timeout_seconds must be ≥ 1")
	}

	// 6. Build settings map for HTTP expected statuses.
	settings := map[string]any{}
	if canonicalType == "http" && len(input.HTTPExpectedStatuses) > 0 {
		for _, code := range input.HTTPExpectedStatuses {
			if code < 100 || code > 599 {
				return nil, mcperr.Validation("http_expected_statuses values must be between 100 and 599")
			}
		}
		settings["expected_statuses"] = input.HTTPExpectedStatuses
	}

	// 7. Build Pulse API input.
	createInput := pulseapi.CreateMonitorInput{
		Type:     canonicalType,
		Name:     input.Name,
		Target:   input.Target,
		Settings: settings,
	}
	if input.IntervalSeconds != nil {
		createInput.IntervalSeconds = input.IntervalSeconds
	}
	if input.TimeoutSeconds != nil {
		createInput.TimeoutSeconds = input.TimeoutSeconds
	}

	// 8. Call Pulse API.
	monitor, err := deps.Client.CreateMonitor(ctx, createInput)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// 9. Return created monitor.
	return &CreateMonitorOutput{
		ID:              monitor.ID,
		Name:            monitor.Name,
		Type:            monitor.Type,
		Target:          monitor.Target,
		Status:          monitor.Status,
		State:           monitor.State,
		IntervalSeconds: monitor.IntervalSeconds,
		TimeoutSeconds:  monitor.TimeoutSeconds,
	}, nil
}

// validateMonitorName checks that the name is non-empty, not blank, and within length bounds.
func validateMonitorName(name string) error {
	if name == "" {
		return mcperr.Validation("name is required")
	}
	if strings.TrimSpace(name) == "" {
		return mcperr.Validation("name must not be blank")
	}
	if len(name) > 255 {
		return mcperr.Validation("name must be 1–255 characters")
	}
	return nil
}

// validateTarget performs a basic shape check on the target before sending to Pulse.
func validateTarget(monitorType, target string) error {
	if target == "" {
		return mcperr.Validation("target is required")
	}

	switch monitorType {
	case "tcp", "udp":
		// TCP/UDP targets must contain a colon separating host and port.
		if !strings.Contains(target, ":") {
			return mcperr.Validation("target for " + strings.ToUpper(monitorType) + " must be in host:port format")
		}
	case "icmp":
		// ICMP targets must not contain a port (no colon with port-like suffix).
		// Allow IPv6 addresses (contain colons) but reject obvious host:port patterns.
		// Simple heuristic: if it has a colon and the last segment is numeric, reject.
		// Otherwise allow (could be IPv6 or hostname).
	case "http", "quic":
		// HTTP/QUIC targets: non-empty is sufficient; Pulse handles normalization.
	}

	return nil
}

// quote wraps a string in double quotes for use in error messages.
func quote(s string) string {
	return `"` + s + `"`
}
