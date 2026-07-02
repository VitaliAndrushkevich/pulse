// Package redact provides secret redaction and log sanitization for the
// Pulse MCP server. It ensures that secrets, credentials, tokens, passwords,
// and encryption keys never appear in log output or MCP tool results.
package redact

import (
	"regexp"
	"sort"
	"strings"
)

// Placeholder is the fixed redaction string that replaces secret values.
// It is chosen to share no characters with typical secret content (alphanumeric,
// special chars). The brackets and uppercase letters in the placeholder are
// deliberately distinct from base64, hex, and common password characters.
const Placeholder = "[REDACTED]"

// RedactionFailureMarker is emitted when a log entry cannot be safely redacted.
// It carries no secret content.
const RedactionFailureMarker = "[REDACTION_FAILURE: entry suppressed]"

// secretKeyPatterns are substrings (case-insensitive) that identify a key as secret.
// Used by IsSecretKey for map-based field filtering.
var secretKeyPatterns = []string{
	"secret",
	"credential",
	"token",
	"password",
	"key",
	"auth",
	"encryption",
	"bearer",
	"authorization",
}

// logFieldPatterns are the subset of secret key patterns used for log field
// matching. "bearer" and "authorization" are excluded because they are handled
// by dedicated header patterns.
var logFieldPatterns = []string{
	"secret",
	"credential",
	"token",
	"password",
	"key",
	"encryption",
}

// authHeaderPattern matches "Authorization: <scheme> <token>" or "Authorization: <token>".
// Uses [^\s,]+ to avoid consuming commas (in case headers are comma-delimited in logs).
var authHeaderPattern = regexp.MustCompile(`(?i)(Authorization\s*:\s*)[^\s,]+(?:\s+[^\s,]+)?`)

// bearerPattern matches standalone "Bearer <token>" not part of an Authorization header.
var bearerPattern = regexp.MustCompile(`(?i)(Bearer\s+)[^\s,]+`)

// secretFieldPattern matches key-value pairs in log strings where the key
// is a known secret field name. Handles patterns like:
//   - "field_name":"value"
//   - "field_name": "value"
//   - field_name=value
//   - field_name: value
//
// Uses logFieldPatterns (excludes "bearer"/"authorization" which are handled
// by dedicated header patterns).
var secretFieldPattern = regexp.MustCompile(
	`(?i)(["']?(?:` + strings.Join(logFieldPatterns, "|") + `)["']?\s*[:=]\s*)(["']?)([^"'\s,}\]]+)(["']?)`,
)

// IsSecretKey reports whether the given key name matches a known secret pattern.
// The comparison is case-insensitive.
func IsSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range secretKeyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// SanitizeLog replaces Authorization/Bearer headers and known secret field
// values in a log entry with Placeholder. If the entry contains patterns that
// cannot be safely redacted (e.g., the entry is empty after sanitization would
// require removing all content, or the placeholder itself appears in what looks
// like a secret value), the entire entry is suppressed and suppressed returns true.
//
// When suppressed is true, the returned string is RedactionFailureMarker.
// When suppressed is false, the returned string is the sanitized entry.
func SanitizeLog(entry string) (sanitized string, suppressed bool) {
	if entry == "" {
		return entry, false
	}

	result := entry

	// Replace Authorization headers (with or without Bearer/Basic prefix).
	result = authHeaderPattern.ReplaceAllString(result, "${1}"+Placeholder)

	// Replace standalone Bearer tokens not already covered by the auth header pattern.
	// After authHeaderPattern runs, "Authorization: Bearer X" becomes "Authorization: [REDACTED]"
	// so bearerPattern won't re-match those (no "Bearer" keyword remains).
	result = bearerPattern.ReplaceAllString(result, "${1}"+Placeholder)

	// Replace known secret field values.
	result = secretFieldPattern.ReplaceAllString(result, "${1}${2}"+Placeholder+"${4}")

	// Safety check: if after redaction the result still contains content that
	// looks like it might be an unredacted secret (the original had secret markers
	// but redaction didn't catch everything), suppress the entry.
	if containsUnredactedSecrets(entry, result) {
		return RedactionFailureMarker, true
	}

	return result, false
}

// containsUnredactedSecrets checks if the sanitized result may still contain
// secret content that wasn't caught by the regex patterns. This is a safety
// net: if the original entry contained secret indicators but the result still
// has the same byte sequences (meaning our patterns missed them), we suppress.
func containsUnredactedSecrets(original, sanitized string) bool {
	// If the sanitized output is identical to the original and the original
	// contains secret-like patterns that should have been redacted, suppress it.
	if original == sanitized {
		// Check if there were secret patterns that should have matched.
		if authHeaderPattern.MatchString(original) {
			return true
		}
		if bearerPattern.MatchString(original) {
			return true
		}
		if secretFieldPattern.MatchString(original) {
			return true
		}
	}
	return false
}

// OmitSecretFields takes a map and removes all entries whose keys match known
// secret patterns. It returns a new map containing only non-secret fields and
// a sorted list of the keys that were withheld.
//
// This is used to sanitize tool results before returning them to MCP clients
// (Req 11.3, 11.5).
func OmitSecretFields(data map[string]any) (cleaned map[string]any, withheld []string) {
	if data == nil {
		return nil, nil
	}

	cleaned = make(map[string]any, len(data))
	for k, v := range data {
		if IsSecretKey(k) {
			withheld = append(withheld, k)
		} else {
			cleaned[k] = v
		}
	}

	sort.Strings(withheld)
	return cleaned, withheld
}
