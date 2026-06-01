package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// apiError writes a standardized error response envelope.
func apiError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// parsePagination extracts page and limit query params with defaults.
func parsePagination(c *gin.Context) (page, limit int) {
	page = 1
	limit = 20

	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	return page, limit
}
