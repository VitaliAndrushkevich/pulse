package pulseapi

import "context"

// PulseClient is the interface through which the MCP server interacts with Pulse.
// All Pulse access flows through this interface, keeping tool handlers testable
// and centralizing auth, timeout, error-envelope parsing, and X-Request-ID handling.
type PulseClient interface {
	// ListMonitors returns a paginated list of monitors matching the query filters.
	ListMonitors(ctx context.Context, q MonitorQuery) (MonitorPage, error)

	// GetMonitor returns the full configuration and current status of a single monitor.
	GetMonitor(ctx context.Context, id string) (Monitor, error)

	// GetMonitorStats returns computed statistics for a monitor, including uptime and SSL info.
	GetMonitorStats(ctx context.Context, id string) (MonitorStats, error)

	// GetMonitorHistory returns check history points for a monitor within the given time range.
	GetMonitorHistory(ctx context.Context, id string, r TimeRange) (History, error)

	// ListIncidents returns a paginated list of incidents matching the query filters.
	ListIncidents(ctx context.Context, q IncidentQuery) (IncidentPage, error)

	// CreateMonitor creates a new monitor and returns the created resource.
	CreateMonitor(ctx context.Context, in CreateMonitorInput) (Monitor, error)
}
