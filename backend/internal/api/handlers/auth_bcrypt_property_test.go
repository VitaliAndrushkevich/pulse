package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

// TestProperty_BcryptSafety_RejectsOver72Bytes verifies Property 4: Bcrypt Safety.
// For any password string that exceeds 72 bytes, the ChangePassword handler returns
// HTTP 400 with VALIDATION_ERROR. This prevents bcrypt from silently truncating passwords.
//
// **Validates: Requirements 4.4**
func TestProperty_BcryptSafety_RejectsOver72Bytes(t *testing.T) {
	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	rapid.Check(t, func(t *rapid.T) {
		// Generate a password that is always > 72 bytes.
		// Start with a base of 73 bytes, add up to 200 more bytes.
		extraLen := rapid.IntRange(0, 200).Draw(t, "extraLen")
		password := strings.Repeat("x", 73+extraLen)

		// Verify our generator actually produces > 72 bytes.
		if len(password) <= 72 {
			t.Fatalf("generator bug: password is %d bytes, expected > 72", len(password))
		}

		body, _ := json.Marshal(map[string]string{
			"current_password": testCurrentPassword,
			"new_password":     password,
		})

		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("password of %d bytes: expected 400, got %d", len(password), w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON response: %v", err)
		}

		errObj, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error envelope in response")
		}
		if errObj["code"] != "VALIDATION_ERROR" {
			t.Fatalf("expected VALIDATION_ERROR, got %v", errObj["code"])
		}
	})
}

// TestProperty_BcryptSafety_AcceptsValidRange verifies Property 4: Bcrypt Safety (inverse).
// For any password string between 8 characters and 72 bytes (inclusive), the handler does NOT
// reject for length reasons (i.e., does not return 400 for password length).
// The request uses the correct current password, so it should succeed with 200.
//
// **Validates: Requirements 4.4**
func TestProperty_BcryptSafety_AcceptsValidRange(t *testing.T) {
	// Pre-compute the bcrypt hash for testCurrentPassword so we can reset it each iteration.
	originalHash, err := bcrypt.GenerateFromPassword([]byte(testCurrentPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	uid, user := newTestUser(t)
	fdb := &authFakeDB{user: user}
	router := setupAuthRouter(fdb, uid)

	rapid.Check(t, func(t *rapid.T) {
		// Reset the password hash before each iteration since ChangePassword mutates it.
		fdb.user.PasswordHash = string(originalHash)

		// Generate a password that is >= 8 characters and <= 72 bytes.
		// Use ASCII characters to keep byte length == character count predictable.
		passwordLen := rapid.IntRange(8, 72).Draw(t, "passwordLen")
		// Use printable ASCII characters (0x21-0x7A), avoids control chars.
		chars := make([]byte, passwordLen)
		for i := range chars {
			chars[i] = byte(rapid.IntRange(0x21, 0x7A).Draw(t, "char"))
		}
		password := string(chars)

		// Verify our constraints: >= 8 chars AND <= 72 bytes.
		if utf8.RuneCountInString(password) < 8 {
			t.Fatalf("generator bug: password has %d chars, expected >= 8", utf8.RuneCountInString(password))
		}
		if len(password) > 72 {
			t.Fatalf("generator bug: password is %d bytes, expected <= 72", len(password))
		}

		body, _ := json.Marshal(map[string]string{
			"current_password": testCurrentPassword,
			"new_password":     password,
		})

		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should succeed (200) — the password is in the valid range and current_password is correct.
		if w.Code != http.StatusOK {
			t.Fatalf("password of %d bytes (%d chars): expected 200, got %d: %s",
				len(password), utf8.RuneCountInString(password), w.Code, w.Body.String())
		}
	})
}
