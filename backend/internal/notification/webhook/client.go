package webhook

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
)

// MaxBodySize is the maximum allowed rendered webhook body size (1 MB).
const MaxBodySize = 1048576

// requestTimeout is the HTTP request timeout for webhook delivery.
const requestTimeout = 10 * time.Second

// WebhookConfig holds the configuration for a webhook notification channel.
type WebhookConfig struct {
	URL          string          `json:"url"`
	Method       string          `json:"method"`
	BodyTemplate string          `json:"body_template"`
	Headers      []WebhookHeader `json:"headers"`
}

// WebhookHeader represents a single custom header for webhook requests.
// The Value field is encrypted at rest (AES-256-GCM, base64-encoded).
type WebhookHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"` // encrypted at rest
}

// Client delivers webhook notifications via HTTP.
type Client struct {
	httpClient *http.Client
	secretKey  []byte
}

// NewClient creates a new webhook Client with the given AES-256-GCM secret key
// used for decrypting custom header values. The HTTP client uses a 10s timeout.
func NewClient(secretKey []byte) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		secretKey: secretKey,
	}
}

// Deliver renders the webhook body template with the provided data and sends
// the HTTP request according to the webhook configuration.
//
// Error classification:
//   - Template render failure → non-retryable
//   - Body exceeds 1MB → non-retryable
//   - Header decryption failure → non-retryable
//   - HTTP 2xx response → success (nil)
//   - HTTP non-2xx response → retryable
//   - Connection/network error → retryable
func (c *Client) Deliver(ctx context.Context, config WebhookConfig, data notification.TemplateData) error {
	// 1. Parse and render the body template.
	tmpl, err := template.New("webhook").Parse(config.BodyTemplate)
	if err != nil {
		return notification.NewNonRetryableError(
			fmt.Errorf("webhook: template parse error: %w", err),
		)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return notification.NewNonRetryableError(
			fmt.Errorf("webhook: template render error: %w", err),
		)
	}

	// 2. Enforce max body size (1 MB).
	if buf.Len() > MaxBodySize {
		return notification.NewNonRetryableError(
			fmt.Errorf("webhook: rendered body size %d exceeds maximum %d bytes", buf.Len(), MaxBodySize),
		)
	}

	// 3. Create HTTP request with configured method and URL.
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, &buf)
	if err != nil {
		return notification.NewNonRetryableError(
			fmt.Errorf("webhook: failed to create request: %w", err),
		)
	}

	// 4. Decrypt and add custom headers.
	hasContentType := false
	for _, h := range config.Headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			hasContentType = true
		}

		decrypted, err := c.decryptHeaderValue(h.Value)
		if err != nil {
			return notification.NewNonRetryableError(
				fmt.Errorf("webhook: failed to decrypt header %q: %w", h.Name, err),
			)
		}

		req.Header.Set(h.Name, decrypted)
	}

	// 5. Set Content-Type: application/json if not explicitly configured.
	if !hasContentType {
		req.Header.Set("Content-Type", "application/json")
	}

	// 6. Send the request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return notification.NewRetryableError(
			fmt.Errorf("webhook: request failed: %w", err),
		)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	// 7. Check response status.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return notification.NewRetryableError(
		fmt.Errorf("webhook: non-2xx response: %d %s", resp.StatusCode, resp.Status),
	)
}

// decryptHeaderValue decodes a base64-encoded ciphertext and decrypts it
// using the client's AES-256-GCM secret key.
func (c *Client) decryptHeaderValue(encrypted string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	plaintext, err := crypto.Decrypt(c.secretKey, ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
