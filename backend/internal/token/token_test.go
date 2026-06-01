package token

import (
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestGenerate(t *testing.T) {
	raw, prefix, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Raw token should be 43 characters (32 bytes base64url, no padding).
	if len(raw) != 43 {
		t.Errorf("raw length = %d, want 43", len(raw))
	}

	// Prefix should be first 8 characters of raw.
	if prefix != raw[:PrefixLen] {
		t.Errorf("prefix = %q, want %q", prefix, raw[:PrefixLen])
	}

	// Raw should decode to exactly 32 bytes.
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("raw token is not valid base64url: %v", err)
	}
	if len(decoded) != RawBytes {
		t.Errorf("decoded length = %d, want %d", len(decoded), RawBytes)
	}

	// Hash should be valid bcrypt with cost >= BcryptCost.
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error: %v", err)
	}
	if cost < BcryptCost {
		t.Errorf("bcrypt cost = %d, want >= %d", cost, BcryptCost)
	}

	// Hash should validate against the raw token.
	if !ValidateHash(raw, hash) {
		t.Error("ValidateHash(raw, hash) = false, want true")
	}
}

func TestGenerateUniqueness(t *testing.T) {
	raw1, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	raw2, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if raw1 == raw2 {
		t.Error("two generated tokens are identical")
	}
}

func TestValidateHash_WrongToken(t *testing.T) {
	_, _, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if ValidateHash("wrong-token-value", hash) {
		t.Error("ValidateHash with wrong token = true, want false")
	}
}

func TestValidateHash_InvalidHash(t *testing.T) {
	raw, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if ValidateHash(raw, "not-a-bcrypt-hash") {
		t.Error("ValidateHash with invalid hash = true, want false")
	}
}
