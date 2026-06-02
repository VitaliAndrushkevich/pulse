package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// MonitorHandler provides CRUD operations for monitors.
type MonitorHandler struct {
	queries *db.Queries
}

// NewMonitorHandler creates a handler with the given query layer.
func NewMonitorHandler(queries *db.Queries) *MonitorHandler {
	return &MonitorHandler{queries: queries}
}

// --- Request/Response types ---

type createMonitorRequest struct {
	Name            string          `json:"name" binding:"required"`
	Type            string          `json:"type" binding:"required"`
	Target          string          `json:"target" binding:"required"`
	IntervalSeconds *int32          `json:"interval_seconds,omitempty"`
	TimeoutSeconds  *int32          `json:"timeout_seconds,omitempty"`
	Status          *string         `json:"status,omitempty"`
	Settings        json.RawMessage `json:"settings,omitempty"`
}

type putMonitorRequest struct {
	Name            string          `json:"name" binding:"required"`
	Type            string          `json:"type" binding:"required"`
	Target          string          `json:"target" binding:"required"`
	IntervalSeconds *int32          `json:"interval_seconds,omitempty"`
	TimeoutSeconds  *int32          `json:"timeout_seconds,omitempty"`
	Status          *string         `json:"status,omitempty"`
	Settings        json.RawMessage `json:"settings,omitempty"`
}

type monitorResponse struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	Target          string          `json:"target"`
	IntervalSeconds int32           `json:"interval_seconds"`
	TimeoutSeconds  int32           `json:"timeout_seconds"`
	Status          string          `json:"status"`
	State           string          `json:"state"`
	LastCheckedAt   *time.Time      `json:"last_checked_at,omitempty"`
	NextCheckAt     *time.Time      `json:"next_check_at,omitempty"`
	Settings        json.RawMessage `json:"settings"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type monitorListResponse struct {
	Data       []monitorResponse `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalPages int               `json:"total_pages"`
}

// --- Route registration ---

// Register mounts monitor routes on the given router group.
func (h *MonitorHandler) Register(rg *gin.RouterGroup) {
	monitors := rg.Group("/monitors")
	monitors.POST("", h.Create)
	monitors.GET("", h.List)
	monitors.GET("/:id", h.Get)
	monitors.PUT("/:id", h.Put)
	monitors.DELETE("/:id", h.Delete)
}

// --- Handlers ---

// Create handles POST /monitors.
func (h *MonitorHandler) Create(c *gin.Context) {
	var req createMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name, type, and target are required")
		return
	}

	if !isValidMonitorType(req.Type) {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, https, tcp, udp, websocket")
		return
	}

	interval := int32(60)
	if req.IntervalSeconds != nil && *req.IntervalSeconds > 0 {
		interval = *req.IntervalSeconds
	}

	timeout := int32(10)
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
		timeout = *req.TimeoutSeconds
	}

	status := "active"
	if req.Status != nil && isValidStatus(*req.Status) {
		status = *req.Status
	}

	settings := json.RawMessage("{}")
	if req.Settings != nil {
		settings = req.Settings
	}

	m, err := h.queries.CreateMonitor(c.Request.Context(), db.CreateMonitorParams{
		Name:            req.Name,
		Type:            req.Type,
		Target:          req.Target,
		IntervalSeconds: interval,
		TimeoutSeconds:  timeout,
		Status:          status,
		State:           "unknown",
		Settings:        settings,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create monitor")
		return
	}

	c.JSON(http.StatusCreated, toMonitorResponse(m))
}

// List handles GET /monitors with pagination.
func (h *MonitorHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	monitors, err := h.queries.ListMonitors(c.Request.Context(), db.ListMonitorsParams{
		Limit:  int32(limit),
		Offset: offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list monitors")
		return
	}

	total, err := h.queries.CountMonitors(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count monitors")
		return
	}

	data := make([]monitorResponse, 0, len(monitors))
	for _, m := range monitors {
		data = append(data, toMonitorResponse(m))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, monitorListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Get handles GET /monitors/:id.
func (h *MonitorHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	m, err := h.queries.GetMonitor(c.Request.Context(), id)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
		return
	}

	c.JSON(http.StatusOK, toMonitorResponse(m))
}

// Put handles PUT /monitors/:id with idempotent create-or-update semantics.
func (h *MonitorHandler) Put(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	var req putMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name, type, and target are required")
		return
	}

	if !isValidMonitorType(req.Type) {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, https, tcp, udp, websocket")
		return
	}

	interval := int32(60)
	if req.IntervalSeconds != nil && *req.IntervalSeconds > 0 {
		interval = *req.IntervalSeconds
	}

	timeout := int32(10)
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
		timeout = *req.TimeoutSeconds
	}

	status := "active"
	if req.Status != nil && isValidStatus(*req.Status) {
		status = *req.Status
	}

	settings := json.RawMessage("{}")
	if req.Settings != nil {
		settings = req.Settings
	}

	m, err := h.queries.UpsertMonitor(c.Request.Context(), db.UpsertMonitorParams{
		ID:              id,
		Name:            req.Name,
		Type:            req.Type,
		Target:          req.Target,
		IntervalSeconds: interval,
		TimeoutSeconds:  timeout,
		Status:          status,
		Settings:        settings,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create or update monitor")
		return
	}

	c.JSON(http.StatusOK, toMonitorResponse(m))
}

// Delete handles DELETE /monitors/:id.
func (h *MonitorHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	if err := h.queries.DeleteMonitor(c.Request.Context(), id); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete monitor")
		return
	}

	c.Status(http.StatusNoContent)
}

// --- Helpers ---

func isValidMonitorType(t string) bool {
	switch t {
	case "http", "https", "tcp", "udp", "websocket":
		return true
	}
	return false
}

func isValidStatus(s string) bool {
	return s == "active" || s == "paused"
}

func toMonitorResponse(m db.Monitor) monitorResponse {
	resp := monitorResponse{
		ID:              m.ID,
		Name:            m.Name,
		Type:            m.Type,
		Target:          m.Target,
		IntervalSeconds: m.IntervalSeconds,
		TimeoutSeconds:  m.TimeoutSeconds,
		Status:          m.Status,
		State:           m.State,
		Settings:        m.Settings,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}

	if m.LastCheckedAt.Valid {
		t := m.LastCheckedAt.Time
		resp.LastCheckedAt = &t
	}
	if m.NextCheckAt.Valid {
		t := m.NextCheckAt.Time
		resp.NextCheckAt = &t
	}

	return resp
}
