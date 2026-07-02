package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
)

// Result is the protocol-neutral outcome of a single monitor check.
type Result struct {
	MonitorID        uuid.UUID
	State            string // "up" or "down"
	LatencyMs        int32
	StatusCode       *int32 // HTTP-only
	SSLDaysRemaining *int32 // HTTPS-only
	Error            string // empty on success
	CheckedAt        time.Time
}

// Checker is the interface every protocol implementation must satisfy.
type Checker interface {
	// Check executes a single health check against the given target.
	// The context carries the per-check timeout deadline.
	// settings is the monitor-specific JSON configuration (may be nil).
	Check(ctx context.Context, target string, settings json.RawMessage) Result
}

// Registry maps monitor type strings to their Checker implementations.
type Registry struct {
	checkers map[string]Checker
}

// NewRegistry creates an empty checker registry.
func NewRegistry() *Registry {
	return &Registry{
		checkers: make(map[string]Checker),
	}
}

// Register adds a checker for the given monitor type.
func (r *Registry) Register(monitorType string, checker Checker) {
	r.checkers[monitorType] = checker
}

// Get returns the checker for the given monitor type, or an error if not found.
func (r *Registry) Get(monitorType string) (Checker, error) {
	c, ok := r.checkers[monitorType]
	if !ok {
		return nil, fmt.Errorf("no checker registered for type %q", monitorType)
	}
	return c, nil
}

// DefaultRegistry returns a registry pre-populated with all built-in checkers.
// The queries parameter is passed to checkers that need database access (e.g., GRPCChecker
// for proto source lookups). It may be nil if proto_json support is not needed.
func DefaultRegistry(queries *db.Queries) *Registry {
	reg := NewRegistry()
	reg.Register("http", &HTTPChecker{})
	reg.Register("http3", &HTTP3Checker{})
	reg.Register("tcp", &TCPChecker{})
	reg.Register("udp", &UDPChecker{})
	reg.Register("websocket", &WebSocketChecker{})
	reg.Register("grpc", NewGRPCChecker(queries))
	reg.Register("dns", &DNSChecker{})
	reg.Register("icmp", &ICMPChecker{})
	reg.Register("smtp", &SMTPChecker{})
	return reg
}
