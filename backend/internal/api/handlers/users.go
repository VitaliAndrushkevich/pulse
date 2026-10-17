package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

type userResponse struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	Role               string    `json:"role"`
	Enabled            bool      `json:"enabled"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
}

func toUserResponse(u db.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, Enabled: u.Enabled, MustChangePassword: u.MustChangePassword, CreatedAt: u.CreatedAt}
}

// UserHandler manages administrator-provisioned accounts.
type UserHandler struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewUserHandler creates the user management handler.
func NewUserHandler(q *db.Queries, p *pgxpool.Pool) *UserHandler { return &UserHandler{q, p} }

// Register mounts administrator-only account routes.
func (h *UserHandler) Register(r *gin.RouterGroup) {
	r.GET("/users", h.List)
	r.POST("/users", h.Create)
	r.PATCH("/users/:id", h.Update)
	r.POST("/users/:id/reset-password", h.ResetPassword)
}
func validRole(role string) bool { return role == "admin" || role == "read_only" }
func (h *UserHandler) List(c *gin.Context) {
	page, limit, paginationErr := parseStrictPagination(c)
	if paginationErr != "" {
		apiError(c, 400, "VALIDATION_ERROR", paginationErr)
		return
	}
	if int64(page-1) > int64(2147483647)/int64(limit) {
		apiError(c, 400, "VALIDATION_ERROR", "page is too large")
		return
	}
	users, err := h.queries.ListUsers(c.Request.Context(), db.ListUsersParams{Limit: int32(limit), Offset: int32((page - 1) * limit)})
	if err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to list users")
		return
	}
	total, err := h.queries.CountUsers(c.Request.Context())
	if err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to count users")
		return
	}
	data := make([]userResponse, 0, len(users))
	for _, u := range users {
		data = append(data, toUserResponse(u))
	}
	c.JSON(200, gin.H{"data": data, "page": page, "limit": limit, "total": total, "total_pages": (total + int64(limit) - 1) / int64(limit)})
}
func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}
	if c.ShouldBindJSON(&req) != nil || !validRole(req.Role) || len(req.Password) < 8 || len(req.Password) > 72 {
		apiError(c, 400, "VALIDATION_ERROR", "valid email, role and password (8–72 bytes) are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		apiError(c, 500, "INTERNAL_ERROR", "failed to hash password")
		return
	}
	user, err := h.queries.CreateManagedUser(c.Request.Context(), db.CreateManagedUserParams{Email: strings.ToLower(strings.TrimSpace(req.Email)), PasswordHash: string(hash), Role: req.Role})
	if err != nil {
		userWriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toUserResponse(user))
}
func userWriteError(c *gin.Context, err error) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		apiError(c, 409, "EMAIL_EXISTS", "email is already in use")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		apiError(c, 404, "NOT_FOUND", "user not found")
		return
	}
	apiError(c, 500, "INTERNAL_ERROR", "failed to update user")
}
func (h *UserHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, 400, "VALIDATION_ERROR", "invalid user id")
		return
	}
	var req struct {
		Role    *string `json:"role"`
		Enabled *bool   `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Role == nil && req.Enabled == nil) || (req.Role != nil && !validRole(*req.Role)) {
		apiError(c, 400, "VALIDATION_ERROR", "role or enabled is required")
		return
	}
	if id.String() == c.GetString("user_id") && ((req.Role != nil && *req.Role != "admin") || (req.Enabled != nil && !*req.Enabled)) {
		apiError(c, 409, "SELF_UPDATE_FORBIDDEN", "cannot disable or demote your own account")
		return
	}
	ctx := c.Request.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		userWriteError(c, err)
		return
	}
	defer tx.Rollback(ctx)
	q := h.queries.WithTx(tx)
	// Serializes membership changes across requests, preventing two admins from
	// concurrently removing one another and leaving no administrator.
	if err = q.LockUsers(ctx); err != nil {
		userWriteError(c, err)
		return
	}
	// Recheck actor while holding the membership lock: another request may have
	// disabled or demoted this administrator since authentication ran.
	actorID, _ := uuid.Parse(c.GetString("user_id"))
	actor, err := q.GetUser(ctx, actorID)
	if err != nil || !actor.Enabled || actor.Role != "admin" || actor.MustChangePassword {
		apiError(c, 403, "FORBIDDEN", "administrator access required")
		return
	}
	user, err := q.GetUser(ctx, id)
	if err != nil {
		userWriteError(c, err)
		return
	}
	role, enabled := user.Role, user.Enabled
	if req.Role != nil {
		role = *req.Role
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if user.Role == "admin" && user.Enabled && (role != "admin" || !enabled) {
		count, e := q.CountActiveAdmins(ctx)
		if e != nil {
			userWriteError(c, e)
			return
		}
		if count <= 1 {
			apiError(c, 409, "LAST_ADMIN", "cannot remove the last active administrator")
			return
		}
	}
	user, err = q.UpdateManagedUser(ctx, db.UpdateManagedUserParams{ID: id, Role: role, Enabled: enabled})
	if err != nil {
		userWriteError(c, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		userWriteError(c, err)
		return
	}
	c.JSON(200, toUserResponse(user))
}
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		apiError(c, 400, "VALIDATION_ERROR", "invalid user id")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Password) < 8 || len(req.Password) > 72 {
		apiError(c, 400, "VALIDATION_ERROR", "password must be between 8 and 72 bytes")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		userWriteError(c, err)
		return
	}
	user, err := h.queries.ResetUserPassword(c.Request.Context(), db.ResetUserPasswordParams{ID: id, PasswordHash: string(hash)})
	if err != nil {
		userWriteError(c, err)
		return
	}
	c.JSON(200, toUserResponse(user))
}
