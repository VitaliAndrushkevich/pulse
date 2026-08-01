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


// TestWebSocketChecker_ExpectedStatuses verifies that when expected_statuses is configured,
// a failed WebSocket upgrade returning a matching HTTP status code is treated as "up".
// This enables monitoring auth-protected endpoints where 401/403 means "alive but unauthorized".
func TestWebSocketChecker_ExpectedStatuses(t *testing.T) {
	tests := []struct {
		name             string
		serverStatusCode int
		expectedStatuses []int
		wantState        string
		wantErrorSubstr  string
	}{
		{
			name:             "401 with expected_statuses=[401] → up",
			serverStatusCode: 401,
			expectedStatuses: []int{401},
			wantState:        "up",
		},
		{
			name:             "403 with expected_statuses=[401,403] → up",
			serverStatusCode: 403,
			expectedStatuses: []int{401, 403},
			wantState:        "up",
		},
		{
			name:             "503 with expected_statuses=[401,403] → down",
			serverStatusCode: 503,
			expectedStatuses: []int{401, 403},
			wantState:        "down",
			wantErrorSubstr:  "HTTP 503",
		},
		{
			name:             "401 with no expected_statuses → down (default behavior)",
			serverStatusCode: 401,
			expectedStatuses: nil,
			wantState:        "down",
			wantErrorSubstr:  "HTTP 401",
		},
		{
			name:             "200 with expected_statuses=[200] → up (non-upgrade success code)",
			serverStatusCode: 200,
			expectedStatuses: []int{200},
			wantState:        "up",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Server that rejects WebSocket upgrade with the configured status code.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatusCode)
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

			settings := WebSocketSettings{
				ExpectedStatuses: tt.expectedStatuses,
			}
			settingsJSON, _ := json.Marshal(settings)

			checker := &WebSocketChecker{}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := checker.Check(ctx, wsURL, settingsJSON)

			if result.State != tt.wantState {
				t.Fatalf("state: got %q, want %q (error: %s)", result.State, tt.wantState, result.Error)
			}
			if tt.wantErrorSubstr != "" && !strings.Contains(result.Error, tt.wantErrorSubstr) {
				t.Fatalf("error %q does not contain %q", result.Error, tt.wantErrorSubstr)
			}
			if tt.wantState == "up" && result.Error != "" {
				t.Fatalf("expected no error for 'up' state, got %q", result.Error)
			}
		})
	}
}

// TestWebSocketChecker_ExpectedStatuses_SuccessfulUpgrade verifies that a successful
// WebSocket upgrade (101) always results in "up", regardless of expected_statuses.
func TestWebSocketChecker_ExpectedStatuses_SuccessfulUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Even with expected_statuses=[401], successful upgrade = up.
	settings := WebSocketSettings{
		ExpectedStatuses: []int{401},
	}
	settingsJSON, _ := json.Marshal(settings)

	checker := &WebSocketChecker{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := checker.Check(ctx, wsURL, settingsJSON)
	if result.State != "up" {
		t.Fatalf("expected 'up' on successful upgrade, got %q (error: %s)", result.State, result.Error)
	}
}

// TestProperty_WSExpectedStatusesValidation verifies the validation logic
// for WebSocket expected_statuses via property-based testing.
func TestProperty_WSExpectedStatusesValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		length := rapid.IntRange(0, 15).Draw(t, "length")
		statuses := make([]int, length)
		for i := range length {
			statuses[i] = rapid.IntRange(-10, 700).Draw(t, "status")
		}

		err := validateWSExpectedStatuses(statuses)

		// Too many entries.
		if length > 10 {
			if err == nil {
				t.Fatal("expected error for >10 entries, got nil")
			}
			return
		}

		// Check for out-of-range values.
		hasInvalid := false
		for _, code := range statuses {
			if code < 100 || code > 599 {
				hasInvalid = true
				break
			}
		}

		if hasInvalid {
			if err == nil {
				t.Fatal("expected error for out-of-range status, got nil")
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error for valid statuses %v: %v", statuses, err)
			}
		}
	})
}
