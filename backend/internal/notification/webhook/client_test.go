package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/google/uuid"
)

// testKey generates a random 32-byte AES-256 key for testing.
func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	return key
}

// encryptValue encrypts a plaintext string and returns the base64-encoded ciphertext.
func encryptValue(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	ciphertext, err := crypto.Encrypt(key, []byte(plaintext))
	if err != nil {
		t.Fatalf("failed to encrypt value: %v", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// sampleTemplateData returns a TemplateData instance with all fields populated.
func sampleTemplateData() notification.TemplateData {
	return notification.TemplateData{
		Monitor: notification.MonitorData{
			ID:     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Name:   "API Health",
			URL:    "https://api.example.com/health",
			Target: "api.example.com",
		},
		Status:         "down",
		PreviousStatus: "up",
		ResponseTime:   1500,
		Incident: notification.IncidentData{
			ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			StartedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Duration:  5 * time.Minute,
		},
		Timestamp: time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC),
	}
}

func TestClient_Deliver_TemplateRendering(t *testing.T) {
	key := testKey(t)

	// Set up a test HTTP server to capture the request.
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: `{"monitor": "{{.Monitor.Name}}", "status": "{{.Status}}"}`,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("expected successful delivery, got error: %v", err)
	}

	expected := `{"monitor": "API Health", "status": "down"}`
	if receivedBody != expected {
		t.Errorf("rendered body mismatch\ngot:  %s\nwant: %s", receivedBody, expected)
	}
}

func TestClient_Deliver_TemplateRenderFailure_NonRetryable(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	config := WebhookConfig{
		URL:          "http://localhost:9999",
		Method:       "POST",
		BodyTemplate: `{{.InvalidField.Missing}}`,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for invalid template field, got nil")
	}

	if notification.IsRetryable(err) {
		t.Error("template render failure should be non-retryable")
	}
}

func TestClient_Deliver_TemplateParseFail_NonRetryable(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	config := WebhookConfig{
		URL:          "http://localhost:9999",
		Method:       "POST",
		BodyTemplate: `{{.Unclosed`,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for invalid template syntax, got nil")
	}

	if notification.IsRetryable(err) {
		t.Error("template parse failure should be non-retryable")
	}
}

func TestClient_Deliver_BodySizeLimit(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	// Create a template that produces output > 1MB.
	bigString := strings.Repeat("x", MaxBodySize+1)
	config := WebhookConfig{
		URL:          "http://localhost:9999",
		Method:       "POST",
		BodyTemplate: bigString,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for body exceeding max size, got nil")
	}

	if notification.IsRetryable(err) {
		t.Error("body size exceeded should be non-retryable")
	}

	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("error should mention size limit, got: %s", err.Error())
	}
}

func TestClient_Deliver_BodyExactlyAtLimit(t *testing.T) {
	key := testKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)

	// Create a template that produces exactly 1MB.
	exactBody := strings.Repeat("x", MaxBodySize)
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: exactBody,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("body at exactly max size should succeed, got: %v", err)
	}
}

func TestClient_Deliver_ContentTypeDefault(t *testing.T) {
	key := testKey(t)

	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: `{"test": true}`,
		Headers:      nil, // no explicit Content-Type
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type: application/json, got: %s", receivedContentType)
	}
}

func TestClient_Deliver_ContentTypeExplicit(t *testing.T) {
	key := testKey(t)

	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: `<xml>test</xml>`,
		Headers: []WebhookHeader{
			{Name: "Content-Type", Value: encryptValue(t, key, "application/xml")},
		},
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedContentType != "application/xml" {
		t.Errorf("expected Content-Type: application/xml, got: %s", receivedContentType)
	}
}

func TestClient_Deliver_CustomHeaders(t *testing.T) {
	key := testKey(t)

	var receivedAuthHeader string
	var receivedCustomHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		receivedCustomHeader = r.Header.Get("X-Custom-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: `{}`,
		Headers: []WebhookHeader{
			{Name: "Authorization", Value: encryptValue(t, key, "Bearer my-secret-token")},
			{Name: "X-Custom-Key", Value: encryptValue(t, key, "custom-value-123")},
		},
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuthHeader != "Bearer my-secret-token" {
		t.Errorf("expected Authorization: Bearer my-secret-token, got: %s", receivedAuthHeader)
	}
	if receivedCustomHeader != "custom-value-123" {
		t.Errorf("expected X-Custom-Key: custom-value-123, got: %s", receivedCustomHeader)
	}
}

func TestClient_Deliver_HeaderDecryptionFailure_NonRetryable(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	// Use a different key to encrypt — decryption with the client's key will fail.
	wrongKey := testKey(t)
	config := WebhookConfig{
		URL:          "http://localhost:9999",
		Method:       "POST",
		BodyTemplate: `{}`,
		Headers: []WebhookHeader{
			{Name: "Authorization", Value: encryptValue(t, wrongKey, "Bearer token")},
		},
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for header decryption failure, got nil")
	}

	if notification.IsRetryable(err) {
		t.Error("header decryption failure should be non-retryable")
	}
}

func TestClient_Deliver_InvalidBase64Header_NonRetryable(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	config := WebhookConfig{
		URL:          "http://localhost:9999",
		Method:       "POST",
		BodyTemplate: `{}`,
		Headers: []WebhookHeader{
			{Name: "Authorization", Value: "not-valid-base64!!!"},
		},
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for invalid base64 header, got nil")
	}

	if notification.IsRetryable(err) {
		t.Error("invalid base64 header should be non-retryable")
	}
}

func TestClient_Deliver_HTTPMethod(t *testing.T) {
	key := testKey(t)

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewClient(key)
			config := WebhookConfig{
				URL:          server.URL,
				Method:       method,
				BodyTemplate: `{}`,
				Headers:      nil,
			}

			err := client.Deliver(context.Background(), config, sampleTemplateData())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if receivedMethod != method {
				t.Errorf("expected method %s, got %s", method, receivedMethod)
			}
		})
	}
}

