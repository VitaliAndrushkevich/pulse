package smtp

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
)

func sampleTemplateData() notification.TemplateData {
	return notification.TemplateData{
		Monitor: notification.MonitorData{
			ID:     uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
			Name:   "API Server",
			URL:    "https://api.example.com/health",
			Target: "https://api.example.com/health",
		},
		Status:         "down",
		PreviousStatus: "up",
		ResponseTime:   1250,
		Incident: notification.IncidentData{
			ID:        uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			StartedAt: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
			Duration:  5*time.Minute + 30*time.Second,
		},
		Timestamp: time.Date(2024, 6, 15, 10, 35, 30, 0, time.UTC),
		BaseURL:   "https://pulse.example.com",
	}
}

func TestFormatSubject(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		monitor  string
		expected string
	}{
		{
			name:     "down status",
			status:   "down",
			monitor:  "API Server",
			expected: "[Pulse] API Server - Down",
		},
		{
			name:     "up status",
			status:   "up",
			monitor:  "Web App",
			expected: "[Pulse] Web App - Recovered",
		},
		{
			name:     "degraded status",
			status:   "degraded",
			monitor:  "Database",
			expected: "[Pulse] Database - Degraded",
		},
		{
			name:     "ssl_expiring status",
			status:   "ssl_expiring",
			monitor:  "Gateway",
			expected: "[Pulse] Gateway - SSL Expiring",
		},
		{
			name:     "n_failures_in_row status",
			status:   "n_failures_in_row",
			monitor:  "Payment",
			expected: "[Pulse] Payment - Consecutive Failures",
		},
		{
			name:     "unknown status",
			status:   "custom_thing",
			monitor:  "Service",
			expected: "[Pulse] Service - Status Change",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := notification.TemplateData{
				Monitor: notification.MonitorData{Name: tt.monitor},
				Status:  tt.status,
			}
			got := FormatSubject(data)
			if got != tt.expected {
				t.Errorf("FormatSubject() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderEmail(t *testing.T) {
	data := sampleTemplateData()
	html, err := RenderEmail(data)
	if err != nil {
		t.Fatalf("RenderEmail() error: %v", err)
	}

	// Verify all required fields are present in the rendered HTML.
	requiredContent := []struct {
		label string
		value string
	}{
		{"monitor name", data.Monitor.Name},
		{"target URL", data.Monitor.Target},
		{"status", data.Status},
		{"previous status", data.PreviousStatus},
		{"response time", "1250ms"},
		{"incident ID", data.Incident.ID.String()},
		{"started at", data.Incident.StartedAt.Format(time.RFC3339)},
		{"duration", "5m 30s"},
		{"monitor link", "https://pulse.example.com/monitors/a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
	}

	for _, rc := range requiredContent {
		if !strings.Contains(html, rc.value) {
			t.Errorf("rendered email missing %s: expected to contain %q", rc.label, rc.value)
		}
	}

	// Verify it's valid HTML with basic structure.
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("rendered email should be a full HTML document")
	}
	if !strings.Contains(html, "Pulse") {
		t.Error("rendered email should contain Pulse branding")
	}
}

func TestRenderEmail_ContainsBrandElements(t *testing.T) {
	data := sampleTemplateData()
	html, err := RenderEmail(data)
	if err != nil {
		t.Fatalf("RenderEmail() error: %v", err)
	}

	// Brand color (sky-500).
	if !strings.Contains(html, "#0ea5e9") {
		t.Error("rendered email should contain Pulse brand color #0ea5e9")
	}

	// ECG motif (SVG path in footer).
	if !strings.Contains(html, "<svg") {
		t.Error("rendered email should contain ECG SVG motif")
	}

	// Brand name text.
	if !strings.Contains(html, ">Pulse<") {
		t.Error("rendered email should contain Pulse brand name")
	}
}

func TestRenderEmail_StatusColors(t *testing.T) {
	tests := []struct {
		status string
		color  string
	}{
		{"down", "#ef4444"},
		{"up", "#22c55e"},
		{"degraded", "#f59e0b"},
		{"ssl_expiring", "#f97316"},
		{"unknown", "#0ea5e9"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := statusColor(tt.status)
			if got != tt.color {
				t.Errorf("statusColor(%q) = %q, want %q", tt.status, got, tt.color)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d      time.Duration
		expect string
	}{
		{0, "N/A"},
		{30 * time.Second, "30s"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{1*time.Hour + 5*time.Minute + 10*time.Second, "1h 5m 10s"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.expect {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.expect)
			}
		})
	}
}

func TestRenderEmail_ViewMonitorButton(t *testing.T) {
	t.Run("button present when BaseURL set", func(t *testing.T) {
		data := sampleTemplateData()
		html, err := RenderEmail(data)
		if err != nil {
			t.Fatalf("RenderEmail() error: %v", err)
		}

		if !strings.Contains(html, "View Monitor") {
			t.Error("rendered email should contain 'View Monitor' button text")
		}
		expectedURL := "https://pulse.example.com/monitors/" + data.Monitor.ID.String()
		if !strings.Contains(html, expectedURL) {
			t.Errorf("rendered email should contain monitor link %q", expectedURL)
		}
	})

	t.Run("button absent when BaseURL empty", func(t *testing.T) {
		data := sampleTemplateData()
		data.BaseURL = ""
		html, err := RenderEmail(data)
		if err != nil {
			t.Fatalf("RenderEmail() error: %v", err)
		}

		if strings.Contains(html, "View Monitor") {
			t.Error("rendered email should NOT contain 'View Monitor' button when BaseURL is empty")
		}
	})
}
