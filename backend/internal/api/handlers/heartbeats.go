package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VitaliAndrushkevich/pulse/internal/heartbeat"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/tags"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// HeartbeatStateClearer is the seam the DELETE handler uses to drop in-memory
// notification dedup state when a heartbeat is removed (design §3.4, R5.7).
// The concrete StateTracker satisfies it; a nil clearer is valid and skips the call.
type HeartbeatStateClearer interface {
	ClearState(id uuid.UUID)
}

const (
	// maxHeartbeatInterval is the upper bound (seconds) for expected_interval_seconds and
	// grace_seconds, matching the DB CHECK constraints (365 days).
	maxHeartbeatInterval = 31536000
)

// HeartbeatHandler provides CRUD operations for heartbeats plus ping-token rotation.
// It mirrors MonitorHandler: transactional create/update with tags, idempotent PUT
// with a row lock, and a write-only ping token returned exactly once on create and
// rotate-token.
type HeartbeatHandler struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	baseURL string                // public base URL for ping URL construction (may be empty)
	state   HeartbeatStateClearer // notification dedup state clearer (may be nil)
}

// NewHeartbeatHandler creates a handler with the given query layer, connection
// pool, public base URL, and optional state clearer.
func NewHeartbeatHandler(queries *db.Queries, pool *pgxpool.Pool, baseURL string, state HeartbeatStateClearer) *HeartbeatHandler {
	return &HeartbeatHandler{queries: queries, pool: pool, baseURL: baseURL, state: state}
}

// --- Request/Response types ---

type createHeartbeatRequest struct {
	Name                    string            `json:"name"`
	ExpectedIntervalSeconds *int32            `json:"expected_interval_seconds"`
	GraceSeconds            *int32            `json:"grace_seconds"`
	Status                  *string           `json:"status,omitempty"`
	HistoryRetentionDays    *int32            `json:"history_retention_days,omitempty"`
	Tags                    []tags.TagRequest `json:"tags,omitempty"`
}

type putHeartbeatRequest = createHeartbeatRequest

// heartbeatResponse is the standard DTO for GET/LIST. It exposes token_prefix and
// token_created_at but NEVER the raw token or its hash (design §3.3, R6.3).
type heartbeatResponse struct {
	ID                      uuid.UUID     `json:"id"`
	Name                    string        `json:"name"`
	ExpectedIntervalSeconds int32         `json:"expected_interval_seconds"`
	GraceSeconds            int32         `json:"grace_seconds"`
	Status                  string        `json:"status"`
	State                   string        `json:"state"`
	LastSuccessAt           *time.Time    `json:"last_success_at,omitempty"`
	NextExpectedDeadline    *time.Time    `json:"next_expected_deadline,omitempty"`
	Tags                    []tagResponse `json:"tags"`
	HistoryRetentionDays    int32         `json:"history_retention_days"`
	TokenPrefix             string        `json:"token_prefix"`
	TokenCreatedAt          time.Time     `json:"token_created_at"`
	CreatedAt               time.Time     `json:"created_at"`
	UpdatedAt               time.Time     `json:"updated_at"`
}

// createHeartbeatResponse is returned only on create. It is one of exactly two
// responses carrying the raw ping token (the other is rotate-token). ping_url /
// ping_fail_url are omitted when PULSE_BASE_URL is empty.
type createHeartbeatResponse struct {
	heartbeatResponse
	PingToken   string `json:"ping_token"`
	PingURL     string `json:"ping_url,omitempty"`
	PingFailURL string `json:"ping_fail_url,omitempty"`
}

// rotateTokenResponse is returned only on rotate-token. It carries the new raw
// ping token exactly once.
type rotateTokenResponse struct {
	ID             uuid.UUID `json:"id"`
	TokenPrefix    string    `json:"token_prefix"`
	PingToken      string    `json:"ping_token"`
	PingURL        string    `json:"ping_url,omitempty"`
	PingFailURL    string    `json:"ping_fail_url,omitempty"`
	TokenCreatedAt time.Time `json:"token_created_at"`
}

