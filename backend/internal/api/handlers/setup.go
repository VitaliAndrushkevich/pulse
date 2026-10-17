package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/VitaliAndrushkevich/pulse/internal/api/middleware"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// SetupHandler provides the initial admin user creation endpoint.
// Only works when no users exist in the database, unless resetAdmin mode is active.
type SetupHandler struct {
	queries    *db.Queries
	pool       *pgxpool.Pool
	jwtSecret  []byte
	jwtExpiry  time.Duration
	resetAdmin bool
}

// NewSetupHandler creates a setup handler with the given dependencies.
// When resetAdmin is true, the handler re-enables the setup flow regardless of existing users.
func NewSetupHandler(queries *db.Queries, jwtSecret []byte, jwtExpiry time.Duration, resetAdmin bool, pools ...*pgxpool.Pool) *SetupHandler {
	var pool *pgxpool.Pool
	if len(pools) > 0 {
		pool = pools[0]
	}
	return &SetupHandler{
		queries:    queries,
		pool:       pool,
		jwtSecret:  jwtSecret,
		jwtExpiry:  jwtExpiry,
		resetAdmin: resetAdmin,
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
// When resetAdmin mode is active, always reports setup_required: true.
func (h *SetupHandler) Status(c *gin.Context) {
	if h.resetAdmin {
		c.JSON(http.StatusOK, setupStatusResponse{SetupRequired: true})
		return
	}

	count, err := h.queries.CountUsers(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check setup status")
		return
	}

	c.JSON(http.StatusOK, setupStatusResponse{
		SetupRequired: count == 0,
	})
}

// Setup creates the initial admin user or overwrites existing credentials in admin reset mode.
// Returns 409 if a user already exists and reset mode is not active.
func (h *SetupHandler) Setup(c *gin.Context) {
	ctx := c.Request.Context()
	q := h.queries
	commit := func() error { return nil }
	if h.pool != nil {
		tx, err := h.pool.Begin(ctx)
		if err != nil {
			apiError(c, 500, "INTERNAL_ERROR", "failed to start setup")
			return
		}
		defer tx.Rollback(ctx)
		q = q.WithTx(tx)
		if err = q.LockUsers(ctx); err != nil {
			apiError(c, 500, "INTERNAL_ERROR", "failed to lock setup")
			return
		}
		commit = func() error { return tx.Commit(ctx) }
	}

	count, err := q.CountUsers(ctx)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check setup status")
		return
	}

	// Reject if user exists and reset mode is off.
	if count > 0 && !h.resetAdmin {
		apiError(c, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "initial setup has already been completed")
		return
	}

	// Validate request body.
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required")
		return
	}

	if len(req.Password) < 8 || len(req.Password) > 72 {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "password must be between 8 and 72 bytes")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	if count > 0 && h.resetAdmin {
		// Overwrite existing user credentials, preserving UUID for FK integrity.
		existingUser, err := q.GetFirstUser(ctx)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve existing user")
			return
		}

		updatedUser, err := q.UpdateUserEmailAndPassword(ctx, db.UpdateUserEmailAndPasswordParams{
			ID:           existingUser.ID,
			Email:        strings.ToLower(strings.TrimSpace(req.Email)),
			PasswordHash: string(hash),
		})
		if err != nil {
			apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update user")
			return
		}

		if err = commit(); err != nil {
			apiError(c, 500, "INTERNAL_ERROR", "failed to commit setup")
			return
		}
		token, expiresAt, err := middleware.GenerateJWT(h.jwtSecret, updatedUser.ID, updatedUser.Email, h.jwtExpiry, updatedUser.SessionVersion)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "user updated but failed to generate token")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_at": expiresAt,
			"user":       toUserResponse(updatedUser),
		})
		return
	}

	// Create new user (no existing user, or reset mode with no user).
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordHash: string(hash),
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user")
		return
	}

	if err = commit(); err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to commit setup")
		return
	}
	token, expiresAt, err := middleware.GenerateJWT(h.jwtSecret, user.ID, user.Email, h.jwtExpiry, user.SessionVersion)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "user created but failed to generate token")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":      token,
		"expires_at": expiresAt,
		"user":       toUserResponse(user),
	})
}
