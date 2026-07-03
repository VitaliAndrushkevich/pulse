package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// templateVariable represents a single template variable in the reference response.
type templateVariable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// templateVariableGroup groups template variables by their dot-notation prefix.
type templateVariableGroup struct {
	Name      string             `json:"name"`
	Variables []templateVariable `json:"variables"`
}

// templateVariablesResponse is the top-level response for the template variables endpoint.
type templateVariablesResponse struct {
	Groups []templateVariableGroup `json:"groups"`
}

// templateVariablesData is the static reference data for all available template variables.
var templateVariablesData = templateVariablesResponse{
	Groups: []templateVariableGroup{
		{
			Name: "monitor",
			Variables: []templateVariable{
				{
					Name:        "monitor.Name",
					Type:        "string",
					Description: "Monitor display name",
					Example:     "API Health Check",
				},
				{
					Name:        "monitor.URL",
					Type:        "string",
					Description: "Monitor target URL",
					Example:     "https://api.example.com/health",
				},
				{
					Name:        "monitor.Target",
					Type:        "string",
					Description: "Monitor target address",
					Example:     "api.example.com",
				},
			},
		},
		{
			Name: "Incident",
			Variables: []templateVariable{
				{
					Name:        "Incident.StartedAt",
					Type:        "time.Time",
					Description: "When the incident started (RFC3339)",
					Example:     "2024-01-15T10:30:00Z",
				},
				{
					Name:        "Incident.Duration",
					Type:        "time.Duration",
					Description: "Duration of the incident",
					Example:     "5m30s",
				},
				{
					Name:        "Incident.ID",
					Type:        "uuid.UUID",
					Description: "Unique incident identifier",
					Example:     "550e8400-e29b-41d4-a716-446655440000",
				},
			},
		},
		{
			Name: "",
			Variables: []templateVariable{
				{
					Name:        "Status",
					Type:        "string",
					Description: "Current monitor status",
					Example:     "down",
				},
				{
					Name:        "PreviousStatus",
					Type:        "string",
					Description: "Previous monitor status before change",
					Example:     "up",
				},
				{
					Name:        "ResponseTime",
					Type:        "int32",
					Description: "Response time in milliseconds",
					Example:     "1250",
				},
				{
					Name:        "Timestamp",
					Type:        "time.Time",
					Description: "Notification timestamp (RFC3339)",
					Example:     "2024-01-15T10:35:30Z",
				},
			},
		},
	},
}

// TemplateVariables handles GET /notifications/template-variables.
// It returns a static reference of all available webhook template variables
// grouped by their dot-notation prefix.
func (h *NotificationChannelHandler) TemplateVariables(c *gin.Context) {
	c.JSON(http.StatusOK, templateVariablesData)
}
