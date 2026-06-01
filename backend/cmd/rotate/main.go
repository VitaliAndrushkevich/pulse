// Command rotate re-encrypts all secret values from an old key to a new key
// within a single database transaction. If any step fails, the entire operation
// rolls back and no data is modified.
//
// Usage:
//
//	PULSE_SECRET_KEY=<old-key-base64> PULSE_SECRET_KEY_NEW=<new-key-base64> \
//	  DATABASE_URL=<postgres-url> go run ./cmd/rotate
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Load old key from PULSE_SECRET_KEY.
	oldKey, err := crypto.LoadKey("PULSE_SECRET_KEY")
	if err != nil {
		log.Fatalf("failed to load old key: %v", err)
	}

	// Load new key from PULSE_SECRET_KEY_NEW.
	newKey, err := crypto.LoadKey("PULSE_SECRET_KEY_NEW")
	if err != nil {
		log.Fatalf("failed to load new key: %v", err)
	}

	// Validate keys are different.
	if base64.StdEncoding.EncodeToString(oldKey) == base64.StdEncoding.EncodeToString(newKey) {
		log.Fatal("old and new keys are identical; nothing to rotate")
	}

	// Connect to database.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Begin transaction.
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() {
		// Rollback is a no-op if tx was already committed.
		_ = tx.Rollback(ctx)
	}()

	queries := db.New(tx)

	// Fetch all secrets.
	secrets, err := queries.ListAllSecrets(ctx)
	if err != nil {
		log.Fatalf("failed to list secrets: %v", err)
	}

	if len(secrets) == 0 {
		fmt.Println("no secrets to rotate")
		return
	}

	fmt.Printf("rotating %d secret(s)...\n", len(secrets))

	// Re-encrypt each secret.
	for i, s := range secrets {
		// Decode the stored base64 ciphertext.
		ciphertext, err := base64.StdEncoding.DecodeString(s.EncryptedValue)
		if err != nil {
			log.Fatalf("secret %s: failed to decode stored value: %v", s.ID, err)
		}

		// Decrypt with old key.
		plaintext, err := crypto.Decrypt(oldKey, ciphertext)
		if err != nil {
			log.Fatalf("secret %s: failed to decrypt with old key: %v", s.ID, err)
		}

		// Encrypt with new key.
		newCiphertext, err := crypto.Encrypt(newKey, plaintext)
		if err != nil {
			log.Fatalf("secret %s: failed to encrypt with new key: %v", s.ID, err)
		}

		// Update in database (within transaction).
		_, err = queries.UpdateSecret(ctx, db.UpdateSecretParams{
			ID:             s.ID,
			Name:           s.Name,
			EncryptedValue: base64.StdEncoding.EncodeToString(newCiphertext),
		})
		if err != nil {
			log.Fatalf("secret %s: failed to update: %v", s.ID, err)
		}

		fmt.Printf("  [%d/%d] rotated secret %q (%s)\n", i+1, len(secrets), s.Name, s.ID)
	}

	// Commit transaction.
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("failed to commit transaction: %v", err)
	}

	fmt.Printf("\nsuccessfully rotated %d secret(s)\n", len(secrets))
	fmt.Println("update PULSE_SECRET_KEY to the new key value and remove PULSE_SECRET_KEY_NEW")
}
