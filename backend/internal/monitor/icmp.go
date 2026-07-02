package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Pinger abstracts ICMP ping operations for testability.
type Pinger interface {
	Ping(ctx context.Context, addr string, count int, useIPv6 bool) (sent, received int, avgRTT time.Duration, err error)
}

// ICMPSettings holds configuration for the ICMP checker.
type ICMPSettings struct {
	PacketCount          int  `json:"packet_count"`           // default: 3, min: 1, max: 10
	LossThresholdPercent int  `json:"loss_threshold_percent"` // default: 100
	UseIPv6              bool `json:"use_ipv6"`               // default: false
}

// ICMPChecker implements the Checker interface for ICMP monitors.
type ICMPChecker struct {
	pinger Pinger
}

// Check executes an ICMP ping check against the given target.
func (c *ICMPChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result {
	start := time.Now()
	result := Result{
		CheckedAt: time.Now().UTC(),
	}

	s := parseICMPSettings(settings)

	// Get or create pinger.
	pinger := c.pinger
	if pinger == nil {
		pinger = defaultPingerInstance()
	}

	// Execute ping.
	sent, received, avgRTT, err := pinger.Ping(ctx, target, s.PacketCount, s.UseIPv6)
	if err != nil {
		result.State = "down"
		result.Error = fmt.Sprintf("icmp: %v", err)
		result.LatencyMs = int32(time.Since(start).Milliseconds())
		return result
	}

	// Calculate latency.
	result.LatencyMs = int32(avgRTT.Milliseconds())
	if result.LatencyMs == 0 {
		result.LatencyMs = int32(time.Since(start).Milliseconds())
	}

	// Handle total loss.
	if received == 0 {
		result.State = "down"
		result.Error = "icmp: 100% packet loss"
		return result
	}

	// Calculate loss percentage.
	lossPct := 0
	if sent > 0 {
		lossPct = (sent - received) * 100 / sent
	}

	// Compare against threshold.
	if lossPct >= s.LossThresholdPercent {
		result.State = "down"
		result.Error = fmt.Sprintf("icmp: packet loss %d%% exceeds threshold %d%%", lossPct, s.LossThresholdPercent)
		return result
	}

	result.State = "up"
	return result
}

// parseICMPSettings unmarshals settings JSON and applies defaults/clamping.
func parseICMPSettings(settings json.RawMessage) ICMPSettings {
	s := ICMPSettings{}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &s)
	}

	// Apply defaults.
	if s.PacketCount < 1 {
		s.PacketCount = 3
	}
	if s.PacketCount > 10 {
		s.PacketCount = 10
	}
	if s.LossThresholdPercent <= 0 {
		s.LossThresholdPercent = 100
	}
	if s.LossThresholdPercent > 100 {
		s.LossThresholdPercent = 100
	}

	return s
}

// --- Production Pinger ---

var (
	prodPinger     Pinger
	prodPingerOnce sync.Once
)

func defaultPingerInstance() Pinger {
	prodPingerOnce.Do(func() {
		prodPinger = &systemPinger{}
	})
	return prodPinger
}

// systemPinger uses the system ping command as a fallback when raw ICMP
// is not available (no CAP_NET_RAW).
type systemPinger struct{}

func (p *systemPinger) Ping(ctx context.Context, addr string, count int, useIPv6 bool) (int, int, time.Duration, error) {
	// Resolve address first.
	resolved, err := resolveAddr(addr, useIPv6)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("resolve: %v", err)
	}

	// Use system ping command.
	cmd := buildPingCommand(ctx, resolved, count, useIPv6)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse output anyway — partial results may be available.
		sent, received, avgRTT := parsePingOutput(string(output))
		if sent > 0 {
			return sent, received, avgRTT, nil
		}
		return count, 0, 0, fmt.Errorf("ping failed: %v", err)
	}

	sent, received, avgRTT := parsePingOutput(string(output))
	return sent, received, avgRTT, nil
}

func resolveAddr(addr string, useIPv6 bool) (string, error) {
	// If it's already an IP, return as-is.
	if ip := net.ParseIP(addr); ip != nil {
		return addr, nil
	}

	// Resolve hostname.
	network := "ip4"
	if useIPv6 {
		network = "ip6"
	}
	ips, err := net.DefaultResolver.LookupIP(context.Background(), network, addr)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no %s address found for %s", network, addr)
	}
	return ips[0].String(), nil
}

func buildPingCommand(ctx context.Context, addr string, count int, useIPv6 bool) *exec.Cmd {
	countStr := strconv.Itoa(count)

	switch runtime.GOOS {
	case "windows":
		if useIPv6 {
			return exec.CommandContext(ctx, "ping", "-6", "-n", countStr, addr)
		}
		return exec.CommandContext(ctx, "ping", "-n", countStr, addr)
	default: // linux, darwin
		pingCmd := "ping"
		if useIPv6 {
			pingCmd = "ping6"
		}
		return exec.CommandContext(ctx, pingCmd, "-c", countStr, "-W", "5", addr)
	}
}

func parsePingOutput(output string) (sent, received int, avgRTT time.Duration) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse packet stats line: "3 packets transmitted, 3 received, 0% packet loss"
		if strings.Contains(line, "packets transmitted") {
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.Contains(part, "transmitted") {
					fmt.Sscanf(part, "%d", &sent)
				} else if strings.Contains(part, "received") {
					fmt.Sscanf(part, "%d", &received)
				}
			}
		}

		// Parse RTT line: "rtt min/avg/max/mdev = 1.234/5.678/9.012/1.234 ms"
		// or "round-trip min/avg/max/stddev = ..." (macOS)
		if strings.Contains(line, "min/avg/max") {
			eqIdx := strings.Index(line, "=")
			if eqIdx > 0 {
				stats := strings.TrimSpace(line[eqIdx+1:])
				// Remove "ms" suffix.
				stats = strings.TrimSuffix(stats, " ms")
				parts := strings.Split(stats, "/")
				if len(parts) >= 2 {
					if avgMs, err := strconv.ParseFloat(parts[1], 64); err == nil {
						avgRTT = time.Duration(avgMs * float64(time.Millisecond))
					}
				}
			}
		}
	}
	return
}
