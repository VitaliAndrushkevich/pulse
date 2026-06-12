package target

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name        string
		monitorType string
		target      string
		want        string
		wantErr     bool
	}{
		// HTTP
		{name: "http bare domain", monitorType: "http", target: "google.com", want: "https://google.com"},
		{name: "http bare domain with path", monitorType: "http", target: "example.com/health", want: "https://example.com/health"},
		{name: "http with port", monitorType: "http", target: "example.com:8080", want: "https://example.com:8080"},
		{name: "http explicit https", monitorType: "http", target: "https://example.com", want: "https://example.com"},
		{name: "http explicit http", monitorType: "http", target: "http://example.com", want: "http://example.com"},
		{name: "http explicit HTTP uppercase", monitorType: "http", target: "HTTP://example.com", want: "HTTP://example.com"},
		{name: "http bad scheme", monitorType: "http", target: "ftp://example.com", wantErr: true},
		{name: "http empty", monitorType: "http", target: "", wantErr: true},
		{name: "http whitespace trimmed", monitorType: "http", target: "  google.com  ", want: "https://google.com"},

		// HTTP/3
		{name: "http3 bare domain", monitorType: "http3", target: "example.com", want: "https://example.com"},
		{name: "http3 explicit https", monitorType: "http3", target: "https://example.com", want: "https://example.com"},

		// TCP
		{name: "tcp valid host:port", monitorType: "tcp", target: "db.example.com:5432", want: "db.example.com:5432"},
		{name: "tcp ipv4:port", monitorType: "tcp", target: "192.168.1.1:80", want: "192.168.1.1:80"},
		{name: "tcp ipv6:port", monitorType: "tcp", target: "[::1]:443", want: "[::1]:443"},
		{name: "tcp missing port", monitorType: "tcp", target: "example.com", wantErr: true},
		{name: "tcp with scheme", monitorType: "tcp", target: "tcp://example.com:80", wantErr: true},
		{name: "tcp with https scheme", monitorType: "tcp", target: "https://example.com:443", wantErr: true},

		// UDP
		{name: "udp valid host:port", monitorType: "udp", target: "dns.example.com:53", want: "dns.example.com:53"},
		{name: "udp missing port", monitorType: "udp", target: "example.com", wantErr: true},
		{name: "udp with scheme", monitorType: "udp", target: "udp://example.com:53", wantErr: true},

		// WebSocket
		{name: "ws bare domain", monitorType: "websocket", target: "example.com/ws", want: "wss://example.com/ws"},
		{name: "ws bare domain no path", monitorType: "websocket", target: "example.com", want: "wss://example.com"},
		{name: "ws with port", monitorType: "websocket", target: "example.com:8080/ws", want: "wss://example.com:8080/ws"},
		{name: "ws explicit wss", monitorType: "websocket", target: "wss://example.com/ws", want: "wss://example.com/ws"},
		{name: "ws explicit ws", monitorType: "websocket", target: "ws://example.com/ws", want: "ws://example.com/ws"},
		{name: "ws bad scheme", monitorType: "websocket", target: "http://example.com/ws", wantErr: true},

		// gRPC
		{name: "grpc valid host:port", monitorType: "grpc", target: "grpc.example.com:443", want: "grpc.example.com:443"},
		{name: "grpc missing port", monitorType: "grpc", target: "example.com", wantErr: true},
		{name: "grpc with scheme", monitorType: "grpc", target: "grpc://example.com:443", wantErr: true},

		// Unknown type passthrough
		{name: "unknown type passthrough", monitorType: "icmp", target: "example.com", want: "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.monitorType, tt.target)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Normalize(%q, %q) error = %v, wantErr %v", tt.monitorType, tt.target, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Normalize(%q, %q) = %q, want %q", tt.monitorType, tt.target, got, tt.want)
			}
		})
	}
}
