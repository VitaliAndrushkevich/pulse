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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/tags"
)

// MonitorHandler provides CRUD operations for monitors.
type MonitorHandler struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	hub     *hub.Hub // WebSocket broadcast hub (may be nil)
}

// NewMonitorHandler creates a handler with the given query layer, connection pool, and WebSocket hub.
func NewMonitorHandler(queries *db.Queries, pool *pgxpool.Pool, wsHub *hub.Hub) *MonitorHandler {
	return &MonitorHandler{queries: queries, pool: pool, hub: wsHub}
}

// --- Request/Response types ---

type createMonitorRequest struct {
	Name            string            `json:"name" binding:"required"`
	Type            string            `json:"type" binding:"required"`
	Target          string            `json:"target" binding:"required"`
	IntervalSeconds *int32            `json:"interval_seconds,omitempty"`
	TimeoutSeconds  *int32            `json:"timeout_seconds,omitempty"`
	Status          *string           `json:"status,omitempty"`
	Settings        json.RawMessage   `json:"settings,omitempty"`
	Tags            []tags.TagRequest `json:"tags,omitempty"`
}

type putMonitorRequest struct {
	Name            string            `json:"name" binding:"required"`
	Type            string            `json:"type" binding:"required"`
	Target          string            `json:"target" binding:"required"`
	IntervalSeconds *int32            `json:"interval_seconds,omitempty"`
	TimeoutSeconds  *int32            `json:"timeout_seconds,omitempty"`
	Status          *string           `json:"status,omitempty"`
	Settings        json.RawMessage   `json:"settings,omitempty"`
	Tags            []tags.TagRequest `json:"tags,omitempty"`
}

type tagResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
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
	Tags            []tagResponse   `json:"tags"`
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
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, http3, tcp, udp, websocket, grpc")
		return
	}

	// Validate tags if provided.
	if req.Tags != nil {
		if err := tags.ValidateTags(req.Tags); err != nil {
			apiError(c, http.StatusBadRequest, "INVALID_TAGS", err.Error())
			return
		}
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

	ctx := c.Request.Context()

	// Use a transaction to persist the monitor and its tags atomically.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to begin transaction")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	m, err := qtx.CreateMonitor(ctx, db.CreateMonitorParams{
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			// CHECK constraint violated: commonly caused by DB enum/check not
			// including the new type (e.g., http3) because migrations weren't applied.
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, http3, tcp, udp, websocket, grpc (ensure database migrations are applied)")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create monitor")
		return
	}

	// Persist tags within the same transaction.
	if len(req.Tags) > 0 {
		if err := qtx.DeleteMonitorTags(ctx, m.ID); err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to set monitor tags")
			return
		}
		for _, tag := range req.Tags {
			if err := qtx.InsertMonitorTag(ctx, db.InsertMonitorTagParams{
				MonitorID: m.ID,
				Key:       tag.Key,
				Value:     tag.Value,
			}); err != nil {
				apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to set monitor tags")
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to commit transaction")
		return
	}

	// Fetch persisted tags to include in response.
	monitorTags, err := h.queries.ListTagsByMonitor(ctx, m.ID)
	if err != nil {
		// Monitor was created successfully; return without tags rather than failing.
		monitorTags = []db.MonitorTag{}
	}

	// Broadcast tag change notification via WebSocket if tags were provided.
	if h.hub != nil && len(req.Tags) > 0 {
		h.hub.Broadcast(hub.NewMonitorTagsChangedMessage(m.ID.String(), toHubTagInfo(monitorTags)))
	}

	c.JSON(http.StatusCreated, toMonitorResponseWithTags(m, monitorTags))
}

