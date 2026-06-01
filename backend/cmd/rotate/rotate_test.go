package main

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
)

// TestRotationRoundTrip verifies the core rotation logic:
// encrypt with old key → decrypt with old key → encrypt with new key → decrypt with new key.
func TestRotationRoundTrip(t *testing.T) {
	// Generate two distinct 32-byte keys.
	oldKey := make([]byte, 32)
	if _, err := rand.Read(oldKey); err != nil {
		t.Fatalf("generate old key: %v", err)
	}
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		t.Fatalf("generate new key: %v", err)
	}

	// Original plaintext.
	plaintext := []byte("super-secret-database-password-123")

	// Encrypt with old key (simulates existing stored value).
	ciphertext, err := crypto.Encrypt(oldKey, plaintext)
	if err != nil {
		t.Fatalf("encrypt with old key: %v", err)
	}

	// Store as base64 (matches how secrets are stored in DB).
	stored := base64.StdEncoding.EncodeToString(ciphertext)

	// --- Rotation logic (mirrors cmd/rotate) ---

	// Decode stored value.
	decoded, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		t.Fatalf("decode stored value: %v", err)
	}

	// Decrypt with old key.
	decrypted, err := crypto.Decrypt(oldKey, decoded)
	if err != nil {
		t.Fatalf("decrypt with old key: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted value mismatch: got %q, want %q", decrypted, plaintext)
	}

	// Encrypt with new key.
	newCiphertext, err := crypto.Encrypt(newKey, decrypted)
	if err != nil {
		t.Fatalf("encrypt with new key: %v", err)
	}

	newStored := base64.StdEncoding.EncodeToString(newCiphertext)

	// --- Verify new key can decrypt ---

	newDecoded, err := base64.StdEncoding.DecodeString(newStored)
	if err != nil {
		t.Fatalf("decode new stored value: %v", err)
	}

	result, err := crypto.Decrypt(newKey, newDecoded)
	if err != nil {
		t.Fatalf("decrypt with new key: %v", err)
	}

	if string(result) != string(plaintext) {
		t.Fatalf("final decrypted value mismatch: got %q, want %q", result, plaintext)
	}

	// Verify old key can no longer decrypt the new ciphertext.
	_, err = crypto.Decrypt(oldKey, newDecoded)
	if err == nil {
		t.Fatal("old key should not be able to decrypt value encrypted with new key")
	}
}
