package native

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareAndRedact(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, raw, edit, field, secret string }{
		{"slack", `{"webhook_url":"https://hooks.slack.com/services/T/B/secret"}`, `{}`, "webhook_url", "https://hooks.slack.com/services/T/B/secret"},
		{"telegram", `{"bot_token":"123:secret","chat_id":"-100123"}`, `{"chat_id":"-100456"}`, "bot_token", "123:secret"},
		{"matrix", `{"homeserver_url":"https://matrix.example","room_id":"!room:example","access_token":"secret"}`, `{"homeserver_url":"https://matrix.example","room_id":"!room:example"}`, "access_token", "secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key := bytes.Repeat([]byte{1}, 32)
			stored, err := Prepare(tc.name, json.RawMessage(tc.raw), nil, key)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(stored), tc.secret) {
				t.Fatal("stored plaintext secret")
			}
			var config map[string]string
			// Decode only the secret since non-secret fields can have non-string values.
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(stored, &fields)
			var encrypted string
			_ = json.Unmarshal(fields[tc.field], &encrypted)
			plain, err := decrypt(encrypted, key)
			if err != nil || plain != tc.secret {
				t.Fatalf("decrypt failed: %v", err)
			}
			edited, err := Prepare(tc.name, json.RawMessage(tc.edit), stored, key)
			if err != nil {
				t.Fatal(err)
			}
			var editedFields map[string]json.RawMessage
			_ = json.Unmarshal(edited, &editedFields)
			if !bytes.Equal(fields[tc.field], editedFields[tc.field]) {
				t.Fatal("edit changed existing secret")
			}
			redacted := Redact(tc.name, stored)
			if strings.Contains(string(redacted), encrypted) || strings.Contains(string(redacted), tc.secret) {
				t.Fatal("response exposed secret")
			}
			var public map[string]any
			_ = json.Unmarshal(redacted, &public)
			if public[tc.field+"_configured"] != true {
				t.Fatal("missing configured flag")
			}
			if _, exists := public[tc.field]; exists {
				t.Fatal("returned secret field")
			}
			if _, err := Prepare(tc.name, json.RawMessage(tc.edit), nil, key); err == nil {
				t.Fatal("create accepted missing secret")
			}
			_ = config
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, kind, raw string }{
		{"slack wrong host", "slack", `{"webhook_url":"https://evil.example/services/T/B/S"}`},
		{"slack HTTP", "slack", `{"webhook_url":"http://hooks.slack.com/services/T/B/S"}`},
		{"slack userinfo", "slack", `{"webhook_url":"https://token@hooks.slack.com/services/T/B/S"}`},
		{"slack wrong path", "slack", `{"webhook_url":"https://hooks.slack.com/other/T/B/S"}`},
		{"telegram bad token", "telegram", `{"bot_token":"123:secret/evil","chat_id":"1"}`},
		{"telegram missing chat", "telegram", `{"bot_token":"123:secret"}`},
		{"telegram invalid thread", "telegram", `{"bot_token":"123:secret","chat_id":"1","message_thread_id":-1}`},
		{"matrix userinfo", "matrix", `{"homeserver_url":"https://secret@matrix.example","room_id":"!room:example"}`},
		{"matrix wrong scheme", "matrix", `{"homeserver_url":"file:///tmp/file","room_id":"!room:example"}`},
		{"matrix missing room", "matrix", `{"homeserver_url":"https://matrix.example"}`},
		{"null", "slack", `null`},
		{"secret null", "slack", `{"webhook_url":null}`},
		{"redacted marker", "slack", `{"webhook_url":"[REDACTED]"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if Validate(tc.kind, json.RawMessage(tc.raw)) == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}

func TestRedactFailsClosed(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"webhook_url":"secret",`, `null`, `[]`, `{"webhook_url":12}`} {
		if string(Redact("slack", json.RawMessage(raw))) != "{}" {
			t.Fatal("corrupt config returned")
		}
	}
}
