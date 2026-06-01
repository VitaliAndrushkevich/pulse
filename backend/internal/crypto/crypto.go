// Package crypto provides AES-256-GCM authenticated encryption helpers
// for encrypting secret values at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

// LoadKey reads the named environment variable, base64-decodes it, and
// validates that the decoded key is exactly 32 bytes (256 bits) suitable
// for AES-256. Returns the raw key bytes or an error describing the issue.
func LoadKey(envKey string) ([]byte, error) {
	raw := os.Getenv(envKey)
	if raw == "" {
		return nil, fmt.Errorf("crypto: environment variable %s is not set or empty", envKey)
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("crypto: %s contains invalid base64: %w", envKey, err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: %s decoded to %d bytes, want 32", envKey, len(key))
	}

	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided 32-byte key.
// The returned ciphertext is formatted as nonce || sealed_data, where the nonce
// is prepended so Decrypt can extract it without additional metadata.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the authenticated ciphertext to nonce, giving us nonce || ciphertext.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt. It expects the input to be
// formatted as nonce || sealed_data. Returns the original plaintext or an error
// if the data is too short or authentication fails.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short (got %d bytes, need at least %d)", len(ciphertext), nonceSize)
	}

	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}

	return plaintext, nil
}
