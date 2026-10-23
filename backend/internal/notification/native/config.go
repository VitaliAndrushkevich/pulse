// Package native implements Pulse-formatted chat notification providers.
package native

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
)

type config struct {
	WebhookURL      string `json:"webhook_url,omitempty"`
	BotToken        string `json:"bot_token,omitempty"`
	ChatID          string `json:"chat_id,omitempty"`
	MessageThreadID *int64 `json:"message_thread_id,omitempty"`
	HomeserverURL   string `json:"homeserver_url,omitempty"`
	RoomID          string `json:"room_id,omitempty"`
	AccessToken     string `json:"access_token,omitempty"`
}

func IsType(kind string) bool { return kind == "slack" || kind == "telegram" || kind == "matrix" }

func secretField(kind string) string {
	switch kind {
	case "slack":
		return "webhook_url"
	case "telegram":
		return "bot_token"
	case "matrix":
		return "access_token"
	}
	return ""
}

func decode(raw json.RawMessage) (config, map[string]json.RawMessage, error) {
	var cfg config
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &cfg) != nil {
		return cfg, nil, fmt.Errorf("invalid channel configuration")
	}
	return cfg, fields, nil
}

// Validate checks public configuration and any supplied plaintext secret. An
// omitted secret is permitted here; Prepare requires a stored or new secret.
func Validate(kind string, raw json.RawMessage) error {
	if !IsType(kind) {
		return fmt.Errorf("unsupported notification provider")
	}
	cfg, fields, err := decode(raw)
	if err != nil {
		return err
	}
	if value, ok := fields[secretField(kind)]; ok {
		var secret string
		if json.Unmarshal(value, &secret) != nil || strings.TrimSpace(secret) == "" || string(value) == "null" || secret == "[REDACTED]" {
			return fmt.Errorf("%s must contain a new secret or be omitted", secretField(kind))
		}
		if len(secret) > 8192 || strings.ContainsAny(secret, "\r\n") {
			return fmt.Errorf("invalid %s", secretField(kind))
		}
	}
	switch kind {
	case "slack":
		if _, ok := fields["webhook_url"]; ok && !validSlackURL(cfg.WebhookURL) {
			return fmt.Errorf("webhook_url must be a Slack HTTPS incoming webhook URL")
		}
	case "telegram":
		if strings.TrimSpace(cfg.ChatID) == "" || len(cfg.ChatID) > 256 {
			return fmt.Errorf("chat_id is required")
		}
		if cfg.MessageThreadID != nil && *cfg.MessageThreadID <= 0 {
			return fmt.Errorf("message_thread_id must be positive")
		}
		if token, ok := fields["bot_token"]; ok {
			var value string
			_ = json.Unmarshal(token, &value)
			if !strings.Contains(value, ":") || strings.ContainsAny(value, "/?# \\:") && strings.Count(value, ":") != 1 || strings.ContainsAny(value, "/?# \\") {
				return fmt.Errorf("invalid bot_token")
			}
		}
	case "matrix":
		u, e := url.Parse(cfg.HomeserverURL)
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("homeserver_url must be an HTTP(S) URL without credentials, query, or fragment")
		}
		if !strings.HasPrefix(cfg.RoomID, "!") || !strings.Contains(cfg.RoomID, ":") || len(cfg.RoomID) > 512 {
			return fmt.Errorf("room_id must be a Matrix room ID")
		}
	}
	return nil
}

func validSlackURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Host != "hooks.slack.com" && u.Host != "hooks.slack-gov.com" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	return len(parts) == 4 && parts[0] == "services" && parts[1] != "" && parts[2] != "" && parts[3] != ""
}

// Prepare encrypts a replacement secret or preserves the existing ciphertext.
// The caller must only supply previous when editing the same provider type.
func Prepare(kind string, raw, previous json.RawMessage, key []byte) (json.RawMessage, error) {
	if err := Validate(kind, raw); err != nil {
		return nil, err
	}
	_, fields, _ := decode(raw)
	name := secretField(kind)
	secret, supplied := fields[name]
	if supplied {
		var plain string
		_ = json.Unmarshal(secret, &plain)
		encrypted, err := crypto.Encrypt(key, []byte(plain))
		if err != nil {
			return nil, fmt.Errorf("cannot encrypt channel secret")
		}
		fields[name], _ = json.Marshal(base64.StdEncoding.EncodeToString(encrypted))
	} else {
		_, old, err := decode(previous)
		if err != nil || len(old[name]) == 0 {
			return nil, fmt.Errorf("%s is required", name)
		}
		var cipher string
		if json.Unmarshal(old[name], &cipher) != nil || cipher == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
		fields[name] = old[name]
	}
	// Only persist recognized fields, never response metadata or arbitrary input.
	encoded, _ := json.Marshal(fields)
	var cfg config
	if json.Unmarshal(encoded, &cfg) != nil {
		return nil, fmt.Errorf("invalid channel configuration")
	}
	switch kind {
	case "slack":
		return json.Marshal(struct {
			URL string `json:"webhook_url"`
		}{cfg.WebhookURL})
	case "telegram":
		return json.Marshal(struct {
			Token  string `json:"bot_token"`
			Chat   string `json:"chat_id"`
			Thread *int64 `json:"message_thread_id,omitempty"`
		}{cfg.BotToken, cfg.ChatID, cfg.MessageThreadID})
	default:
		return json.Marshal(struct {
			URL   string `json:"homeserver_url"`
			Room  string `json:"room_id"`
			Token string `json:"access_token"`
		}{strings.TrimRight(cfg.HomeserverURL, "/"), cfg.RoomID, cfg.AccessToken})
	}
}

// Redact builds an allowlisted public projection and fails closed on corruption.
func Redact(kind string, raw json.RawMessage) json.RawMessage {
	cfg, _, err := decode(raw)
	if err != nil || !IsType(kind) {
		return json.RawMessage(`{}`)
	}
	public := map[string]any{}
	switch kind {
	case "slack":
		public["webhook_url_configured"] = cfg.WebhookURL != ""
	case "telegram":
		public["bot_token_configured"] = cfg.BotToken != ""
		public["chat_id"] = cfg.ChatID
		if cfg.MessageThreadID != nil {
			public["message_thread_id"] = *cfg.MessageThreadID
		}
	case "matrix":
		public["access_token_configured"] = cfg.AccessToken != ""
		public["homeserver_url"] = cfg.HomeserverURL
		public["room_id"] = cfg.RoomID
	}
	result, err := json.Marshal(public)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return result
}

func decrypt(value string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt channel secret")
	}
	plain, err := crypto.Decrypt(key, data)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt channel secret")
	}
	return string(plain), nil
}
