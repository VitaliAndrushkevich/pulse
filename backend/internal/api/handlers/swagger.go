package handlers

import (
	"embed"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

//go:embed swagger_assets/index.html
var swaggerAssets embed.FS

// RegisterSwaggerRoutes registers Swagger UI and OpenAPI spec endpoints.
// specDir is the absolute path to the directory containing openapi.yaml
// (typically the repo's backend/api/ directory). The spec is served from disk
// so edits are reflected immediately without rebuilding.
func RegisterSwaggerRoutes(r *gin.Engine, specDir string) {
	specPath := filepath.Join(specDir, "openapi.yaml")

	// Serve the raw OpenAPI spec directly from disk.
	r.GET("/swagger/openapi.yaml", func(c *gin.Context) {
		c.File(specPath)
	})

	// Serve the Swagger UI page.
	r.GET("/swagger", func(c *gin.Context) {
		data, err := swaggerAssets.ReadFile("swagger_assets/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "swagger UI not available")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}
