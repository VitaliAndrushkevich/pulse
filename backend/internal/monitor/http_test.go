package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestHTTPCheckerInjectsAllCredentialsCorrectly verifies Property 5:
// HTTP Checker injects all credentials correctly.
//
// For any set of monitor credentials (mix of bearer, basic, and header types),
// when the HTTP Checker executes CheckWithAuth, the outbound HTTP request
// SHALL contain the correctly-formatted headers for each credential.
//
// **Validates: Requirements 5.1, 5.2, 5.3, 5.4**
func TestHTTPCheckerInjectsAllCredentialsCorrectly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random set of credentials (1 to 10).
		creds := rapid.SliceOfN(credentialGen(), 1, 10).Draw(t, "credentials")

		// Start a test HTTP server that captures incoming request headers.
		var capturedHeaders http.Header
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		// Execute CheckWithAuth against the test server.
		checker := &HTTPChecker{}
		settings, _ := json.Marshal(HTTPSettings{Method: "GET"})
		result := checker.CheckWithAuth(context.Background(), ts.URL, settings, creds)

		// The check should succeed (status 200 is within default expected range).
		if result.State != "up" {
			t.Fatalf("expected state 'up', got %q (error: %s)", result.State, result.Error)
		}

		// Verify all credentials are correctly injected.
		// Note: When multiple bearer/basic credentials exist, later ones overwrite
		// earlier ones via Header.Set. We track the last bearer/basic credential
		// to verify the final Authorization header value.
		var lastBearer *AuthCredential
		var lastBasic *AuthCredential
		headerCreds := []AuthCredential{}

		for i := range creds {
			switch creds[i].AuthType {
			case "bearer":
				lastBearer = &creds[i]
			case "basic":
				lastBasic = &creds[i]
			case "header":
				headerCreds = append(headerCreds, creds[i])
			}
		}

		// Determine which auth type set Authorization last (bearer or basic).
		// The last one in the slice wins because Header.Set overwrites.
		var lastAuthCred *AuthCredential
		for i := len(creds) - 1; i >= 0; i-- {
			if creds[i].AuthType == "bearer" || creds[i].AuthType == "basic" {
				lastAuthCred = &creds[i]
				break
			}
		}

		// Verify Authorization header based on which credential was applied last.
		if lastAuthCred != nil {
			authHeader := capturedHeaders.Get("Authorization")
			switch lastAuthCred.AuthType {
			case "bearer":
				expected := "Bearer " + lastAuthCred.Token
				if authHeader != expected {
					t.Fatalf("Authorization header mismatch for bearer:\ngot:  %q\nwant: %q", authHeader, expected)
				}
			case "basic":
				expectedEncoded := base64.StdEncoding.EncodeToString(
					[]byte(lastAuthCred.Username + ":" + lastAuthCred.Password),
				)
				expected := "Basic " + expectedEncoded
				if authHeader != expected {
					t.Fatalf("Authorization header mismatch for basic:\ngot:  %q\nwant: %q", authHeader, expected)
				}
			}
		} else {
			// No bearer or basic credentials — Authorization header should be absent.
			if authHeader := capturedHeaders.Get("Authorization"); authHeader != "" {
				t.Fatalf("unexpected Authorization header: %q", authHeader)
			}
		}

		// For bearer: if there was any bearer credential, verify the final value.
		if lastBearer != nil && (lastAuthCred == nil || lastAuthCred.AuthType != "bearer") {
			// Bearer was overwritten by a later basic credential — that's fine,
			// we already verified the basic header above.
		}

		// For basic: if there was any basic credential, verify the final value.
		if lastBasic != nil && (lastAuthCred == nil || lastAuthCred.AuthType != "basic") {
			// Basic was overwritten by a later bearer credential — that's fine.
		}

		// Verify all custom header credentials are present.
		// Note: when multiple header credentials have the same header name,
		// Header.Set means the last one wins.
		lastHeaderByName := map[string]string{}
		for _, cred := range creds {
			if cred.AuthType == "header" {
				lastHeaderByName[http.CanonicalHeaderKey(cred.HeaderName)] = cred.HeaderValue
			}
		}

		for name, expectedValue := range lastHeaderByName {
			got := capturedHeaders.Get(name)
			if got != expectedValue {
				t.Fatalf("custom header %q mismatch:\ngot:  %q\nwant: %q", name, got, expectedValue)
			}
		}
	})
}

// credentialGen returns a rapid generator for AuthCredential values.
func credentialGen() *rapid.Generator[AuthCredential] {
	return rapid.Custom[AuthCredential](func(t *rapid.T) AuthCredential {
		authType := rapid.SampledFrom([]string{"bearer", "basic", "header"}).Draw(t, "authType")

		switch authType {
		case "bearer":
			return AuthCredential{
				AuthType: "bearer",
				Token:    rapid.StringMatching(`[a-zA-Z0-9._\-]{1,64}`).Draw(t, "token"),
			}
		case "basic":
			return AuthCredential{
				AuthType: "basic",
				Username: rapid.StringMatching(`[a-zA-Z0-9._]{1,32}`).Draw(t, "username"),
				Password: rapid.StringMatching(`[a-zA-Z0-9!@#$%^&*]{1,32}`).Draw(t, "password"),
			}
		case "header":
			return AuthCredential{
				AuthType:    "header",
				HeaderName:  validHTTPHeaderNameGen().Draw(t, "headerName"),
				HeaderValue: rapid.StringMatching(`[a-zA-Z0-9_\-./=]{1,64}`).Draw(t, "headerValue"),
			}
		}
		// Unreachable, but satisfies compiler.
		return AuthCredential{}
	})
}

// validHTTPHeaderNameGen generates valid HTTP header names (alphanumeric + hyphens,
// starting with a letter, 1–30 chars).
func validHTTPHeaderNameGen() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		// Generate header names that are valid HTTP header tokens.
		// Must start with a letter and contain only letters, digits, and hyphens.
		// Avoid "Authorization" to not conflict with bearer/basic credential injection.
		for {
			name := rapid.StringMatching(`[A-Z][a-zA-Z0-9\-]{0,29}`).Draw(t, "rawHeaderName")
			if !strings.EqualFold(name, "Authorization") && !strings.EqualFold(name, "User-Agent") {
				return name
			}
		}
	})
}
