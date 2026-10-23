package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/VitaliAndrushkevich/pulse/internal/notification/native"
	smtpclient "github.com/VitaliAndrushkevich/pulse/internal/notification/smtp"
	"github.com/VitaliAndrushkevich/pulse/internal/notification/webhook"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// emailRegex is a basic RFC 5322 email format validation pattern.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// validWebhookMethods is the set of accepted HTTP methods for webhook channels.
var validWebhookMethods = map[string]bool{
	"GET":    true,
	"POST":   true,
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

// NotificationChannelHandler provides CRUD operations for notification channels.
type NotificationChannelHandler struct {
	queries       *db.Queries
	pool          *pgxpool.Pool
	secretKey     []byte
	webhookClient *webhook.Client
	smtpClient    *smtpclient.Client // nil if SMTP not configured
	smtpMu        sync.RWMutex
	baseURL       string // public base URL for notification links
	testSlots     chan struct{}
}

// NewNotificationChannelHandler creates a handler for notification channel operations.
func NewNotificationChannelHandler(queries *db.Queries, secretKey []byte, baseURL string) *NotificationChannelHandler {
	return &NotificationChannelHandler{
		queries:       queries,
		secretKey:     secretKey,
		webhookClient: webhook.NewClient(secretKey),
		baseURL:       baseURL,
		testSlots:     make(chan struct{}, 4),
	}
}

// SetPool enables atomic secret-preserving updates in production.
func (h *NotificationChannelHandler) SetPool(pool *pgxpool.Pool) { h.pool = pool }

// SetSMTPClient sets the SMTP client used for test email delivery.
// This is optional — if not set, test notifications for email channels will
// return an error indicating SMTP is not configured.
func (h *NotificationChannelHandler) SetSMTPClient(client *smtpclient.Client) {
	h.smtpMu.Lock()
	defer h.smtpMu.Unlock()
	h.smtpClient = client
}

// getSMTPClient returns the current SMTP client (thread-safe).
func (h *NotificationChannelHandler) getSMTPClient() *smtpclient.Client {
	h.smtpMu.RLock()
	defer h.smtpMu.RUnlock()
	return h.smtpClient
}

// Register mounts notification channel routes on the given router group.
func (h *NotificationChannelHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/notifications/channels", h.Create)
	rg.POST("/notifications/channels/test", h.TestDraft)
	rg.GET("/notifications/channels", h.List)
	rg.GET("/notifications/channels/:id", h.Get)
	rg.PUT("/notifications/channels/:id", h.Update)
	rg.DELETE("/notifications/channels/:id", h.Delete)
	rg.POST("/notifications/channels/:id/test", h.Test)
	rg.GET("/notifications/channels/:id/delivery-logs", h.ListDeliveryLogs)

	// Template variable reference
	rg.GET("/notifications/template-variables", h.TemplateVariables)

	// SMTP settings (UI-managed)
	rg.GET("/notifications/smtp-settings", h.GetSMTPSettings)
	rg.PUT("/notifications/smtp-settings", h.UpdateSMTPSettings)
	rg.DELETE("/notifications/smtp-settings", h.DeleteSMTPSettings)
	rg.POST("/notifications/smtp-settings/test", h.TestSMTPConnection)
}

// --- Request/Response types ---

type channelRequest struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
}

type emailConfig struct {
	Recipients []string `json:"recipients"`
}

type webhookHeader struct {
	Name            string  `json:"name"`
	Value           *string `json:"value,omitempty"`
	ValueConfigured bool    `json:"value_configured,omitempty"`
}

type webhookConfig struct {
	URL          string          `json:"url"`
	Method       string          `json:"method"`
	BodyTemplate string          `json:"body_template"`
	Headers      []webhookHeader `json:"headers"`
}

