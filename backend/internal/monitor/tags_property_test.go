package monitor

import (
	"regexp"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

// validKeyGen generates a random key matching ^[a-z][a-z0-9_-]{0,63}$ that does
// not start with "__". This isolates set-constraint testing from key/value format
// validation.
func validKeyGen() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		// First character: lowercase letter.
		first := rapid.ByteRange('a', 'z').Draw(t, "first")

		// Remaining 0-63 characters from [a-z0-9_-].
		restLen := rapid.IntRange(0, 63).Draw(t, "restLen")
		rest := make([]byte, restLen)
		charset := "abcdefghijklmnopqrstuvwxyz0123456789_-"
		for i := range rest {
			rest[i] = charset[rapid.IntRange(0, len(charset)-1).Draw(t, "charIdx")]
		}

		key := string(first) + string(rest)

		// Ensure no reserved prefix (extremely unlikely but guard against it).
		if len(key) >= 2 && key[0] == '_' && key[1] == '_' {
			key = "a" + key[1:]
		}
		return key
	})
}

// validValueGen generates a random value of 1-256 printable characters (no
// control characters). This isolates set-constraint testing from value
// validation.
func validValueGen() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		length := rapid.IntRange(1, 256).Draw(t, "valueLen")
		// Use printable ASCII range 0x20-0x7E for simplicity.
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = byte(rapid.IntRange(0x20, 0x7E).Draw(t, "char"))
		}
		return string(buf)
	})
}

// validTagGen generates a single TagRequest with valid key and value.
func validTagGen() *rapid.Generator[TagRequest] {
	return rapid.Custom(func(t *rapid.T) TagRequest {
		return TagRequest{
			Key:   validKeyGen().Draw(t, "key"),
			Value: validValueGen().Draw(t, "value"),
		}
	})
}

// hasDuplicatePair returns true if the tag set contains any duplicate (key, value) pair.
func hasDuplicatePair(tags []TagRequest) bool {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		pair := tag.Key + "=" + tag.Value
		if _, exists := seen[pair]; exists {
			return true
		}
		seen[pair] = struct{}{}
	}
	return false
}

// referenceKeyPattern is the oracle regex for tag key validation.
var referenceKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// referenceKeyValid checks whether a string should be accepted as a valid tag
// key per Requirements 2.1 and 2.2.
func referenceKeyValid(s string) bool {
	return referenceKeyPattern.MatchString(s) && !strings.HasPrefix(s, "__")
}

// TestPropertyTagKeyValidation verifies Property 2: Tag Key Validation Correctness.
//
// Generate random strings; verify accepted iff matches ^[a-z][a-z0-9_-]{0,63}$
// AND not __ prefix.
//
// **Validates: Requirements 2.1, 2.2**
func TestPropertyTagKeyValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random strings covering a mix of valid and invalid keys.
		key := rapid.OneOf(
			// Completely random strings (mostly invalid)
			rapid.StringMatching(`[a-zA-Z0-9_\-. ]{0,80}`),
			// Strings that look like valid keys (high hit rate for valid)
			rapid.StringMatching(`[a-z][a-z0-9_-]{0,63}`),
			// Edge cases: reserved prefix
			rapid.Map(rapid.StringMatching(`[a-z0-9_-]{0,62}`), func(s string) string {
				return "__" + s
			}),
			// Empty string
			rapid.Just(""),
		).Draw(t, "key")

		// Use a valid value so only the key validation is exercised.
		tags := []TagRequest{{Key: key, Value: "valid-value"}}
		err := ValidateTags(tags)

		expected := referenceKeyValid(key)

		if expected && err != nil {
			t.Fatalf("key %q should be accepted (matches regex, no __ prefix) but got error: %v", key, err)
		}
		if !expected && err == nil {
			t.Fatalf("key %q should be rejected (fails regex or has __ prefix) but was accepted", key)
		}
	})
}

// TestPropertyTagSetConstraintValidation verifies Property 4: Tag Set Constraint
// Validation.
//
// Generate random tag sets with valid keys and values; verify rejected iff
// len > 20 or contains duplicate (key, value) pairs.
//
// **Validates: Requirements 2.5, 2.6**
func TestPropertyTagSetConstraintValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a tag set of size 0-30 (exceeding the 20 limit to test both sides).
		n := rapid.IntRange(0, 30).Draw(t, "numTags")
		tags := make([]TagRequest, n)
		for i := range tags {
			tags[i] = validTagGen().Draw(t, "tag")
		}

		err := ValidateTags(tags)

		tooMany := len(tags) > 20
		hasDup := hasDuplicatePair(tags)

		if tooMany || hasDup {
			// Should be rejected.
			if err == nil {
				t.Fatalf("expected rejection: tooMany=%v, hasDup=%v, tags=%d",
					tooMany, hasDup, len(tags))
			}
		} else {
			// Should be accepted (regarding set constraints).
			if err != nil {
				t.Fatalf("expected acceptance but got error: %v (tags=%d, hasDup=%v)",
					err, len(tags), hasDup)
			}
		}
	})
}

