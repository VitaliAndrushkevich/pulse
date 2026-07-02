package config

import (
	"os"
	"testing"
	"time"
)

// setEnv is a test helper that sets an env var and registers cleanup.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func clearAllPulseMCPEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PULSE_MCP_API_BASE_URL",
		"PULSE_MCP_API_TOKEN",
		"PULSE_MCP_ACCESS_MODE",
		"PULSE_MCP_TRANSPORT",
		"PULSE_MCP_HTTP_ADDR",
		"PULSE_MCP_REQUEST_TIMEOUT",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoad_MissingToken_ReturnsError(t *testing.T) {
	clearAllPulseMCPEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when PULSE_MCP_API_TOKEN is missing")
	}
	if err != ErrMissingAPIToken {
		t.Fatalf("expected ErrMissingAPIToken, got: %v", err)
	}
}

func TestLoad_EmptyToken_ReturnsError(t *testing.T) {
	clearAllPulseMCPEnv(t)
	setEnv(t, "PULSE_MCP_API_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when PULSE_MCP_API_TOKEN is empty")
	}
	if err != ErrMissingAPIToken {
		t.Fatalf("expected ErrMissingAPIToken, got: %v", err)
	}
}

func TestLoad_WhitespaceOnlyToken_ReturnsError(t *testing.T) {
	clearAllPulseMCPEnv(t)
	setEnv(t, "PULSE_MCP_API_TOKEN", "   \t\n  ")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when PULSE_MCP_API_TOKEN is whitespace-only")
	}
	if err != ErrMissingAPIToken {
		t.Fatalf("expected ErrMissingAPIToken, got: %v", err)
	}
}

func TestLoad_ValidToken_DefaultConfig(t *testing.T) {
	clearAllPulseMCPEnv(t)
	setEnv(t, "PULSE_MCP_API_TOKEN", "test-token-123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIToken != "test-token-123" {
		t.Errorf("APIToken = %q, want %q", cfg.APIToken, "test-token-123")
	}
	if cfg.APIBaseURL != DefaultAPIBaseURL {
		t.Errorf("APIBaseURL = %q, want %q", cfg.APIBaseURL, DefaultAPIBaseURL)
	}
	if cfg.AccessMode != ReadOnly {
		t.Errorf("AccessMode = %q, want %q", cfg.AccessMode, ReadOnly)
	}
	if cfg.Transport != DefaultTransport {
		t.Errorf("Transport = %q, want %q", cfg.Transport, DefaultTransport)
	}
	if cfg.HTTPAddr != DefaultHTTPAddr {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, DefaultHTTPAddr)
	}
	if cfg.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("RequestTimeout = %v, want %v", cfg.RequestTimeout, DefaultRequestTimeout)
	}
}

func TestLoad_AllEnvVarsSet(t *testing.T) {
	clearAllPulseMCPEnv(t)
	setEnv(t, "PULSE_MCP_API_TOKEN", "my-secret-token")
	setEnv(t, "PULSE_MCP_API_BASE_URL", "https://pulse.example.com/api/v1")
	setEnv(t, "PULSE_MCP_ACCESS_MODE", "read-write")
	setEnv(t, "PULSE_MCP_TRANSPORT", "http")
	setEnv(t, "PULSE_MCP_HTTP_ADDR", ":8081")
	setEnv(t, "PULSE_MCP_REQUEST_TIMEOUT", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIToken != "my-secret-token" {
		t.Errorf("APIToken = %q, want %q", cfg.APIToken, "my-secret-token")
	}
	if cfg.APIBaseURL != "https://pulse.example.com/api/v1" {
		t.Errorf("APIBaseURL = %q, want %q", cfg.APIBaseURL, "https://pulse.example.com/api/v1")
	}
	if cfg.AccessMode != ReadWrite {
		t.Errorf("AccessMode = %q, want %q", cfg.AccessMode, ReadWrite)
	}
	if cfg.Transport != "http" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "http")
	}
	if cfg.HTTPAddr != ":8081" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8081")
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("RequestTimeout = %v, want %v", cfg.RequestTimeout, 30*time.Second)
	}
}

func TestParseAccessMode(t *testing.T) {
	tests := []struct {
		input string
		want  AccessMode
	}{
		{"read-write", ReadWrite},
		{"READ-WRITE", ReadWrite},
		{"Read-Write", ReadWrite},
		{" read-write ", ReadWrite},
		{"read-only", ReadOnly},
		{"READ-ONLY", ReadOnly},
		{"", ReadOnly},
		{"invalid", ReadOnly},
		{"readwrite", ReadOnly},
		{"write", ReadOnly},
		{"  ", ReadOnly},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseAccessMode(tt.input)
			if got != tt.want {
				t.Errorf("parseAccessMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"", DefaultRequestTimeout},
		{"5s", 5 * time.Second},
		{"1m", time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"invalid", DefaultRequestTimeout},
		{"-1s", DefaultRequestTimeout},
		{"0s", DefaultRequestTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTimeout(tt.input)
			if got != tt.want {
				t.Errorf("parseTimeout(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
