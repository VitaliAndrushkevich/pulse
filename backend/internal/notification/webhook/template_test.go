package webhook

import (
	"strings"
	"testing"
	"text/template"
)

func TestValidateWebhookTemplate_ValidTemplates(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
	}{
		{
			name: "simple field",
			tmpl: `{"text": "{{.Monitor.Name}} is {{.Status}}"}`,
		},
		{
			name: "all known variables",
			tmpl: `{{.Monitor.Name}} {{.Monitor.URL}} {{.Monitor.Target}} {{.Status}} {{.PreviousStatus}} {{.ResponseTime}} {{.Incident.StartedAt}} {{.Incident.Duration}} {{.Incident.ID}} {{.Timestamp}}`,
		},
		{
			name: "struct-level access",
			tmpl: `{{.Monitor}}`,
		},
		{
			name: "incident struct-level",
			tmpl: `{{.Incident}}`,
		},
		{
			name: "with conditional",
			tmpl: `{{if .Status}}down{{end}}`,
		},
		{
			name: "static text only",
			tmpl: `Hello, this is a plain text template`,
		},
		{
			name: "empty template",
			tmpl: ``,
		},
		{
			name: "with range on nothing referencing known vars",
			tmpl: `{{.Monitor.Name}}: {{.Status}} (was {{.PreviousStatus}})`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookTemplate(tt.tmpl)
			if err != nil {
				t.Errorf("expected valid template, got error: %v", err)
			}
		})
	}
}

func TestValidateWebhookTemplate_InvalidTemplates(t *testing.T) {
	tests := []struct {
		name      string
		tmpl      string
		wantError string
	}{
		{
			name:      "unknown variable",
			tmpl:      `{{.Unknown}}`,
			wantError: "unknown template variable: Unknown",
		},
		{
			name:      "unknown nested variable",
			tmpl:      `{{.Monitor.Invalid}}`,
			wantError: "unknown template variable: Monitor.Invalid",
		},
		{
			name:      "unknown top-level struct",
			tmpl:      `{{.Foo.Bar}}`,
			wantError: "unknown template variable: Foo.Bar",
		},
		{
			name:      "parse error - unclosed action",
			tmpl:      `{{.Monitor.Name`,
			wantError: "template parse error",
		},
		{
			name:      "parse error - invalid syntax",
			tmpl:      `{{invalid syntax here}}`,
			wantError: "template parse error",
		},
		{
			name:      "mixed valid and invalid",
			tmpl:      `{{.Monitor.Name}} {{.BadVar}}`,
			wantError: "unknown template variable: BadVar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookTemplate(tt.tmpl)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("expected error containing %q, got: %v", tt.wantError, err)
			}
		})
	}
}

func TestIsKnownTemplateVar(t *testing.T) {
	// All exact known vars should be recognized.
	knownVars := []string{
		"Monitor.ID", "Monitor.Name", "Monitor.URL", "Monitor.Target",
		"Status", "PreviousStatus", "ResponseTime",
		"Incident.StartedAt", "Incident.Duration", "Incident.ID",
		"Timestamp", "BaseURL",
	}
	for _, v := range knownVars {
		if !isKnownTemplateVar(v) {
			t.Errorf("expected %q to be a known variable", v)
		}
	}

	// Struct-level prefixes should be recognized.
	prefixes := []string{"Monitor", "Incident"}
	for _, v := range prefixes {
		if !isKnownTemplateVar(v) {
			t.Errorf("expected struct prefix %q to be recognized", v)
		}
	}

	// Unknown vars should be rejected.
	unknownVars := []string{
		"Unknown", "Foo.Bar", "Monitor.Invalid",
		"Incident.Unknown", "monitor.name", // case-sensitive
	}
	for _, v := range unknownVars {
		if isKnownTemplateVar(v) {
			t.Errorf("expected %q to be unknown", v)
		}
	}
}

func TestExtractTemplateVars(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		expected []string
	}{
		{
			name:     "single var",
			tmpl:     `{{.Status}}`,
			expected: []string{"Status"},
		},
		{
			name:     "nested var",
			tmpl:     `{{.Monitor.Name}}`,
			expected: []string{"Monitor.Name"},
		},
		{
			name:     "multiple vars",
			tmpl:     `{{.Monitor.Name}} - {{.Status}}`,
			expected: []string{"Monitor.Name", "Status"},
		},
		{
			name:     "no vars",
			tmpl:     `plain text`,
			expected: []string{},
		},
		{
			name:     "var in if condition",
			tmpl:     `{{if .Status}}yes{{end}}`,
			expected: []string{"Status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := parseTemplate(tt.tmpl)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			vars := extractTemplateVars(tmpl)
			if len(vars) != len(tt.expected) {
				t.Fatalf("expected %d vars, got %d: %v", len(tt.expected), len(vars), vars)
			}
			for i, v := range vars {
				if v != tt.expected[i] {
					t.Errorf("var[%d]: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}

func parseTemplate(s string) (*template.Template, error) {
	return template.New("test").Parse(s)
}
