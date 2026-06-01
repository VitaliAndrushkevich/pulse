package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"
)

// testKey generates a random 32-byte key for use in tests.
func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("hello, pulse secrets!")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecryptEmptyPlaintext(t *testing.T) {
	key := testKey(t)
	plaintext := []byte{}

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("empty round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestDifferentPlaintextsProduceDifferentCiphertexts(t *testing.T) {
	key := testKey(t)

	ct1, err := Encrypt(key, []byte("secret-a"))
	if err != nil {
		t.Fatalf("Encrypt a: %v", err)
	}

	ct2, err := Encrypt(key, []byte("secret-b"))
	if err != nil {
		t.Fatalf("Encrypt b: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("different plaintexts produced identical ciphertexts")
	}
}

func TestSamePlaintextProducesDifferentCiphertexts(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("same input every time")

	ct1, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}

	ct2, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("same plaintext encrypted twice produced identical ciphertexts (nonce reuse?)")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("tamper test")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a byte in the sealed portion (after the nonce).
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xff

	_, err = Decrypt(key, tampered)
	if err == nil {
		t.Error("Decrypt should fail on tampered ciphertext")
	}
}

func TestDecryptTruncatedCiphertext(t *testing.T) {
	key := testKey(t)

	// Provide fewer bytes than the nonce size (12 bytes for GCM).
	short := []byte("short")
	_, err := Decrypt(key, short)
	if err == nil {
		t.Error("Decrypt should fail on truncated ciphertext")
	}
}

func TestDecryptEmptyCiphertext(t *testing.T) {
	key := testKey(t)
	_, err := Decrypt(key, []byte{})
	if err == nil {
		t.Error("Decrypt should fail on empty ciphertext")
	}
}

func TestLoadKey_Valid(t *testing.T) {
	key := testKey(t)
	encoded := base64.StdEncoding.EncodeToString(key)

	const envName = "TEST_PULSE_SECRET_KEY"
	os.Setenv(envName, encoded)
	defer os.Unsetenv(envName)

	got, err := LoadKey(envName)
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}

	if !bytes.Equal(got, key) {
		t.Errorf("LoadKey returned wrong key")
	}
}

func TestLoadKey_Missing(t *testing.T) {
	const envName = "TEST_PULSE_MISSING_KEY"
	os.Unsetenv(envName)

	_, err := LoadKey(envName)
	if err == nil {
		t.Error("LoadKey should fail when env var is missing")
	}
}

func TestLoadKey_Empty(t *testing.T) {
	const envName = "TEST_PULSE_EMPTY_KEY"
	os.Setenv(envName, "")
	defer os.Unsetenv(envName)

	_, err := LoadKey(envName)
	if err == nil {
		t.Error("LoadKey should fail when env var is empty")
	}
}

func TestLoadKey_InvalidBase64(t *testing.T) {
	const envName = "TEST_PULSE_BAD_B64"
	os.Setenv(envName, "not-valid-base64!!!")
	defer os.Unsetenv(envName)

	_, err := LoadKey(envName)
	if err == nil {
		t.Error("LoadKey should fail on invalid base64")
	}
}

func TestLoadKey_WrongLength(t *testing.T) {
	// 16 bytes instead of 32.
	short := make([]byte, 16)
	if _, err := rand.Read(short); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(short)

	const envName = "TEST_PULSE_SHORT_KEY"
	os.Setenv(envName, encoded)
	defer os.Unsetenv(envName)

	_, err := LoadKey(envName)
	if err == nil {
		t.Error("LoadKey should fail when key is not 32 bytes")
	}
}
