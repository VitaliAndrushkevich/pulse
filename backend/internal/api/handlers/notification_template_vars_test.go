package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTemplateVarsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handler := &NotificationChannelHandler{
		queries:   nil,
		secretKey: make([]byte, 32),
	}

	r.GET("/api/v1/notifications/template-variables", handler.TemplateVariables)
	return r
}

// TestTemplateVariables_ReturnsAllVariables verifies that the endpoint returns
// all defined template variables grouped by dot-notation prefix.
// Validates: Requirements 9.1, 9.2, 9.3, 9.4
func TestTemplateVariables_ReturnsAllVariables(t *testing.T) {
	router := setupTemplateVarsRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/template-variables", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp templateVariablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify we have 3 groups: monitor, Incident, and top-level (empty string).
	if len(resp.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(resp.Groups))
	}

	// Map groups by name for easier assertion.
	groupMap := make(map[string]templateVariableGroup)
	for _, g := range resp.Groups {
		groupMap[g.Name] = g
	}

	// Check "monitor" group.
	monitorGroup, ok := groupMap["monitor"]
	if !ok {
		t.Fatal("missing 'monitor' group")
	}
	if len(monitorGroup.Variables) != 3 {
		t.Errorf("expected 3 monitor variables, got %d", len(monitorGroup.Variables))
	}

	// Check "Incident" group.
	incidentGroup, ok := groupMap["Incident"]
	if !ok {
		t.Fatal("missing 'Incident' group")
	}
	if len(incidentGroup.Variables) != 3 {
		t.Errorf("expected 3 Incident variables, got %d", len(incidentGroup.Variables))
	}

	// Check top-level group (empty string name).
	topLevel, ok := groupMap[""]
	if !ok {
		t.Fatal("missing top-level group (empty name)")
	}
	if len(topLevel.Variables) != 4 {
		t.Errorf("expected 4 top-level variables, got %d", len(topLevel.Variables))
	}
}

// TestTemplateVariables_VariableFields verifies each variable has all required fields populated.
// Validates: Requirements 9.1
func TestTemplateVariables_VariableFields(t *testing.T) {
	router := setupTemplateVarsRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/template-variables", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp templateVariablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	for _, group := range resp.Groups {
		for _, v := range group.Variables {
			if v.Name == "" {
				t.Errorf("variable in group %q has empty name", group.Name)
			}
			if v.Type == "" {
				t.Errorf("variable %q has empty type", v.Name)
			}
			if v.Description == "" {
				t.Errorf("variable %q has empty description", v.Name)
			}
			if v.Example == "" {
				t.Errorf("variable %q has empty example", v.Name)
			}
		}
	}
}

// TestTemplateVariables_SpecificVariables verifies the exact set of variables
// defined in the design document are present.
// Validates: Requirements 9.2
func TestTemplateVariables_SpecificVariables(t *testing.T) {
	router := setupTemplateVarsRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/template-variables", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp templateVariablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Collect all variable names from the response.
	varNames := make(map[string]bool)
	for _, group := range resp.Groups {
		for _, v := range group.Variables {
			varNames[v.Name] = true
		}
	}

	// These are all variables defined in the design document's Template_Variable set.
	expected := []string{
		"monitor.Name",
		"monitor.URL",
		"monitor.Target",
		"Status",
		"PreviousStatus",
		"ResponseTime",
		"Incident.StartedAt",
		"Incident.Duration",
		"Incident.ID",
		"Timestamp",
	}

	for _, name := range expected {
		if !varNames[name] {
			t.Errorf("missing expected variable: %s", name)
		}
	}

	// Verify total count matches expected (no extra variables).
	if len(varNames) != len(expected) {
		t.Errorf("expected %d total variables, got %d", len(expected), len(varNames))
	}
}
