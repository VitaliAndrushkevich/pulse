// Package config handles environment-based configuration for the pulse-mcp server.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// AccessMode represents the server's capability level.
type AccessMode string

const (
	// ReadOnly allows only read tools to be registered and invoked.
	ReadOnly AccessMode = "read-only"
	// ReadWrite allows read tools and permitted write tools.
	ReadWrite AccessMode = "read-write"
)

// Config holds all runtime configuration for the pulse-mcp server.
type Config struct {
	// APIBaseURL is the base URL of the Pulse REST API.
	APIBaseURL string

	// APIToken is the Pulse Bearer API token used for authentication.
	APIToken string

	// AccessMode controls whether write tools are available.
	AccessMode AccessMode

	// Transport selects the MCP transport: "stdio" or "http".
	Transport string

	// HTTPAddr is the bind address when transport is "http".
	HTTPAddr string

	// RequestTimeout is the per-request timeout to the Pulse API.
	RequestTimeout time.Duration
}

// Default configuration values.
const (
	DefaultAPIBaseURL      = "http://localhost:8080/api/v1"
	DefaultAccessMode      = ReadOnly
	DefaultTransport       = "stdio"
	DefaultHTTPAddr        = ":9090"
	DefaultRequestTimeout  = 15 * time.Second
)

// ErrMissingAPIToken is returned when PULSE_MCP_API_TOKEN is absent, empty, or whitespace-only.
var ErrMissingAPIToken = errors.New("PULSE_MCP_API_TOKEN is required but was absent, empty, or whitespace-only")

// Load reads configuration from environment variables and applies defaults.
// It returns an error if PULSE_MCP_API_TOKEN is absent, empty, or whitespace-only.
func Load() (*Config, error) {
	token := os.Getenv("PULSE_MCP_API_TOKEN")
	if strings.TrimSpace(token) == "" {
		return nil, ErrMissingAPIToken
	}

	cfg := &Config{
		APIBaseURL:     envOrDefault("PULSE_MCP_API_BASE_URL", DefaultAPIBaseURL),
		APIToken:       token,
		AccessMode:     parseAccessMode(os.Getenv("PULSE_MCP_ACCESS_MODE")),
		Transport:      envOrDefault("PULSE_MCP_TRANSPORT", DefaultTransport),
		HTTPAddr:       envOrDefault("PULSE_MCP_HTTP_ADDR", DefaultHTTPAddr),
		RequestTimeout: parseTimeout(os.Getenv("PULSE_MCP_REQUEST_TIMEOUT")),
	}

	return cfg, nil
}

// parseAccessMode maps a raw string to an AccessMode.
// Anything other than "read-write" resolves to ReadOnly.
func parseAccessMode(raw string) AccessMode {
	if strings.TrimSpace(strings.ToLower(raw)) == "read-write" {
		return ReadWrite
	}
	return ReadOnly
}

// parseTimeout parses a duration string, falling back to DefaultRequestTimeout
// on empty or malformed input.
func parseTimeout(raw string) time.Duration {
	if raw == "" {
		return DefaultRequestTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultRequestTimeout
	}
	return d
}

// envOrDefault returns the value of the environment variable named by key,
// or fallback if the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
