package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"pgregory.net/rapid"
)

// TestWebSocketCheckerInjectsAllCredentials verifies Property 6:
// WebSocket Checker injects all credentials correctly.
// For any set of monitor credentials, CheckWithAuth SHALL include
// correctly-formatted headers in the WebSocket upgrade request.
//
// The implementation uses Header.Set which means the last credential of each
// type that writes to the same header key wins. This test generates one
// credential per type to verify correct injection of all auth types.
//
// **Validates: Requirements 6.1, 6.2, 6.3**
func TestWebSocketCheckerInjectsAllCredentials(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate one credential of each type to avoid header overwrite ambiguity
		// (Header.Set replaces existing values for the same key).
		var creds []AuthCredential

		// Randomly include 0 or 1 bearer credential.
		if rapid.Bool().Draw(t, "includeBearer") {
			creds = append(creds, AuthCredential{
				AuthType: "bearer",
				Token:    rapid.StringMatching(`[A-Za-z0-9._]{1,64}`).Draw(t, "bearerToken"),
			})
		}

		// Randomly include 0 or 1 basic credential.
		if rapid.Bool().Draw(t, "includeBasic") {
			creds = append(creds, AuthCredential{
				AuthType: "basic",
				Username: rapid.StringMatching(`[A-Za-z0-9]{1,32}`).Draw(t, "username"),
				Password: rapid.StringMatching(`[A-Za-z0-9]{1,32}`).Draw(t, "password"),
			})
		}

		// Include 0 to 3 custom header credentials with unique names.
		numHeaders := rapid.IntRange(0, 3).Draw(t, "numHeaders")
		for i := range numHeaders {
			name := rapid.StringMatching(`[A-Za-z]{3,15}`).Draw(t, "headerSuffix")
			creds = append(creds, AuthCredential{
				AuthType:    "header",
				HeaderName:  "X-Custom-" + name + "-" + strings.Repeat("x", i),
				HeaderValue: rapid.StringMatching(`[A-Za-z0-9._]{1,64}`).Draw(t, "headerValue"),
			})
		}

		// Ensure at least one credential is present.
		if len(creds) == 0 {
			creds = append(creds, AuthCredential{
				AuthType: "bearer",
				Token:    rapid.StringMatching(`[A-Za-z0-9._]{1,64}`).Draw(t, "fallbackToken"),
			})
		}

		// Start a test WebSocket server that captures the upgrade request headers.
		var mu sync.Mutex
		var capturedHeaders http.Header

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			capturedHeaders = r.Header.Clone()
			mu.Unlock()

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			// Keep connection open briefly so the client can complete the handshake.
			time.Sleep(50 * time.Millisecond)
		}))
		defer server.Close()

		// Convert http:// URL to ws:// for the WebSocket dialer.
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		// Call CheckWithAuth with generated credentials.
		checker := &WebSocketChecker{}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result := checker.CheckWithAuth(ctx, wsURL, json.RawMessage(`{}`), creds)

		// The connection should succeed.
		if result.State != "up" {
			t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
		}

		// Verify all credentials were injected correctly into the upgrade request headers.
		mu.Lock()
		defer mu.Unlock()

		if capturedHeaders == nil {
			t.Fatal("no headers captured — server handler was not called")
		}

		// Determine the expected Authorization header value.
		// Since Header.Set is used, the last bearer or basic credential determines
		// the final Authorization header value.
		var expectedAuth string
		for _, cred := range creds {
			switch cred.AuthType {
			case "bearer":
				expectedAuth = "Bearer " + cred.Token
			case "basic":
				encoded := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password))
				expectedAuth = "Basic " + encoded
			}
		}

		// Verify Authorization header if any bearer/basic credential was present.
		if expectedAuth != "" {
			got := capturedHeaders.Get("Authorization")
			if got != expectedAuth {
				t.Fatalf("Authorization header mismatch: got %q, want %q", got, expectedAuth)
			}
		}

		// Verify custom header credentials. For headers with the same name,
		// the last value wins (Header.Set behavior).
		headerExpected := make(map[string]string)
		for _, cred := range creds {
			if cred.AuthType == "header" {
				headerExpected[cred.HeaderName] = cred.HeaderValue
			}
		}
		for name, expectedValue := range headerExpected {
			got := capturedHeaders.Get(name)
			if got != expectedValue {
				t.Fatalf("custom header %q: got %q, want %q", name, got, expectedValue)
			}
		}
	})
}
