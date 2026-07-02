package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vandrushkevich/pulse/mcp/internal/config"
	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// createFakeClient returns a fakePulseClient with a configurable CreateMonitor func.
func createFakeClient(fn func(ctx context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error)) *fakePulseClient {
	return &fakePulseClient{
		createMonitorFunc: fn,
	}
}

func TestHandleCreateMonitor_ReadOnlyMode(t *testing.T) {
	called := false
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		called = true
		return pulseapi.Monitor{}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:   "HTTP",
		Name:   "Test Monitor",
		Target: "https://example.com",
	})

	if err == nil {
		t.Fatal("expected error for read-only mode")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeWriteDisabled {
		t.Errorf("expected code %q, got %q", mcperr.CodeWriteDisabled, mcpErr.Code)
	}
	if called {
		t.Error("Pulse API should NOT be called in read-only mode")
	}
}

func TestHandleCreateMonitor_InvalidType(t *testing.T) {
	cases := []string{"gRPC", "DNS", "SMTP", "QUIC", "websocket", ""}

	for _, typ := range cases {
		t.Run(typ, func(t *testing.T) {
			called := false
			client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
				called = true
				return pulseapi.Monitor{}, nil
			})

			deps := Deps{Client: client, AccessMode: config.ReadWrite}
			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   typ,
				Name:   "Test",
				Target: "example.com",
			})

			if err == nil {
				t.Fatal("expected error for unsupported type")
			}
			var mcpErr *mcperr.MCPError
			if !errors.As(err, &mcpErr) {
				t.Fatalf("expected MCPError, got %T", err)
			}
			if mcpErr.Code != mcperr.CodeInvalidType {
				t.Errorf("expected code %q, got %q", mcperr.CodeInvalidType, mcpErr.Code)
			}
			if !strings.Contains(mcpErr.Message, "HTTP, TCP, UDP, ICMP") {
				t.Errorf("error message should list supported types, got: %s", mcpErr.Message)
			}
			if called {
				t.Error("Pulse API should NOT be called for invalid type")
			}
		})
	}
}

func TestHandleCreateMonitor_TypeNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HTTP", "http"},
		{"http", "http"},
		{"Http", "http"},
		{"TCP", "tcp"},
		{"tcp", "tcp"},
		{"UDP", "udp"},
		{"udp", "udp"},
		{"ICMP", "icmp"},
		{"icmp", "icmp"},
		{" HTTP ", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var gotType string
			client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
				gotType = in.Type
				return pulseapi.Monitor{
					ID: "mon-1", Name: "Test", Type: in.Type,
					Target: in.Target, Status: "pending", State: "active",
					IntervalSeconds: 60, TimeoutSeconds: 10,
				}, nil
			})

			deps := Deps{Client: client, AccessMode: config.ReadWrite}
			target := "https://example.com"
			if tt.expected == "tcp" || tt.expected == "udp" {
				target = "example.com:80"
			}
			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   tt.input,
				Name:   "Test",
				Target: target,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.expected {
				t.Errorf("expected normalized type %q, got %q", tt.expected, gotType)
			}
		})
	}
}

func TestHandleCreateMonitor_NameValidation(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"tabs only", "\t\t", true},
		{"too long", strings.Repeat("a", 256), true},
		{"valid short", "X", false},
		{"valid 255", strings.Repeat("b", 255), false},
		{"valid normal", "My Monitor", false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
				called = true
				return pulseapi.Monitor{
					ID: "mon-1", Name: in.Name, Type: "http",
					Target: "https://example.com", Status: "pending", State: "active",
					IntervalSeconds: 60, TimeoutSeconds: 10,
				}, nil
			})

			deps := Deps{Client: client, AccessMode: config.ReadWrite}
			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   "HTTP",
				Name:   tt.input,
				Target: "https://example.com",
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid name")
				}
				var mcpErr *mcperr.MCPError
				if !errors.As(err, &mcpErr) {
					t.Fatalf("expected MCPError, got %T", err)
				}
				if mcpErr.Code != mcperr.CodeValidationError {
					t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
				}
				if called {
					t.Error("Pulse API should NOT be called for invalid name")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !called {
					t.Error("Pulse API should have been called for valid name")
				}
			}
		})
	}
}

