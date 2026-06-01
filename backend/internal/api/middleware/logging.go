package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// sanitizeHeaders returns a copy of the provided headers with sensitive values
// replaced by "[REDACTED]". Currently redacts the Authorization header.
func sanitizeHeaders(headers http.Header) http.Header {
	clean := headers.Clone()
	if clean.Get("Authorization") != "" {
		clean.Set("Authorization", "[REDACTED]")
	}
	return clean
}

// SanitizedLogger returns a gin.HandlerFunc that logs requests using
// gin.LoggerWithFormatter, ensuring that Authorization header values never
// appear in log output.
func SanitizedLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		sanitized := sanitizeHeaders(param.Request.Header)
		auth := sanitized.Get("Authorization")

		if auth != "" {
			return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s | auth=%s\n",
				param.TimeStamp.Format(time.RFC3339),
				param.StatusCode,
				param.Latency,
				param.Method,
				param.Path,
				auth,
			)
		}

		return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s\n",
			param.TimeStamp.Format(time.RFC3339),
			param.StatusCode,
			param.Latency,
			param.Method,
			param.Path,
		)
	})
}