// containsControl returns true if s contains any Unicode control character.
func containsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// TestPropertyTagValueValidationCorrectness verifies Property 3: Tag Value
// Validation Correctness.
//
// Generate random strings; verify accepted iff length 1–256 and no control
// characters (using unicode.IsControl as oracle).
//
// **Validates: Requirements 2.3, 2.4**
func TestPropertyTagValueValidationCorrectness(t *testing.T) {
	const validKey = "testkey"

	rapid.Check(t, func(t *rapid.T) {
		// Generate a random string of length 0–300 that may include control
		// characters. We use a broad rune pool covering printable ranges plus
		// the Unicode Cc (control) category.
		value := rapid.StringOfN(
			rapid.RuneFrom(nil,
				unicode.Latin,
				unicode.Cyrillic,
				unicode.Han,
				unicode.Cc, // control characters
			),
			0, 300, -1,
		).Draw(t, "value")

		err := ValidateTags([]TagRequest{{Key: validKey, Value: value}})

		// Oracle: value is accepted iff byte length is 1–256 and it has no
		// control characters.
		shouldAccept := len(value) >= 1 && len(value) <= 256 && !containsControl(value)

		if shouldAccept && err != nil {
			t.Fatalf("expected value %q (len=%d) to be accepted, but got error: %v",
				value, len(value), err)
		}
		if !shouldAccept && err == nil {
			t.Fatalf("expected value %q (len=%d) to be rejected, but got nil error",
				value, len(value))
		}
	})
}

// genMixedTagRequest generates a random TagRequest with a mix of valid and
// invalid keys/values to exercise all validation paths.
func genMixedTagRequest() *rapid.Generator[TagRequest] {
	return rapid.Custom(func(t *rapid.T) TagRequest {
		key := rapid.OneOf(
			// Valid keys
			rapid.Custom(func(t *rapid.T) string {
				first := string(rune(rapid.IntRange('a', 'z').Draw(t, "first")))
				restLen := rapid.IntRange(0, 63).Draw(t, "restLen")
				charset := "abcdefghijklmnopqrstuvwxyz0123456789_-"
				rest := make([]byte, restLen)
				for i := range rest {
					rest[i] = charset[rapid.IntRange(0, len(charset)-1).Draw(t, "c")]
				}
				return first + string(rest)
			}),
			// Invalid keys (random strings)
			rapid.String(),
		).Draw(t, "key")

		value := rapid.OneOf(
			// Valid values (1-256 printable chars)
			rapid.Custom(func(t *rapid.T) string {
				length := rapid.IntRange(1, 256).Draw(t, "vLen")
				buf := make([]byte, length)
				for i := range buf {
					buf[i] = byte(rapid.IntRange(0x20, 0x7E).Draw(t, "vc"))
				}
				return string(buf)
			}),
			// Potentially invalid values (random strings including empty, control chars, etc.)
			rapid.String(),
		).Draw(t, "value")

		return TagRequest{Key: key, Value: value}
	})
}

// genMixedTagSet generates a random slice of TagRequests (0 to 25 elements)
// covering both within-limit and over-limit sizes, with both valid and invalid
// individual tags.
func genMixedTagSet() *rapid.Generator[[]TagRequest] {
	return rapid.Custom(func(t *rapid.T) []TagRequest {
		size := rapid.IntRange(0, 25).Draw(t, "setSize")
		tags := make([]TagRequest, size)
		for i := range tags {
			tags[i] = genMixedTagRequest().Draw(t, "tag")
		}
		return tags
	})
}

// TestPropertyValidationDeterminism verifies Property 5: Tag Validation Determinism.
//
// For any input tag set T, calling ValidateTags(T) multiple times always produces
// the same result. This proves the function is pure with no side effects.
//
// **Validates: Requirements 2.8**
func TestPropertyValidationDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tags := genMixedTagSet().Draw(t, "tags")

		result1 := ValidateTags(tags)
		result2 := ValidateTags(tags)

		// Both nil or both non-nil.
		if (result1 == nil) != (result2 == nil) {
			t.Fatalf("determinism violated: first call returned %v, second returned %v",
				result1, result2)
		}

		// If both are errors, messages must be identical.
		if result1 != nil && result2 != nil {
			if result1.Error() != result2.Error() {
				t.Fatalf("determinism violated: error messages differ:\n  first:  %q\n  second: %q",
					result1.Error(), result2.Error())
			}
		}
	})
}
