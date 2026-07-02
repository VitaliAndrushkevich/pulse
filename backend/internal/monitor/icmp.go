package monitor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
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

// --- Production Pinger (raw ICMP sockets, no exec) ---

var (
	prodPinger     Pinger
	prodPingerOnce sync.Once
)

func defaultPingerInstance() Pinger {
	prodPingerOnce.Do(func() {
		prodPinger = &rawPinger{}
	})
	return prodPinger
}

// rawPinger sends ICMP Echo Request packets using raw sockets.
// Requires CAP_NET_RAW on Linux or running as root, or uses
// unprivileged "udp" ICMP on supported kernels (Linux 3.0+, macOS).
type rawPinger struct{}

func (p *rawPinger) Ping(ctx context.Context, addr string, count int, useIPv6 bool) (int, int, time.Duration, error) {
	// Resolve address.
	resolved, err := resolveAddr(addr, useIPv6)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("resolve: %v", err)
	}

	// Determine network and ICMP type.
	network := "ip4:icmp"
	icmpType := uint8(8) // Echo Request for IPv4
	if useIPv6 {
		network = "ip6:ipv6-icmp"
		icmpType = 128 // Echo Request for IPv6
	}

	// Try privileged raw socket first, fall back to unprivileged UDP ICMP.
	conn, err := net.ListenPacket(network, "")
	if err != nil {
		// Fallback: unprivileged ICMP via UDP (Linux 3.0+ with net.ipv4.ping_group_range).
		udpNetwork := "udp4"
		if useIPv6 {
			udpNetwork = "udp6"
		}
		conn, err = net.ListenPacket(udpNetwork, "")
		if err != nil {
			return 0, 0, 0, fmt.Errorf("icmp listen: %v (try running with CAP_NET_RAW)", err)
		}
	}
	defer conn.Close()

	// Build destination address.
	var dst net.Addr
	if useIPv6 {
		dst = &net.IPAddr{IP: net.ParseIP(resolved)}
	} else {
		dst = &net.IPAddr{IP: net.ParseIP(resolved)}
	}
	// If conn is UDP-based, use UDPAddr instead.
	if _, ok := conn.(*net.UDPConn); ok {
		port := 0 // kernel assigns ICMP id from source port
		if useIPv6 {
			dst = &net.UDPAddr{IP: net.ParseIP(resolved), Port: port}
		} else {
			dst = &net.UDPAddr{IP: net.ParseIP(resolved), Port: port}
		}
	}

	id := uint16(rand.Intn(0xffff))
	var totalRTT time.Duration
	received := 0

	for seq := 0; seq < count; seq++ {
		// Check context before each packet.
		select {
		case <-ctx.Done():
			return count, received, avgDuration(totalRTT, received), ctx.Err()
		default:
		}

		msg := buildICMPEchoRequest(icmpType, id, uint16(seq), []byte("pulse-ping"))

		// Set deadline from context or 5s per packet.
		deadline := time.Now().Add(5 * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = conn.SetWriteDeadline(deadline)
		_ = conn.SetReadDeadline(deadline)

		sendTime := time.Now()
		_, err := conn.WriteTo(msg, dst)
		if err != nil {
			continue // count as lost
		}

		// Read reply.
		buf := make([]byte, 1500)
		for {
			// Keep reading until we get our echo reply or timeout.
			n, _, readErr := conn.ReadFrom(buf)
			if readErr != nil {
				break // timeout or error → packet lost
			}

			if matchesEchoReply(buf[:n], id, uint16(seq), useIPv6) {
				received++
				totalRTT += time.Since(sendTime)
				break
			}
			// Not our packet, keep reading until deadline.
		}
	}

	return count, received, avgDuration(totalRTT, received), nil
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

// buildICMPEchoRequest constructs an ICMP Echo Request packet with checksum.
func buildICMPEchoRequest(icmpType uint8, id, seq uint16, payload []byte) []byte {
	// ICMP header: Type(1) + Code(1) + Checksum(2) + ID(2) + Seq(2) + Payload
	msgLen := 8 + len(payload)
	msg := make([]byte, msgLen)

	msg[0] = icmpType // Type
	msg[1] = 0        // Code
	// Checksum filled after construction.
	binary.BigEndian.PutUint16(msg[4:6], id)
	binary.BigEndian.PutUint16(msg[6:8], seq)
	copy(msg[8:], payload)

	// Compute checksum.
	cs := icmpChecksum(msg)
	binary.BigEndian.PutUint16(msg[2:4], cs)

	return msg
}

// icmpChecksum computes the ICMP checksum per RFC 1071.
func icmpChecksum(data []byte) uint16 {
	var sum uint32
	length := len(data)

	for i := 0; i+1 < length; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if length%2 == 1 {
		sum += uint32(data[length-1]) << 8
	}

	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return ^uint16(sum)
}

// matchesEchoReply checks if the received packet is an ICMP Echo Reply matching our id/seq.
func matchesEchoReply(data []byte, id, seq uint16, useIPv6 bool) bool {
	// For raw IPv4 sockets, the kernel prepends the IP header (20 bytes typically).
	// For IPv6 and UDP ICMP sockets, no IP header is present.
	var icmpData []byte

	if !useIPv6 && len(data) >= 28 {
		// Check if this looks like it has an IPv4 header (version nibble = 4).
		if data[0]>>4 == 4 {
			hdrLen := int(data[0]&0x0f) * 4
			if len(data) < hdrLen+8 {
				return false
			}
			icmpData = data[hdrLen:]
		} else {
			icmpData = data
		}
	} else {
		icmpData = data
	}

	if len(icmpData) < 8 {
		return false
	}

	// Echo Reply type: 0 for IPv4, 129 for IPv6.
	expectedType := uint8(0)
	if useIPv6 {
		expectedType = 129
	}

	if icmpData[0] != expectedType {
		return false
	}

	replyID := binary.BigEndian.Uint16(icmpData[4:6])
	replySeq := binary.BigEndian.Uint16(icmpData[6:8])

	return replyID == id && replySeq == seq
}

func avgDuration(total time.Duration, count int) time.Duration {
	if count == 0 {
		return 0
	}
	return total / time.Duration(count)
}
