package notification

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
)

func TestRotateChannelConfig(t *testing.T) {
	t.Parallel()
	oldKey, newKey := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	cipher, err := crypto.Encrypt(oldKey, []byte("original-secret"))
	if err != nil {
		t.Fatal(err)
	}
	stored := base64.StdEncoding.EncodeToString(cipher)
	cases := []struct{ name, field string }{{"slack", "webhook_url"}, {"telegram", "bot_token"}, {"matrix", "access_token"}, {"webhook", ""}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := map[string]any{tc.field: stored, "chat_id": "-123"}
			if tc.name == "webhook" {
				input = map[string]any{"headers": []map[string]string{{"name": "Authorization", "value": stored}, {"name": "X-Empty", "value": ""}}}
			}
			raw, _ := json.Marshal(input)
			rotated, err := RotateChannelConfig(tc.name, raw, oldKey, newKey)
			if err != nil {
				t.Fatal(err)
			}
			var output map[string]json.RawMessage
			json.Unmarshal(rotated, &output)
			values := []string{}
			if tc.name == "webhook" {
				var headers []map[string]string
				json.Unmarshal(output["headers"], &headers)
				for _, h := range headers {
					values = append(values, h["value"])
				}
			} else {
				var value string
				json.Unmarshal(output[tc.field], &value)
				values = append(values, value)
				if string(output["chat_id"]) != `"-123"` {
					t.Fatal("lost public config")
				}
			}
			for i, value := range values {
				cipher, err := base64.StdEncoding.DecodeString(value)
				if err != nil {
					t.Fatal(err)
				}
				plain, err := crypto.Decrypt(newKey, cipher)
				if err != nil {
					t.Fatal(err)
				}
				want := "original-secret"
				if i == 1 {
					want = ""
				}
				if string(plain) != want {
					t.Fatal("changed secret")
				}
				if _, err := crypto.Decrypt(oldKey, cipher); err == nil {
					t.Fatal("old key still works")
				}
			}
		})
	}
}

func TestRotateChannelConfigFailsClosed(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{1}, 32)
	for _, tc := range []struct{ name, kind, raw string }{{"unknown", "future", `{}`}, {"malformed", "slack", `{`}, {"invalid cipher", "telegram", `{"bot_token":"bad"}`}, {"invalid headers", "webhook", `{"headers":42}`}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RotateChannelConfig(tc.kind, json.RawMessage(tc.raw), key, key); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := RotateChannelConfig("webhook", json.RawMessage(`{"url":"https://example.com"}`), key, key); err != nil {
		t.Fatal(err)
	}
}
