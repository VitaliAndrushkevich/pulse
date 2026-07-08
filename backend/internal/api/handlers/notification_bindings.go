package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// NotificationBindingHandler provides CRUD for channel-monitor bindings.
type NotificationBindingHandler struct {
	queries *db.Queries
}

// NewNotificationBindingHandler creates a handler with the given query layer.
func NewNotificationBindingHandler(queries *db.Queries) *NotificationBindingHandler {
	return &NotificationBindingHandler{queries: queries}
}

// Register mounts binding routes on the given router group.
func (h *NotificationBindingHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/monitors/:id/notification-bindings", h.Create)
	rg.GET("/monitors/:id/notification-bindings", h.List)
	rg.PUT("/monitors/:id/notification-bindings/:bindingId", h.Update)
	rg.DELETE("/monitors/:id/notification-bindings/:bindingId", h.Delete)
	rg.GET("/monitors/:id/delivery-logs", h.ListDeliveryLogs)
}

// minReminderInterval is the minimum allowed reminder interval in minutes.
const minReminderInterval = 30

// maxReminderInterval is the maximum allowed reminder interval in minutes (24 hours).
const maxReminderInterval = 1440

type createBindingRequest struct {
	ChannelID               uuid.UUID                      `json:"channel_id"`
	Triggers                []notification.TriggerCondition `json:"triggers"`
	ReminderIntervalMinutes *int                           `json:"reminder_interval_minutes"`
}

type updateBindingRequest struct {
	Triggers                []notification.TriggerCondition `json:"triggers"`
	ReminderIntervalMinutes *int                           `json:"reminder_interval_minutes"`
}

type bindingResponse struct {
	ID                      uuid.UUID                      `json:"id"`
	ChannelID               uuid.UUID                      `json:"channel_id"`
	MonitorID               uuid.UUID                      `json:"monitor_id"`
	Triggers                []notification.TriggerCondition `json:"triggers"`
	ReminderIntervalMinutes *int32                         `json:"reminder_interval_minutes,omitempty"`
	CreatedAt               time.Time                      `json:"created_at"`
	UpdatedAt               time.Time                      `json:"updated_at"`
}

