package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// SetupHandler provides the initial admin user creation endpoint.
// Only works when no users exist in the database.
type SetupHandler struct {
	queries   *db.Queries
	jwtSecret []byte
	jwtExpiry time.Duration
}

// NewSetupHandler creates a setup handler with the given dependencies.
func NewSetupHandler(queries *db.Queries, jwtSecret []byte, jwtExpiry time.Duration) *SetupHandler {
	return &SetupHandler{
		queries:   queries,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

type setupRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type setupStatusResponse struct {
	SetupRequired bool `json:"setup_required"`
}

// Register mounts setup routes on the given router group (public, no auth required).
func (h *SetupHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/auth/setup", h.Status)
	rg.POST("/auth/setup", h.Setup)
}

// Status returns whether initial setup is required (no users exist).
func (h *SetupHandler) Status(c *gin.Context) {
	count, err := h.queries.CountUsers(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check setup status")
		return
	}

	c.JSON(http.StatusOK, setupStatusResponse{
		SetupRequired: count == 0,
	})
}

// Setup creates the initial admin user. Returns 409 if any user already exists.
func (h *SetupHandler) Setup(c *gin.Context) {
	count, err := h.queries.CountUsers(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check setup status")
		return
	}

	if count > 0 {
		apiError(c, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "initial setup has already been completed")
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required")
		return
	}

	if len(req.Password) < 8 {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	user, err := h.queries.CreateUser(c.Request.Context(), db.CreateUserParams{
		Email:        req.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user")
		return
	}

	// Auto-login: return a JWT so the user is immediately authenticated.
	token, expiresAt, err := middleware.GenerateJWT(h.jwtSecret, user.ID, user.Email, h.jwtExpiry)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "user created but failed to generate token")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":      token,
		"expires_at": expiresAt,
	})
}
