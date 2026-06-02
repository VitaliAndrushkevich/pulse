package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// TCPChecker implements the Checker interface for TCP monitors.
// It dials the target host:port and measures connection latency.
type TCPChecker struct{}

func (t *TCPChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	// Use a net.Dialer that respects context for timeout and cancellation.
	dialer := &net.Dialer{}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	latency := time.Since(start)
	result.LatencyMs = int32(latency.Milliseconds())

	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("tcp dial: %v", err)
		return result
	}
	defer conn.Close()

	result.State = "up"
	return result
}
