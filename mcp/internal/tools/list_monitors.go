package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// recognizedTypes maps lowercase user input to the canonical Pulse wire form.
var recognizedTypes = map[string]string{
	"http":      "http",
	"https":     "https",
	"http/3":    "http3",
	"tcp":       "tcp",
	"udp":       "udp",
	"websocket": "websocket",
	"grpc":      "grpc",
	"dns":       "dns",
	"icmp":      "icmp",
	"smtp":      "smtp",
	"quic":      "quic",
}

// recognizedTypeNames is the sorted list shown in error messages.
var recognizedTypeNames = []string{
	"DNS", "HTTP", "HTTP/3", "HTTPS", "ICMP", "QUIC", "SMTP", "TCP", "UDP", "WebSocket", "gRPC",
}

// ListMonitorsInput is the input schema for the list-monitors tool.
type ListMonitorsInput struct {
	Type  string   `json:"type,omitempty" jsonschema:"Filter by monitor type (case-insensitive). Recognized: HTTP, HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP, QUIC"`
	Tags  []string `json:"tags,omitempty" jsonschema:"Filter by tags (key:value format, AND semantics)"`
	Page  int      `json:"page,omitempty" jsonschema:"Page number (minimum 1, default 1)"`
	Limit int      `json:"limit,omitempty" jsonschema:"Results per page (1-100, default 50)"`
}

// MonitorItem is a single monitor in the list-monitors output.
type MonitorItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Status string `json:"status"`
	State  string `json:"state"`
}

// ListMonitorsOutput is the result returned by the list-monitors tool.
type ListMonitorsOutput struct {
	Monitors    []MonitorItem `json:"monitors"`
	Page        int           `json:"page"`
	Limit       int           `json:"limit"`
	Total       int           `json:"total"`
	TotalPages  int           `json:"total_pages"`
	HasNextPage bool          `json:"has_next_page"`
}

// HandleListMonitors implements the list-monitors tool.
// It validates inputs, normalizes the type filter, calls the Pulse API,
// and returns paginated monitor results.
func HandleListMonitors(ctx context.Context, deps Deps, input ListMonitorsInput) (*ListMonitorsOutput, error) {
	// Apply defaults.
	page := input.Page
	if page == 0 {
		page = 1
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}

	// Validate page range.
	if page < 1 {
		return nil, mcperr.InvalidRange("page must be ≥ 1")
	}

	// Validate limit range.
	if limit < 1 || limit > 100 {
		return nil, mcperr.InvalidRange("limit must be between 1 and 100")
	}

	// Validate and normalize type filter.
	var normalizedType string
	if input.Type != "" {
		lower := strings.ToLower(strings.TrimSpace(input.Type))
		canonical, ok := recognizedTypes[lower]
		if !ok {
			return nil, mcperr.InvalidType(
				fmt.Sprintf("unrecognized monitor type %q; recognized types: %s",
					input.Type, strings.Join(recognizedTypeNames, ", ")),
			)
		}
		normalizedType = canonical
	}

	// Build query and call Pulse API.
	query := pulseapi.MonitorQuery{
		Type:  normalizedType,
		Tags:  input.Tags,
		Page:  page,
		Limit: limit,
	}

	result, err := deps.Client.ListMonitors(ctx, query)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Build output monitors.
	monitors := make([]MonitorItem, 0, len(result.Monitors))
	for _, m := range result.Monitors {
		monitors = append(monitors, MonitorItem{
			ID:     m.ID,
			Name:   m.Name,
			Type:   m.Type,
			Target: m.Target,
			Status: m.Status,
			State:  m.State,
		})
	}

	totalPages := result.TotalPages
	hasNextPage := page < totalPages

	return &ListMonitorsOutput{
		Monitors:    monitors,
		Page:        page,
		Limit:       limit,
		Total:       result.Total,
		TotalPages:  totalPages,
		HasNextPage: hasNextPage,
	}, nil
}
