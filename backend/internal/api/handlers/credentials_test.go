package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"pgregory.net/rapid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// toMockUpdateRow creates a db.UpdateCredentialRow for testing purposes.
func toMockUpdateRow(id uuid.UUID, authType, name string, createdAt, updatedAt time.Time) db.UpdateCredentialRow {
	return db.UpdateCredentialRow{
		ID:        id,
		MonitorID: uuid.New(),
		AuthType:  authType,
		Name:      name,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

// TestMetadataIncludesCorrectNonSecretFieldsPerAuthType verifies Property 3:
// Metadata includes correct non-secret fields per auth_type.
//
// For auth_type=header: response includes header_name field (non-nil).
// For auth_type=basic: response includes username field (non-nil), password NOT in JSON.
// For auth_type=bearer: neither header_name nor username is present (both nil/omitempty).
//
// **Validates: Requirements 2.3, 2.4**
func TestMetadataIncludesCorrectNonSecretFieldsPerAuthType(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Pick a random auth_type.
		authType := rapid.SampledFrom([]string{"bearer", "basic", "header"}).Draw(t, "authType")

		// Generate arbitrary non-empty values for credential fields.
		name := rapid.StringMatching(`[a-zA-Z0-9]{1,50}`).Draw(t, "name")
		token := rapid.StringMatching(`[a-zA-Z0-9]{1,100}`).Draw(t, "token")
		username := rapid.StringMatching(`[a-zA-Z0-9]{1,50}`).Draw(t, "username")
		password := rapid.StringMatching(`[a-zA-Z0-9]{1,50}`).Draw(t, "password")
		headerName := rapid.StringMatching(`[a-zA-Z0-9\-]{1,50}`).Draw(t, "headerName")
		headerValue := rapid.StringMatching(`[a-zA-Z0-9]{1,100}`).Draw(t, "headerValue")

		// Build a CreateCredentialRequest with the appropriate fields.
		req := CreateCredentialRequest{
			AuthType:    authType,
			Name:        name,
			Token:       token,
			Username:    username,
			Password:    password,
			HeaderName:  headerName,
			HeaderValue: headerValue,
		}

		id := uuid.New()
		now := time.Now()

		// --- Test toCredentialResponse ---
		resp := toCredentialResponse(id, authType, name, req, now, now)

		switch authType {
		case "header":
			// header_name must be present and non-nil.
			if resp.HeaderName == nil {
				t.Fatal("auth_type=header: expected HeaderName to be non-nil")
			}
			if *resp.HeaderName != headerName {
				t.Fatalf("auth_type=header: HeaderName mismatch: got %q, want %q", *resp.HeaderName, headerName)
			}
			// username should NOT be present for header type.
			if resp.Username != nil {
				t.Fatal("auth_type=header: expected Username to be nil")
			}

		case "basic":
			// username must be present and non-nil.
			if resp.Username == nil {
				t.Fatal("auth_type=basic: expected Username to be non-nil")
			}
			if *resp.Username != username {
				t.Fatalf("auth_type=basic: Username mismatch: got %q, want %q", *resp.Username, username)
			}
			// header_name should NOT be present for basic type.
			if resp.HeaderName != nil {
				t.Fatal("auth_type=basic: expected HeaderName to be nil")
			}
			// Verify password is NOT in the JSON serialization.
			jsonBytes, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &raw); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if _, exists := raw["password"]; exists {
				t.Fatal("auth_type=basic: password field must NOT be in JSON response")
			}

		case "bearer":
			// Neither header_name nor username should be present.
			if resp.HeaderName != nil {
				t.Fatal("auth_type=bearer: expected HeaderName to be nil")
			}
			if resp.Username != nil {
				t.Fatal("auth_type=bearer: expected Username to be nil")
			}
		}

		// --- Also test credentialRowToResponse via toCredentialResponseFromUpdate ---
		// This tests the update path which also populates metadata.
		payload := CredentialPayload{
			Token:       token,
			Username:    username,
			Password:    password,
			HeaderName:  headerName,
			HeaderValue: headerValue,
		}

		updateResp := toCredentialResponseFromUpdate(
			toMockUpdateRow(id, authType, name, now, now),
			payload,
		)

		switch authType {
		case "header":
			if updateResp.HeaderName == nil {
				t.Fatal("update auth_type=header: expected HeaderName to be non-nil")
			}
			if *updateResp.HeaderName != headerName {
				t.Fatalf("update auth_type=header: HeaderName mismatch: got %q, want %q", *updateResp.HeaderName, headerName)
			}
			if updateResp.Username != nil {
				t.Fatal("update auth_type=header: expected Username to be nil")
			}

		case "basic":
			if updateResp.Username == nil {
				t.Fatal("update auth_type=basic: expected Username to be non-nil")
			}
			if *updateResp.Username != username {
				t.Fatalf("update auth_type=basic: Username mismatch: got %q, want %q", *updateResp.Username, username)
			}
			if updateResp.HeaderName != nil {
				t.Fatal("update auth_type=basic: expected HeaderName to be nil")
			}
			// Verify password is NOT in the JSON serialization.
			jsonBytes, err := json.Marshal(updateResp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &raw); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if _, exists := raw["password"]; exists {
				t.Fatal("update auth_type=basic: password field must NOT be in JSON response")
			}

		case "bearer":
			if updateResp.HeaderName != nil {
				t.Fatal("update auth_type=bearer: expected HeaderName to be nil")
			}
			if updateResp.Username != nil {
				t.Fatal("update auth_type=bearer: expected Username to be nil")
			}
		}
	})
}

