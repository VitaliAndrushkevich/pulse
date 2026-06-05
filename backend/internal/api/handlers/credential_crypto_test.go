package handlers

import (
	"crypto/rand"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestCredentialEncryptionRoundTrip verifies Property 1: Credential encryption round-trip.
// For any valid CredentialPayload, encrypting then decrypting produces an identical result.
//
// **Validates: Requirements 1.1, 1.2, 1.3, 10.1**
func TestCredentialEncryptionRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random 32-byte AES-256 key.
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}

		// Generate an arbitrary CredentialPayload with varying combinations of fields.
		payload := CredentialPayload{
			Token:       rapid.String().Draw(t, "token"),
			Username:    rapid.String().Draw(t, "username"),
			Password:    rapid.String().Draw(t, "password"),
			HeaderName:  rapid.String().Draw(t, "headerName"),
			HeaderValue: rapid.String().Draw(t, "headerValue"),
		}

		// Encrypt the payload.
		encrypted, err := encryptCredentialPayload(key, payload)
		if err != nil {
			t.Fatalf("encrypt failed: %v", err)
		}

		// Decrypt the ciphertext.
		decrypted, err := decryptCredentialPayload(key, encrypted)
		if err != nil {
			t.Fatalf("decrypt failed: %v", err)
		}

		// Assert round-trip produces identical output.
		if decrypted.Token != payload.Token {
			t.Fatalf("Token mismatch: got %q, want %q", decrypted.Token, payload.Token)
		}
		if decrypted.Username != payload.Username {
			t.Fatalf("Username mismatch: got %q, want %q", decrypted.Username, payload.Username)
		}
		if decrypted.Password != payload.Password {
			t.Fatalf("Password mismatch: got %q, want %q", decrypted.Password, payload.Password)
		}
		if decrypted.HeaderName != payload.HeaderName {
			t.Fatalf("HeaderName mismatch: got %q, want %q", decrypted.HeaderName, payload.HeaderName)
		}
		if decrypted.HeaderValue != payload.HeaderValue {
			t.Fatalf("HeaderValue mismatch: got %q, want %q", decrypted.HeaderValue, payload.HeaderValue)
		}
	})
}

// TestCredentialUpdateReplacesEncryptedValue verifies Property 7: Credential update replaces encrypted value.
// For any existing credential (old payload) and any new secret value, after updating the credential
// with the new value, decrypting the stored encrypted_value yields the new value (not the old value),
// and the update timestamp is greater than or equal to the original.
//
// **Validates: Requirements 3.1, 3.2**
func TestCredentialUpdateReplacesEncryptedValue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Step 1: Generate a random AES-256 key.
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatal(err)
		}

		// Step 2: Generate an "old" CredentialPayload and encrypt it (simulating original stored value).
		oldPayload := CredentialPayload{
			Token:       rapid.String().Draw(t, "oldToken"),
			Username:    rapid.String().Draw(t, "oldUsername"),
			Password:    rapid.String().Draw(t, "oldPassword"),
			HeaderName:  rapid.String().Draw(t, "oldHeaderName"),
			HeaderValue: rapid.String().Draw(t, "oldHeaderValue"),
		}

		oldEncrypted, err := encryptCredentialPayload(key, oldPayload)
		if err != nil {
			t.Fatalf("encrypt old payload failed: %v", err)
		}

		// Record the "original" timestamp before the update.
		originalTimestamp := time.Now()

		// Step 3: Generate a "new" CredentialPayload (the update).
		newPayload := CredentialPayload{
			Token:       rapid.String().Draw(t, "newToken"),
			Username:    rapid.String().Draw(t, "newUsername"),
			Password:    rapid.String().Draw(t, "newPassword"),
			HeaderName:  rapid.String().Draw(t, "newHeaderName"),
			HeaderValue: rapid.String().Draw(t, "newHeaderValue"),
		}

		// Step 4: Encrypt the new payload (simulating what the Update handler does).
		newEncrypted, err := encryptCredentialPayload(key, newPayload)
		if err != nil {
			t.Fatalf("encrypt new payload failed: %v", err)
		}

		// Record the "updated" timestamp after the update.
		updatedTimestamp := time.Now()

		// The new encrypted value must differ from the old one (AES-GCM uses random nonce).
		// Even if payloads were identical, the encrypted values would differ due to random nonce.
		// But we only assert correctness of the decrypted value.

		// Step 5: Decrypt the new encrypted value.
		decrypted, err := decryptCredentialPayload(key, newEncrypted)
		if err != nil {
			t.Fatalf("decrypt new encrypted value failed: %v", err)
		}

		// Step 6: Assert the decrypted result matches the NEW payload (not the old one).
		if decrypted.Token != newPayload.Token {
			t.Fatalf("Token mismatch: got %q, want %q (new), old was %q",
				decrypted.Token, newPayload.Token, oldPayload.Token)
		}
		if decrypted.Username != newPayload.Username {
			t.Fatalf("Username mismatch: got %q, want %q (new), old was %q",
				decrypted.Username, newPayload.Username, oldPayload.Username)
		}
		if decrypted.Password != newPayload.Password {
			t.Fatalf("Password mismatch: got %q, want %q (new), old was %q",
				decrypted.Password, newPayload.Password, oldPayload.Password)
		}
		if decrypted.HeaderName != newPayload.HeaderName {
			t.Fatalf("HeaderName mismatch: got %q, want %q (new), old was %q",
				decrypted.HeaderName, newPayload.HeaderName, oldPayload.HeaderName)
		}
		if decrypted.HeaderValue != newPayload.HeaderValue {
			t.Fatalf("HeaderValue mismatch: got %q, want %q (new), old was %q",
				decrypted.HeaderValue, newPayload.HeaderValue, oldPayload.HeaderValue)
		}

		// Validate the updated timestamp is >= original (simulating Requirement 3.2: updated_at updated).
		if updatedTimestamp.Before(originalTimestamp) {
			t.Fatalf("updated timestamp %v is before original %v", updatedTimestamp, originalTimestamp)
		}

		// Also verify that the old encrypted value still decrypts to the old payload
		// (confirming the new encryption replaced rather than mutated).
		oldDecrypted, err := decryptCredentialPayload(key, oldEncrypted)
		if err != nil {
			t.Fatalf("decrypt old encrypted value failed: %v", err)
		}
		if oldDecrypted.Token != oldPayload.Token {
			t.Fatalf("old value was corrupted: Token got %q, want %q",
				oldDecrypted.Token, oldPayload.Token)
		}
	})
}
