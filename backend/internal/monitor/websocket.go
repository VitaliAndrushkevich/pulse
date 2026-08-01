package monitor

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketSettings holds optional configuration for the WebSocket checker.
type WebSocketSettings struct {
	// Headers are additional request headers for the upgrade handshake.
	Headers map[string]string `json:"headers,omitempty"`
	// HandshakeMessage is an optional message to send after connecting.
	HandshakeMessage string `json:"handshake_message,omitempty"`
	// ExpectedResponse is the expected reply after sending the handshake message.
	ExpectedResponse string `json:"expected_response,omitempty"`
	// SkipTLSVerify disables certificate chain and hostname verification (default: false).
	SkipTLSVerify bool `json:"skip_tls_verify,omitempty"`
	// ExpectedStatuses is an optional list of HTTP status codes considered acceptable.
	// When set, a failed WebSocket upgrade that returns one of these codes is treated
	// as "up" (the server responded as expected). Useful for monitoring endpoints
	// behind authentication where 401/403 indicates the server is alive.
	// Each value must be in 100–599; max 10 entries.
	ExpectedStatuses []int `json:"expected_statuses,omitempty"`
}

// WebSocketChecker implements the Checker and AuthenticatedChecker interfaces
// for WebSocket monitors.
type WebSocketChecker struct{}

func (w *WebSocketChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	return w.check(ctx, target, settings, nil)
}

func (w *WebSocketChecker) CheckWithAuth(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	return w.check(ctx, target, settings, creds)
}

// check is the shared implementation used by both Check and CheckWithAuth.
func (w *WebSocketChecker) check(ctx context.Context, target string, settings json.RawMessage, creds []AuthCredential) Result {
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	var s WebSocketSettings
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}

	// Validate expected_statuses early.
	if err := validateWSExpectedStatuses(s.ExpectedStatuses); err != nil {
		result.State = "down"
		result.Error = err.Error()
		return result
	}

	// Build request headers.
	header := http.Header{}
	for key, value := range s.Headers {
		header.Set(key, value)
	}

	// Auto-set Origin header if not explicitly provided (browsers always send it).
	if header.Get("Origin") == "" {
		if origin := websocketOrigin(target); origin != "" {
			header.Set("Origin", origin)
		}
	}

	// Inject auth credential headers into the upgrade request.
	for _, cred := range creds {
		switch cred.AuthType {
		case "bearer":
			header.Set("Authorization", "Bearer "+cred.Token)
		case "basic":
			encoded := base64.StdEncoding.EncodeToString(
				[]byte(cred.Username + ":" + cred.Password),
			)
			header.Set("Authorization", "Basic "+encoded)
		case "header":
			header.Set(cred.HeaderName, cred.HeaderValue)
		}
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: time.Until(deadlineFromContext(ctx)),
	}

	if s.SkipTLSVerify {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	start := time.Now()
	conn, resp, err := dialer.DialContext(ctx, target, header)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	if err != nil {
		// If expected_statuses is set and the server responded with a matching HTTP code,
		// consider the monitor "up" — the server is alive and responding as configured.
		if resp != nil && len(s.ExpectedStatuses) > 0 {
			if wsStatusExpected(resp.StatusCode, s.ExpectedStatuses) {
				result.State = "up"
				return result
			}
		}
		result.State = "down"
		if resp != nil {
			result.Error = fmt.Sprintf("websocket dial: %v (HTTP %d)", err, resp.StatusCode)
		} else {
			result.Error = fmt.Sprintf("websocket dial: %v", err)
		}
		return result
	}
	defer conn.Close()

	// If no handshake message is specified, connection success = up.
	if s.HandshakeMessage == "" {
		result.State = "up"
		return result
	}

	// Send handshake message.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(s.HandshakeMessage)); err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("websocket write: %v", err)
		return result
	}

	// If no expected response, write success = up.
	if s.ExpectedResponse == "" {
		result.State = "up"
		return result
	}

	// Read and validate response.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("websocket read: %v", err)
		return result
	}

	if string(msg) != s.ExpectedResponse {
		result.State = "down"
		result.Error = fmt.Sprintf("websocket response mismatch: got %q, want %q",
			string(msg), s.ExpectedResponse)
		return result
	}

	result.State = "up"
	return result
}

// deadlineFromContext extracts the deadline from context, or returns a far-future time.
func deadlineFromContext(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now().Add(30 * time.Second)
}

// websocketOrigin derives an Origin header value from a ws:// or wss:// target URL.
// Browsers always send Origin on WebSocket upgrades; many servers require it.
func websocketOrigin(target string) string {
	switch {
	case len(target) > 6 && strings.EqualFold(target[:6], "wss://"):
		return "https://" + strings.SplitN(target[6:], "/", 2)[0]
	case len(target) > 5 && strings.EqualFold(target[:5], "ws://"):
		return "http://" + strings.SplitN(target[5:], "/", 2)[0]
	default:
		return ""
	}
}

// validateWSExpectedStatuses checks that expected status codes are valid HTTP codes.
// Each value must be in [100, 599], max 10 entries.
func validateWSExpectedStatuses(statuses []int) error {
	if len(statuses) == 0 {
		return nil
	}
	if len(statuses) > 10 {
		return fmt.Errorf("expected_statuses must have at most 10 entries, got %d", len(statuses))
	}
	for _, code := range statuses {
		if code < 100 || code > 599 {
			return fmt.Errorf("expected_statuses values must be between 100 and 599, got %d", code)
		}
	}
	return nil
}

// wsStatusExpected returns true if the given HTTP status code is in the expected list.
func wsStatusExpected(code int, expected []int) bool {
	for _, e := range expected {
		if code == e {
			return true
		}
	}
	return false
}
