package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
)

// encryptCredentialPayload serializes the payload to JSON and encrypts it
// using AES-256-GCM. Returns a base64-encoded string suitable for database storage.
func encryptCredentialPayload(key []byte, payload CredentialPayload) (string, error) {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("credential crypto: marshal payload: %w", err)
	}

	ciphertext, err := crypto.Encrypt(key, jsonBytes)
	if err != nil {
		return "", fmt.Errorf("credential crypto: encrypt: %w", err)
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptCredentialPayload decodes the base64 string, decrypts the ciphertext,
// and unmarshals the resulting JSON into a CredentialPayload.
func decryptCredentialPayload(key []byte, encrypted string) (CredentialPayload, error) {
	var payload CredentialPayload

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return payload, fmt.Errorf("credential crypto: decode base64: %w", err)
	}

	plaintext, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		return payload, fmt.Errorf("credential crypto: decrypt: %w", err)
	}

	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return payload, fmt.Errorf("credential crypto: unmarshal payload: %w", err)
	}

	return payload, nil
}