func TestHandleCreateMonitor_TargetValidation(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		target  string
		wantErr bool
	}{
		{"empty target", "HTTP", "", true},
		{"http valid url", "HTTP", "https://example.com", false},
		{"http bare host", "HTTP", "example.com", false},
		{"tcp with port", "TCP", "example.com:5432", false},
		{"tcp missing port", "TCP", "example.com", true},
		{"udp with port", "UDP", "example.com:53", false},
		{"udp missing port", "UDP", "example.com", true},
		{"icmp hostname", "ICMP", "example.com", false},
		{"icmp ip", "ICMP", "192.168.1.1", false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
				return pulseapi.Monitor{
					ID: "mon-1", Name: "Test", Type: in.Type,
					Target: in.Target, Status: "pending", State: "active",
					IntervalSeconds: 60, TimeoutSeconds: 10,
				}, nil
			})

			deps := Deps{Client: client, AccessMode: config.ReadWrite}
			_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
				Type:   tt.typ,
				Name:   "Test",
				Target: tt.target,
			})

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHandleCreateMonitor_Success(t *testing.T) {
	client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		return pulseapi.Monitor{
			ID:              "mon-abc-123",
			Name:            in.Name,
			Type:            in.Type,
			Target:          in.Target,
			Status:          "pending",
			State:           "active",
			IntervalSeconds: 60,
			TimeoutSeconds:  10,
		}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	out, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:   "HTTP",
		Name:   "My Website",
		Target: "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.ID != "mon-abc-123" {
		t.Errorf("expected id 'mon-abc-123', got %q", out.ID)
	}
	if out.Name != "My Website" {
		t.Errorf("expected name 'My Website', got %q", out.Name)
	}
	if out.Type != "http" {
		t.Errorf("expected type 'http', got %q", out.Type)
	}
	if out.Target != "https://example.com" {
		t.Errorf("expected target 'https://example.com', got %q", out.Target)
	}
	if out.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", out.Status)
	}
	if out.State != "active" {
		t.Errorf("expected state 'active', got %q", out.State)
	}
	if out.IntervalSeconds != 60 {
		t.Errorf("expected interval 60, got %d", out.IntervalSeconds)
	}
	if out.TimeoutSeconds != 10 {
		t.Errorf("expected timeout 10, got %d", out.TimeoutSeconds)
	}
}

func TestHandleCreateMonitor_WithOptionalFields(t *testing.T) {
	interval := 30
	timeout := 5

	var gotInput pulseapi.CreateMonitorInput
	client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		gotInput = in
		return pulseapi.Monitor{
			ID: "mon-1", Name: in.Name, Type: in.Type, Target: in.Target,
			Status: "pending", State: "active",
			IntervalSeconds: 30, TimeoutSeconds: 5,
		}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	out, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:            "HTTP",
		Name:            "API Check",
		Target:          "https://api.example.com/health",
		IntervalSeconds: &interval,
		TimeoutSeconds:  &timeout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotInput.IntervalSeconds == nil || *gotInput.IntervalSeconds != 30 {
		t.Error("expected interval_seconds=30 passed to Pulse")
	}
	if gotInput.TimeoutSeconds == nil || *gotInput.TimeoutSeconds != 5 {
		t.Error("expected timeout_seconds=5 passed to Pulse")
	}
	if out.IntervalSeconds != 30 {
		t.Errorf("expected output interval 30, got %d", out.IntervalSeconds)
	}
	if out.TimeoutSeconds != 5 {
		t.Errorf("expected output timeout 5, got %d", out.TimeoutSeconds)
	}
}

func TestHandleCreateMonitor_HTTPExpectedStatuses(t *testing.T) {
	var gotSettings map[string]any
	client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		gotSettings = in.Settings
		return pulseapi.Monitor{
			ID: "mon-1", Name: in.Name, Type: in.Type, Target: in.Target,
			Status: "pending", State: "active",
			IntervalSeconds: 60, TimeoutSeconds: 10,
		}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:                 "HTTP",
		Name:                 "Status Check",
		Target:               "https://example.com",
		HTTPExpectedStatuses: []int{200, 201, 301},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, ok := gotSettings["expected_statuses"]
	if !ok {
		t.Fatal("expected 'expected_statuses' in settings")
	}
	statusList, ok := statuses.([]int)
	if !ok {
		t.Fatalf("expected []int, got %T", statuses)
	}
	if len(statusList) != 3 {
		t.Errorf("expected 3 statuses, got %d", len(statusList))
	}
}

