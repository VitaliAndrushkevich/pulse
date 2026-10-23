package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

type channelFakeDB struct {
	mu       sync.Mutex
	channels map[uuid.UUID]db.NotificationChannel
	writes   int
}

func TestNotificationChannelRetainsLegacyEmptyHeader(t *testing.T) {
	h, _, _ := channelTestHandler(t)
	previous := json.RawMessage(`{"url":"https://example.com","method":"POST","body_template":"ok","headers":[{"name":"Authorization","value":""}]}`)
	raw := json.RawMessage(`{"url":"https://example.com","method":"POST","body_template":"ok","headers":[{"name":"authorization"}]}`)
	stored, err := h.processConfigForStorage("webhook", raw, previous)
	if err != nil {
		t.Fatal(err)
	}
	var cfg webhookConfig
	if err := json.Unmarshal(stored, &cfg); err != nil {
		t.Fatal(err)
	}
	cipher, err := base64.StdEncoding.DecodeString(*cfg.Headers[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := crypto.Decrypt(h.secretKey, cipher)
	if err != nil || len(plain) != 0 {
		t.Fatalf("legacy empty header not encrypted: %v", err)
	}
}

func (f *channelFakeDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec")
}
func (f *channelFakeDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected Query")
}
func (f *channelFakeDB) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ch db.NotificationChannel
	switch {
	case strings.Contains(query, "name: CreateChannel "):
		ch = db.NotificationChannel{ID: uuid.New(), Name: args[0].(string), ChannelType: args[1].(string), Config: append(json.RawMessage(nil), args[2].(json.RawMessage)...), CreatedAt: time.Now(), UpdatedAt: time.Now()}
		f.channels[ch.ID] = ch
		f.writes++
	case strings.Contains(query, "name: GetChannel "):
		var ok bool
		ch, ok = f.channels[args[0].(uuid.UUID)]
		if !ok {
			return channelFakeRow{err: pgx.ErrNoRows}
		}
	case strings.Contains(query, "name: UpdateChannel "):
		var ok bool
		ch, ok = f.channels[args[0].(uuid.UUID)]
		if !ok {
			return channelFakeRow{err: pgx.ErrNoRows}
		}
		ch.Name = args[1].(string)
		ch.ChannelType = args[2].(string)
		ch.Config = append(json.RawMessage(nil), args[3].(json.RawMessage)...)
		f.channels[ch.ID] = ch
		f.writes++
	default:
		return channelFakeRow{err: fmt.Errorf("unexpected query")}
	}
	return channelFakeRow{channel: ch}
}

type channelFakeRow struct {
	channel db.NotificationChannel
	err     error
}

func (r channelFakeRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*uuid.UUID) = r.channel.ID
	*dest[1].(*string) = r.channel.Name
	*dest[2].(*string) = r.channel.ChannelType
	*dest[3].(*json.RawMessage) = r.channel.Config
	*dest[4].(*time.Time) = r.channel.CreatedAt
	*dest[5].(*time.Time) = r.channel.UpdatedAt
	return nil
}
func channelTestHandler(t *testing.T) (*NotificationChannelHandler, *channelFakeDB, *gin.Engine) {
	t.Helper()
	f := &channelFakeDB{channels: map[uuid.UUID]db.NotificationChannel{}}
	h := NewNotificationChannelHandler(db.New(f), bytes.Repeat([]byte{1}, 32), "https://pulse.example")
	router := gin.New()
	h.Register(router.Group("/api/v1"))
	return h, f, router
}
func channelTestRequest(t *testing.T, router http.Handler, method, path string, body interface{}, status int) []byte {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("%s %s: status %d want %d: %s", method, path, rec.Code, status, rec.Body.String())
	}
	return rec.Body.Bytes()
}
func channelTestWebhook(url string, value interface{}) map[string]interface{} {
	header := map[string]interface{}{"name": "Authorization"}
	if value != nil {
		header["value"] = value
	}
	return map[string]interface{}{"name": "Webhook", "type": "webhook", "config": map[string]interface{}{"url": url, "method": "POST", "body_template": `{"text":"Pulse test"}`, "headers": []interface{}{header}}}
}
func TestNotificationChannelAuthorizationSurvivesEdit(t *testing.T) {
	received := make(chan string, 8)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	_, f, router := channelTestHandler(t)
	raw := channelTestRequest(t, router, "POST", "/api/v1/notifications/channels", channelTestWebhook(receiver.URL, "Bearer original-token"), http.StatusCreated)
	var ch channelResponse
	if err := json.Unmarshal(raw, &ch); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/notifications/channels/" + ch.ID.String()
	for i := 0; i < 3; i++ {
		raw = channelTestRequest(t, router, "GET", path, nil, http.StatusOK)
		if strings.Contains(string(raw), "original-token") || strings.Contains(string(raw), "[REDACTED]") {
			t.Fatal("read exposed secret or submitted placeholder")
		}
		var edit map[string]interface{}
		if err := json.Unmarshal(raw, &edit); err != nil {
			t.Fatal(err)
		}
		edit["name"] = fmt.Sprintf("Renamed %d", i)
		headers := edit["config"].(map[string]interface{})["headers"].([]interface{})
		headers[0].(map[string]interface{})["name"] = "authorization"
		channelTestRequest(t, router, "PUT", path, edit, http.StatusOK)
		result := channelTestRequest(t, router, "POST", path+"/test", nil, http.StatusOK)
		if !strings.Contains(string(result), `"success":true`) {
			t.Fatalf("delivery failed: %s", result)
		}
		if actual := <-received; actual != "Bearer original-token" {
			t.Fatalf("Authorization changed: %q", actual)
		}
	}
	if f.writes != 4 {
		t.Fatalf("unexpected writes: %d", f.writes)
	}
}
func TestNotificationChannelDraftDoesNotPersist(t *testing.T) {
	received := make(chan string, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	_, f, router := channelTestHandler(t)
	// A new draft sends but creates no channel, delivery log, or retry query.
	result := channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", channelTestWebhook(receiver.URL, "Bearer draft"), 200)
	if !strings.Contains(string(result), `"success":true`) || strings.Contains(string(result), `"channel_id"`) {
		t.Fatalf("unexpected result %s", result)
	}
	if <-received != "Bearer draft" || f.writes != 0 || len(f.channels) != 0 {
		t.Fatal("draft mutated storage or sent wrong secret")
	}
	raw := channelTestRequest(t, router, "POST", "/api/v1/notifications/channels", channelTestWebhook(receiver.URL, "Bearer stored"), 201)
	var ch channelResponse
	if err := json.Unmarshal(raw, &ch); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), f.channels[ch.ID].Config...)
	draft := channelTestWebhook(receiver.URL, nil)
	draft["existing_channel_id"] = ch.ID.String()
	result = channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", draft, 200)
	if !strings.Contains(string(result), `"success":true`) || <-received != "Bearer stored" {
		t.Fatalf("draft did not resolve stored secret: %s", result)
	}
	if f.writes != 1 || !bytes.Equal(original, f.channels[ch.ID].Config) {
		t.Fatal("draft changed stored channel")
	}
	draft["config"].(map[string]interface{})["headers"].([]interface{})[0].(map[string]interface{})["value"] = "Bearer replacement"
	channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", draft, 200)
	if <-received != "Bearer replacement" || !bytes.Equal(original, f.channels[ch.ID].Config) {
		t.Fatal("draft replacement persisted or sent wrong secret")
	}
}
func TestNotificationChannelHeaderSecretSemantics(t *testing.T) {
	h, _, _ := channelTestHandler(t)
	first, _ := json.Marshal(channelTestWebhook("https://example.com", "Bearer token")["config"])
	stored, err := h.processConfigForStorage("webhook", first, nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		headers string
		wantErr bool
		want    string
		count   int
	}{
		{"preserve omitted", `[{"name":"authorization"}]`, false, "Bearer token", 1},
		{"legacy placeholder", `[{"name":"Authorization","value":"[REDACTED]"}]`, false, "Bearer token", 1},
		{"replace", `[{"name":"Authorization","value":"Bearer new"}]`, false, "Bearer new", 1},
		{"explicit empty", `[{"name":"Authorization","value":""}]`, false, "", 1},
		{"remove", `[]`, false, "", 0},
		{"new missing secret", `[{"name":"X-New"}]`, true, "", 0},
		{"renamed placeholder", `[{"name":"X-New","value":"[REDACTED]"}]`, true, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(`{"url":"https://example.com","method":"POST","body_template":"ok","headers":` + tt.headers + `}`)
			out, err := h.processConfigForStorage("webhook", raw, stored)
			if tt.wantErr {
				if err == nil {
					t.Fatal("wanted validation failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var cfg webhookConfig
			if err := json.Unmarshal(out, &cfg); err != nil {
				t.Fatal(err)
			}
			if len(cfg.Headers) != tt.count {
				t.Fatal("wrong header count")
			}
			if tt.count > 0 {
				var delivered string
				receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					delivered = r.Header.Get("Authorization")
					w.WriteHeader(204)
				}))
				defer receiver.Close()
				cfg.URL = receiver.URL
				out, _ = json.Marshal(cfg)
				err = h.testWebhookDelivery(t.Context(), db.NotificationChannel{Config: out}, notificationTestData())
				if err != nil || delivered != tt.want {
					t.Fatalf("wrong delivery value %q, err=%v", delivered, err)
				}
			}
		})
	}
}
func notificationTestData() notification.TemplateData { return notification.TemplateData{} }
func TestNotificationChannelValidationAndRedaction(t *testing.T) {
	h, _, router := channelTestHandler(t)
	for _, headers := range []string{`[{"name":"Authorization","value":"one"},{"name":"authorization","value":"two"}]`, `[{"name":"Bad Name","value":"one"}]`, `[{"name":"Authorization","value":"line\r\nbreak"}]`} {
		raw := json.RawMessage(`{"name":"test","type":"webhook","config":{"url":"https://example.com","method":"POST","body_template":"ok","headers":` + headers + `}}`)
		channelTestRequest(t, router, "POST", "/api/v1/notifications/channels", raw, 400)
	}
	if out := redactChannelConfig("webhook", json.RawMessage(`{"headers": "secret`)); string(out) != "{}" {
		t.Fatalf("redaction failed open: %s", out)
	}
	if errs := h.validateWebhookConfig(json.RawMessage(`{"url":"https://example.com","method":"POST","body_template":"ok","headers":[]}`)); len(errs) != 0 {
		t.Fatal(errs)
	}
}
func TestNotificationChannelTestLimit(t *testing.T) {
	h, _, router := channelTestHandler(t)
	for i := 0; i < cap(h.testSlots); i++ {
		h.testSlots <- struct{}{}
	}
	channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", channelTestWebhook("https://example.com", "token"), 429)
}