// List handles GET /monitors with pagination and optional type/tag filters.
func (h *MonitorHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)

	// Enforce max page size of 100.
	if limit > 100 {
		limit = 100
	}

	offset := int32((page - 1) * limit)
	ctx := c.Request.Context()

	// Parse optional type filter.
	typeFilter := c.Query("type")

	// Parse optional tag filters (format: key:value, AND semantics).
	tagFilters := c.QueryArray("tag")
	if tagFilters == nil {
		tagFilters = []string{}
	}

	monitors, err := h.queries.ListMonitorsFiltered(ctx, db.ListMonitorsFilteredParams{
		Column1: typeFilter,
		Column2: tagFilters,
		Limit:   int32(limit),
		Offset:  offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list monitors")
		return
	}

	total, err := h.queries.CountMonitorsFiltered(ctx, db.CountMonitorsFilteredParams{
		Column1: typeFilter,
		Column2: tagFilters,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count monitors")
		return
	}

	data := make([]monitorResponse, 0, len(monitors))
	for _, m := range monitors {
		monitorTags, err := h.queries.ListTagsByMonitor(ctx, m.ID)
		if err != nil {
			monitorTags = []db.MonitorTag{}
		}
		data = append(data, toMonitorResponseWithTags(m, monitorTags))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
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

	ctx := c.Request.Context()

	m, err := h.queries.GetMonitor(ctx, id)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "monitor not found")
		return
	}

	monitorTags, err := h.queries.ListTagsByMonitor(ctx, m.ID)
	if err != nil {
		monitorTags = []db.MonitorTag{}
	}

	c.JSON(http.StatusOK, toMonitorResponseWithTags(m, monitorTags))
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
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, http3, tcp, udp, websocket, grpc")
		return
	}

	// Validate tags if provided.
	if req.Tags != nil {
		if err := tags.ValidateTags(req.Tags); err != nil {
			apiError(c, http.StatusBadRequest, "INVALID_TAGS", err.Error())
			return
		}
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

	ctx := c.Request.Context()

	// Use a transaction with row-level locking for concurrent modification safety.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to begin transaction")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	// Acquire row-level lock on the monitor (SELECT FOR UPDATE).
	// This prevents concurrent modifications from corrupting tag state.
	_, lockErr := qtx.GetMonitorForUpdate(ctx, id)
	if lockErr != nil && !errors.Is(lockErr, pgx.ErrNoRows) {
		// Check for lock conflict / serialization failure.
		var pgErr *pgconn.PgError
		if errors.As(lockErr, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "55P03") {
			apiError(c, http.StatusConflict, "CONFLICT", "concurrent modification detected, please retry")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to lock monitor for update")
		return
	}

	// Upsert the monitor.
	m, err := qtx.UpsertMonitor(ctx, db.UpsertMonitorParams{
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "type must be one of: http, http3, tcp, udp, websocket, grpc (ensure database migrations are applied)")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create or update monitor")
		return
	}

	// Replace tags within the same transaction.
	if err := qtx.DeleteMonitorTags(ctx, m.ID); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to replace monitor tags")
		return
	}
	for _, tag := range req.Tags {
		if err := qtx.InsertMonitorTag(ctx, db.InsertMonitorTagParams{
			MonitorID: m.ID,
			Key:       tag.Key,
			Value:     tag.Value,
		}); err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to replace monitor tags")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		// Check for serialization failure on commit.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01") {
			apiError(c, http.StatusConflict, "CONFLICT", "concurrent modification detected, please retry")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to commit transaction")
		return
	}

	// Fetch persisted tags to include in response.
	monitorTags, err := h.queries.ListTagsByMonitor(ctx, m.ID)
	if err != nil {
		monitorTags = []db.MonitorTag{}
	}

	// Broadcast tag change notification via WebSocket.
	// Always broadcast on PUT since tags are fully replaced (even if empty).
	if h.hub != nil {
		h.hub.Broadcast(hub.NewMonitorTagsChangedMessage(m.ID.String(), toHubTagInfo(monitorTags)))
	}

	c.JSON(http.StatusOK, toMonitorResponseWithTags(m, monitorTags))
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
	case "http", "http3", "tcp", "udp", "websocket", "grpc":
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
		Tags:            []tagResponse{},
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

func toMonitorResponseWithTags(m db.Monitor, tags []db.MonitorTag) monitorResponse {
	resp := toMonitorResponse(m)
	if len(tags) > 0 {
		resp.Tags = make([]tagResponse, 0, len(tags))
		for _, t := range tags {
			resp.Tags = append(resp.Tags, tagResponse{Key: t.Key, Value: t.Value})
		}
	}
	return resp
}

// toHubTagInfo converts persisted tags to the hub TagInfo slice for WebSocket broadcast.
func toHubTagInfo(tags []db.MonitorTag) []hub.TagInfo {
	info := make([]hub.TagInfo, 0, len(tags))
	for _, t := range tags {
		info = append(info, hub.TagInfo{Key: t.Key, Value: t.Value})
	}
	return info
}
