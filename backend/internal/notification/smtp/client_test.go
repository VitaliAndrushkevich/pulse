package smtp

import (
	"testing"

	"github.com/google/uuid"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

func TestNewClient_ValidConfig(t *testing.T) {
	// Generate a test secret key (32 bytes).
	secretKey := make([]byte, 32)
	for i := range secretKey {
		secretKey[i] = byte(i)
	}

	// Encrypt a test password.
	password := "smtp-password-123"
	encPassword, err := crypto.Encrypt(secretKey, []byte(password))
	if err != nil {
		t.Fatalf("failed to encrypt password: %v", err)
	}

	username := "user@example.com"
	settings := db.SmtpSetting{
		ID:          uuid.New(),
		Host:        "smtp.example.com",
		Port:        587,
		Username:    &username,
		PasswordEnc: encPassword,
		FromAddress: "noreply@example.com",
		TlsEnabled:  true,
	}

	client, err := NewClient(settings, secretKey)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	if client.config.Host != "smtp.example.com" {
		t.Errorf("expected host smtp.example.com, got %s", client.config.Host)
	}
	if client.config.Port != 587 {
		t.Errorf("expected port 587, got %d", client.config.Port)
	}
	if client.config.Username != "user@example.com" {
		t.Errorf("expected username user@example.com, got %s", client.config.Username)
	}
	if client.config.Password != password {
		t.Errorf("expected decrypted password %q, got %q", password, client.config.Password)
	}
	if client.config.FromAddress != "noreply@example.com" {
		t.Errorf("expected from noreply@example.com, got %s", client.config.FromAddress)
	}
	if !client.config.TLSEnabled {
		t.Error("expected TLS enabled")
	}
}

func TestNewClient_NoPassword(t *testing.T) {
	secretKey := make([]byte, 32)
	for i := range secretKey {
		secretKey[i] = byte(i)
	}

	settings := db.SmtpSetting{
		ID:          uuid.New(),
		Host:        "smtp.example.com",
		Port:        25,
		Username:    nil,
		PasswordEnc: nil,
		FromAddress: "noreply@example.com",
		TlsEnabled:  false,
	}

	client, err := NewClient(settings, secretKey)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	if client.config.Username != "" {
		t.Errorf("expected empty username, got %q", client.config.Username)
	}
	if client.config.Password != "" {
		t.Errorf("expected empty password, got %q", client.config.Password)
	}
}

func TestNewClient_IncompleteConfig(t *testing.T) {
	secretKey := make([]byte, 32)

	// Missing host.
	settings := db.SmtpSetting{
		ID:          uuid.New(),
		Host:        "",
		Port:        587,
		FromAddress: "noreply@example.com",
		TlsEnabled:  true,
	}

	_, err := NewClient(settings, secretKey)
	if err == nil {
		t.Error("expected error for incomplete config, got nil")
	}
}

func TestNewClient_BadDecryption(t *testing.T) {
	secretKey := make([]byte, 32)
	for i := range secretKey {
		secretKey[i] = byte(i)
	}

	// Use garbage data as encrypted password — should fail decryption.
	settings := db.SmtpSetting{
		ID:          uuid.New(),
		Host:        "smtp.example.com",
		Port:        587,
		PasswordEnc: []byte("not-valid-encrypted-data-xxxxxxxx"),
		FromAddress: "noreply@example.com",
		TlsEnabled:  true,
	}

	_, err := NewClient(settings, secretKey)
	if err == nil {
		t.Error("expected decryption error, got nil")
	}
}

func TestSMTPConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name   string
		config SMTPConfig
		want   bool
	}{
		{
			name:   "fully configured",
			config: SMTPConfig{Host: "smtp.example.com", Port: 587, FromAddress: "a@b.com"},
			want:   true,
		},
		{
			name:   "missing host",
			config: SMTPConfig{Host: "", Port: 587, FromAddress: "a@b.com"},
			want:   false,
		},
		{
			name:   "zero port",
			config: SMTPConfig{Host: "smtp.example.com", Port: 0, FromAddress: "a@b.com"},
			want:   false,
		},
		{
			name:   "missing from address",
			config: SMTPConfig{Host: "smtp.example.com", Port: 587, FromAddress: ""},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsConfigured()
			if got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildMessage(t *testing.T) {
	msg := buildMessage(
		"noreply@example.com",
		[]string{"admin@example.com", "team@example.com"},
		"[Pulse] API - Down",
		"<html><body>Hello</body></html>",
	)

	expected := []string{
		"From: noreply@example.com\r\n",
		"To: admin@example.com, team@example.com\r\n",
		"Subject: [Pulse] API - Down\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/html; charset=\"UTF-8\"\r\n",
		"<html><body>Hello</body></html>",
	}

	for _, exp := range expected {
		if !contains(msg, exp) {
			t.Errorf("message missing: %q", exp)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
