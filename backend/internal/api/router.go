package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// Deps holds shared dependencies injected into the router.
type Deps struct {
	Queries   *db.Queries
	SecretKey []byte
}

func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SanitizedLogger())
	r.Use(requestIDMiddleware())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	v1 := r.Group("/api/v1")
	v1.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Protected group — requires valid Bearer token
	protected := v1.Group("")
	protected.Use(middleware.BearerAuth(deps.Queries))

	// Token management endpoints (protected)
	tokenHandler := handlers.NewTokenHandler(deps.Queries)
	tokenHandler.Register(protected)

	// Secret write-only API (TASK-010)
	secretHandler := handlers.NewSecretHandler(deps.Queries, deps.SecretKey)
	secretHandler.Register(protected)

	return r
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	}
}
