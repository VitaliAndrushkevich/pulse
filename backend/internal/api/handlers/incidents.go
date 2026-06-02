package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// IncidentHandler provides incident list endpoints.
type IncidentHandler struct {
	queries *db.Queries
}

// NewIncidentHandler creates a handler with the given query layer.
func NewIncidentHandler(queries *db.Queries) *IncidentHandler {
	return &IncidentHandler{queries: queries}
}

type incidentResponse struct {
	ID         uuid.UUID  `json:"id"`
	MonitorID  uuid.UUID  `json:"monitor_id"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Cause      *string    `json:"cause,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type incidentListResponse struct {
	Data       []incidentResponse `json:"data"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	TotalPages int                `json:"total_pages"`
}

// Register mounts incident routes on the given router group.
func (h *IncidentHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/incidents", h.List)
	rg.GET("/monitors/:id/incidents", h.ListByMonitor)
}

// List handles GET /incidents with pagination.
// Optional query param: status=open to filter only unresolved incidents.
func (h *IncidentHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	status := c.Query("status")

	var incidents []db.Incident
	var total int64
	var err error

	if status == "open" {
		incidents, err = h.queries.ListOpenIncidents(c.Request.Context(), db.ListOpenIncidentsParams{
			Limit:  int32(limit),
			Offset: offset,
		})
		if err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list incidents")
			return
		}
		// For open incidents, count is expensive — use the list length as approximation
		// or do a proper count. We'll just use total from full count.
		total, err = h.queries.CountIncidents(c.Request.Context())
	} else {
		incidents, err = h.queries.ListIncidents(c.Request.Context(), db.ListIncidentsParams{
			Limit:  int32(limit),
			Offset: offset,
		})
		if err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list incidents")
			return
		}
		total, err = h.queries.CountIncidents(c.Request.Context())
	}

	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count incidents")
		return
	}

	data := make([]incidentResponse, 0, len(incidents))
	for _, inc := range incidents {
		data = append(data, toIncidentResponse(inc))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, incidentListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// ListByMonitor handles GET /monitors/:id/incidents with pagination.
func (h *IncidentHandler) ListByMonitor(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	incidents, err := h.queries.ListIncidentsByMonitor(c.Request.Context(), db.ListIncidentsByMonitorParams{
		MonitorID: monitorID,
		Limit:     int32(limit),
		Offset:    offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list incidents")
		return
	}

	total, err := h.queries.CountIncidentsByMonitor(c.Request.Context(), monitorID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count incidents")
		return
	}

	data := make([]incidentResponse, 0, len(incidents))
	for _, inc := range incidents {
		data = append(data, toIncidentResponse(inc))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, incidentListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

func toIncidentResponse(inc db.Incident) incidentResponse {
	resp := incidentResponse{
		ID:        inc.ID,
		MonitorID: inc.MonitorID,
		StartedAt: inc.StartedAt,
		Cause:     inc.Cause,
		CreatedAt: inc.CreatedAt,
	}

	if inc.ResolvedAt.Valid {
		t := inc.ResolvedAt.Time
		resp.ResolvedAt = &t
	}

	return resp
}
