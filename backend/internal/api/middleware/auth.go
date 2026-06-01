// Package middleware provides HTTP middleware for the Pulse API.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/token"
)

// dummyHash is pre-computed at init time so that failure paths always perform
// a bcrypt comparison, ensuring uniform response timing regardless of whether
// a prefix match was found.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-comparison-target"), bcrypt.DefaultCost)

// BearerAuth returns a gin middleware that validates Bearer tokens against the
// database. On success it sets "user_id" in the gin context and calls c.Next().
// On any failure it returns a 401 with a static error message.
func BearerAuth(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Extract Authorization: Bearer <token> header.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			failWithDummyCompare(c)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		if len(rawToken) < token.PrefixLen {
			failWithDummyCompare(c)
			return
		}

		// 2. Parse prefix (first 8 chars) from the token.
		prefix := rawToken[:token.PrefixLen]

		// 3. Query ListAPITokensByPrefix(prefix) — returns candidate rows.
		candidates, err := queries.ListAPITokensByPrefix(c.Request.Context(), prefix)
		if err != nil || len(candidates) == 0 {
			failWithDummyCompare(c)
			return
		}

		// 4. For each candidate, bcrypt.CompareHashAndPassword(hash, token).
		var matched *db.ApiToken
		for i := range candidates {
			if bcrypt.CompareHashAndPassword([]byte(candidates[i].TokenHash), []byte(rawToken)) == nil {
				matched = &candidates[i]
				break
			}
		}

		if matched == nil {
			failWithDummyCompare(c)
			return
		}

		// 5. Defense-in-depth: check revoked_at (must be NULL) and expires_at (must be NULL or future).
		// The SQL query already filters these, but we verify in Go as well.
		if matched.RevokedAt.Valid {
			failWithDummyCompare(c)
			return
		}
		if matched.ExpiresAt.Valid && matched.ExpiresAt.Time.Before(time.Now()) {
			failWithDummyCompare(c)
			return
		}

		// 6. On success: TouchAPIToken(id), set user_id in gin context, call c.Next().
		_ = queries.TouchAPIToken(c.Request.Context(), matched.ID)
		c.Set("user_id", matched.UserID.String())
		c.Next()
	}
}

// failWithDummyCompare performs a dummy bcrypt comparison to ensure uniform
// timing on all failure paths, then returns a 401 with a static error message.
func failWithDummyCompare(c *gin.Context) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("not-a-real-token"))
	c.JSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "UNAUTHORIZED",
			"message": "invalid or expired token",
		},
	})
	c.Abort()
}
