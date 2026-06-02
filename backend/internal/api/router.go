package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// Deps holds shared dependencies injected into the router.
type Deps struct {
	Queries        *db.Queries
	SecretKey      []byte
	JWTSecret      []byte
	JWTExpiry      time.Duration
	TimescaleStore *timescale.Store
	Metrics        *handlers.Metrics
	PromRegistry   *prometheus.Registry
	Hub            *hub.Hub
	DevMode        bool
	OpenAPIDir     string // directory containing openapi.yaml, used for swagger in dev mode
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

	// Swagger UI available only in dev mode (PULSE_DEV=true).
	if deps.DevMode {
		handlers.RegisterSwaggerRoutes(r, deps.OpenAPIDir)
	}

	// Prometheus metrics endpoint (TASK-026) — no auth required for scraping.
	if deps.PromRegistry != nil {
		handlers.RegisterMetricsRoute(r, deps.PromRegistry)
	}

	// Authenticated WebSocket endpoint (TASK-030).
	// Auth is validated via ?token= query parameter before HTTP upgrade.
	if deps.Hub != nil {
		wsHandler := handlers.NewWSHandler(deps.Hub, deps.Queries, deps.JWTSecret)
		r.GET("/ws", wsHandler.Handle)
	}

	v1 := r.Group("/api/v1")
	v1.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Public auth endpoints (TASK-022).
	authHandler := handlers.NewAuthHandler(deps.Queries, deps.JWTSecret, deps.JWTExpiry)
	authHandler.Register(v1)

	// Protected group — requires valid JWT or Bearer API token.
	protected := v1.Group("")
	protected.Use(combinedAuth(deps.Queries, deps.JWTSecret))

	// Token management endpoints (TASK-011).
	tokenHandler := handlers.NewTokenHandler(deps.Queries)
	tokenHandler.Register(protected)

	// Secret write-only API (TASK-010).
	secretHandler := handlers.NewSecretHandler(deps.Queries, deps.SecretKey)
	secretHandler.Register(protected)

	// Monitor CRUD (TASK-023).
	monitorHandler := handlers.NewMonitorHandler(deps.Queries)
	monitorHandler.Register(protected)

	// Monitor history (TASK-024).
	if deps.TimescaleStore != nil {
		historyHandler := handlers.NewHistoryHandler(deps.Queries, deps.TimescaleStore)
		historyHandler.Register(protected)
	}

	// Incidents (TASK-025).
	incidentHandler := handlers.NewIncidentHandler(deps.Queries)
	incidentHandler.Register(protected)

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

// dummyHash is pre-computed for uniform timing on API token auth failures.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-comparison-target"), bcrypt.DefaultCost)

// combinedAuth tries JWT auth first (if the token looks like a JWT), then
// falls back to Bearer API token auth. This allows both session-based (JWT)
// and programmatic (API token) access on the same protected routes.
func combinedAuth(queries *db.Queries, jwtSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			unauthorized(c)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")

		// Heuristic: JWT tokens contain two dots (header.payload.signature).
		// API tokens are 43-char base64url without dots.
		if strings.Count(rawToken, ".") == 2 {
			// Try JWT validation.
			claims := &middleware.JWTClaims{}
			tok, err := jwt.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return jwtSecret, nil
			})
			if err == nil && tok.Valid {
				c.Set("user_id", claims.UserID)
				c.Set("email", claims.Email)
				c.Next()
				return
			}
		}

		// Fall back to API token auth (prefix-based lookup + bcrypt).
		if len(rawToken) < token.PrefixLen {
			dummyCompare()
			unauthorized(c)
			return
		}

		prefix := rawToken[:token.PrefixLen]
		candidates, err := queries.ListAPITokensByPrefix(c.Request.Context(), prefix)
		if err != nil || len(candidates) == 0 {
			dummyCompare()
			unauthorized(c)
			return
		}

		var matched *db.ApiToken
		for i := range candidates {
			if bcrypt.CompareHashAndPassword([]byte(candidates[i].TokenHash), []byte(rawToken)) == nil {
				matched = &candidates[i]
				break
			}
		}

		if matched == nil {
			unauthorized(c)
			return
		}

		if matched.RevokedAt.Valid {
			dummyCompare()
			unauthorized(c)
			return
		}

		if matched.ExpiresAt.Valid && matched.ExpiresAt.Time.Before(time.Now()) {
			dummyCompare()
			unauthorized(c)
			return
		}

		_ = queries.TouchAPIToken(c.Request.Context(), matched.ID)
		c.Set("user_id", matched.UserID.String())
		c.Next()
	}
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "UNAUTHORIZED",
			"message": "invalid or expired token",
		},
	})
	c.Abort()
}

func dummyCompare() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("not-a-real-token"))
}
