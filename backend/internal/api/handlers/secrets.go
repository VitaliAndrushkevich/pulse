// Package handlers contains HTTP handler implementations for the Pulse API.
package handlers

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// SecretHandler provides CRUD operations for secrets with write-only semantics.
// Secret values are encrypted at rest and never returned in API responses.
type SecretHandler struct {
	queries *db.Queries
	key     []byte
}

// NewSecretHandler creates a handler with the given query layer and encryption key.
func NewSecretHandler(queries *db.Queries, key []byte) *SecretHandler {
	return &SecretHandler{queries: queries, key: key}
}

// secretResponse is the API representation of a secret. The value is always
// redacted — only metadata is exposed.
type secretResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// createSecretRequest is the expected body for POST /secrets.
type createSecretRequest struct {
	Name  string `json:"name" binding:"required"`
	Value string `json:"value" binding:"required"`
}

// updateSecretRequest is the expected body for PUT /secrets/:id.
type updateSecretRequest struct {
	Name  string `json:"name" binding:"required"`
	Value string `json:"value" binding:"required"`
}

// listResponse wraps paginated list results.
type listResponse struct {
	Data       []secretResponse `json:"data"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"total_pages"`
}

// Register mounts secret routes on the given router group.
func (h *SecretHandler) Register(rg *gin.RouterGroup) {
	secrets := rg.Group("/secrets")
	secrets.POST("", h.Create)
	secrets.GET("", h.List)
	secrets.GET("/:id", h.Get)
	secrets.PUT("/:id", h.Update)
	secrets.DELETE("/:id", h.Delete)
}

// Create handles POST /secrets.
func (h *SecretHandler) Create(c *gin.Context) {
	var req createSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name and value are required")
		return
	}

	encrypted, err := crypto.Encrypt(h.key, []byte(req.Value))
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to encrypt secret value")
		return
	}

	secret, err := h.queries.CreateSecret(c.Request.Context(), db.CreateSecretParams{
		Name:           req.Name,
		EncryptedValue: base64.StdEncoding.EncodeToString(encrypted),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to create secret")
		return
	}

	c.JSON(http.StatusCreated, secretResponse{
		ID:        secret.ID,
		Name:      secret.Name,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	})
}

// List handles GET /secrets with pagination.
func (h *SecretHandler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := int32((page - 1) * limit)

	secrets, err := h.queries.ListSecrets(c.Request.Context(), db.ListSecretsParams{
		Limit:  int32(limit),
		Offset: offset,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list secrets")
		return
	}

	total, err := h.queries.CountSecrets(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to count secrets")
		return
	}

	data := make([]secretResponse, 0, len(secrets))
	for _, s := range secrets {
		data = append(data, secretResponse{
			ID:        s.ID,
			Name:      s.Name,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
		})
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, listResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// Get handles GET /secrets/:id.
func (h *SecretHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	secret, err := h.queries.GetSecret(c.Request.Context(), id)
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "secret not found")
		return
	}

	c.JSON(http.StatusOK, secretResponse{
		ID:        secret.ID,
		Name:      secret.Name,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	})
}

// Update handles PUT /secrets/:id.
func (h *SecretHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	var req updateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name and value are required")
		return
	}

	encrypted, err := crypto.Encrypt(h.key, []byte(req.Value))
	if err != nil {
		apiError(c, http.StatusInternalServerError, "ENCRYPTION_ERROR", "failed to encrypt secret value")
		return
	}

	secret, err := h.queries.UpdateSecret(c.Request.Context(), db.UpdateSecretParams{
		ID:             id,
		Name:           req.Name,
		EncryptedValue: base64.StdEncoding.EncodeToString(encrypted),
	})
	if err != nil {
		apiError(c, http.StatusNotFound, "NOT_FOUND", "secret not found")
		return
	}

	c.JSON(http.StatusOK, secretResponse{
		ID:        secret.ID,
		Name:      secret.Name,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	})
}

// Delete handles DELETE /secrets/:id.
func (h *SecretHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID")
		return
	}

	if err := h.queries.DeleteSecret(c.Request.Context(), id); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete secret")
		return
	}

	c.Status(http.StatusNoContent)
}
