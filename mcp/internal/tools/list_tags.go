package tools

import (
	"context"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
)

// ListTagsInput defines the input schema for the list-tags tool.
type ListTagsInput struct {
	// Key is an optional tag key to list values for. When empty, returns all tag keys.
	// When provided, returns all distinct values for that key.
	Key *string `json:"key,omitempty" jsonschema:"Optional tag key to list values for. Omit to get all tag keys."`
}

// ListTagsOutput contains the tag keys or values.
type ListTagsOutput struct {
	// Mode indicates what was returned: "keys" or "values".
	Mode string `json:"mode"`

	// Key is the tag key (only set when mode is "values").
	Key string `json:"key,omitempty"`

	// Items is the list of tag keys or values.
	Items []string `json:"items"`

	// Count is the number of items returned.
	Count int `json:"count"`
}

// HandleListTags returns tag keys or values for a given key.
func HandleListTags(ctx context.Context, deps Deps, input ListTagsInput) (*ListTagsOutput, error) {
	if input.Key != nil && *input.Key != "" {
		key := *input.Key
		if len(key) > 255 {
			return nil, mcperr.Validation("key must be 1–255 characters")
		}

		values, err := deps.Client.ListTagValues(ctx, key)
		if err != nil {
			return nil, mapPulseError(err)
		}
		if values == nil {
			values = []string{}
		}
		return &ListTagsOutput{
			Mode:  "values",
			Key:   key,
			Items: values,
			Count: len(values),
		}, nil
	}

	keys, err := deps.Client.ListTags(ctx)
	if err != nil {
		return nil, mapPulseError(err)
	}
	if keys == nil {
		keys = []string{}
	}
	return &ListTagsOutput{
		Mode:  "keys",
		Items: keys,
		Count: len(keys),
	}, nil
}
