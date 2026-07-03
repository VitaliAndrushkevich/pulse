// Package smtp implements SMTP email delivery for the Pulse notification
// subsystem. It handles SMTP connections with TLS support, credential
// decryption, and HTML email rendering using Pulse-branded templates.
package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// sendTimeout is the maximum duration allowed for sending a single email.
const sendTimeout = 30 * time.Second

// SMTPConfig holds the decrypted SMTP configuration ready for use.
type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string // decrypted in-memory only
	FromAddress string
	TLSEnabled  bool
}

// IsConfigured returns true if the SMTP configuration has the minimum
// required fields to attempt delivery.
func (c *SMTPConfig) IsConfigured() bool {
	return c.Host != "" && c.Port > 0 && c.FromAddress != ""
}

// Client sends email notifications via SMTP using instance-level settings
// stored in the database.
type Client struct {
	config    *SMTPConfig
	secretKey []byte
}

// NewClient creates a new SMTP client from database settings. The password
// is decrypted using the provided AES-256-GCM secret key. Returns nil if
// SMTP is not configured in the database.
func NewClient(settings db.SmtpSetting, secretKey []byte) (*Client, error) {
	cfg := &SMTPConfig{
		Host:        settings.Host,
		Port:        int(settings.Port),
		FromAddress: settings.FromAddress,
		TLSEnabled:  settings.TlsEnabled,
	}

	if settings.Username != nil {
		cfg.Username = *settings.Username
	}

	// Decrypt the SMTP password if present.
	if len(settings.PasswordEnc) > 0 {
		plaintext, err := crypto.Decrypt(secretKey, settings.PasswordEnc)
		if err != nil {
			return nil, fmt.Errorf("smtp: password decryption failed: %w", err)
		}
		cfg.Password = string(plaintext)
	}

	if !cfg.IsConfigured() {
		return nil, fmt.Errorf("smtp: configuration incomplete (host=%q port=%d from=%q)", cfg.Host, cfg.Port, cfg.FromAddress)
	}

	return &Client{
		config:    cfg,
		secretKey: secretKey,
	}, nil
}

// Send delivers an HTML email to the specified recipients with the given subject
// and body. It uses a context-derived timeout of 30 seconds for the entire
// send operation.
func (c *Client) Send(ctx context.Context, recipients []string, subject string, htmlBody string) error {
	if c.config == nil || !c.config.IsConfigured() {
		return fmt.Errorf("smtp: not configured")
	}

	if len(recipients) == 0 {
		return fmt.Errorf("smtp: no recipients specified")
	}

	// Apply send timeout.
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	// Establish TCP connection with context deadline.
	deadline, _ := ctx.Deadline()
	dialer := net.Dialer{Deadline: deadline}

	var conn net.Conn
	var err error

	if c.config.TLSEnabled {
		// TLS connection (implicit TLS on port 465 or explicit STARTTLS).
		tlsConfig := &tls.Config{
			ServerName: c.config.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// Set deadline on the connection for the entire SMTP conversation.
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("smtp: set deadline: %w", err)
	}

	// Create SMTP client on existing connection.
	client, err := smtp.NewClient(conn, c.config.Host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Close()

	// Issue EHLO.
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp: hello: %w", err)
	}

	// Authenticate if credentials are provided.
	if c.config.Username != "" && c.config.Password != "" {
		auth := smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	// Set the sender.
	if err := client.Mail(c.config.FromAddress); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}

	// Add recipients.
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: rcpt to %s: %w", rcpt, err)
		}
	}

	// Write the message body.
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}

	msg := buildMessage(c.config.FromAddress, recipients, subject, htmlBody)
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp: write body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}

	// Quit gracefully.
	if err := client.Quit(); err != nil {
		// Log but don't fail — message was already accepted.
		log.Printf("smtp: quit warning: %v", err)
	}

	return nil
}

// SendNotification renders the Pulse-branded HTML email template with the given
// data and delivers it to the specified recipients.
func (c *Client) SendNotification(ctx context.Context, recipients []string, data notification.TemplateData) error {
	subject := FormatSubject(data)
	htmlBody, err := RenderEmail(data)
	if err != nil {
		return fmt.Errorf("smtp: render template: %w", err)
	}
	return c.Send(ctx, recipients, subject, htmlBody)
}

// buildMessage constructs a complete RFC 2822 email message with MIME headers.
func buildMessage(from string, to []string, subject, htmlBody string) string {
	var sb strings.Builder

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody)

	return sb.String()
}