func TestClient_Deliver_Non2xx_Retryable(t *testing.T) {
	key := testKey(t)

	statusCodes := []int{400, 401, 403, 404, 500, 502, 503}

	for _, code := range statusCodes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			client := NewClient(key)
			config := WebhookConfig{
				URL:          server.URL,
				Method:       "POST",
				BodyTemplate: `{}`,
				Headers:      nil,
			}

			err := client.Deliver(context.Background(), config, sampleTemplateData())
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", code)
			}

			if !notification.IsRetryable(err) {
				t.Errorf("HTTP %d should be retryable, but was classified as non-retryable", code)
			}
		})
	}
}

func TestClient_Deliver_ConnectionError_Retryable(t *testing.T) {
	key := testKey(t)
	client := NewClient(key)

	// Use an address that will refuse connections.
	config := WebhookConfig{
		URL:          "http://127.0.0.1:1", // port 1 should refuse connection
		Method:       "POST",
		BodyTemplate: `{}`,
		Headers:      nil,
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err == nil {
		t.Fatal("expected error for connection failure, got nil")
	}

	if !notification.IsRetryable(err) {
		t.Error("connection error should be retryable")
	}
}

func TestClient_Deliver_2xxSuccess(t *testing.T) {
	key := testKey(t)

	successCodes := []int{200, 201, 202, 204}

	for _, code := range successCodes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			client := NewClient(key)
			config := WebhookConfig{
				URL:          server.URL,
				Method:       "POST",
				BodyTemplate: `{}`,
				Headers:      nil,
			}

			err := client.Deliver(context.Background(), config, sampleTemplateData())
			if err != nil {
				t.Errorf("expected success for status %d, got error: %v", code, err)
			}
		})
	}
}

func TestClient_Deliver_AllTemplateVariables(t *testing.T) {
	key := testKey(t)

	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)
	config := WebhookConfig{
		URL:    server.URL,
		Method: "POST",
		BodyTemplate: `{"monitor":"{{.Monitor.Name}}","url":"{{.Monitor.URL}}","target":"{{.Monitor.Target}}",` +
			`"status":"{{.Status}}","prev":"{{.PreviousStatus}}","rt":{{.ResponseTime}},` +
			`"incident":"{{.Incident.ID}}","ts":"{{.Timestamp}}"}`,
		Headers: nil,
	}

	data := sampleTemplateData()
	err := client.Deliver(context.Background(), config, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify key fields are present in the rendered body.
	if !strings.Contains(receivedBody, "API Health") {
		t.Error("rendered body should contain monitor name")
	}
	if !strings.Contains(receivedBody, "https://api.example.com/health") {
		t.Error("rendered body should contain monitor URL")
	}
	if !strings.Contains(receivedBody, "api.example.com") {
		t.Error("rendered body should contain monitor target")
	}
	if !strings.Contains(receivedBody, "down") {
		t.Error("rendered body should contain status")
	}
	if !strings.Contains(receivedBody, "up") {
		t.Error("rendered body should contain previous status")
	}
	if !strings.Contains(receivedBody, "1500") {
		t.Error("rendered body should contain response time")
	}
	if !strings.Contains(receivedBody, "11111111-1111-1111-1111-111111111111") {
		t.Error("rendered body should contain incident ID")
	}
}

func TestClient_Deliver_ContentTypeCaseInsensitive(t *testing.T) {
	key := testKey(t)

	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(key)

	// Use lowercase "content-type" — should still prevent the default from being set.
	config := WebhookConfig{
		URL:          server.URL,
		Method:       "POST",
		BodyTemplate: `plain text`,
		Headers: []WebhookHeader{
			{Name: "content-type", Value: encryptValue(t, key, "text/plain")},
		},
	}

	err := client.Deliver(context.Background(), config, sampleTemplateData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedContentType != "text/plain" {
		t.Errorf("expected Content-Type: text/plain, got: %s", receivedContentType)
	}
}
