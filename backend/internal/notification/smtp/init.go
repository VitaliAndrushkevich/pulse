package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// startupValidationTimeout is the maximum time allowed for the SMTP
// connectivity check during application startup.
const startupValidationTimeout = 10 * time.Second

// ValidateOnStartup reads SMTP settings from the database and validates
// connectivity. It returns a configured *Client if SMTP is available, or nil
// if SMTP is not configured or validation fails. The application MUST NOT
// terminate if this function returns nil — it only means email notifications
// are unavailable.
func ValidateOnStartup(ctx context.Context, queries *db.Queries, secretKey []byte) *Client {
	settings, err := queries.GetSMTPSettings(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Printf("notification: SMTP not configured, email notifications disabled")
			return nil
		}
		log.Printf("notification: failed to read SMTP settings from database: %v", err)
		return nil
	}

	// Build the client (decrypts password).
	client, err := NewClient(settings, secretKey)
	if err != nil {
		log.Printf("notification: SMTP configuration invalid: %v", err)
		return nil
	}

	// Validate connectivity with a 10s timeout.
	validCtx, cancel := context.WithTimeout(ctx, startupValidationTimeout)
	defer cancel()

	if err := validateSMTPConnectivity(validCtx, client.config); err != nil {
		log.Printf("notification: WARNING SMTP connectivity check failed: %v (email notifications may not work)", err)
		// Return the client anyway — the config is valid, just connectivity
		// failed. The dispatcher can retry delivery later when the SMTP
		// server becomes available.
		return client
	}

	log.Printf("notification: SMTP connectivity validated successfully (host=%s port=%d)", client.config.Host, client.config.Port)
	return client
}

// validateSMTPConnectivity performs a TCP connection and EHLO handshake
// against the configured SMTP server with the context deadline. It optionally
// attempts AUTH if credentials are configured.
func validateSMTPConnectivity(ctx context.Context, cfg *SMTPConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(startupValidationTimeout)
	}

	dialer := net.Dialer{Deadline: deadline}

	var conn net.Conn
	var err error

	if cfg.TLSEnabled {
		tlsConfig := &tls.Config{
			ServerName: cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connection to %s failed: %w", addr, err)
	}
	defer conn.Close()

	// Set deadline for the SMTP conversation.
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	// Create SMTP client on the connection.
	smtpClient, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("SMTP handshake failed: %w", err)
	}
	defer smtpClient.Close()

	// Send EHLO.
	if err := smtpClient.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}

	// Attempt AUTH if credentials are configured.
	if cfg.Username != "" && cfg.Password != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := smtpClient.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Graceful quit.
	_ = smtpClient.Quit()

	return nil
}

// DecryptSettings is a helper that converts database SMTP settings into
// a ready-to-use SMTPConfig by decrypting the password. This is useful for
// callers that need the config without a full client.
func DecryptSettings(settings db.SmtpSetting, secretKey []byte) (*SMTPConfig, error) {
	cfg := &SMTPConfig{
		Host:        settings.Host,
		Port:        int(settings.Port),
		FromAddress: settings.FromAddress,
		TLSEnabled:  settings.TlsEnabled,
	}

	if settings.Username != nil {
		cfg.Username = *settings.Username
	}

	if len(settings.PasswordEnc) > 0 {
		plaintext, err := crypto.Decrypt(secretKey, settings.PasswordEnc)
		if err != nil {
			return nil, fmt.Errorf("smtp: password decryption failed: %w", err)
		}
		cfg.Password = string(plaintext)
	}

	return cfg, nil
}