type channelResponse struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type channelListResponse struct {
	Data       []channelResponse `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalPages int               `json:"total_pages"`
}

type deliveryLogResponse struct {
	ID          uuid.UUID  `json:"id"`
	ChannelID   uuid.UUID  `json:"channel_id"`
	MonitorID   uuid.UUID  `json:"monitor_id"`
	BindingID   *uuid.UUID `json:"binding_id"`
	TriggerType string     `json:"trigger_type"`
	Attempt     int32      `json:"attempt"`
	Status      string     `json:"status"`
	ErrorDetail *string    `json:"error_detail"`
	CreatedAt   time.Time  `json:"created_at"`
}

type deliveryLogListResponse struct {
	Data       []deliveryLogResponse `json:"data"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	TotalPages int                   `json:"total_pages"`
}

// --- Handlers ---

// Create handles POST /notifications/channels.
func (h *NotificationChannelHandler) Create(c *gin.Context) {
	var req channelRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	errs := h.validateChannelRequest(req)
	if len(errs) > 0 {
		apiValidationError(c, "channel configuration is invalid", errs)
		return
	}

	// Process config: encrypt header values for webhook channels.
	configJSON, err := h.processConfigForStorage(req.Type, req.Config, nil)
	if err != nil {
		apiValidationError(c, "channel configuration is invalid", []fieldError{{Field: "config", Message: err.Error()}})
		return
	}

	ctx := c.Request.Context()
	channel, err := h.queries.CreateChannel(ctx, db.CreateChannelParams{
		Name:        req.Name,
		ChannelType: req.Type,
		Config:      configJSON,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create channel")
		return
	}

	c.JSON(http.StatusCreated, toChannelResponse(channel, h.secretKey))
}

// List handles GET /notifications/channels.
func (h *NotificationChannelHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)
	ctx := c.Request.Context()

	channels, err := h.queries.ListChannels(ctx, db.ListChannelsParams{
		Limit:  int32(limit),
		Offset: offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list channels")
		return
	}

	total, err := h.queries.CountChannels(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count channels")
		return
	}

	data := make([]channelResponse, 0, len(channels))
	for _, ch := range channels {
		data = append(data, toChannelResponse(ch, h.secretKey))
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, channelListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Get handles GET /notifications/channels/:id.
func (h *NotificationChannelHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()
	channel, err := h.queries.GetChannel(ctx, id)
	if err != nil {
		if isNotFound(err) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
		return
	}

	c.JSON(http.StatusOK, toChannelResponse(channel, h.secretKey))
}

// Update handles PUT /notifications/channels/:id.
func (h *NotificationChannelHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	var req channelRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	errs := h.validateChannelRequest(req)
	if len(errs) > 0 {
		apiValidationError(c, "channel configuration is invalid", errs)
		return
	}

	// Verify channel exists before updating.
	ctx := c.Request.Context()
	q := h.queries
	var tx pgx.Tx
	var existing db.NotificationChannel
	if h.pool != nil {
		tx, err = h.pool.Begin(ctx)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to begin channel update")
			return
		}
		defer tx.Rollback(ctx)
		q = h.queries.WithTx(tx)
		existing, err = q.GetChannelForUpdate(ctx, id)
	} else {
		existing, err = q.GetChannel(ctx, id)
	}
	if err != nil {
		if isNotFound(err) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
		return
	}

	// Process config: encrypt header values for webhook channels.
	var previous json.RawMessage
	if existing.ChannelType == req.Type {
		previous = existing.Config
	}
	configJSON, err := h.processConfigForStorage(req.Type, req.Config, previous)
	if err != nil {
		apiValidationError(c, "channel configuration is invalid", []fieldError{{Field: "config", Message: err.Error()}})
		return
	}

	channel, err := q.UpdateChannel(ctx, db.UpdateChannelParams{
		ID:          id,
		Name:        req.Name,
		ChannelType: req.Type,
		Config:      configJSON,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to update channel")
		return
	}

	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to commit channel update")
			return
		}
	}
	c.JSON(http.StatusOK, toChannelResponse(channel, h.secretKey))
}

// Delete handles DELETE /notifications/channels/:id.
// Cascade delete of bindings is handled by the FK ON DELETE CASCADE constraint.
func (h *NotificationChannelHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	// Verify channel exists before deleting.
	_, err = h.queries.GetChannel(ctx, id)
	if err != nil {
		if isNotFound(err) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
		return
	}

	if err := h.queries.DeleteChannel(ctx, id); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete channel")
		return
	}

	c.Status(http.StatusNoContent)
}

