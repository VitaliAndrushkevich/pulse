package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// TagHandler provides tag autocomplete endpoints.
type TagHandler struct {
	queries *db.Queries
}

// NewTagHandler creates a handler with the given query layer.
func NewTagHandler(queries *db.Queries) *TagHandler {
	return &TagHandler{queries: queries}
}

// Register mounts tag routes on the given router group.
func (h *TagHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/tags", h.ListKeys)
	rg.GET("/tags/:key", h.ListValues)
}

// ListKeys handles GET /api/v1/tags — returns distinct tag keys.
func (h *TagHandler) ListKeys(c *gin.Context) {
	keys, err := h.queries.ListAllTagKeys(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list tag keys")
		return
	}

	if keys == nil {
		keys = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"data": keys})
}

// ListValues handles GET /api/v1/tags/:key — returns distinct values for a key.
func (h *TagHandler) ListValues(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		apiError(c, http.StatusBadRequest, "INVALID_KEY", "tag key must not be empty")
		return
	}

	values, err := h.queries.ListTagValues(c.Request.Context(), key)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to list tag values")
		return
	}

	if values == nil {
		values = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"data": values})
}
