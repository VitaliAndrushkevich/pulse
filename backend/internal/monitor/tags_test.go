package monitor

import (
	"strings"
	"testing"
)

func TestValidateTags_ValidInput(t *testing.T) {
	tests := []struct {
		name string
		tags []TagRequest
	}{
		{
			name: "empty slice",
			tags: []TagRequest{},
		},
		{
			name: "single valid tag",
			tags: []TagRequest{{Key: "env", Value: "production"}},
		},
		{
			name: "multiple valid tags",
			tags: []TagRequest{
				{Key: "env", Value: "production"},
				{Key: "team", Value: "platform"},
				{Key: "region", Value: "us-east-1"},
			},
		},
		{
			name: "key with hyphens and underscores",
			tags: []TagRequest{{Key: "my-tag_name", Value: "value"}},
		},
		{
			name: "key at max length (64 chars)",
			tags: []TagRequest{{Key: "a" + strings.Repeat("b", 63), Value: "ok"}},
		},
		{
			name: "value at max length (256 chars)",
			tags: []TagRequest{{Key: "key", Value: strings.Repeat("x", 256)}},
		},
		{
			name: "value with unicode",
			tags: []TagRequest{{Key: "team", Value: "données-français"}},
		},
		{
			name: "same key different values (not duplicates)",
			tags: []TagRequest{
				{Key: "env", Value: "prod"},
				{Key: "env", Value: "staging"},
			},
		},
		{
			name: "exactly 20 tags",
			tags: func() []TagRequest {
				tags := make([]TagRequest, 20)
				for i := range tags {
					tags[i] = TagRequest{Key: "key", Value: strings.Repeat("a", i+1)}
				}
				return tags
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTags(tc.tags)
			if err != nil {
				t.Errorf("expected nil error, got: %v", err)
			}
		})
	}
}

func TestValidateTags_InvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantMsg string
	}{
		{name: "uppercase", key: "Env", wantMsg: "does not match"},
		{name: "starts with number", key: "1env", wantMsg: "does not match"},
		{name: "starts with hyphen", key: "-env", wantMsg: "does not match"},
		{name: "starts with underscore", key: "_env", wantMsg: "does not match"},
		{name: "contains space", key: "my key", wantMsg: "does not match"},
		{name: "contains dot", key: "my.key", wantMsg: "does not match"},
		{name: "empty key", key: "", wantMsg: "does not match"},
		{name: "too long (65 chars)", key: "a" + strings.Repeat("b", 64), wantMsg: "does not match"},
		{name: "reserved prefix __ (starts with underscore fails regex)", key: "__internal", wantMsg: "does not match"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTags([]TagRequest{{Key: tc.key, Value: "valid"}})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestValidateTags_InvalidValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantMsg string
	}{
		{name: "empty value", value: "", wantMsg: "must not be empty"},
		{name: "too long (257 chars)", value: strings.Repeat("x", 257), wantMsg: "exceeds maximum"},
		{name: "contains null byte", value: "hello\x00world", wantMsg: "control character"},
		{name: "contains newline", value: "hello\nworld", wantMsg: "control character"},
		{name: "contains tab", value: "hello\tworld", wantMsg: "control character"},
		{name: "contains carriage return", value: "hello\rworld", wantMsg: "control character"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTags([]TagRequest{{Key: "valid", Value: tc.value}})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestValidateTags_TooManyTags(t *testing.T) {
	tags := make([]TagRequest, 21)
	for i := range tags {
		tags[i] = TagRequest{Key: "key", Value: strings.Repeat("v", i+1)}
	}

	err := ValidateTags(tags)
	if err == nil {
		t.Fatal("expected error for 21 tags, got nil")
	}
	if !strings.Contains(err.Error(), "too many tags") {
		t.Errorf("error %q does not mention 'too many tags'", err.Error())
	}
}

func TestValidateTags_DuplicatePairs(t *testing.T) {
	tags := []TagRequest{
		{Key: "env", Value: "prod"},
		{Key: "team", Value: "infra"},
		{Key: "env", Value: "prod"}, // duplicate
	}

	err := ValidateTags(tags)
	if err == nil {
		t.Fatal("expected error for duplicate pair, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error %q does not mention 'duplicate'", err.Error())
	}
}
