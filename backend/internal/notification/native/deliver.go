package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/google/uuid"
)

const maxResponseBytes = 65536

// Deliver sends one notification. Secrets and remote response bodies are never
// included in returned errors, including transport errors containing URLs.
func Deliver(ctx context.Context, kind string, raw json.RawMessage, key []byte, data notification.TemplateData, deliveryID uuid.UUID) error {
	return deliverWithClient(ctx, &http.Client{Timeout: 10 * time.Second}, kind, raw, key, data, deliveryID)
}

func deliverWithClient(ctx context.Context, client *http.Client, kind string, raw json.RawMessage, key []byte, data notification.TemplateData, deliveryID uuid.UUID) error {
	cfg, _, err := decode(raw)
	if err != nil || !IsType(kind) {
		return notification.NewNonRetryableError(fmt.Errorf("invalid native notification configuration"))
	}
	storedSecret := cfg.WebhookURL
	if kind == "telegram" {
		storedSecret = cfg.BotToken
	}
	if kind == "matrix" {
		storedSecret = cfg.AccessToken
	}
	secret, err := decrypt(storedSecret, key)
	if err != nil {
		return notification.NewNonRetryableError(err)
	}
	fields := map[string]any{}
	switch kind {
	case "slack":
		fields["webhook_url"] = secret
	case "telegram":
		fields["bot_token"] = secret
		fields["chat_id"] = cfg.ChatID
		fields["message_thread_id"] = cfg.MessageThreadID
	case "matrix":
		fields["homeserver_url"] = cfg.HomeserverURL
		fields["room_id"] = cfg.RoomID
		fields["access_token"] = secret
	}
	check, _ := json.Marshal(fields)
	if err := Validate(kind, check); err != nil {
		return notification.NewNonRetryableError(fmt.Errorf("invalid %s configuration", kind))
	}
	// Clone rather than mutate a shared injected client. Never follow redirects:
	// provider credentials must not travel to another URL, even on the same host.
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if safeClient.Timeout <= 0 {
		safeClient.Timeout = 10 * time.Second
	}
	text := message(data)
	var endpoint, auth string
	var payload any
	method := http.MethodPost
	switch kind {
	case "slack":
		endpoint = secret
		payload = map[string]any{"text": text, "blocks": []any{map[string]any{"type": "header", "text": map[string]string{"type": "plain_text", "text": "Pulse notification"}}, map[string]any{"type": "section", "text": map[string]string{"type": "plain_text", "text": text}}}}
	case "telegram":
		endpoint = "https://api.telegram.org/bot" + secret + "/sendMessage"
		p := map[string]any{"chat_id": cfg.ChatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
		if cfg.MessageThreadID != nil {
			p["message_thread_id"] = *cfg.MessageThreadID
		}
		payload = p
	case "matrix":
		auth = "Bearer " + secret
		room := strings.TrimRight(cfg.HomeserverURL, "/") + "/_matrix/client/v3/rooms/" + url.PathEscape(cfg.RoomID)
		// A plaintext-only sender must fail closed for encrypted or unreadable rooms.
		body, status, err := request(ctx, &safeClient, http.MethodGet, room+"/state/m.room.encryption", auth, nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK {
			return notification.NewNonRetryableError(fmt.Errorf("Matrix encrypted rooms are not supported"))
		}
		if status != http.StatusNotFound {
			return statusError("matrix", status)
		}
		var state struct {
			Code string `json:"errcode"`
		}
		if json.Unmarshal(body, &state) != nil || state.Code != "M_NOT_FOUND" {
			return notification.NewNonRetryableError(fmt.Errorf("cannot verify Matrix room encryption state"))
		}
		if deliveryID == uuid.Nil {
			deliveryID = uuid.New()
		}
		endpoint = room + "/send/m.room.message/" + deliveryID.String()
		method = http.MethodPut
		payload = map[string]string{"msgtype": "m.notice", "body": text}
	}
	body, status, err := request(ctx, &safeClient, method, endpoint, auth, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return statusError(kind, status)
	}
	switch kind {
	case "slack":
		if strings.TrimSpace(string(body)) != "ok" {
			return notification.NewNonRetryableError(fmt.Errorf("Slack rejected the notification"))
		}
	case "telegram":
		var response struct {
			OK   bool `json:"ok"`
			Code int  `json:"error_code"`
		}
		if json.Unmarshal(body, &response) != nil {
			return notification.NewRetryableError(fmt.Errorf("invalid Telegram response"))
		}
		if !response.OK {
			if response.Code == 429 {
				return rateLimit(body, "")
			}
			return statusError(kind, response.Code)
		}
	case "matrix":
		var response struct {
			ID string `json:"event_id"`
		}
		if json.Unmarshal(body, &response) != nil || response.ID == "" {
			return notification.NewRetryableError(fmt.Errorf("invalid Matrix response"))
		}
	}
	return nil
}

func request(ctx context.Context, client *http.Client, method, endpoint, auth string, payload any) ([]byte, int, error) {
	var encoded []byte
	if payload != nil {
		var err error
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, 0, notification.NewNonRetryableError(fmt.Errorf("cannot encode notification"))
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, notification.NewNonRetryableError(fmt.Errorf("invalid notification URL"))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, notification.NewRetryableError(fmt.Errorf("notification request failed"))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, notification.NewRetryableError(fmt.Errorf("cannot read notification response"))
	}
	if len(body) > maxResponseBytes {
		return nil, 0, notification.NewNonRetryableError(fmt.Errorf("notification response exceeds size limit"))
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, resp.StatusCode, rateLimit(body, resp.Header.Get("Retry-After"))
	}
	return body, resp.StatusCode, nil
}

func rateLimit(body []byte, header string) error {
	var response struct {
		RetryAfterMS int64 `json:"retry_after_ms"`
		Parameters   struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(body, &response)
	delay := time.Duration(0)
	// Bound untrusted server delays before converting to avoid duration overflow.
	if response.RetryAfterMS > 0 {
		delay = time.Duration(min(response.RetryAfterMS, int64(86400000))) * time.Millisecond
	}
	if response.Parameters.RetryAfter > 0 {
		delay = max(delay, time.Duration(min(response.Parameters.RetryAfter, int64(86400)))*time.Second)
	}
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds > 0 {
		delay = max(delay, time.Duration(min(seconds, int64(86400)))*time.Second)
	} else if at, err := http.ParseTime(header); err == nil {
		delay = max(delay, min(time.Until(at), 24*time.Hour))
	}
	return notification.NewRateLimitError(fmt.Errorf("notification provider rate limit exceeded"), delay)
}

func statusError(kind string, status int) error {
	err := fmt.Errorf("%s notification failed (HTTP %d)", kind, status)
	if status == http.StatusTooManyRequests || status >= 500 {
		return notification.NewRetryableError(err)
	}
	return notification.NewNonRetryableError(err)
}

func message(data notification.TemplateData) string {
	kind := "Monitor"
	path := "monitors"
	if data.ResourceKind == "heartbeat" {
		kind = "Heartbeat"
		path = "heartbeats"
	}
	result := fmt.Sprintf("Pulse · %s %s\n%s", kind, data.Status, truncate(data.Monitor.Name, 256))
	if data.TriggerType != "" {
		result += "\nAlert: " + truncate(data.TriggerType, 100)
	}
	if data.Monitor.Target != "" {
		result += "\nTarget: " + truncate(data.Monitor.Target, 512)
	}
	if kind == "Monitor" {
		result += fmt.Sprintf("\nResponse: %d ms", data.ResponseTime)
	}
	if data.Incident.Duration > 0 {
		result += "\nIncident duration: " + data.Incident.Duration.Round(time.Second).String()
	}
	if !data.Timestamp.IsZero() {
		result += "\nTime: " + data.Timestamp.UTC().Format(time.RFC3339)
	}
	base, err := url.Parse(data.BaseURL)
	if err == nil && base.Host != "" && base.User == nil && (base.Scheme == "http" || base.Scheme == "https") && data.Monitor.ID != uuid.Nil {
		result += "\nView in Pulse: " + strings.TrimRight(data.BaseURL, "/") + "/" + path + "/" + data.Monitor.ID.String()
	}
	return truncate(result, 2800)
}

func truncate(value string, max int) string {
	r := []rune(value)
	if len(r) <= max {
		return value
	}
	return string(r[:max-1]) + "…"
}
