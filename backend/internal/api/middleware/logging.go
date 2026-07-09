package middleware

import (
	"fmt"
	"net/http"
	"strings"
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

// sanitizePath redacts the token query parameter from URL paths to prevent
// credential leakage in logs (e.g. /ws?token=eyJ... → /ws?token=[REDACTED]).
func sanitizePath(rawPath string) string {
	idx := strings.Index(rawPath, "token=")
	if idx == -1 {
		return rawPath
	}
	// Find the end of the token value (next & or end of string).
	valueStart := idx + len("token=")
	valueEnd := strings.Index(rawPath[valueStart:], "&")
	if valueEnd == -1 {
		return rawPath[:valueStart] + "[REDACTED]"
	}
	return rawPath[:valueStart] + "[REDACTED]" + rawPath[valueStart+valueEnd:]
}

// SanitizedLogger returns a gin.HandlerFunc that logs requests using
// gin.LoggerWithFormatter, ensuring that Authorization header values and
// token query parameters never appear in log output.
func SanitizedLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		sanitized := sanitizeHeaders(param.Request.Header)
		auth := sanitized.Get("Authorization")
		logPath := sanitizePath(param.Path)

		if auth != "" {
			return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s | auth=%s\n",
				param.TimeStamp.Format(time.RFC3339),
				param.StatusCode,
				param.Latency,
				param.Method,
				logPath,
				auth,
			)
		}

		return fmt.Sprintf("[PULSE] %s | %d | %s | %s | %s\n",
			param.TimeStamp.Format(time.RFC3339),
			param.StatusCode,
			param.Latency,
			param.Method,
			logPath,
		)
	})
}
