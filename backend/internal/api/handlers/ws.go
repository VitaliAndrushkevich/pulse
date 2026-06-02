package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, restrict to known origins.
		// For now, allow all origins for development.
		return true
	},
}

// WSHandler handles the authenticated WebSocket endpoint.
type WSHandler struct {
	hub       *hub.Hub
	queries   *db.Queries
	jwtSecret []byte
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(h *hub.Hub, queries *db.Queries, jwtSecret []byte) *WSHandler {
	return &WSHandler{
		hub:       h,
		queries:   queries,
		jwtSecret: jwtSecret,
	}
}

// Handle processes the WebSocket upgrade request.
// Auth is validated via ?token= query parameter before the HTTP upgrade.
// Browsers cannot send Authorization headers on WebSocket connections,
// so query-param auth is the standard approach.
func (wh *WSHandler) Handle(c *gin.Context) {
	// Extract token from query parameter.
	rawToken := c.Query("token")
	if rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "missing token query parameter",
			},
		})
		return
	}

	// Validate token (JWT or API token) before upgrade.
	if !wh.validateToken(c, rawToken) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "invalid or expired token",
			},
		})
		return
	}

	// Upgrade HTTP connection to WebSocket.
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade failed: %v", err)
		return
	}

	// Register client with the hub.
	client := wh.hub.RegisterClient(conn)

	// Send initial connected message.
	connMsg := hub.NewConnectedMessage(client.ID)
	data, _ := json.Marshal(connMsg)
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	conn.WriteMessage(websocket.TextMessage, data)
}

// dummyHash for uniform timing on API token validation failures.
var wsDummyHash, _ = bcrypt.GenerateFromPassword([]byte("ws-dummy-comparison"), bcrypt.DefaultCost)

// validateToken checks if the token is a valid JWT or API token.
// Returns true if authentication succeeds.
func (wh *WSHandler) validateToken(c *gin.Context, rawToken string) bool {
	// Heuristic: JWT tokens contain two dots (header.payload.signature).
	if strings.Count(rawToken, ".") == 2 {
		claims := &middleware.JWTClaims{}
		tok, err := jwt.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return wh.jwtSecret, nil
		})
		if err == nil && tok.Valid {
			c.Set("user_id", claims.UserID)
			return true
		}
	}

	// Fall back to API token auth (prefix-based lookup + bcrypt).
	if len(rawToken) < token.PrefixLen {
		_ = bcrypt.CompareHashAndPassword(wsDummyHash, []byte("not-a-real-token"))
		return false
	}

	prefix := rawToken[:token.PrefixLen]
	candidates, err := wh.queries.ListAPITokensByPrefix(c.Request.Context(), prefix)
	if err != nil || len(candidates) == 0 {
		_ = bcrypt.CompareHashAndPassword(wsDummyHash, []byte("not-a-real-token"))
		return false
	}

	for i := range candidates {
		if bcrypt.CompareHashAndPassword([]byte(candidates[i].TokenHash), []byte(rawToken)) == nil {
			if candidates[i].RevokedAt.Valid {
				continue
			}
			if candidates[i].ExpiresAt.Valid && candidates[i].ExpiresAt.Time.Before(time.Now()) {
				continue
			}
			c.Set("user_id", candidates[i].UserID.String())
			return true
		}
	}

	return false
}