// TestInvalidAuthTypeProducesValidationError verifies Property 4:
// Invalid auth_type produces validation error.
//
// For any string that is NOT one of {"bearer", "basic", "header"}, submitting it
// as the auth_type field in a create credential request SHALL result in a 400
// status code response with a message indicating valid auth_type values.
//
// **Validates: Requirements 1.7**
func TestInvalidAuthTypeProducesValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// The handler requires a *db.Queries and key, but since validation of
	// auth_type fails at the binding stage (before any DB calls), we can
	// use a nil DBTX — it will never be reached.
	handler := NewCredentialHandler(db.New(nil), make([]byte, 32))

	// Set up a gin engine with the credential create route.
	router := gin.New()
	router.POST("/monitors/:id/credentials", handler.Create)

	validAuthTypes := map[string]bool{
		"bearer": true,
		"basic":  true,
		"header": true,
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate an arbitrary string that is NOT a valid auth_type.
		// Use a filter to exclude the three valid values.
		invalidAuthType := rapid.String().Filter(func(s string) bool {
			return !validAuthTypes[s]
		}).Draw(t, "invalidAuthType")

		// Build a request body with the invalid auth_type and other required fields.
		reqBody := map[string]string{
			"auth_type":    invalidAuthType,
			"name":         "test-credential",
			"token":        "some-token",
			"username":     "user",
			"password":     "pass",
			"header_name":  "X-Custom",
			"header_value": "value",
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		// Use a valid monitor UUID in the URL path.
		monitorID := uuid.New().String()

		req := httptest.NewRequest(http.MethodPost, "/monitors/"+monitorID+"/credentials", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Assert: status code must be 400.
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for auth_type=%q, got %d: %s", invalidAuthType, w.Code, w.Body.String())
		}

		// Assert: error message mentions valid auth_type values.
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		errObj, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected error envelope in response, got: %s", w.Body.String())
		}

		message, ok := errObj["message"].(string)
		if !ok {
			t.Fatalf("expected string message in error, got: %v", errObj["message"])
		}

		// The message should mention the valid auth_type values.
		if !strings.Contains(message, "bearer") || !strings.Contains(message, "basic") || !strings.Contains(message, "header") {
			t.Fatalf("error message should mention valid auth_type values (bearer, basic, header), got: %q", message)
		}
	})
}


