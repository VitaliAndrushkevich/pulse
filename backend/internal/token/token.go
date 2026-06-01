// Package token provides cryptographic token generation and validation
// for API Bearer authentication.
package token

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// RawBytes is the number of random bytes used to generate a token (256 bits).
	RawBytes = 32
	// PrefixLen is the number of leading base64url characters stored as a lookup hint.
	PrefixLen = 8
	// BcryptCost is the bcrypt cost factor used when hashing tokens.
	BcryptCost = 10
)

// Generate creates a new raw token string (43 chars, base64url, no padding)
// and returns it along with its prefix and bcrypt hash.
func Generate() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, RawBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("token: read crypto/rand: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw[:PrefixLen]

	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), BcryptCost)
	if err != nil {
		return "", "", "", fmt.Errorf("token: bcrypt hash: %w", err)
	}

	hash = string(hashed)
	return raw, prefix, hash, nil
}

// ValidateHash compares a raw token against a bcrypt hash.
// Uses bcrypt.CompareHashAndPassword (constant-time).
func ValidateHash(raw, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw))
	return err == nil
}
