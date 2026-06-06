package monitor

import "github.com/VitaliAndrushkevich/pulse/internal/tags"

// TagRequest represents a key-value tag pair submitted via the API.
// Re-exported from the tags package to avoid import cycles.
type TagRequest = tags.TagRequest

// ValidateTags validates a slice of tag requests.
// Re-exported from the tags package to avoid import cycles.
var ValidateTags = tags.ValidateTags
