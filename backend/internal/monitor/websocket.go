package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
}

// WebSocketChecker implements the Checker interface for WebSocket monitors.
type WebSocketChecker struct{}

func (w *WebSocketChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	var s WebSocketSettings
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}

	// Build request headers.
	header := http.Header{}
	for key, value := range s.Headers {
		header.Set(key, value)
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: time.Until(deadlineFromContext(ctx)),
	}

	start := time.Now()
	conn, _, err := dialer.DialContext(ctx, target, header)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("websocket dial: %v", err)
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
