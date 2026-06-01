package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSanitizeHeaders_RedactsAuthorization(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret-token-value")
	headers.Set("Content-Type", "application/json")

	sanitized := sanitizeHeaders(headers)

	if sanitized.Get("Authorization") != "[REDACTED]" {
		t.Errorf("expected Authorization to be [REDACTED], got %q", sanitized.Get("Authorization"))
	}
	if sanitized.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type to be preserved, got %q", sanitized.Get("Content-Type"))
	}
}

func TestSanitizeHeaders_NoAuthHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	sanitized := sanitizeHeaders(headers)

	if sanitized.Get("Authorization") != "" {
		t.Errorf("expected Authorization to remain empty, got %q", sanitized.Get("Authorization"))
	}
	if sanitized.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type to be preserved, got %q", sanitized.Get("Content-Type"))
	}
}

func TestSanitizeHeaders_DoesNotMutateOriginal(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer my-secret")

	_ = sanitizeHeaders(headers)

	if headers.Get("Authorization") != "Bearer my-secret" {
		t.Errorf("original headers were mutated: got %q", headers.Get("Authorization"))
	}
}

func TestSanitizedLogger_RedactsTokenInOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput string
	logger := gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		sanitized := sanitizeHeaders(param.Request.Header)
		logOutput = sanitized.Get("Authorization")
		return ""
	})

	r := gin.New()
	r.Use(logger)
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token-12345")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if strings.Contains(logOutput, "super-secret-token-12345") {
		t.Error("raw token appeared in log output")
	}
	if logOutput != "[REDACTED]" {
		t.Errorf("expected [REDACTED] in log output, got %q", logOutput)
	}
}