// ListDeliveryLogs handles GET /notifications/channels/:id/delivery-logs.
// Returns a paginated list of delivery log entries for a given channel,
// ordered by created_at descending.
func (h *NotificationChannelHandler) ListDeliveryLogs(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	// Verify channel exists.
	_, err = h.queries.GetChannel(ctx, id)
	if err != nil {
		if isNotFound(err) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
		return
	}

	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	logs, err := h.queries.ListDeliveryLogsByChannel(ctx, db.ListDeliveryLogsByChannelParams{
		ChannelID: id,
		Limit:     int32(limit),
		Offset:    offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list delivery logs")
		return
	}

	total, err := h.queries.CountDeliveryLogsByChannel(ctx, id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count delivery logs")
		return
	}

	data := make([]deliveryLogResponse, 0, len(logs))
	for _, l := range logs {
		resp := deliveryLogResponse{
			ID:          l.ID,
			ChannelID:   l.ChannelID,
			MonitorID:   l.MonitorID,
			TriggerType: l.TriggerType,
			Attempt:     l.Attempt,
			Status:      l.Status,
			CreatedAt:   l.CreatedAt,
		}
		if l.BindingID.Valid {
			id := uuid.UUID(l.BindingID.Bytes)
			resp.BindingID = &id
		}
		if l.ErrorDetail != nil {
			resp.ErrorDetail = l.ErrorDetail
		}
		data = append(data, resp)
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, deliveryLogListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// --- Test Notification ---

// testNotificationResponse represents the response from the test notification endpoint.
type testNotificationResponse struct {
	Success     bool   `json:"success"`
	ChannelType string `json:"channel_type"`
	ChannelID   string `json:"channel_id,omitempty"`
	Error       string `json:"error,omitempty"`
}

// testTimeout is the maximum duration for a test notification delivery.
const testTimeout = 10 * time.Second

// Test handles POST /notifications/channels/:id/test.
// It dispatches a test notification with sample data synchronously and returns
// the result. Test notifications do NOT create delivery_log records and are NOT
// queued for retry on failure.
func (h *NotificationChannelHandler) Test(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()
	channel, err := h.queries.GetChannel(ctx, id)
	if err != nil {
		if isNotFound(err) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
		return
	}

	h.runTest(c, channel)
}

// TestDraft sends the submitted configuration without saving it. Stored secrets
// may only be resolved from the explicitly selected channel of the same type.
func (h *NotificationChannelHandler) TestDraft(c *gin.Context) {
	var req struct {
		channelRequest
		ExistingChannelID string `json:"existing_channel_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	if errs := h.validateChannelRequest(req.channelRequest); len(errs) > 0 {
		apiValidationError(c, "channel configuration is invalid", errs)
		return
	}
	var previous json.RawMessage
	if req.ExistingChannelID != "" {
		id, err := uuid.Parse(req.ExistingChannelID)
		if err != nil {
			apiError(c, http.StatusBadRequest, "INVALID_ID", "existing_channel_id must be a valid UUID")
			return
		}
		existing, err := h.queries.GetChannel(c.Request.Context(), id)
		if err != nil {
			if isNotFound(err) {
				apiError(c, http.StatusNotFound, "NOT_FOUND", "channel not found")
			} else {
				apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get channel")
			}
			return
		}
		if existing.ChannelType == req.Type {
			previous = existing.Config
		}
	}
	config, err := h.processConfigForStorage(req.Type, req.Config, previous)
	if err != nil {
		apiValidationError(c, "channel configuration is invalid", []fieldError{{Field: "config", Message: err.Error()}})
		return
	}
	h.runTest(c, db.NotificationChannel{ChannelType: req.Type, Config: config})
}

func (h *NotificationChannelHandler) runTest(c *gin.Context, channel db.NotificationChannel) {
	select {
	case h.testSlots <- struct{}{}:
		defer func() { <-h.testSlots }()
	default:
		apiError(c, http.StatusTooManyRequests, "TEST_BUSY", "too many notification tests in progress")
		return
	}
	now := time.Now()
	sampleData := notification.TemplateData{
		Monitor: notification.MonitorData{ID: uuid.New(), Name: "[TEST] Sample Monitor", URL: "https://example.com/health", Target: "example.com"},
		Status:  "down", PreviousStatus: "up", ResponseTime: 1500,
		Incident:  notification.IncidentData{ID: uuid.New(), StartedAt: now.Add(-5 * time.Minute), Duration: 5 * time.Minute},
		Timestamp: now, BaseURL: h.baseURL,
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), testTimeout)
	defer cancel()
	var err error
	switch channel.ChannelType {
	case "email":
		err = h.testEmailDelivery(ctx, channel)
	case "webhook":
		err = h.testWebhookDelivery(ctx, channel, sampleData)
	default:
		if !native.IsType(channel.ChannelType) {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "unsupported channel type")
			return
		}
		err = native.Deliver(ctx, channel.ChannelType, channel.Config, h.secretKey, sampleData, uuid.New())
	}
	resp := testNotificationResponse{Success: err == nil, ChannelType: channel.ChannelType}
	if channel.ID != uuid.Nil {
		resp.ChannelID = channel.ID.String()
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(http.StatusOK, resp)
}

// testEmailDelivery sends a test email notification synchronously.
func (h *NotificationChannelHandler) testEmailDelivery(ctx context.Context, channel db.NotificationChannel) error {
	client := h.getSMTPClient()
	if client == nil {
		return fmt.Errorf("SMTP is not configured")
	}

	var cfg emailConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("invalid email channel config: %w", err)
	}

	if len(cfg.Recipients) == 0 {
		return fmt.Errorf("no recipients configured")
	}

	// Build sample data for the email.
	now := time.Now()
	sampleData := notification.TemplateData{
		Monitor: notification.MonitorData{
			ID:     uuid.New(),
			Name:   "[TEST] Sample Monitor",
			URL:    "https://example.com/health",
			Target: "example.com",
		},
		Status:         "down",
		PreviousStatus: "up",
		ResponseTime:   1500,
		Incident: notification.IncidentData{
			ID:        uuid.New(),
			StartedAt: now.Add(-5 * time.Minute),
			Duration:  5 * time.Minute,
		},
		Timestamp: now,
		BaseURL:   h.baseURL,
	}

	return client.SendNotification(ctx, cfg.Recipients, sampleData)
}

// testWebhookDelivery sends a test webhook notification synchronously.
func (h *NotificationChannelHandler) testWebhookDelivery(ctx context.Context, channel db.NotificationChannel, data notification.TemplateData) error {
	var cfg webhook.WebhookConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("invalid webhook channel config: %w", err)
	}

	return h.webhookClient.Deliver(ctx, cfg, data)
}

// --- Validation ---

// validateChannelRequest validates the channel creation/update request and returns
// field-level errors.
func (h *NotificationChannelHandler) validateChannelRequest(req channelRequest) []fieldError {
	var errs []fieldError

	// Validate name.
	if len(req.Name) == 0 {
		errs = append(errs, fieldError{Field: "name", Message: "is required"})
	} else if len(req.Name) > 100 {
		errs = append(errs, fieldError{Field: "name", Message: "must be between 1 and 100 characters"})
	}

	// Validate type.
	if req.Type != "email" && req.Type != "webhook" && !native.IsType(req.Type) {
		errs = append(errs, fieldError{Field: "type", Message: "unsupported channel type"})
		return errs // Can't validate config without knowing type.
	}

	// Validate type-specific config.
	if req.Config == nil || len(req.Config) == 0 {
		errs = append(errs, fieldError{Field: "config", Message: "is required"})
		return errs
	}

	switch req.Type {
	case "email":
		errs = append(errs, h.validateEmailConfig(req.Config)...)
	case "webhook":
		errs = append(errs, h.validateWebhookConfig(req.Config)...)
	default:
		if err := native.Validate(req.Type, req.Config); err != nil {
			errs = append(errs, fieldError{Field: "config", Message: err.Error()})
		}
	}

	return errs
}

// validateEmailConfig validates email channel configuration.
func (h *NotificationChannelHandler) validateEmailConfig(raw json.RawMessage) []fieldError {
	var errs []fieldError
	var cfg emailConfig

	if err := json.Unmarshal(raw, &cfg); err != nil {
		errs = append(errs, fieldError{Field: "config", Message: "invalid email configuration"})
		return errs
	}

	if len(cfg.Recipients) == 0 {
		errs = append(errs, fieldError{Field: "config.recipients", Message: "must contain 1-50 valid email addresses"})
		return errs
	}
	if len(cfg.Recipients) > 50 {
		errs = append(errs, fieldError{Field: "config.recipients", Message: "must contain 1-50 valid email addresses"})
		return errs
	}

	for i, email := range cfg.Recipients {
		if len(email) > 254 {
			errs = append(errs, fieldError{
				Field:   fmt.Sprintf("config.recipients[%d]", i),
				Message: "must not exceed 254 characters",
			})
			continue
		}
		if !emailRegex.MatchString(email) {
			errs = append(errs, fieldError{
				Field:   fmt.Sprintf("config.recipients[%d]", i),
				Message: "must be a valid email address (RFC 5322)",
			})
		}
	}

	return errs
}

// validateWebhookConfig validates webhook channel configuration.
func (h *NotificationChannelHandler) validateWebhookConfig(raw json.RawMessage) []fieldError {
	var errs []fieldError
	var cfg webhookConfig

	if err := json.Unmarshal(raw, &cfg); err != nil {
		errs = append(errs, fieldError{Field: "config", Message: "invalid webhook configuration"})
		return errs
	}

	// Validate URL.
	if cfg.URL == "" {
		errs = append(errs, fieldError{Field: "config.url", Message: "is required"})
	} else if len(cfg.URL) > 2048 {
		errs = append(errs, fieldError{Field: "config.url", Message: "must not exceed 2048 characters"})
	} else {
		parsed, err := url.Parse(cfg.URL)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			errs = append(errs, fieldError{Field: "config.url", Message: "must be a valid http or https URL"})
		}
	}

	// Validate method.
	if cfg.Method == "" {
		errs = append(errs, fieldError{Field: "config.method", Message: "is required"})
	} else if !validWebhookMethods[strings.ToUpper(cfg.Method)] {
		errs = append(errs, fieldError{Field: "config.method", Message: "must be one of: GET, POST, PUT, PATCH, DELETE"})
	}

	// Validate body template.
	if cfg.BodyTemplate == "" {
		errs = append(errs, fieldError{Field: "config.body_template", Message: "is required"})
	} else {
		if err := webhook.ValidateWebhookTemplate(cfg.BodyTemplate); err != nil {
			errs = append(errs, fieldError{Field: "config.body_template", Message: err.Error()})
		}
	}

	// Validate headers (0-20, name ≤128, value ≤8192).
	if len(cfg.Headers) > 20 {
		errs = append(errs, fieldError{Field: "config.headers", Message: "must not exceed 20 headers"})
	} else {
		seen := make(map[string]bool)
		for i, header := range cfg.Headers {
			prefix := fmt.Sprintf("config.headers[%d]", i)
			if header.Name == "" {
				errs = append(errs, fieldError{Field: prefix + ".name", Message: "is required"})
			} else if len(header.Name) > 128 {
				errs = append(errs, fieldError{Field: prefix + ".name", Message: "must not exceed 128 characters"})
			}
			canonical := strings.ToLower(header.Name)
			if seen[canonical] || !validHeaderName(header.Name) {
				errs = append(errs, fieldError{Field: prefix + ".name", Message: "must be a unique valid HTTP header name"})
			}
			seen[canonical] = true
			if header.Value != nil && strings.ContainsAny(*header.Value, "\r\n") {
				errs = append(errs, fieldError{Field: prefix + ".value", Message: "must not contain line breaks"})
			}
			if header.Value != nil && len(*header.Value) > 8192 {
				errs = append(errs, fieldError{Field: prefix + ".value", Message: "must not exceed 8192 characters"})
			}
		}
	}

	return errs
}

// --- Config processing ---

// processConfigForStorage encrypts sensitive fields in webhook config before storage.
func (h *NotificationChannelHandler) processConfigForStorage(channelType string, raw, previous json.RawMessage) (json.RawMessage, error) {
	if native.IsType(channelType) {
		return native.Prepare(channelType, raw, previous, h.secretKey)
	}
	if channelType != "webhook" {
		var cfg emailConfig
		if json.Unmarshal(raw, &cfg) != nil {
			return nil, fmt.Errorf("invalid email configuration")
		}
		return json.Marshal(cfg)
	}
	var cfg webhookConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid webhook configuration")
	}
	var old webhookConfig
	if len(previous) > 0 {
		if err := json.Unmarshal(previous, &old); err != nil {
			return nil, fmt.Errorf("stored configuration is invalid")
		}
	}
	values := make(map[string]*string, len(old.Headers))
	for _, header := range old.Headers {
		values[strings.ToLower(header.Name)] = header.Value
	}
	cfg.Method = strings.ToUpper(cfg.Method)
	if cfg.Headers == nil {
		cfg.Headers = []webhookHeader{}
	}
	for i, header := range cfg.Headers {
		if header.Value == nil || *header.Value == "[REDACTED]" {
			value := values[strings.ToLower(header.Name)]
			if value == nil {
				return nil, fmt.Errorf("header %d requires a value", i)
			}
			cfg.Headers[i].Value = value
			// Older releases stored explicit empty values without encryption.
			if *value == "" {
				encrypted, err := crypto.Encrypt(h.secretKey, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to encrypt header")
				}
				encoded := base64.StdEncoding.EncodeToString(encrypted)
				cfg.Headers[i].Value = &encoded
			}
		} else {
			encrypted, err := crypto.Encrypt(h.secretKey, []byte(*header.Value))
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt header")
			}
			value := base64.StdEncoding.EncodeToString(encrypted)
			cfg.Headers[i].Value = &value
		}
		cfg.Headers[i].ValueConfigured = false
	}
	return json.Marshal(cfg)
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch) {
			continue
		}
		return false
	}
	return true
}

// --- Response helpers ---

// toChannelResponse converts a DB model to an API response, redacting
// webhook header values.
func toChannelResponse(ch db.NotificationChannel, secretKey []byte) channelResponse {
	config := redactChannelConfig(ch.ChannelType, ch.Config)
	return channelResponse{
		ID:        ch.ID,
		Name:      ch.Name,
		Type:      ch.ChannelType,
		Config:    config,
		CreatedAt: ch.CreatedAt,
		UpdatedAt: ch.UpdatedAt,
	}
}

// redactChannelConfig exposes only configured metadata for webhook secrets.
// Email configs are returned as-is.
func redactChannelConfig(channelType string, raw json.RawMessage) json.RawMessage {
	if native.IsType(channelType) {
		return native.Redact(channelType, raw)
	}
	if channelType != "webhook" {
		var cfg emailConfig
		if channelType != "email" || json.Unmarshal(raw, &cfg) != nil {
			return json.RawMessage(`{}`)
		}
		result, err := json.Marshal(cfg)
		if err != nil {
			return json.RawMessage(`{}`)
		}
		return result
	}

	var cfg webhookConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		// Fail closed: malformed stored configuration must never escape redaction.
		return json.RawMessage(`{}`)
	}

	for i := range cfg.Headers {
		cfg.Headers[i].ValueConfigured = cfg.Headers[i].Value != nil
		cfg.Headers[i].Value = nil
	}

	redacted, err := json.Marshal(cfg)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return redacted
}

// --- Shared helpers ---

// fieldError represents a single field-level validation error.
type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// apiValidationError writes a validation error response with field-level details.
func apiValidationError(c *gin.Context, message string, details []fieldError) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"code":    "VALIDATION_ERROR",
			"message": message,
			"details": details,
		},
	})
}

// isNotFound checks if the error is a pgx ErrNoRows.
func isNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
