package handlers

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	smtpclient "github.com/VitaliAndrushkevich/pulse/internal/notification/smtp"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// smtpSettingsSingletonID is a deterministic UUID used for the singleton SMTP settings row.
var smtpSettingsSingletonID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// smtpTestTimeout is the connection timeout for SMTP connectivity tests.
const smtpTestTimeout = 10 * time.Second

// --- Request/Response types ---

type smtpSettingsRequest struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	FromAddress string  `json:"from_address"`
	TLSEnabled  bool    `json:"tls_enabled"`
}

type smtpSettingsResponse struct {
	Configured  bool    `json:"configured"`
	Host        string  `json:"host,omitempty"`
	Port        int     `json:"port,omitempty"`
	Username    *string `json:"username,omitempty"`
	FromAddress string  `json:"from_address,omitempty"`
	TLSEnabled  bool    `json:"tls_enabled,omitempty"`
	PasswordSet bool    `json:"password_set"`
}

type smtpTestResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// --- Handlers ---

// GetSMTPSettings handles GET /notifications/smtp-settings.
// Returns the current SMTP configuration without the raw password.
func (h *NotificationChannelHandler) GetSMTPSettings(c *gin.Context) {
	ctx := c.Request.Context()

	settings, err := h.queries.GetSMTPSettings(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusOK, smtpSettingsResponse{Configured: false})
			return
		}
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get SMTP settings")
		return
	}

	resp := smtpSettingsResponse{
		Configured:  true,
		Host:        settings.Host,
		Port:        int(settings.Port),
		Username:    settings.Username,
		FromAddress: settings.FromAddress,
		TLSEnabled:  settings.TlsEnabled,
		PasswordSet: len(settings.PasswordEnc) > 0,
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateSMTPSettings handles PUT /notifications/smtp-settings.
// Creates or updates the singleton SMTP configuration.
func (h *NotificationChannelHandler) UpdateSMTPSettings(c *gin.Context) {
	var req smtpSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	// Validate fields.
	errs := validateSMTPSettings(req)
	if len(errs) > 0 {
		apiValidationError(c, "SMTP settings are invalid", errs)
		return
	}

	// Encrypt password if provided.
	var passwordEnc []byte
	if req.Password != nil && *req.Password != "" {
		encrypted, err := crypto.Encrypt(h.secretKey, []byte(*req.Password))
		if err != nil {
			apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encrypt password")
			return
		}
		passwordEnc = encrypted
	} else {
		// If password is not provided, preserve the existing password.
		ctx := c.Request.Context()
		existing, err := h.queries.GetSMTPSettings(ctx)
		if err == nil {
			passwordEnc = existing.PasswordEnc
		}
		// If no existing settings, passwordEnc remains nil (no password set).
	}

	ctx := c.Request.Context()
	var username *string
	if req.Username != nil {
		username = req.Username
	}

	settings, err := h.queries.UpsertSMTPSettings(ctx, db.UpsertSMTPSettingsParams{
		ID:          smtpSettingsSingletonID,
		Host:        req.Host,
		Port:        int32(req.Port),
		Username:    username,
		PasswordEnc: passwordEnc,
		FromAddress: req.FromAddress,
		TlsEnabled:  req.TLSEnabled,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to save SMTP settings")
		return
	}

	// Recreate the SMTP client with the new settings so that test/send
	// operations use the updated configuration without requiring a server restart.
	newClient, clientErr := smtpclient.NewClient(settings, h.secretKey)
	if clientErr != nil {
		// Settings were saved but client creation failed (e.g., incomplete config).
		// Log the issue but don't fail the request — settings are persisted.
		fmt.Printf("smtp: client rebuild after settings update failed: %v\n", clientErr)
	} else {
		h.SetSMTPClient(newClient)
	}

	resp := smtpSettingsResponse{
		Configured:  true,
		Host:        settings.Host,
		Port:        int(settings.Port),
		Username:    settings.Username,
		FromAddress: settings.FromAddress,
		TLSEnabled:  settings.TlsEnabled,
		PasswordSet: len(settings.PasswordEnc) > 0,
	}

	c.JSON(http.StatusOK, resp)
}

// DeleteSMTPSettings handles DELETE /notifications/smtp-settings.
// Removes the SMTP settings row, disabling email notifications.
func (h *NotificationChannelHandler) DeleteSMTPSettings(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.queries.DeleteSMTPSettings(ctx); err != nil {
		apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to delete SMTP settings")
		return
	}

	// Clear the SMTP client so test/send operations reflect the deletion.
	h.SetSMTPClient(nil)

	c.Status(http.StatusNoContent)
}

// TestSMTPConnection handles POST /notifications/smtp-settings/test.
// If a request body with SMTP fields is provided, it tests those values directly
// (useful for testing before saving). If no body is provided, it reads from DB.
func (h *NotificationChannelHandler) TestSMTPConnection(c *gin.Context) {
	ctx := c.Request.Context()

	var host string
	var port int
	var username, password string
	var tlsEnabled bool

	// Try to read inline test body.
	var req smtpSettingsRequest
	hasBody := c.ShouldBindJSON(&req) == nil && req.Host != ""

	if hasBody {
		// Test the provided values directly (no DB read required).
		host = req.Host
		port = req.Port
		tlsEnabled = req.TLSEnabled
		if req.Username != nil {
			username = *req.Username
		}
		if req.Password != nil && *req.Password != "" {
			password = *req.Password
		} else {
			// No password in body — try to use existing one from DB.
			existing, err := h.queries.GetSMTPSettings(ctx)
			if err == nil && len(existing.PasswordEnc) > 0 {
				plaintext, decErr := crypto.Decrypt(h.secretKey, existing.PasswordEnc)
				if decErr == nil {
					password = string(plaintext)
				}
			}
		}
	} else {
		// No body provided — read settings from database.
		settings, err := h.queries.GetSMTPSettings(ctx)
		if err != nil {
			if err == pgx.ErrNoRows {
				apiError(c, http.StatusBadRequest, "SMTP_NOT_CONFIGURED", "SMTP settings are not configured")
				return
			}
			apiError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get SMTP settings")
			return
		}

		host = settings.Host
		port = int(settings.Port)
		tlsEnabled = settings.TlsEnabled
		if settings.Username != nil {
			username = *settings.Username
		}
		if len(settings.PasswordEnc) > 0 {
			plaintext, err := crypto.Decrypt(h.secretKey, settings.PasswordEnc)
			if err != nil {
				c.JSON(http.StatusOK, smtpTestResponse{
					Success: false,
					Error:   "failed to decrypt SMTP password",
				})
				return
			}
			password = string(plaintext)
		}
	}

	// Validate minimum fields for test.
	if host == "" || port < 1 {
		c.JSON(http.StatusOK, smtpTestResponse{
			Success: false,
			Error:   "host and port are required to test connectivity",
		})
		return
	}

	// Test connectivity.
	testErr := testSMTPConnectivity(host, port, username, password, tlsEnabled)
	if testErr != nil {
		c.JSON(http.StatusOK, smtpTestResponse{
			Success: false,
			Error:   testErr.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, smtpTestResponse{Success: true})
}

// --- Validation ---

// validateSMTPSettings validates the SMTP settings request fields.
func validateSMTPSettings(req smtpSettingsRequest) []fieldError {
	var errs []fieldError

	if req.Host == "" {
		errs = append(errs, fieldError{Field: "host", Message: "is required"})
	}

	if req.Port < 1 || req.Port > 65535 {
		errs = append(errs, fieldError{Field: "port", Message: "must be between 1 and 65535"})
	}

	if req.FromAddress == "" {
		errs = append(errs, fieldError{Field: "from_address", Message: "is required"})
	} else if !emailRegex.MatchString(req.FromAddress) {
		errs = append(errs, fieldError{Field: "from_address", Message: "must be a valid email address (RFC 5322)"})
	}

	return errs
}

// --- SMTP connectivity test ---

// testSMTPConnectivity attempts a TCP connection to the SMTP server,
// performs an EHLO handshake, and optionally attempts AUTH.
func testSMTPConnectivity(host string, port int, username, password string, tlsEnabled bool) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	var conn net.Conn
	var err error

	if tlsEnabled {
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		dialer := &net.Dialer{Timeout: smtpTestTimeout}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, smtpTestTimeout)
	}
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set a deadline for the entire SMTP conversation.
	if err := conn.SetDeadline(time.Now().Add(smtpTestTimeout)); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	// Create SMTP client on the existing connection.
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP handshake failed: %w", err)
	}
	defer client.Close()

	// Send EHLO.
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}

	// Attempt AUTH if credentials are provided.
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Gracefully close.
	if err := client.Quit(); err != nil {
		// Non-fatal: connectivity was validated.
		return nil
	}

	return nil
}
