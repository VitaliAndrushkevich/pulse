package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	"github.com/VitaliAndrushkevich/pulse/internal/frontend"
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

	// Initial setup endpoint — create first admin user (no auth required).
	setupHandler := handlers.NewSetupHandler(deps.Queries, deps.JWTSecret, deps.JWTExpiry)
	setupHandler.Register(v1)

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

	// SPA catch-all: serve embedded frontend assets when available (TASK-036).
	if frontend.HasAssets() {
		distFS, _ := fs.Sub(frontend.FS, "dist")
		r.NoRoute(spaHandler(distFS))
	}

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

// apiPrefixes are URL path prefixes reserved for API/system routes.
// Requests matching these should return a JSON 404, not the SPA fallback.
var apiPrefixes = []string{"/api/", "/ws", "/metrics", "/healthz", "/swagger"}

// spaHandler returns a gin.HandlerFunc that serves the embedded frontend.
// For API/system paths it returns a JSON 404; for everything else it tries to
// serve the exact file from the embedded FS or falls back to index.html.
func spaHandler(distFS fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(distFS))

	return func(c *gin.Context) {
		reqPath := c.Request.URL.Path

		// API/system paths that don't match a registered route → JSON 404.
		for _, prefix := range apiPrefixes {
			if strings.HasPrefix(reqPath, prefix) || reqPath == prefix {
				c.JSON(http.StatusNotFound, gin.H{
					"error": gin.H{
						"code":    "NOT_FOUND",
						"message": "resource not found",
					},
				})
				return
			}
		}

		// Try to serve the exact file from the embedded FS.
		cleanPath := strings.TrimPrefix(reqPath, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		if file, err := distFS.(fs.ReadFileFS).ReadFile(cleanPath); err == nil {
			setCacheHeader(c, cleanPath)
			contentType := detectContentType(cleanPath)
			c.Data(http.StatusOK, contentType, file)
			return
		}

		// Fall back to index.html for SPA client-side routing.
		indexData, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			// No index.html in the embedded FS — serve via file server as last resort.
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		setCacheHeader(c, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
	}
}

// setCacheHeader sets Cache-Control based on whether the file is a hashed asset.
// Hashed assets (files under _app/ with hash-like patterns) get immutable caching.
// index.html and other files get no-cache to ensure fresh content on deploys.
func setCacheHeader(c *gin.Context, filePath string) {
	if isHashedAsset(filePath) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
}

// isHashedAsset returns true if the file path looks like a hashed asset
// (e.g., _app/immutable/chunks/entry.Dh3Xk2f1.js).
func isHashedAsset(filePath string) bool {
	// SvelteKit puts hashed assets under _app/immutable/
	if strings.HasPrefix(filePath, "_app/") {
		return true
	}
	return false
}

// detectContentType returns the MIME type based on file extension.
func detectContentType(filePath string) string {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".webp":
		return "image/webp"
	case ".webm":
		return "video/webm"
	case ".mp4":
		return "video/mp4"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".xml":
		return "application/xml"
	case ".wasm":
		return "application/wasm"
	default:
		return "application/octet-stream"
	}
}