type heartbeatListResponse struct {
	Data       []heartbeatResponse `json:"data"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalPages int                 `json:"total_pages"`
}

// --- Route registration ---

// Register mounts heartbeat routes on the given router group.
func (h *HeartbeatHandler) Register(rg *gin.RouterGroup) {
	hb := rg.Group("/heartbeats")
	hb.POST("", h.Create)
	hb.GET("", h.List)
	hb.GET("/:id", h.Get)
	hb.PUT("/:id", h.Put)
	hb.DELETE("/:id", h.Delete)
	hb.POST("/:id/rotate-token", h.RotateToken)
}

// --- Handlers ---

// Create handles POST /heartbeats.
func (h *HeartbeatHandler) Create(c *gin.Context) {
	var req createHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	interval, grace, status, verr := validateHeartbeatFields(name, req.ExpectedIntervalSeconds, req.GraceSeconds, req.Status)
	if verr != "" {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", verr)
		return
	}

	retentionDays, rerr := validateHeartbeatRetentionDays(req.HistoryRetentionDays)
	if rerr != nil {
		apiError(c, http.StatusBadRequest, "INVALID_RETENTION_PERIOD", rerr.Error())
		return
	}

	if req.Tags != nil {
		if err := tags.ValidateTags(req.Tags); err != nil {
			apiError(c, http.StatusBadRequest, "INVALID_TAGS", err.Error())
			return
		}
	}

	// Generate the ping token: a 256-bit random secret. We persist only the
	// SHA-256 hex digest + prefix (design §6) — the raw token is returned once.
	raw, prefix, err := token.GenerateRaw()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate ping token")
		return
	}
	tokenHash := token.HashRawSHA256(raw)

	ctx := c.Request.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to begin transaction")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	hb, err := qtx.CreateHeartbeat(ctx, db.CreateHeartbeatParams{
		Name:                    name,
		ExpectedIntervalSeconds: interval,
		GraceSeconds:            grace,
		Status:                  status,
		HistoryRetentionDays:    retentionDays,
		TokenPrefix:             prefix,
		TokenHash:               tokenHash,
	})
	if err != nil {
		if isCheckConstraintErr(err) {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid heartbeat values (ensure database migrations are applied)")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create heartbeat")
		return
	}

	if len(req.Tags) > 0 {
		if err := qtx.DeleteHeartbeatTags(ctx, hb.ID); err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to set heartbeat tags")
			return
		}
		for _, tag := range req.Tags {
			if err := qtx.InsertHeartbeatTag(ctx, db.InsertHeartbeatTagParams{
				HeartbeatID: hb.ID,
				Key:         tag.Key,
				Value:       tag.Value,
			}); err != nil {
				apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to set heartbeat tags")
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to commit transaction")
		return
	}

	hbTags, err := h.queries.ListTagsByHeartbeat(ctx, hb.ID)
	if err != nil {
		hbTags = []db.HeartbeatTag{}
	}

	resp := createHeartbeatResponse{
		heartbeatResponse: toHeartbeatResponse(hb, hbTags),
		PingToken:         raw,
	}
	resp.PingURL, resp.PingFailURL = h.pingURLs(raw)
	c.JSON(http.StatusCreated, resp)
}

// List handles GET /heartbeats with pagination (max limit 100).
func (h *HeartbeatHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	if limit > 100 {
		limit = 100
	}
	offset := int32((page - 1) * limit)
	ctx := c.Request.Context()

	hbs, err := h.queries.ListHeartbeats(ctx, db.ListHeartbeatsParams{
		Limit:  int32(limit),
		Offset: offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list heartbeats")
		return
	}

	total, err := h.queries.CountHeartbeats(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count heartbeats")
		return
	}

	data := make([]heartbeatResponse, 0, len(hbs))
	for _, hb := range hbs {
		hbTags, err := h.queries.ListTagsByHeartbeat(ctx, hb.ID)
		if err != nil {
			hbTags = []db.HeartbeatTag{}
		}
		data = append(data, toHeartbeatResponse(hb, hbTags))
	}

	c.JSON(http.StatusOK, heartbeatListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: pageCount(total, limit),
	})
}

// Get handles GET /heartbeats/:id.
func (h *HeartbeatHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()
	hb, err := h.queries.GetHeartbeat(ctx, id)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
		return
	}

	hbTags, err := h.queries.ListTagsByHeartbeat(ctx, hb.ID)
	if err != nil {
		hbTags = []db.HeartbeatTag{}
	}

	c.JSON(http.StatusOK, toHeartbeatResponse(hb, hbTags))
}

// Put handles PUT /heartbeats/:id with idempotent create-or-update semantics.
// It never rotates the ping token. When interval/grace change while a
// last_success_at exists, the next_expected_deadline is recomputed (design §3.4).
func (h *HeartbeatHandler) Put(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	var req putHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	interval, grace, status, verr := validateHeartbeatFields(name, req.ExpectedIntervalSeconds, req.GraceSeconds, req.Status)
	if verr != "" {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", verr)
		return
	}

	retentionDays, rerr := validateHeartbeatRetentionDays(req.HistoryRetentionDays)
	if rerr != nil {
		apiError(c, http.StatusBadRequest, "INVALID_RETENTION_PERIOD", rerr.Error())
		return
	}

	if req.Tags != nil {
		if err := tags.ValidateTags(req.Tags); err != nil {
			apiError(c, http.StatusBadRequest, "INVALID_TAGS", err.Error())
			return
		}
	}

	ctx := c.Request.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to begin transaction")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := h.queries.WithTx(tx)

	// Acquire a row lock for concurrent-modification safety (mirrors MonitorHandler).
	existing, lockErr := qtx.GetHeartbeatForUpdate(ctx, id)
	existed := true
	if lockErr != nil {
		if errors.Is(lockErr, pgx.ErrNoRows) {
			existed = false
		} else if isSerializationErr(lockErr) {
			apiError(c, http.StatusConflict, "CONFLICT", "concurrent modification detected, please retry")
			return
		} else {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to lock heartbeat for update")
			return
		}
	}

	// On create via PUT, generate a fresh token; on update, UpsertHeartbeat
	// ignores the token params (PUT never rotates — design §2.2), but the
	// non-null columns still require placeholder values, so reuse the existing.
	var tokenPrefix, tokenHash, rawForCreate string
	if existed {
		tokenPrefix = existing.TokenPrefix
		tokenHash = existing.TokenHash
	} else {
		raw, prefix, gerr := token.GenerateRaw()
		if gerr != nil {
			apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate ping token")
			return
		}
		rawForCreate = raw
		tokenPrefix = prefix
		tokenHash = token.HashRawSHA256(raw)
	}

	hb, err := qtx.UpsertHeartbeat(ctx, db.UpsertHeartbeatParams{
		ID:                      id,
		Name:                    name,
		ExpectedIntervalSeconds: interval,
		GraceSeconds:            grace,
		Status:                  status,
		HistoryRetentionDays:    retentionDays,
		TokenPrefix:             tokenPrefix,
		TokenHash:               tokenHash,
	})
	if err != nil {
		if isCheckConstraintErr(err) {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid heartbeat values (ensure database migrations are applied)")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create or update heartbeat")
		return
	}

	// Recompute the deadline when interval/grace changed and a success exists.
	if hb.LastSuccessAt.Valid {
		intervalOrGraceChanged := !existed ||
			existing.ExpectedIntervalSeconds != interval ||
			existing.GraceSeconds != grace
		if intervalOrGraceChanged {
			deadline := heartbeat.NextDeadline(hb.LastSuccessAt.Time, int(interval), int(grace))
			hb, err = qtx.UpdateHeartbeatDeadline(ctx, db.UpdateHeartbeatDeadlineParams{
				ID:                   id,
				State:                hb.State,
				LastSuccessAt:        hb.LastSuccessAt,
				NextExpectedDeadline: pgtype.Timestamptz{Time: deadline, Valid: true},
			})
			if err != nil {
				apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to recompute heartbeat deadline")
				return
			}
		}
	}

	// Replace tags within the same transaction.
	if err := qtx.DeleteHeartbeatTags(ctx, hb.ID); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to replace heartbeat tags")
		return
	}
	for _, tag := range req.Tags {
		if err := qtx.InsertHeartbeatTag(ctx, db.InsertHeartbeatTagParams{
			HeartbeatID: hb.ID,
			Key:         tag.Key,
			Value:       tag.Value,
		}); err != nil {
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to replace heartbeat tags")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		if isSerializationErr(err) {
			apiError(c, http.StatusConflict, "CONFLICT", "concurrent modification detected, please retry")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to commit transaction")
		return
	}

	hbTags, err := h.queries.ListTagsByHeartbeat(ctx, hb.ID)
	if err != nil {
		hbTags = []db.HeartbeatTag{}
	}

	// A PUT that created a new heartbeat returns the raw token once (it is the
	// only time the token can be surfaced), mirroring POST.
	if !existed {
		resp := createHeartbeatResponse{
			heartbeatResponse: toHeartbeatResponse(hb, hbTags),
			PingToken:         rawForCreate,
		}
		resp.PingURL, resp.PingFailURL = h.pingURLs(rawForCreate)
		c.JSON(http.StatusOK, resp)
		return
	}

	c.JSON(http.StatusOK, toHeartbeatResponse(hb, hbTags))
}

// Delete handles DELETE /heartbeats/:id. Deletion cascades tags + bindings; the
// handler then clears any in-memory notification dedup state (design §3.4).
func (h *HeartbeatHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	if err := h.queries.DeleteHeartbeat(c.Request.Context(), id); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete heartbeat")
		return
	}

	if h.state != nil {
		h.state.ClearState(id)
	}

	c.Status(http.StatusNoContent)
}

// RotateToken handles POST /heartbeats/:id/rotate-token. It issues a new ping
// token, invalidating all existing ping URLs, and returns the raw token once.
func (h *HeartbeatHandler) RotateToken(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	ctx := c.Request.Context()

	// Ensure the heartbeat exists before rotating (RotateHeartbeatToken would
	// otherwise return pgx.ErrNoRows, but an explicit 404 is clearer).
	if _, err := h.queries.GetHeartbeat(ctx, id); err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
		return
	}

	raw, prefix, err := token.GenerateRaw()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate ping token")
		return
	}
	tokenHash := token.HashRawSHA256(raw)

	hb, err := h.queries.RotateHeartbeatToken(ctx, db.RotateHeartbeatTokenParams{
		ID:          id,
		TokenPrefix: prefix,
		TokenHash:   tokenHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiError(c, http.StatusNotFound, "NOT_FOUND", "heartbeat not found")
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to rotate ping token")
		return
	}

	resp := rotateTokenResponse{
		ID:             hb.ID,
		TokenPrefix:    hb.TokenPrefix,
		PingToken:      raw,
		TokenCreatedAt: hb.TokenCreatedAt,
	}
	resp.PingURL, resp.PingFailURL = h.pingURLs(raw)
	c.JSON(http.StatusOK, resp)
}

// --- Helpers ---

// validateHeartbeatFields validates and normalizes the create/PUT body fields.
// It returns the effective interval, grace, and status, or a non-empty error
// message describing the first validation failure.
func validateHeartbeatFields(name string, intervalPtr, gracePtr *int32, statusPtr *string) (interval, grace int32, status string, errMsg string) {
	if name == "" {
		return 0, 0, "", "name is required"
	}

	if intervalPtr == nil {
		return 0, 0, "", "expected_interval_seconds is required"
	}
	interval = *intervalPtr
	if interval < 1 || interval > maxHeartbeatInterval {
		return 0, 0, "", fmt.Sprintf("expected_interval_seconds must be between 1 and %d", maxHeartbeatInterval)
	}

	grace = 0
	if gracePtr != nil {
		grace = *gracePtr
		if grace < 0 || grace > maxHeartbeatInterval {
			return 0, 0, "", fmt.Sprintf("grace_seconds must be between 0 and %d", maxHeartbeatInterval)
		}
	}

	status = "active"
	if statusPtr != nil && (*statusPtr == "active" || *statusPtr == "paused") {
		status = *statusPtr
	}

	return interval, grace, status, ""
}

// validateHeartbeatRetentionDays validates the optional history_retention_days
// field, mirroring the monitor path's validateRetentionDays. Returns the
// effective retention period: default 30 if nil, the provided value if in
// [1, 365], or an error if out of range.
func validateHeartbeatRetentionDays(days *int32) (int32, error) {
	if days == nil {
		return 30, nil
	}
	if *days < 1 || *days > 365 {
		return 0, fmt.Errorf("history_retention_days must be between 1 and 365, got %d", *days)
	}
	return *days, nil
}

// pingURLs builds the success + failure ping URLs from the configured base URL.
// When baseURL is empty both are empty (the raw token is still returned).
func (h *HeartbeatHandler) pingURLs(raw string) (pingURL, pingFailURL string) {
	if h.baseURL == "" {
		return "", ""
	}
	base := strings.TrimRight(h.baseURL, "/")
	return base + "/ping/" + raw, base + "/ping/" + raw + "/fail"
}

// toHeartbeatResponse maps a DB heartbeat + tags to the standard DTO. It applies
// the status='paused' → observed state='paused' presentation mapping (design §1.1)
// and never includes raw-token or hash material.
func toHeartbeatResponse(hb db.Heartbeat, hbTags []db.HeartbeatTag) heartbeatResponse {
	resp := heartbeatResponse{
		ID:                      hb.ID,
		Name:                    hb.Name,
		ExpectedIntervalSeconds: hb.ExpectedIntervalSeconds,
		GraceSeconds:            hb.GraceSeconds,
		Status:                  hb.Status,
		State:                   hb.State,
		HistoryRetentionDays:    hb.HistoryRetentionDays,
		Tags:                    []tagResponse{},
		TokenPrefix:             hb.TokenPrefix,
		TokenCreatedAt:          hb.TokenCreatedAt,
		CreatedAt:               hb.CreatedAt,
		UpdatedAt:               hb.UpdatedAt,
	}

	// Presentation mapping: a paused heartbeat surfaces observed state "paused".
	if hb.Status == "paused" {
		resp.State = "paused"
	}

	if hb.LastSuccessAt.Valid {
		t := hb.LastSuccessAt.Time
		resp.LastSuccessAt = &t
	}
	if hb.NextExpectedDeadline.Valid {
		t := hb.NextExpectedDeadline.Time
		resp.NextExpectedDeadline = &t
	}

	if len(hbTags) > 0 {
		resp.Tags = make([]tagResponse, 0, len(hbTags))
		for _, t := range hbTags {
			resp.Tags = append(resp.Tags, tagResponse{Key: t.Key, Value: t.Value})
		}
	}

	return resp
}

// pageCount computes the number of pages for a total count and page size.
func pageCount(total int64, limit int) int {
	if total == 0 || limit <= 0 {
		return 0
	}
	pages := int(total) / limit
	if int(total)%limit != 0 {
		pages++
	}
	return pages
}

// isCheckConstraintErr reports whether err is a Postgres CHECK constraint violation (23514).
func isCheckConstraintErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

// isSerializationErr reports whether err is a serialization/lock failure that
// should map to 409 CONFLICT (40001 serialization_failure, 40P01 deadlock_detected,
// 55P03 lock_not_available).
func isSerializationErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "55P03")
}