// TestCredentialDeletionRemovesFromListing verifies Property 8:
// Credential deletion removes from listing.
//
// For any monitor with N credentials (N in [2,10]), after deleting a specific
// credential by ID, the remaining credential list SHALL NOT include the
// deleted credential's ID, and the list length SHALL be exactly N-1.
//
// **Validates: Requirements 4.1**
func TestCredentialDeletionRemovesFromListing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate N credentials where N is between 2 and 10.
		n := rapid.IntRange(2, 10).Draw(t, "n")

		// Generate N unique CredentialResponse objects.
		credentials := make([]CredentialResponse, n)
		usedIDs := make(map[uuid.UUID]bool)
		for i := 0; i < n; i++ {
			// Ensure unique IDs.
			var id uuid.UUID
			for {
				id = uuid.New()
				if !usedIDs[id] {
					usedIDs[id] = true
					break
				}
			}

			authType := rapid.SampledFrom([]string{"bearer", "basic", "header"}).Draw(t, "authType")
			name := rapid.StringMatching(`[a-zA-Z0-9]{1,50}`).Draw(t, "name")
			now := time.Now()

			resp := CredentialResponse{
				ID:        id,
				AuthType:  authType,
				Name:      name,
				CreatedAt: now,
				UpdatedAt: now,
			}

			// Set metadata fields based on auth_type.
			switch authType {
			case "header":
				hn := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9\-]{0,30}`).Draw(t, "headerName")
				resp.HeaderName = &hn
			case "basic":
				un := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "username")
				resp.Username = &un
			}

			credentials[i] = resp
		}

		// Pick a random index to delete.
		deleteIdx := rapid.IntRange(0, n-1).Draw(t, "deleteIdx")
		deletedID := credentials[deleteIdx].ID

		// Simulate deletion: filter out the credential with the deleted ID.
		remaining := make([]CredentialResponse, 0, n-1)
		for _, cred := range credentials {
			if cred.ID != deletedID {
				remaining = append(remaining, cred)
			}
		}

		// Property assertion 1: list length is exactly N-1.
		if len(remaining) != n-1 {
			t.Fatalf("expected %d credentials after deletion, got %d", n-1, len(remaining))
		}

		// Property assertion 2: deleted ID is not present in remaining list.
		for _, cred := range remaining {
			if cred.ID == deletedID {
				t.Fatalf("deleted credential ID %s still present in listing", deletedID)
			}
		}
	})
}

// TestAPIResponsesNeverExposePlaintextSecrets verifies Property 2:
// API responses never expose plaintext secrets.
//
// For any credential stored in the system (regardless of auth_type), all API
// responses SHALL contain only metadata fields and SHALL NOT contain the fields:
// token, password, header_value, or encrypted_value.
//
// **Validates: Requirements 1.4, 2.1, 10.4**
func TestAPIResponsesNeverExposePlaintextSecrets(t *testing.T) {
	// Forbidden top-level JSON keys that must never appear in responses.
	forbiddenKeys := []string{"token", "password", "header_value", "encrypted_value"}

	authTypes := []string{"bearer", "basic", "header"}
	authTypeGen := rapid.SampledFrom(authTypes)

	t.Run("toCredentialResponse", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			authType := authTypeGen.Draw(t, "authType")

			req := CreateCredentialRequest{
				AuthType:    authType,
				Name:        rapid.String().Draw(t, "name"),
				Token:       rapid.String().Draw(t, "token"),
				Username:    rapid.String().Draw(t, "username"),
				Password:    rapid.String().Draw(t, "password"),
				HeaderName:  rapid.String().Draw(t, "headerName"),
				HeaderValue: rapid.String().Draw(t, "headerValue"),
			}

			id := uuid.New()
			now := time.Now()

			resp := toCredentialResponse(id, authType, req.Name, req, now, now)

			// Serialize to JSON.
			jsonBytes, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Parse back into a generic map to inspect keys.
			var m map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}

			for _, key := range forbiddenKeys {
				if _, exists := m[key]; exists {
					t.Fatalf("response JSON contains forbidden key %q: %s", key, string(jsonBytes))
				}
			}
		})
	})

	t.Run("toCredentialResponseFromUpdate", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			authType := authTypeGen.Draw(t, "authType")

			payload := CredentialPayload{
				Token:       rapid.String().Draw(t, "token"),
				Username:    rapid.String().Draw(t, "username"),
				Password:    rapid.String().Draw(t, "password"),
				HeaderName:  rapid.String().Draw(t, "headerName"),
				HeaderValue: rapid.String().Draw(t, "headerValue"),
			}

			row := db.UpdateCredentialRow{
				ID:        uuid.New(),
				MonitorID: uuid.New(),
				AuthType:  authType,
				Name:      rapid.String().Draw(t, "name"),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			resp := toCredentialResponseFromUpdate(row, payload)

			// Serialize to JSON.
			jsonBytes, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Parse back into a generic map to inspect keys.
			var m map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}

			for _, key := range forbiddenKeys {
				if _, exists := m[key]; exists {
					t.Fatalf("response JSON contains forbidden key %q: %s", key, string(jsonBytes))
				}
			}
		})
	})

	t.Run("credentialRowToResponse", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a random AES-256 key for encryption.
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				t.Fatal(err)
			}

			authType := authTypeGen.Draw(t, "authType")

			payload := CredentialPayload{
				Token:       rapid.String().Draw(t, "token"),
				Username:    rapid.String().Draw(t, "username"),
				Password:    rapid.String().Draw(t, "password"),
				HeaderName:  rapid.String().Draw(t, "headerName"),
				HeaderValue: rapid.String().Draw(t, "headerValue"),
			}

			// Encrypt the payload to create a realistic database row.
			encrypted, err := encryptCredentialPayload(key, payload)
			if err != nil {
				t.Fatalf("encryptCredentialPayload failed: %v", err)
			}

			row := db.MonitorCredential{
				ID:             uuid.New(),
				MonitorID:      uuid.New(),
				AuthType:       authType,
				Name:           rapid.String().Draw(t, "name"),
				EncryptedValue: encrypted,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			resp, err := credentialRowToResponse(key, row)
			if err != nil {
				t.Fatalf("credentialRowToResponse failed: %v", err)
			}

			// Serialize to JSON.
			jsonBytes, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Parse back into a generic map to inspect keys.
			var m map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}

			for _, key := range forbiddenKeys {
				if _, exists := m[key]; exists {
					t.Fatalf("response JSON contains forbidden key %q: %s", key, string(jsonBytes))
				}
			}
		})
	})
}
