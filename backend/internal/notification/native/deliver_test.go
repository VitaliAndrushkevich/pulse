package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/google/uuid"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestDeliverProviderRequests(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	resourceID := uuid.New()
	cases := []struct{ name, raw string }{
		{"slack", `{"webhook_url":"https://hooks.slack.com/services/T/B/secret"}`},
		{"telegram", `{"bot_token":"123:secret","chat_id":"-100123","message_thread_id":7}`},
		{"matrix", `{"homeserver_url":"https://matrix.example","room_id":"!room:example","access_token":"secret"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key := bytes.Repeat([]byte{1}, 32)
			stored, err := Prepare(tc.name, json.RawMessage(tc.raw), nil, key)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if tc.name == "matrix" {
					if req.Header.Get("Authorization") != "Bearer secret" {
						t.Error("wrong Matrix auth")
					}
					if calls == 1 {
						if req.Method != "GET" || !strings.HasSuffix(req.URL.Path, "/state/m.room.encryption") {
							t.Error("missing encryption check")
						}
						return response(404, `{"errcode":"M_NOT_FOUND"}`), nil
					}
					if req.Method != "PUT" || !strings.HasSuffix(req.URL.Path, "/send/m.room.message/"+id.String()) {
						t.Error("wrong Matrix send URL")
					}
				} else if req.Method != "POST" {
					t.Error("wrong request method")
				}
				if req.Header.Get("Content-Type") != "application/json" {
					t.Error("wrong content type")
				}
				var body map[string]any
				if json.NewDecoder(req.Body).Decode(&body) != nil {
					t.Fatal("invalid JSON body")
				}
				switch tc.name {
				case "slack":
					if req.URL.Host != "hooks.slack.com" || body["blocks"] == nil {
						t.Error("wrong Slack request")
					}
					return response(200, "ok"), nil
				case "telegram":
					if req.URL.Host != "api.telegram.org" || req.URL.Path != "/bot123:secret/sendMessage" || body["chat_id"] != "-100123" || body["message_thread_id"] != float64(7) {
						t.Error("wrong Telegram request")
					}
					return response(200, `{"ok":true}`), nil
				default:
					if body["msgtype"] != "m.notice" {
						t.Error("wrong Matrix message type")
					}
					return response(200, `{"event_id":"$event"}`), nil
				}
			})}
			data := notification.TemplateData{ResourceKind: "heartbeat", TriggerType: "heartbeat_missed", Monitor: notification.MonitorData{ID: resourceID, Name: "A < & \" B"}, BaseURL: "https://pulse.example", Timestamp: time.Now(), Status: "down"}
			if err := deliverWithClient(t.Context(), client, tc.name, stored, key, data, id); err != nil {
				t.Fatal(err)
			}
			want := 1
			if tc.name == "matrix" {
				want = 2
			}
			if calls != want {
				t.Fatalf("got %d requests", calls)
			}
		})
	}
}

func TestDeliverFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		status         int
		body           string
		transportError bool
		retryable      bool
	}{
		{"auth", 401, "secret", false, false}, {"forbidden", 403, "secret", false, false}, {"server", 503, "secret", false, true},
		{"rate limit", 429, `{"parameters":{"retry_after":42}}`, false, true},
		{"invalid success", 200, "not JSON", false, true}, {"provider auth", 200, `{"ok":false,"error_code":401}`, false, false},
		{"provider rate", 200, `{"ok":false,"error_code":429,"parameters":{"retry_after":42}}`, false, true},
		{"transport", 0, "", true, true}, {"oversized", 200, strings.Repeat("x", maxResponseBytes+1), false, false},
		{"redirect", 302, "secret", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key := bytes.Repeat([]byte{1}, 32)
			stored, err := Prepare("telegram", json.RawMessage(`{"bot_token":"123:secret","chat_id":"1"}`), nil, key)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.transportError {
					return nil, fmt.Errorf("secret URL /123:secret")
				}
				return response(tc.status, tc.body), nil
			})}
			err = deliverWithClient(t.Context(), client, "telegram", stored, key, notification.TemplateData{}, uuid.New())
			if err == nil {
				t.Fatal("expected failure")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("leaked secret")
			}
			if notification.IsRetryable(err) != tc.retryable {
				t.Fatalf("retryable=%v err=%v", notification.IsRetryable(err), err)
			}
			if tc.name == "rate limit" || tc.name == "provider rate" {
				var de *notification.DeliveryError
				if !errors.As(err, &de) || de.RetryAfter != 42*time.Second {
					t.Fatal("lost retry hint")
				}
			}
		})
	}
}

func TestMatrixRejectsEncryptedAndUnknownRooms(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 401, 403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			key := bytes.Repeat([]byte{1}, 32)
			stored, _ := Prepare("matrix", json.RawMessage(`{"homeserver_url":"https://matrix.example","room_id":"!room:example","access_token":"secret"}`), nil, key)
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return response(status, `{"error":"secret"}`), nil
			})}
			if deliverWithClient(t.Context(), client, "matrix", stored, key, notification.TemplateData{}, uuid.New()) == nil || calls != 1 {
				t.Fatal("sent to encrypted or unverified room")
			}
		})
	}
}

func TestMessageResourceAndBounds(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	data := notification.TemplateData{ResourceKind: "heartbeat", Monitor: notification.MonitorData{ID: id, Name: strings.Repeat("界", 500)}, BaseURL: "https://pulse.example", Status: "down"}
	text := message(data)
	if !strings.Contains(text, "/heartbeats/"+id.String()) || strings.Contains(text, "Response:") || len([]rune(text)) > 2800 {
		t.Fatal("wrong heartbeat message")
	}
	data.ResourceKind = "monitor"
	if !strings.Contains(message(data), "/monitors/"+id.String()) {
		t.Fatal("wrong monitor URL")
	}
	data.BaseURL = "https://secret@pulse.example"
	if strings.Contains(message(data), "View in Pulse") {
		t.Fatal("included credential URL")
	}
}

func TestRequestCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, req.Context().Err() })}
	if _, _, err := request(ctx, client, "POST", "https://example.com", "", nil); err == nil {
		t.Fatal("expected cancellation")
	}
}
