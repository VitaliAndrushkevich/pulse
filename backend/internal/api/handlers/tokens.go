package handlers

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// TokenHandler provides create, list, and revoke operations for API tokens.
type TokenHandler struct {
	queries *db.Queries
}

// NewTokenHandler creates a handler with the given query layer.
func NewTokenHandler(queries *db.Queries) *TokenHandler {
	return &TokenHandler{queries: queries}
}

// --- Request/Response types ---

// createTokenRequest is the expected body for POST /tokens.
type createTokenRequest struct {
	Name      string  `json:"name"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

// createTokenResponse is returned only on successful token creation.
// It is the only time the raw token value is exposed.
type createTokenResponse struct {
	Token     string     `json:"token"`
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// tokenResponse is the API representation of a token (list/revoke responses).
// It never includes the token_hash.
type tokenResponse struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// tokenListResponse wraps paginated token list results.
type tokenListResponse struct {
	Data       []tokenResponse `json:"data"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
	TotalPages int             `json:"total_pages"`
}

// --- Route registration ---

// Register mounts token routes on the given router group.
func (h *TokenHandler) Register(rg *gin.RouterGroup) {
	tokens := rg.Group("/tokens")
	tokens.POST("", h.Create)
	tokens.GET("", h.List)
	tokens.DELETE("/:id", h.Revoke)
}

// --- Handlers ---

// Create handles POST /tokens.
func (h *TokenHandler) Create(c *gin.Context) {
	var req createTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "request body must be valid JSON")
		return
	}

	// Validate name: trim whitespace, must be 1-128 chars.
	name := strings.TrimSpace(req.Name)
	if name == "" {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if len(name) > 128 {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name must be at most 128 characters")
		return
	}

	// Validate optional expires_at.
	var expiresAt pgtype.Timestamptz
	var expiresAtPtr *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "expires_at must be a valid RFC 3339 timestamp")
			return
		}
		if !t.After(time.Now()) {
			apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "expires_at must be in the future")
			return
		}
		expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
		expiresAtPtr = &t
	}

	// Generate token (crypto/rand + bcrypt).
	raw, prefix, hash, err := token.Generate()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
		return
	}

	// Get user_id from context (set by auth middleware).
	userIDStr := c.GetString("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing user identity")
		return
	}

	// Persist token.
	apiToken, err := h.queries.CreateAPIToken(c.Request.Context(), db.CreateAPITokenParams{
		UserID:    userID,
		Name:      name,
		Prefix:    prefix,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		// Do NOT return the raw token on DB failure.
		_ = raw
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create token")
		return
	}

	c.JSON(http.StatusCreated, createTokenResponse{
		Token:     raw,
		ID:        apiToken.ID,
		Name:      apiToken.Name,
		ExpiresAt: expiresAtPtr,
		CreatedAt: apiToken.CreatedAt,
	})
}

// List handles GET /tokens with strict pagination validation.
func (h *TokenHandler) List(c *gin.Context) {
	// Strict pagination validation: return 400 for any invalid input.
	page, limit, err := parseStrictPagination(c)
	if err != "" {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err)
		return
	}

	// Get user_id from context.
	userIDStr := c.GetString("user_id")
	userID, parseErr := uuid.Parse(userIDStr)
	if parseErr != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing user identity")
		return
	}

	offset := int32((page - 1) * limit)

	tokens, dbErr := h.queries.ListAPITokensByUser(c.Request.Context(), db.ListAPITokensByUserParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: offset,
	})
	if dbErr != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tokens")
		return
	}

	total, dbErr := h.queries.CountAPITokensByUser(c.Request.Context(), userID)
	if dbErr != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to count tokens")
		return
	}

	data := make([]tokenResponse, 0, len(tokens))
	for _, t := range tokens {
		data = append(data, toTokenResponse(t))
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	c.JSON(http.StatusOK, tokenListResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Revoke handles DELETE /tokens/:id.
func (h *TokenHandler) Revoke(c *gin.Context) {
	// Validate UUID path param.
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	// Get user_id from context.
	userIDStr := c.GetString("user_id")
	userID, parseErr := uuid.Parse(userIDStr)
	if parseErr != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing user identity")
		return
	}

	// Revoke token (idempotent via COALESCE in SQL).
	apiToken, dbErr := h.queries.RevokeAPIToken(c.Request.Context(), db.RevokeAPITokenParams{
		ID:     id,
		UserID: userID,
	})
	if dbErr != nil {
		// Token not found or belongs to another user.
		apiError(c, http.StatusNotFound, "NOT_FOUND", "token not found")
		return
	}

	c.JSON(http.StatusOK, toTokenResponse(apiToken))
}

// --- Helpers ---

// parseStrictPagination validates page and limit query params strictly.
// Returns an error message string if validation fails (empty string on success).
func parseStrictPagination(c *gin.Context) (page, limit int, errMsg string) {
	pageStr := c.Query("page")
	limitStr := c.Query("limit")

	// Default values.
	page = 1
	limit = 20

	// Validate page if provided.
	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			return 0, 0, "page must be a positive integer"
		}
		if p < 1 {
			return 0, 0, "page must be at least 1"
		}
		page = p
	}

	// Validate limit if provided.
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, "limit must be a positive integer"
		}
		if l < 1 {
			return 0, 0, "limit must be at least 1"
		}
		if l > 100 {
			return 0, 0, "limit must be at most 100"
		}
		limit = l
	}

	return page, limit, ""
}

// toTokenResponse converts a DB ApiToken to the API response type.
// It never includes token_hash.
func toTokenResponse(t db.ApiToken) tokenResponse {
	resp := tokenResponse{
		ID:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt,
	}

	if t.LastUsedAt.Valid {
		ts := t.LastUsedAt.Time
		resp.LastUsedAt = &ts
	}
	if t.ExpiresAt.Valid {
		ts := t.ExpiresAt.Time
		resp.ExpiresAt = &ts
	}
	if t.RevokedAt.Valid {
		ts := t.RevokedAt.Time
		resp.RevokedAt = &ts
	}

	return resp
}
