package token

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
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

// --- Heartbeat ping-token helper tests (SHA-256 fast path) ---

// TestGenerateRaw_EntropyAndPrefix verifies that GenerateRaw yields a 256-bit
// base64url secret, an 8-char prefix matching the raw head, and skips bcrypt.
func TestGenerateRaw_EntropyAndPrefix(t *testing.T) {
	raw, prefix, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}

	if len(raw) != 43 {
		t.Errorf("raw length = %d, want 43 (32 bytes base64url, no padding)", len(raw))
	}
	if len(prefix) != PrefixLen {
		t.Errorf("prefix length = %d, want %d", len(prefix), PrefixLen)
	}
	if prefix != raw[:PrefixLen] {
		t.Errorf("prefix = %q, want %q", prefix, raw[:PrefixLen])
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("raw token is not valid base64url: %v", err)
	}
	if len(decoded) != RawBytes {
		t.Errorf("decoded length = %d, want %d (256 bits of entropy)", len(decoded), RawBytes)
	}
}

// TestGenerateRaw_Uniqueness verifies independent calls produce distinct tokens.
func TestGenerateRaw_Uniqueness(t *testing.T) {
	raw1, _, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}
	raw2, _, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two GenerateRaw tokens are identical")
	}
}

// TestHashRawSHA256_Determinism verifies the digest is a stable 64-char hex
// string equal to the standard-library SHA-256 of the input.
func TestHashRawSHA256_Determinism(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"short", "abc"},
		{"token-like", "a1B2c3D4e5F6g7H8i9J0kLmNoPqRsTuVwXyZ012345"},
		{"unicode", "héllo-wörld-日本語"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HashRawSHA256(tc.raw)

			// Deterministic: hashing twice yields the same digest.
			if again := HashRawSHA256(tc.raw); got != again {
				t.Errorf("HashRawSHA256 not deterministic: %q != %q", got, again)
			}

			// 64 lowercase hex chars (32-byte digest).
			if len(got) != sha256.Size*2 {
				t.Errorf("digest length = %d, want %d", len(got), sha256.Size*2)
			}
			if got != strings.ToLower(got) {
				t.Errorf("digest is not lowercase hex: %q", got)
			}

			// Matches the standard-library computation.
			want := func() string {
				sum := sha256.Sum256([]byte(tc.raw))
				return hex.EncodeToString(sum[:])
			}()
			if got != want {
				t.Errorf("HashRawSHA256 = %q, want %q", got, want)
			}
		})
	}
}

// TestCompareSHA256Hex verifies constant-time compare returns true only for a
// digest produced from the same raw token.
func TestCompareSHA256Hex(t *testing.T) {
	raw, _, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}
	stored := HashRawSHA256(raw)

	other, _, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}

	tests := []struct {
		name   string
		raw    string
		stored string
		want   bool
	}{
		{"matching token", raw, stored, true},
		{"wrong token", other, stored, false},
		{"empty raw", "", stored, false},
		{"empty stored", raw, "", false},
		{"tampered digest", raw, strings.Repeat("f", len(stored)), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompareSHA256Hex(tc.raw, tc.stored); got != tc.want {
				t.Errorf("CompareSHA256Hex(%q, ...) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestDummyCompareSHA256 verifies the timing-safety helper runs without panicking
// and never reports a match against the fixed dummy digest for a real token.
func TestDummyCompareSHA256(t *testing.T) {
	raw, _, err := GenerateRaw()
	if err != nil {
		t.Fatalf("GenerateRaw() error: %v", err)
	}
	if DummyCompareSHA256(raw) {
		t.Error("DummyCompareSHA256 matched a real token against the dummy digest")
	}
	// Also exercise the empty-input path (prefix-miss on a malformed token).
	if DummyCompareSHA256("") {
		t.Error("DummyCompareSHA256(\"\") unexpectedly matched the dummy digest")
	}
}
