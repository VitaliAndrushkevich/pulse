package redact

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: mcp-server, Property 26: Secrets and tokens never leak, and placeholders are disjoint
//
// For ANY generated secret value (non-empty strings) placed into a log entry as:
// - An Authorization header value (e.g., "Authorization: Bearer <secret>")
// - A known secret field value (e.g., "password":<secret>, token=<secret>)
// The output of SanitizeLog NEVER contains the original secret value AND the output
// only contains the Placeholder "[REDACTED]" in place of the secret.
//
// Also: if the result is NOT suppressed, the original secret value must not appear in
// the sanitized output. If the result IS suppressed, the RedactionFailureMarker must
// not contain the secret value.
//
// **Validates: Requirements 3.6, 11.1, 11.2, 11.3, 11.4**
func TestProperty26_SecretsNeverLeakAndPlaceholdersDisjoint(t *testing.T) {
	// Generator for secret values: non-empty strings that look like real secrets
	// (alphanumeric + special chars, no whitespace/commas/quotes that would break
	// the field patterns).
	secretGen := rapid.StringMatching(`[A-Za-z0-9\-_\.~!@#\$%\^&\*\+=/]{3,64}`)

	// Sub-property: Authorization Bearer header secrets never leak
	t.Run("authorization_bearer_header", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			secret := secretGen.Draw(t, "secret")

			entry := "Authorization: Bearer " + secret
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				// Suppressed: the marker must not contain the secret
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				// Not suppressed: secret must not appear in sanitized output
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				// Placeholder must be present
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: Authorization header without Bearer (e.g., Basic) secrets never leak
	t.Run("authorization_basic_header", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			secret := secretGen.Draw(t, "secret")

			entry := "Authorization: Basic " + secret
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: Standalone Bearer token secrets never leak
	t.Run("standalone_bearer_token", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			secret := secretGen.Draw(t, "secret")

			entry := "sending request with Bearer " + secret
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: JSON password field secrets never leak
	t.Run("json_password_field", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			secret := secretGen.Draw(t, "secret")

			entry := `{"password":"` + secret + `"}`
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: key=value format secret fields never leak
	t.Run("key_value_secret_field", func(t *testing.T) {
		// Use known secret field names from logFieldPatterns
		fieldNameGen := rapid.SampledFrom([]string{
			"secret", "credential", "token", "password", "key", "encryption",
		})

		rapid.Check(t, func(t *rapid.T) {
			field := fieldNameGen.Draw(t, "field")
			secret := secretGen.Draw(t, "secret")

			entry := field + "=" + secret
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: quoted key: "value" format secret fields never leak
	t.Run("quoted_key_colon_value", func(t *testing.T) {
		fieldNameGen := rapid.SampledFrom([]string{
			"secret", "credential", "token", "password", "key", "encryption",
		})

		rapid.Check(t, func(t *rapid.T) {
			field := fieldNameGen.Draw(t, "field")
			secret := secretGen.Draw(t, "secret")

			entry := `"` + field + `": "` + secret + `"`
			sanitized, suppressed := SanitizeLog(entry)

			if suppressed {
				if strings.Contains(RedactionFailureMarker, secret) {
					t.Fatalf("RedactionFailureMarker contains secret %q", secret)
				}
			} else {
				if strings.Contains(sanitized, secret) {
					t.Fatalf("sanitized output still contains secret %q\n  input:  %q\n  output: %q",
						secret, entry, sanitized)
				}
				if !strings.Contains(sanitized, Placeholder) {
					t.Fatalf("sanitized output does not contain placeholder %q\n  input:  %q\n  output: %q",
						Placeholder, entry, sanitized)
				}
			}
		})
	})

	// Sub-property: Placeholder is disjoint from generated secrets
	// The placeholder "[REDACTED]" should not be a substring of any generated secret.
	// This verifies the design intent that the placeholder shares no content with secrets.
	t.Run("placeholder_disjoint_from_secrets", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			secret := secretGen.Draw(t, "secret")

			// The placeholder must not equal the secret
			if secret == Placeholder {
				t.Fatalf("generated secret equals the placeholder: %q", secret)
			}
			// The placeholder must not be a substring of the secret
			if strings.Contains(secret, Placeholder) {
				t.Fatalf("secret %q contains the placeholder %q", secret, Placeholder)
			}
		})
	})
}

// Feature: mcp-server, Property 27: Secret-bearing fields are omitted with a withheld indication
//
// For ANY generated map[string]any containing a mix of secret-keyed and non-secret-keyed fields:
// - OmitSecretFields returns a cleaned map that contains NO keys matching IsSecretKey
// - The withheld list is non-empty (when secret keys existed in input)
// - The withheld list contains exactly the secret keys from the original
// - All non-secret keys are preserved with their original values
//
// **Validates: Requirements 11.3, 11.5**
func TestProperty27_SecretFieldsOmittedWithWithheldIndication(t *testing.T) {
	// Generator for non-secret key names (things that won't match secretKeyPatterns)
	nonSecretKeyGen := rapid.SampledFrom([]string{
		"id", "name", "type", "target", "status", "state", "interval_seconds",
		"timeout_seconds", "monitor_id", "created_at", "updated_at", "url",
		"host", "port", "method", "region", "description", "enabled",
	})

	// Generator for secret key names (things that WILL match secretKeyPatterns)
	secretKeyGen := rapid.SampledFrom([]string{
		"password", "api_secret", "access_token", "api_key", "auth_header",
		"authorization", "bearer_token", "encryption_key", "credential",
		"secret_value", "master_key", "token_hash",
	})

	// Sub-property: mixed map produces correct split
	t.Run("mixed_map_correct_split", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate 1-5 non-secret fields
			numNonSecret := rapid.IntRange(1, 5).Draw(t, "numNonSecret")
			// Generate 1-5 secret fields
			numSecret := rapid.IntRange(1, 5).Draw(t, "numSecret")

			input := make(map[string]any)
			expectedSecretKeys := make(map[string]bool)
			expectedNonSecretKeys := make(map[string]bool)

			// Add non-secret fields with unique suffixes to avoid collisions
			for i := 0; i < numNonSecret; i++ {
				base := nonSecretKeyGen.Draw(t, "nonSecretKey")
				key := base
				if _, exists := input[key]; exists {
					key = base + "_" + rapid.StringMatching(`[0-9]{3}`).Draw(t, "suffix")
				}
				// Only add if actually non-secret (double check)
				if !IsSecretKey(key) {
					val := rapid.StringMatching(`[a-z0-9\-]{1,20}`).Draw(t, "nonSecretVal")
					input[key] = val
					expectedNonSecretKeys[key] = true
				}
			}

			// Add secret fields with unique suffixes
			for i := 0; i < numSecret; i++ {
				base := secretKeyGen.Draw(t, "secretKey")
				key := base
				if _, exists := input[key]; exists {
					key = base + "_" + rapid.StringMatching(`[0-9]{3}`).Draw(t, "suffix")
				}
				// Only add if actually secret (double check)
				if IsSecretKey(key) {
					val := rapid.StringMatching(`[A-Za-z0-9]{5,40}`).Draw(t, "secretVal")
					input[key] = val
					expectedSecretKeys[key] = true
				}
			}

			// Skip if we ended up with no secret or no non-secret keys
			if len(expectedSecretKeys) == 0 || len(expectedNonSecretKeys) == 0 {
				return
			}

			cleaned, withheld := OmitSecretFields(input)

			// 1. Cleaned map must NOT contain any secret keys
			for key := range cleaned {
				if IsSecretKey(key) {
					t.Fatalf("cleaned map still contains secret key %q", key)
				}
			}

			// 2. Withheld list must be non-empty when secret keys existed
			if len(withheld) == 0 {
				t.Fatalf("withheld list is empty but input had %d secret keys", len(expectedSecretKeys))
			}

			// 3. Withheld list must contain exactly the secret keys from the original
			withheldSet := make(map[string]bool)
			for _, w := range withheld {
				withheldSet[w] = true
			}
			for key := range expectedSecretKeys {
				if !withheldSet[key] {
					t.Fatalf("secret key %q missing from withheld list", key)
				}
			}
			if len(withheld) != len(expectedSecretKeys) {
				t.Fatalf("withheld count %d != expected secret key count %d",
					len(withheld), len(expectedSecretKeys))
			}

			// 4. All non-secret keys must be preserved with their original values
			for key := range expectedNonSecretKeys {
				cleanedVal, ok := cleaned[key]
				if !ok {
					t.Fatalf("non-secret key %q was removed from cleaned map", key)
				}
				if cleanedVal != input[key] {
					t.Fatalf("non-secret key %q value changed: got %v, want %v",
						key, cleanedVal, input[key])
				}
			}
		})
	})

	// Sub-property: withheld list is sorted
	t.Run("withheld_list_sorted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			numSecret := rapid.IntRange(2, 6).Draw(t, "numSecret")

			input := make(map[string]any)
			for i := 0; i < numSecret; i++ {
				key := secretKeyGen.Draw(t, "secretKey")
				if _, exists := input[key]; exists {
					key = key + "_" + rapid.StringMatching(`[0-9]{3}`).Draw(t, "suffix")
				}
				if IsSecretKey(key) {
					input[key] = "secret-value"
				}
			}

			// Add at least one non-secret key to make it a valid mixed map
			input["id"] = "test-id"

			_, withheld := OmitSecretFields(input)

			// Verify sorted order
			for i := 1; i < len(withheld); i++ {
				if withheld[i-1] > withheld[i] {
					t.Fatalf("withheld list not sorted: %q > %q at index %d",
						withheld[i-1], withheld[i], i)
				}
			}
		})
	})

	// Sub-property: nil input returns nil
	t.Run("nil_input_returns_nil", func(t *testing.T) {
		cleaned, withheld := OmitSecretFields(nil)
		if cleaned != nil {
			t.Fatalf("OmitSecretFields(nil) returned non-nil cleaned: %v", cleaned)
		}
		if withheld != nil {
			t.Fatalf("OmitSecretFields(nil) returned non-nil withheld: %v", withheld)
		}
	})

	// Sub-property: map with no secret keys returns all keys and empty withheld
	t.Run("no_secret_keys_preserves_all", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			numFields := rapid.IntRange(1, 8).Draw(t, "numFields")

			input := make(map[string]any)
			for i := 0; i < numFields; i++ {
				key := nonSecretKeyGen.Draw(t, "key")
				if _, exists := input[key]; exists {
					key = key + "_" + rapid.StringMatching(`[0-9]{3}`).Draw(t, "suffix")
				}
				if !IsSecretKey(key) {
					input[key] = rapid.StringMatching(`[a-z0-9]{1,10}`).Draw(t, "val")
				}
			}

			if len(input) == 0 {
				return
			}

			cleaned, withheld := OmitSecretFields(input)

			if len(withheld) != 0 {
				t.Fatalf("expected no withheld fields for non-secret input, got %v", withheld)
			}
			if len(cleaned) != len(input) {
				t.Fatalf("cleaned map size %d != input size %d", len(cleaned), len(input))
			}
			for key, val := range input {
				if cleaned[key] != val {
					t.Fatalf("value for key %q changed: got %v, want %v", key, cleaned[key], val)
				}
			}
		})
	})
}
