package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// AuthHandler provides authentication endpoints.
type AuthHandler struct {
	queries   *db.Queries
	jwtSecret []byte
	jwtExpiry time.Duration
}

// NewAuthHandler creates an auth handler with the given dependencies.
func NewAuthHandler(queries *db.Queries, jwtSecret []byte, jwtExpiry time.Duration) *AuthHandler {
	return &AuthHandler{
		queries:   queries,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Register mounts auth routes on the given router group (public, no auth required).
func (h *AuthHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.Login)
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required")
		return
	}

	user, err := h.queries.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		// Use dummy bcrypt compare to prevent timing attacks revealing whether email exists.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"), []byte(req.Password))
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}

	token, expiresAt, err := middleware.GenerateJWT(h.jwtSecret, user.ID, user.Email, h.jwtExpiry)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}