func TestHandleCreateMonitor_HTTPExpectedStatusesInvalid(t *testing.T) {
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		t.Fatal("Pulse API should not be called")
		return pulseapi.Monitor{}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:                 "HTTP",
		Name:                 "Status Check",
		Target:               "https://example.com",
		HTTPExpectedStatuses: []int{200, 600},
	})
	if err == nil {
		t.Fatal("expected error for invalid status code")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleCreateMonitor_HTTPExpectedStatusesIgnoredForNonHTTP(t *testing.T) {
	var gotSettings map[string]any
	client := createFakeClient(func(_ context.Context, in pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		gotSettings = in.Settings
		return pulseapi.Monitor{
			ID: "mon-1", Name: "Test", Type: in.Type, Target: in.Target,
			Status: "pending", State: "active",
			IntervalSeconds: 60, TimeoutSeconds: 10,
		}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:                 "TCP",
		Name:                 "TCP Check",
		Target:               "example.com:5432",
		HTTPExpectedStatuses: []int{200},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := gotSettings["expected_statuses"]; ok {
		t.Error("expected_statuses should NOT be set for non-HTTP type")
	}
}

func TestHandleCreateMonitor_PulseRejectsTarget(t *testing.T) {
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		return pulseapi.Monitor{}, &pulseapi.PulseError{
			Code:       "INVALID_TARGET",
			Message:    "target 'not-a-valid-url' is not a valid HTTP URL",
			RequestID:  "req-456",
			HTTPStatus: 422,
		}
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:   "HTTP",
		Name:   "Bad Target",
		Target: "not-a-valid-url",
	})
	if err == nil {
		t.Fatal("expected error when Pulse rejects target")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != "INVALID_TARGET" {
		t.Errorf("expected preserved Pulse code 'INVALID_TARGET', got %q", mcpErr.Code)
	}
	if mcpErr.RequestID != "req-456" {
		t.Errorf("expected request ID 'req-456', got %q", mcpErr.RequestID)
	}
}

func TestHandleCreateMonitor_IntervalValidation(t *testing.T) {
	invalid := 0
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		t.Fatal("Pulse API should not be called")
		return pulseapi.Monitor{}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:            "HTTP",
		Name:            "Test",
		Target:          "https://example.com",
		IntervalSeconds: &invalid,
	})
	if err == nil {
		t.Fatal("expected error for interval < 1")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleCreateMonitor_TimeoutValidation(t *testing.T) {
	invalid := -1
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		t.Fatal("Pulse API should not be called")
		return pulseapi.Monitor{}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadWrite}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:           "HTTP",
		Name:           "Test",
		Target:         "https://example.com",
		TimeoutSeconds: &invalid,
	})
	if err == nil {
		t.Fatal("expected error for timeout < 1")
	}
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeValidationError {
		t.Errorf("expected code %q, got %q", mcperr.CodeValidationError, mcpErr.Code)
	}
}

func TestHandleCreateMonitor_ValidationOrder(t *testing.T) {
	// When read-only mode AND invalid type AND invalid name are all present,
	// the first error should be WRITE_DISABLED (validation order rule).
	client := createFakeClient(func(_ context.Context, _ pulseapi.CreateMonitorInput) (pulseapi.Monitor, error) {
		t.Fatal("Pulse API should not be called")
		return pulseapi.Monitor{}, nil
	})

	deps := Deps{Client: client, AccessMode: config.ReadOnly}
	_, err := HandleCreateMonitor(context.Background(), deps, CreateMonitorToolInput{
		Type:   "INVALID",
		Name:   "",
		Target: "",
	})
	var mcpErr *mcperr.MCPError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("expected MCPError, got %T", err)
	}
	if mcpErr.Code != mcperr.CodeWriteDisabled {
		t.Errorf("expected WRITE_DISABLED first in validation order, got %q", mcpErr.Code)
	}
}
