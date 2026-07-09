package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
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

// ChangePassword handles PUT /auth/password.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new password must be at least 8 characters")
		return
	}

	if len(req.NewPassword) > 72 {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new password must be at most 72 bytes")
		return
	}

	userID := c.GetString("user_id")
	uid, _ := uuid.Parse(userID)

	user, err := h.queries.GetUser(c.Request.Context(), uid)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve user")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "current password is incorrect")
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)

	_, err = h.queries.UpdateUserPassword(c.Request.Context(), db.UpdateUserPasswordParams{
		ID:           uid,
		PasswordHash: string(hash),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update password")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}
