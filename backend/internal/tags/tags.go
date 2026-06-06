package tags

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// TagRequest represents a key-value tag pair submitted via the API.
type TagRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// keyPattern enforces: starts with lowercase letter, followed by 0-63 lowercase
// alphanumeric, underscore, or hyphen characters.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// maxTagsPerMonitor is the maximum number of tags allowed on a single monitor.
const maxTagsPerMonitor = 20

// maxValueLength is the maximum allowed length for a tag value.
const maxValueLength = 256

// ValidateTags validates a slice of tag requests.
// It returns nil if all tags are valid, or a descriptive error identifying
// specific validation failures.
//
// Validation rules:
//   - Maximum 20 tags per monitor
//   - Key must match ^[a-z][a-z0-9_-]{0,63}$
//   - Key must not start with "__" (reserved prefix)
//   - Value must be 1–256 characters
//   - Value must contain only printable UTF-8 (no control characters)
//   - No duplicate (key, value) pairs
//
// This function is pure — no side effects or external state.
func ValidateTags(t []TagRequest) error {
	if len(t) > maxTagsPerMonitor {
		return fmt.Errorf("too many tags: got %d, maximum is %d", len(t), maxTagsPerMonitor)
	}

	seen := make(map[string]struct{}, len(t))

	for i, tag := range t {
		// Validate key format.
		if !keyPattern.MatchString(tag.Key) {
			return fmt.Errorf("tag[%d]: key %q does not match required pattern ^[a-z][a-z0-9_-]{0,63}$", i, tag.Key)
		}

		// Validate reserved prefix.
		if strings.HasPrefix(tag.Key, "__") {
			return fmt.Errorf("tag[%d]: key %q uses reserved prefix \"__\"", i, tag.Key)
		}

		// Validate value length.
		if len(tag.Value) == 0 {
			return fmt.Errorf("tag[%d]: value must not be empty", i)
		}
		if len(tag.Value) > maxValueLength {
			return fmt.Errorf("tag[%d]: value length %d exceeds maximum of %d", i, len(tag.Value), maxValueLength)
		}

		// Validate value contains only printable UTF-8 (no control characters).
		for j, r := range tag.Value {
			if unicode.IsControl(r) {
				return fmt.Errorf("tag[%d]: value contains control character at position %d", i, j)
			}
		}

		// Check for duplicate (key, value) pairs.
		pair := tag.Key + "=" + tag.Value
		if _, exists := seen[pair]; exists {
			return fmt.Errorf("tag[%d]: duplicate tag (key=%q, value=%q)", i, tag.Key, tag.Value)
		}
		seen[pair] = struct{}{}
	}

	return nil
}
