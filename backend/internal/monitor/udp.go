package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// UDPSettings holds optional configuration for the UDP checker.
type UDPSettings struct {
	// Payload is a base64-encoded payload to send (optional).
	// If empty, the checker only tests reachability by sending a zero-byte packet.
	Payload string `json:"payload,omitempty"`
	// ExpectedResponse is a base64-encoded expected response (optional).
	// If empty, any response (or successful send for reachability mode) means "up".
	ExpectedResponse string `json:"expected_response,omitempty"`
}

// UDPChecker implements the Checker interface for UDP monitors.
// In reachability mode (no payload/expected_response), it sends a zero-byte
// datagram and considers the target "up" if no ICMP unreachable is received
// within the timeout. When payload and expected_response are set, it validates
// the response content.
type UDPChecker struct{}

func (u *UDPChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	var s UDPSettings
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}

	// Resolve and dial UDP.
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", target)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("udp dial: %v", err)
		return result
	}
	defer conn.Close()

	// Determine payload bytes.
	var payload []byte
	if s.Payload != "" {
		payload, err = base64.StdEncoding.DecodeString(s.Payload)
		if err != nil {
			result.State = "down"
			result.Error = fmt.Sprintf("invalid payload encoding: %v", err)
			return result
		}
	} else {
		// Reachability mode: send a single zero-byte datagram.
		payload = []byte{}
	}

	// Set write/read deadline from context.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	start := time.Now()
	_, err = conn.Write(payload)
	if err != nil {
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		result.State = "down"
		result.Error = fmt.Sprintf("udp write: %v", err)
		return result
	}

	// If no expected response is set, just treat a successful write as "up"
	// (reachability mode). UDP is connectionless — we can't guarantee delivery,
	// but an ICMP port-unreachable would surface as a read error.
	if s.ExpectedResponse == "" && s.Payload == "" {
		// Attempt a brief read to detect ICMP unreachable.
		buf := make([]byte, 1)
		_, readErr := conn.Read(buf)
		latency := time.Since(start)
		result.LatencyMs = int32(latency.Milliseconds())

		if readErr != nil {
			// Timeout is expected for reachability — means no ICMP rejection.
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				result.State = "up"
				return result
			}
			// Non-timeout error (e.g., connection refused) means port is not reachable.
			result.State = "down"
			result.Error = fmt.Sprintf("udp read: %v", readErr)
			return result
		}
		// Got a response — definitely up.
		result.State = "up"
		return result
	}

	// Response validation mode: read and compare.
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			result.State = "down"
			result.Error = "udp response timeout"
			return result
		}
		result.State = "down"
		result.Error = fmt.Sprintf("udp read: %v", err)
		return result
	}

	// If we have an expected response, validate it.
	if s.ExpectedResponse != "" {
		expected, decErr := base64.StdEncoding.DecodeString(s.ExpectedResponse)
		if decErr != nil {
			result.State = "down"
			result.Error = fmt.Sprintf("invalid expected_response encoding: %v", decErr)
			return result
		}
		if string(buf[:n]) != string(expected) {
			result.State = "down"
			result.Error = "udp response mismatch"
			return result
		}
	}

	result.State = "up"
	return result
}
