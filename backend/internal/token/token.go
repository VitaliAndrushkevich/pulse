// Package token provides cryptographic token generation and validation
// for API Bearer authentication.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
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

// --- Heartbeat ping-token helpers (SHA-256 fast path) ---
//
// Heartbeat ping tokens deliberately diverge from the bcrypt rule used for API
// tokens and passwords. A ping token is a 256-bit crypto/rand secret verified on
// an unauthenticated endpoint that is hit on every cron run; bcrypt's ~60-100ms
// cost per verification would be a CPU-exhaustion vector and buys nothing against
// a high-entropy random secret (there is no dictionary to slow-guess). We store a
// one-way SHA-256 hex digest instead and compare it in constant time. This scoped
// deviation applies ONLY to ping tokens — Generate()/ValidateHash() (bcrypt) stay
// the authentication path for API tokens and passwords.

// GenerateRaw creates a new 256-bit raw ping token (43 chars, base64url, no
// padding) and its 8-char lookup prefix. Unlike Generate it does NOT compute a
// bcrypt hash — the caller persists HashRawSHA256(raw) and the prefix.
func GenerateRaw() (raw string, prefix string, err error) {
	buf := make([]byte, RawBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("token: read crypto/rand: %w", err)
	}

	raw = base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw[:PrefixLen]
	return raw, prefix, nil
}

// HashRawSHA256 returns the lowercase hex-encoded SHA-256 digest of a raw token.
// This is the at-rest representation of a ping token (one-way, never reversible).
func HashRawSHA256(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CompareSHA256Hex reports whether the SHA-256 hex digest of raw equals the
// stored hex digest, using a constant-time comparison to avoid timing leaks.
func CompareSHA256Hex(raw, storedHex string) bool {
	computed := HashRawSHA256(raw)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHex)) == 1
}

// DummyCompareSHA256 performs a constant-time comparison against a fixed dummy
// digest. Call it on a prefix miss (no candidate rows) so the no-match path spends
// comparable time to the match path, keeping token verification timing-safe.
func DummyCompareSHA256(raw string) bool {
	const dummyHex = "0000000000000000000000000000000000000000000000000000000000000000"
	computed := HashRawSHA256(raw)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(dummyHex)) == 1
}
