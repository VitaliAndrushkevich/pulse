package notification

import (
	"testing"
)

func intPtr(v int) *int { return &v }

func TestValidateTriggers_EmptySlice(t *testing.T) {
	errs := ValidateTriggers(nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "triggers" {
		t.Errorf("expected field 'triggers', got %q", errs[0].Field)
	}
}

func TestValidateTriggers_InvalidType(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "invalid_type"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "triggers[0].type" {
		t.Errorf("expected field 'triggers[0].type', got %q", errs[0].Field)
	}
}

func TestValidateTriggers_ValidSimpleTypes(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "monitor_down"},
		{Type: "monitor_up"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %+v", len(errs), errs)
	}
}

func TestValidateTriggers_DegradedMissingThreshold(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "degraded"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "triggers[0].threshold_ms" {
		t.Errorf("expected field 'triggers[0].threshold_ms', got %q", errs[0].Field)
	}
}

func TestValidateTriggers_DegradedOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"below_min", 0},
		{"above_max", 60001},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "degraded", ThresholdMs: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 1 {
				t.Fatalf("expected 1 error, got %d: %+v", len(errs), errs)
			}
			if errs[0].Field != "triggers[0].threshold_ms" {
				t.Errorf("expected field 'triggers[0].threshold_ms', got %q", errs[0].Field)
			}
		})
	}
}

func TestValidateTriggers_DegradedValidBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"min", 1},
		{"max", 60000},
		{"mid", 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "degraded", ThresholdMs: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 0 {
				t.Fatalf("expected no errors, got %d: %+v", len(errs), errs)
			}
		})
	}
}

func TestValidateTriggers_SSLExpiringMissingThreshold(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "ssl_expiring"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "triggers[0].days_before" {
		t.Errorf("expected field 'triggers[0].days_before', got %q", errs[0].Field)
	}
}

func TestValidateTriggers_SSLExpiringOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"below_min", 0},
		{"above_max", 366},
		{"negative", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "ssl_expiring", DaysBefore: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 1 {
				t.Fatalf("expected 1 error, got %d: %+v", len(errs), errs)
			}
			if errs[0].Field != "triggers[0].days_before" {
				t.Errorf("expected field 'triggers[0].days_before', got %q", errs[0].Field)
			}
		})
	}
}

func TestValidateTriggers_SSLExpiringValidBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"min", 1},
		{"max", 365},
		{"mid", 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "ssl_expiring", DaysBefore: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 0 {
				t.Fatalf("expected no errors, got %d: %+v", len(errs), errs)
			}
		})
	}
}

func TestValidateTriggers_NFailuresMissingThreshold(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "n_failures_in_row"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "triggers[0].count" {
		t.Errorf("expected field 'triggers[0].count', got %q", errs[0].Field)
	}
}

func TestValidateTriggers_NFailuresOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"below_min", 0},
		{"above_max", 101},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "n_failures_in_row", Count: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 1 {
				t.Fatalf("expected 1 error, got %d: %+v", len(errs), errs)
			}
			if errs[0].Field != "triggers[0].count" {
				t.Errorf("expected field 'triggers[0].count', got %q", errs[0].Field)
			}
		})
	}
}

func TestValidateTriggers_NFailuresValidBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"min", 1},
		{"max", 100},
		{"mid", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triggers := []TriggerCondition{
				{Type: "n_failures_in_row", Count: intPtr(tt.value)},
			}
			errs := ValidateTriggers(triggers)
			if len(errs) != 0 {
				t.Fatalf("expected no errors, got %d: %+v", len(errs), errs)
			}
		})
	}
}

func TestValidateTriggers_MultipleTriggers(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "monitor_down"},
		{Type: "degraded", ThresholdMs: intPtr(5000)},
		{Type: "ssl_expiring", DaysBefore: intPtr(14)},
		{Type: "n_failures_in_row", Count: intPtr(5)},
		{Type: "monitor_up"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %+v", len(errs), errs)
	}
}

func TestValidateTriggers_MultipleErrors(t *testing.T) {
	triggers := []TriggerCondition{
		{Type: "invalid"},
		{Type: "degraded", ThresholdMs: intPtr(0)},
		{Type: "ssl_expiring"},
	}
	errs := ValidateTriggers(triggers)
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors, got %d: %+v", len(errs), errs)
	}
}
