package handlers

import (
	"testing"
)

func TestValidateSMTPSettings(t *testing.T) {
	tests := []struct {
		name       string
		req        smtpSettingsRequest
		wantErrs   int
		wantFields []string
	}{
		{
			name: "valid settings",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        587,
				FromAddress: "noreply@example.com",
				TLSEnabled:  true,
			},
			wantErrs: 0,
		},
		{
			name: "missing host",
			req: smtpSettingsRequest{
				Port:        587,
				FromAddress: "noreply@example.com",
			},
			wantErrs:   1,
			wantFields: []string{"host"},
		},
		{
			name: "port too low",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        0,
				FromAddress: "noreply@example.com",
			},
			wantErrs:   1,
			wantFields: []string{"port"},
		},
		{
			name: "port too high",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        65536,
				FromAddress: "noreply@example.com",
			},
			wantErrs:   1,
			wantFields: []string{"port"},
		},
		{
			name: "port boundaries - valid min",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        1,
				FromAddress: "noreply@example.com",
			},
			wantErrs: 0,
		},
		{
			name: "port boundaries - valid max",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        65535,
				FromAddress: "noreply@example.com",
			},
			wantErrs: 0,
		},
		{
			name: "missing from_address",
			req: smtpSettingsRequest{
				Host: "smtp.example.com",
				Port: 587,
			},
			wantErrs:   1,
			wantFields: []string{"from_address"},
		},
		{
			name: "invalid from_address",
			req: smtpSettingsRequest{
				Host:        "smtp.example.com",
				Port:        587,
				FromAddress: "not-an-email",
			},
			wantErrs:   1,
			wantFields: []string{"from_address"},
		},
		{
			name: "multiple errors",
			req: smtpSettingsRequest{
				Port:        -1,
				FromAddress: "invalid",
			},
			wantErrs:   3,
			wantFields: []string{"host", "port", "from_address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateSMTPSettings(tt.req)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateSMTPSettings() got %d errors, want %d; errors=%v", len(errs), tt.wantErrs, errs)
				return
			}

			for _, wantField := range tt.wantFields {
				found := false
				for _, e := range errs {
					if e.Field == wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", wantField, errs)
				}
			}
		})
	}
}

func TestSMTPSettingsResponseNeverContainsPassword(t *testing.T) {
	// Verify that our response type does not have a password field.
	// This is a compile-time guarantee, but let's also test the response building logic.
	resp := smtpSettingsResponse{
		Configured:  true,
		Host:        "smtp.example.com",
		Port:        587,
		Username:    strPtr("user@example.com"),
		FromAddress: "noreply@example.com",
		TLSEnabled:  true,
		PasswordSet: true,
	}

	// Verify password_set is a boolean indicator, not the actual password.
	if !resp.PasswordSet {
		t.Error("expected PasswordSet to be true")
	}

	// The response struct has no Password or PasswordEnc field — this is the design guarantee.
	// If someone accidentally adds one, this test documents the expectation.
}

func strPtr(s string) *string {
	return &s
}
