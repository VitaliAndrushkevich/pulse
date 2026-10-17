package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      userResponse `json:"user"`
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

	user, err := h.queries.GetUserByEmail(c.Request.Context(), strings.TrimSpace(req.Email))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "authentication is temporarily unavailable")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Use dummy bcrypt compare to prevent timing attacks revealing whether email exists.
		_ = bcrypt.CompareHashAndPassword(wsDummyHash, []byte(req.Password))
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil || !user.Enabled {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}

	token, expiresAt, err := middleware.GenerateJWT(h.jwtSecret, user.ID, user.Email, h.jwtExpiry, user.SessionVersion)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      toUserResponse(user),
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

	if user.SessionVersion != c.GetInt64("session_version") {
		apiError(c, 401, "UNAUTHORIZED", "session changed; sign in again")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "current password is incorrect")
		return
	}

	if req.CurrentPassword == req.NewPassword {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new password must differ from current password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	user, err = h.queries.UpdateUserPassword(c.Request.Context(), db.UpdateUserPasswordParams{
		ID:             uid,
		PasswordHash:   string(hash),
		SessionVersion: user.SessionVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiError(c, 401, "UNAUTHORIZED", "session changed; sign in again")
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update password")
		return
	}

	h.respondSession(c, http.StatusOK, user)
}

// Me returns the currently authenticated identity without exposing credentials.
func (h *AuthHandler) Me(c *gin.Context) {
	uid, _ := uuid.Parse(c.GetString("user_id"))
	user, err := h.queries.GetUser(c.Request.Context(), uid)
	if err != nil {
		apiError(c, 401, "UNAUTHORIZED", "invalid user")
		return
	}
	c.JSON(200, gin.H{"user": toUserResponse(user)})
}

func (h *AuthHandler) respondSession(c *gin.Context, status int, user db.User) {
	token, expiry, err := middleware.GenerateJWT(h.jwtSecret, user.ID, user.Email, h.jwtExpiry, user.SessionVersion)
	if err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to generate token")
		return
	}
	c.JSON(status, loginResponse{Token: token, ExpiresAt: expiry, User: toUserResponse(user)})
}