type bindingListResponse struct {
	Data       []bindingResponse `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalPages int               `json:"total_pages"`
}

// Create handles POST /monitors/:id/notification-bindings.
func (h *NotificationBindingHandler) Create(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "monitor id must be a valid UUID")
		return
	}

	var req createBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	// Validate channel_id is present.
	if req.ChannelID == uuid.Nil {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "channel_id is required", []notification.FieldError{
			{Field: "channel_id", Message: "is required"},
		})
		return
	}

	// Validate triggers.
	if fieldErrs := notification.ValidateTriggers(req.Triggers); len(fieldErrs) > 0 {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "trigger validation failed", fieldErrs)
		return
	}

	// Validate reminder_interval_minutes.
	if req.ReminderIntervalMinutes != nil {
		v := *req.ReminderIntervalMinutes
		if v < minReminderInterval || v > maxReminderInterval {
			apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid reminder interval", []notification.FieldError{
				{Field: "reminder_interval_minutes", Message: "must be between 30 and 1440"},
			})
			return
		}
	}

	ctx := c.Request.Context()

	// Verify monitor exists.
	if _, err := h.queries.GetMonitor(ctx, monitorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify monitor")
		return
	}

	// Verify channel exists.
	if _, err := h.queries.GetChannel(ctx, req.ChannelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify channel")
		return
	}

	// Marshal triggers to JSON.
	triggersJSON, err := json.Marshal(req.Triggers)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encode triggers")
		return
	}

	// Convert reminder interval.
	var reminderInterval *int32
	if req.ReminderIntervalMinutes != nil {
		v := int32(*req.ReminderIntervalMinutes)
		reminderInterval = &v
	}

	binding, err := h.queries.CreateBinding(ctx, db.CreateBindingParams{
		ChannelID:               req.ChannelID,
		MonitorID:               monitorID,
		Triggers:                triggersJSON,
		ReminderIntervalMinutes: reminderInterval,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			apiError(c, http.StatusConflict, "CONFLICT", "a binding already exists for this channel and monitor")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create binding")
		return
	}

	c.JSON(http.StatusCreated, toBindingResponse(binding))
}

// List handles GET /monitors/:id/notification-bindings.
func (h *NotificationBindingHandler) List(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "monitor id must be a valid UUID")
		return
	}

	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)
	ctx := c.Request.Context()

	// Verify monitor exists.
	if _, err := h.queries.GetMonitor(ctx, monitorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify monitor")
		return
	}

	bindings, err := h.queries.ListBindingsByMonitorPaginated(ctx, db.ListBindingsByMonitorPaginatedParams{
		MonitorID: monitorID,
		Limit:     int32(limit),
		Offset:    offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list bindings")
		return
	}

	total, err := h.queries.CountBindingsByMonitor(ctx, monitorID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count bindings")
		return
	}

	data := make([]bindingResponse, 0, len(bindings))
	for _, b := range bindings {
		data = append(data, toBindingResponse(b))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, bindingListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Update handles PUT /monitors/:id/notification-bindings/:bindingId.
func (h *NotificationBindingHandler) Update(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "monitor id must be a valid UUID")
		return
	}

	bindingID, err := uuid.Parse(c.Param("bindingId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "binding id must be a valid UUID")
		return
	}

	var req updateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	// Validate triggers.
	if fieldErrs := notification.ValidateTriggers(req.Triggers); len(fieldErrs) > 0 {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "trigger validation failed", fieldErrs)
		return
	}

	// Validate reminder_interval_minutes.
	if req.ReminderIntervalMinutes != nil {
		v := *req.ReminderIntervalMinutes
		if v < minReminderInterval || v > maxReminderInterval {
			apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid reminder interval", []notification.FieldError{
				{Field: "reminder_interval_minutes", Message: "must be between 30 and 1440"},
			})
			return
		}
	}

	ctx := c.Request.Context()

	// Verify monitor exists.
	if _, err := h.queries.GetMonitor(ctx, monitorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify monitor")
		return
	}

	// Verify binding exists and belongs to this monitor.
	existing, err := h.queries.GetBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get binding")
		return
	}

	if existing.MonitorID != monitorID {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
		return
	}

	// Marshal triggers to JSON.
	triggersJSON, err := json.Marshal(req.Triggers)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encode triggers")
		return
	}

	// Convert reminder interval.
	var reminderInterval *int32
	if req.ReminderIntervalMinutes != nil {
		v := int32(*req.ReminderIntervalMinutes)
		reminderInterval = &v
	}

	binding, err := h.queries.UpdateBinding(ctx, db.UpdateBindingParams{
		ID:                      bindingID,
		Triggers:                triggersJSON,
		ReminderIntervalMinutes: reminderInterval,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to update binding")
		return
	}

	c.JSON(http.StatusOK, toBindingResponse(binding))
}

// Delete handles DELETE /monitors/:id/notification-bindings/:bindingId.
func (h *NotificationBindingHandler) Delete(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "monitor id must be a valid UUID")
		return
	}

	bindingID, err := uuid.Parse(c.Param("bindingId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "binding id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	// Verify binding exists and belongs to this monitor.
	existing, err := h.queries.GetBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get binding")
		return
	}

	if existing.MonitorID != monitorID {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
		return
	}

	if err := h.queries.DeleteBinding(ctx, bindingID); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete binding")
		return
	}

	c.Status(http.StatusNoContent)
}

// apiErrorWithDetails writes a standardized error response with field-level details.
func apiErrorWithDetails(c *gin.Context, status int, code, message string, details []notification.FieldError) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
			"details": details,
		},
	})
}

// toBindingResponse converts a DB binding model to the API response.
func toBindingResponse(b db.ChannelBinding) bindingResponse {
	var triggers []notification.TriggerCondition
	_ = json.Unmarshal(b.Triggers, &triggers)

	return bindingResponse{
		ID:                      b.ID,
		ChannelID:               b.ChannelID,
		MonitorID:               b.MonitorID,
		Triggers:                triggers,
		ReminderIntervalMinutes: b.ReminderIntervalMinutes,
		CreatedAt:               b.CreatedAt,
		UpdatedAt:               b.UpdatedAt,
	}
}

// ListDeliveryLogs handles GET /monitors/:id/delivery-logs.
// Returns a paginated list of delivery log entries for a given monitor.
func (h *NotificationBindingHandler) ListDeliveryLogs(c *gin.Context) {
	monitorID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "monitor id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	// Verify monitor exists.
	if _, err := h.queries.GetMonitor(ctx, monitorID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify monitor")
		return
	}

	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	logs, err := h.queries.ListDeliveryLogsByMonitor(ctx, db.ListDeliveryLogsByMonitorParams{
		MonitorID: monitorID,
		Limit:     int32(limit),
		Offset:    offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list delivery logs")
		return
	}

	total, err := h.queries.CountDeliveryLogsByMonitor(ctx, monitorID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count delivery logs")
		return
	}

	type deliveryLogEntry struct {
		ID          uuid.UUID  `json:"id"`
		ChannelID   uuid.UUID  `json:"channel_id"`
		MonitorID   uuid.UUID  `json:"monitor_id"`
		BindingID   *uuid.UUID `json:"binding_id,omitempty"`
		TriggerType string     `json:"trigger_type"`
		Attempt     int32      `json:"attempt"`
		Status      string     `json:"status"`
		ErrorDetail *string    `json:"error_detail,omitempty"`
		CreatedAt   time.Time  `json:"created_at"`
	}

	data := make([]deliveryLogEntry, 0, len(logs))
	for _, l := range logs {
		entry := deliveryLogEntry{
			ID:          l.ID,
			ChannelID:   l.ChannelID,
			MonitorID:   l.MonitorID,
			TriggerType: l.TriggerType,
			Attempt:     l.Attempt,
			Status:      l.Status,
			ErrorDetail: l.ErrorDetail,
			CreatedAt:   l.CreatedAt,
		}
		if l.BindingID.Valid {
			id := uuid.UUID(l.BindingID.Bytes)
			entry.BindingID = &id
		}
		data = append(data, entry)
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        data,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}