func TestNotificationChannelNativeSecretsSurviveEdit(t *testing.T) {
	tests := []struct{ name, kind, field, secret, config string }{
		{"Slack", "slack", "webhook_url", "https://hooks.slack.com/services/T/B/private-secret", `{}`},
		{"Telegram", "telegram", "bot_token", "123456:private-secret", `{"chat_id":"-100123","message_thread_id":42}`},
		{"Matrix", "matrix", "access_token", "private-secret", `{"homeserver_url":"https://matrix.example","room_id":"!room:example"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, f, router := channelTestHandler(t)
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(tt.config), &config); err != nil {
				t.Fatal(err)
			}
			config[tt.field] = tt.secret
			req := map[string]interface{}{"name": tt.name, "type": tt.kind, "config": config}
			raw := channelTestRequest(t, router, "POST", "/api/v1/notifications/channels", req, 201)
			if strings.Contains(string(raw), tt.secret) {
				t.Fatal("native secret exposed")
			}
			var ch channelResponse
			if err := json.Unmarshal(raw, &ch); err != nil {
				t.Fatal(err)
			}
			cipher := append([]byte(nil), f.channels[ch.ID].Config...)
			var edit map[string]interface{}
			if err := json.Unmarshal(raw, &edit); err != nil {
				t.Fatal(err)
			}
			edit["name"] = "Renamed"
			raw = channelTestRequest(t, router, "PUT", "/api/v1/notifications/channels/"+ch.ID.String(), edit, 200)
			if strings.Contains(string(raw), tt.secret) || !bytes.Equal(cipher, f.channels[ch.ID].Config) {
				t.Fatal("native edit exposed or replaced secret")
			}
			delete(config, tt.field)
			channelTestRequest(t, router, "POST", "/api/v1/notifications/channels", req, 400)
		})
	}
}

func TestNotificationChannelDraftValidation(t *testing.T) {
	_, _, router := channelTestHandler(t)
	req := channelTestWebhook("https://example.com", nil)
	channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", req, 400)
	req["existing_channel_id"] = "not-a-uuid"
	channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", req, 400)
	req["existing_channel_id"] = uuid.New().String()
	channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", req, 404)
}

func TestNotificationChannelDraftFailureReleasesSlot(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer receiver.Close()
	h, f, router := channelTestHandler(t)
	raw := channelTestRequest(t, router, "POST", "/api/v1/notifications/channels/test", channelTestWebhook(receiver.URL, "Bearer secret"), 200)
	var result testNotificationResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Error == "" || strings.Contains(result.Error, "secret") {
		t.Fatalf("unexpected failure result %s", raw)
	}
	if len(h.testSlots) != 0 || f.writes != 0 {
		t.Fatal("failed draft leaked slot or changed storage")
	}
}
