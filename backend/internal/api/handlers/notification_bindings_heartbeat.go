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

// HeartbeatBindingHandler provides CRUD for heartbeat-channel notification
// bindings. It mirrors NotificationBindingHandler but validates triggers against
// the heartbeat vocabulary {heartbeat_missed, heartbeat_recovered} via
// notification.ValidateHeartbeatTriggers.
type HeartbeatBindingHandler struct {
	queries *db.Queries
}

// NewHeartbeatBindingHandler creates a handler with the given query layer.
func NewHeartbeatBindingHandler(queries *db.Queries) *HeartbeatBindingHandler {
	return &HeartbeatBindingHandler{queries: queries}
}

// Register mounts heartbeat binding routes on the given router group.
func (h *HeartbeatBindingHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/heartbeats/:id/notification-bindings", h.Create)
	rg.GET("/heartbeats/:id/notification-bindings", h.List)
	rg.PUT("/heartbeats/:id/notification-bindings/:bindingId", h.Update)
	rg.DELETE("/heartbeats/:id/notification-bindings/:bindingId", h.Delete)
}

type heartbeatBindingResponse struct {
	ID                      uuid.UUID                       `json:"id"`
	ChannelID               uuid.UUID                       `json:"channel_id"`
	HeartbeatID             uuid.UUID                       `json:"heartbeat_id"`
	Triggers                []notification.TriggerCondition `json:"triggers"`
	ReminderIntervalMinutes *int32                          `json:"reminder_interval_minutes,omitempty"`
	CreatedAt               time.Time                       `json:"created_at"`
	UpdatedAt               time.Time                       `json:"updated_at"`
}

type heartbeatBindingListResponse struct {
	Data       []heartbeatBindingResponse `json:"data"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	Limit      int                        `json:"limit"`
	TotalPages int                        `json:"total_pages"`
}

// Create handles POST /heartbeats/:id/notification-bindings.
func (h *HeartbeatBindingHandler) Create(c *gin.Context) {
	heartbeatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "heartbeat id must be a valid UUID")
		return
	}

	var req createBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	if req.ChannelID == uuid.Nil {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "channel_id is required", []notification.FieldError{
			{Field: "channel_id", Message: "is required"},
		})
		return
	}

	if fieldErrs := notification.ValidateHeartbeatTriggers(req.Triggers); len(fieldErrs) > 0 {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "trigger validation failed", fieldErrs)
		return
	}

	if ferr := validateReminderInterval(req.ReminderIntervalMinutes); ferr != nil {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid reminder interval", []notification.FieldError{*ferr})
		return
	}

	ctx := c.Request.Context()

	if _, err := h.queries.GetHeartbeat(ctx, heartbeatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify heartbeat")
		return
	}

	if _, err := h.queries.GetChannel(ctx, req.ChannelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify channel")
		return
	}

	triggersJSON, err := json.Marshal(req.Triggers)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encode triggers")
		return
	}

	binding, err := h.queries.CreateHeartbeatBinding(ctx, db.CreateHeartbeatBindingParams{
		ChannelID:               req.ChannelID,
		HeartbeatID:             heartbeatID,
		Triggers:                triggersJSON,
		ReminderIntervalMinutes: toInt32Ptr(req.ReminderIntervalMinutes),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			apiError(c, http.StatusConflict, "CONFLICT", "a binding already exists for this channel and heartbeat")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create binding")
		return
	}

	c.JSON(http.StatusCreated, toHeartbeatBindingResponse(binding))
}

// List handles GET /heartbeats/:id/notification-bindings.
func (h *HeartbeatBindingHandler) List(c *gin.Context) {
	heartbeatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "heartbeat id must be a valid UUID")
		return
	}

	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)
	ctx := c.Request.Context()

	if _, err := h.queries.GetHeartbeat(ctx, heartbeatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to verify heartbeat")
		return
	}

	bindings, err := h.queries.ListBindingsByHeartbeatPaginated(ctx, db.ListBindingsByHeartbeatPaginatedParams{
		HeartbeatID: heartbeatID,
		Limit:       int32(limit),
		Offset:      offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list bindings")
		return
	}

	total, err := h.queries.CountBindingsByHeartbeat(ctx, heartbeatID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count bindings")
		return
	}

	data := make([]heartbeatBindingResponse, 0, len(bindings))
	for _, b := range bindings {
		data = append(data, toHeartbeatBindingResponse(b))
	}

	c.JSON(http.StatusOK, heartbeatBindingListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: pageCount(total, limit),
	})
}

// Update handles PUT /heartbeats/:id/notification-bindings/:bindingId.
func (h *HeartbeatBindingHandler) Update(c *gin.Context) {
	heartbeatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "heartbeat id must be a valid UUID")
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

	if fieldErrs := notification.ValidateHeartbeatTriggers(req.Triggers); len(fieldErrs) > 0 {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "trigger validation failed", fieldErrs)
		return
	}

	if ferr := validateReminderInterval(req.ReminderIntervalMinutes); ferr != nil {
		apiErrorWithDetails(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid reminder interval", []notification.FieldError{*ferr})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.queries.GetHeartbeatBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get binding")
		return
	}

	if existing.HeartbeatID != heartbeatID {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
		return
	}

	triggersJSON, err := json.Marshal(req.Triggers)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encode triggers")
		return
	}

	binding, err := h.queries.UpdateHeartbeatBinding(ctx, db.UpdateHeartbeatBindingParams{
		ID:                      bindingID,
		Triggers:                triggersJSON,
		ReminderIntervalMinutes: toInt32Ptr(req.ReminderIntervalMinutes),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to update binding")
		return
	}

	c.JSON(http.StatusOK, toHeartbeatBindingResponse(binding))
}

// Delete handles DELETE /heartbeats/:id/notification-bindings/:bindingId.
func (h *HeartbeatBindingHandler) Delete(c *gin.Context) {
	heartbeatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "heartbeat id must be a valid UUID")
		return
	}

	bindingID, err := uuid.Parse(c.Param("bindingId"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "binding id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	existing, err := h.queries.GetHeartbeatBinding(ctx, bindingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get binding")
		return
	}

	if existing.HeartbeatID != heartbeatID {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "binding not found")
		return
	}

	if err := h.queries.DeleteHeartbeatBinding(ctx, bindingID); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete binding")
		return
	}

	c.Status(http.StatusNoContent)
}

// --- Helpers ---

// validateReminderInterval checks the optional reminder interval against the
// [30, 1440] minute bounds, returning a field error when out of range.
func validateReminderInterval(v *int) *notification.FieldError {
	if v == nil {
		return nil
	}
	if *v < minReminderInterval || *v > maxReminderInterval {
		return &notification.FieldError{Field: "reminder_interval_minutes", Message: "must be between 30 and 1440"}
	}
	return nil
}

// toInt32Ptr converts an optional int to an optional int32 for the DB layer.
func toInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// toHeartbeatBindingResponse maps a DB heartbeat binding to the API response.
func toHeartbeatBindingResponse(b db.HeartbeatBinding) heartbeatBindingResponse {
	var triggers []notification.TriggerCondition
	_ = json.Unmarshal(b.Triggers, &triggers)

	return heartbeatBindingResponse{
		ID:                      b.ID,
		ChannelID:               b.ChannelID,
		HeartbeatID:             b.HeartbeatID,
		Triggers:                triggers,
		ReminderIntervalMinutes: b.ReminderIntervalMinutes,
		CreatedAt:               b.CreatedAt,
		UpdatedAt:               b.UpdatedAt,
	}
}
