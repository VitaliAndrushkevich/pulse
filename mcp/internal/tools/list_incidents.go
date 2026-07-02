package tools

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
	"github.com/vandrushkevich/pulse/mcp/internal/resolve"
)

// ListIncidentsInput is the input schema for the list-incidents tool.
type ListIncidentsInput struct {
	Monitor  string `json:"monitor,omitempty" jsonschema:"Monitor identifier (UUID or name). When present, returns incidents for that monitor only"`
	OpenOnly *bool  `json:"open_only,omitempty" jsonschema:"When true, return only open (unresolved) incidents. Default false"`
	Page     int    `json:"page,omitempty" jsonschema:"Page number (minimum 1, default 1)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Results per page (1-100, default 20)"`
}

// IncidentItem is a single incident in the list-incidents output.
type IncidentItem struct {
	ID         string  `json:"id"`
	MonitorID  string  `json:"monitor_id"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"started_at"`
	ResolvedAt *string `json:"resolved_at,omitempty"`
}

// ListIncidentsOutput is the result returned by the list-incidents tool.
type ListIncidentsOutput struct {
	Incidents   []IncidentItem `json:"incidents"`
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
	Total       int            `json:"total"`
	TotalPages  int            `json:"total_pages"`
	HasNextPage bool           `json:"has_next_page"`
}

// HandleListIncidents implements the list-incidents tool.
// It validates inputs, resolves the optional monitor reference, and returns
// paginated incident results ordered by started_at descending.
//
// When both monitor and open_only are specified, client-side filtering is applied
// because the per-monitor endpoint does not support a status parameter.
func HandleListIncidents(ctx context.Context, deps Deps, input ListIncidentsInput) (*ListIncidentsOutput, error) {
	// Apply defaults.
	page := input.Page
	if page == 0 {
		page = 1
	}
	limit := input.Limit
	if limit == 0 {
		limit = 20
	}

	// Validate page range.
	if page < 1 {
		return nil, mcperr.InvalidRange("page must be ≥ 1")
	}

	// Validate limit range.
	if limit < 1 || limit > 100 {
		return nil, mcperr.InvalidRange("limit must be between 1 and 100")
	}

	openOnly := input.OpenOnly != nil && *input.OpenOnly
	monitor := strings.TrimSpace(input.Monitor)

	// When monitor + open_only are both set, we need client-side filtering.
	if monitor != "" && openOnly {
		return handlePerMonitorOpenOnly(ctx, deps, monitor, page, limit)
	}

	// Resolve monitor ID if provided.
	var monitorID string
	if monitor != "" {
		id, err := resolve.Monitor(ctx, deps.Client, monitor)
		if err != nil {
			return nil, mapResolveError(err)
		}
		monitorID = id
	}

	// Build query and call Pulse API.
	query := pulseapi.IncidentQuery{
		MonitorID: monitorID,
		OpenOnly:  openOnly,
		Page:      page,
		Limit:     limit,
	}

	result, err := deps.Client.ListIncidents(ctx, query)
	if err != nil {
		return nil, mapPulseError(err)
	}

	// Sort by started_at descending to guarantee order.
	sortIncidentsDesc(result.Incidents)

	// Build output.
	incidents := buildIncidentItems(result.Incidents)

	totalPages := result.TotalPages
	hasNextPage := page < totalPages

	return &ListIncidentsOutput{
		Incidents:   incidents,
		Page:        page,
		Limit:       limit,
		Total:       result.Total,
		TotalPages:  totalPages,
		HasNextPage: hasNextPage,
	}, nil
}

// handlePerMonitorOpenOnly handles the case where both monitor and open_only are set.
// The per-monitor endpoint does not support status filtering, so we fetch all
// incidents for the monitor, filter client-side, then apply pagination.
func handlePerMonitorOpenOnly(ctx context.Context, deps Deps, monitor string, page, limit int) (*ListIncidentsOutput, error) {
	// Resolve monitor ID.
	monitorID, err := resolve.Monitor(ctx, deps.Client, monitor)
	if err != nil {
		return nil, mapResolveError(err)
	}

	// Fetch all incidents for this monitor (paginate through all pages).
	var allIncidents []pulseapi.Incident
	fetchPage := 1
	const fetchLimit = 100

	for {
		result, err := deps.Client.ListIncidents(ctx, pulseapi.IncidentQuery{
			MonitorID: monitorID,
			OpenOnly:  false,
			Page:      fetchPage,
			Limit:     fetchLimit,
		})
		if err != nil {
			return nil, mapPulseError(err)
		}

		allIncidents = append(allIncidents, result.Incidents...)

		if fetchPage >= result.TotalPages || len(result.Incidents) == 0 {
			break
		}
		fetchPage++
	}

	// Filter to open incidents only.
	var filtered []pulseapi.Incident
	for i := range allIncidents {
		if allIncidents[i].Status == "open" {
			filtered = append(filtered, allIncidents[i])
		}
	}

	// Sort by started_at descending.
	sortIncidentsDesc(filtered)

	// Calculate pagination metadata over filtered results.
	total := len(filtered)
	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	// Slice the filtered results for the requested page.
	start := (page - 1) * limit
	end := start + limit

	var pageIncidents []pulseapi.Incident
	if start < total {
		if end > total {
			end = total
		}
		pageIncidents = filtered[start:end]
	}

	incidents := buildIncidentItems(pageIncidents)
	hasNextPage := page < totalPages

	return &ListIncidentsOutput{
		Incidents:   incidents,
		Page:        page,
		Limit:       limit,
		Total:       total,
		TotalPages:  totalPages,
		HasNextPage: hasNextPage,
	}, nil
}

// sortIncidentsDesc sorts incidents by StartedAt in descending order (newest first).
func sortIncidentsDesc(incidents []pulseapi.Incident) {
	sort.Slice(incidents, func(i, j int) bool {
		return incidents[i].StartedAt.After(incidents[j].StartedAt)
	})
}

// buildIncidentItems converts Pulse incidents to output items.
// resolved_at is included only when the incident is resolved.
func buildIncidentItems(incidents []pulseapi.Incident) []IncidentItem {
	items := make([]IncidentItem, 0, len(incidents))
	for _, inc := range incidents {
		item := IncidentItem{
			ID:        inc.ID,
			MonitorID: inc.MonitorID,
			Status:    inc.Status,
			StartedAt: inc.StartedAt.UTC().Format(time.RFC3339),
		}
		// Include resolved_at only when resolved (Req 8.6).
		if inc.Status == "resolved" && inc.ResolvedAt != nil {
			resolved := inc.ResolvedAt.UTC().Format(time.RFC3339)
			item.ResolvedAt = &resolved
		}
		items = append(items, item)
	}
	return items
}
