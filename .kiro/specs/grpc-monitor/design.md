# Design Document: gRPC Monitor

## Overview

This design introduces a gRPC health-check monitor type into the Pulse uptime monitoring platform. The new `GRPCChecker` follows the same patterns as the existing HTTP, TCP, UDP, and WebSocket checkers: it implements the `Checker` interface, is registered in the `Registry` at startup, and is dispatched by the bounded worker pool `Scheduler`.

The gRPC checker connects to a target `host:port`, invokes a configurable unary RPC (defaulting to the standard `grpc.health.v1.Health/Check`), and reports "up" or "down" based on gRPC response status codes. It supports TLS with certificate expiry monitoring, custom request metadata, configurable expected status codes, and raw protobuf request payloads.

### Key Design Decisions

1. **Use `google.golang.org/grpc` client library** — the canonical Go gRPC implementation. It provides dial options for TLS, per-RPC metadata, deadlines from context, and raw codec invocation.
2. **Raw codec invocation** — instead of generating stubs for arbitrary services, the checker uses `grpc.Invoke` with a raw byte codec. This allows calling any method on any service without needing compiled protobuf definitions.
3. **Single file (`grpc.go`)** — follows the same convention as other checkers. All gRPC-specific logic lives in `backend/internal/monitor/grpc.go`.
4. **No reflection or streaming** — the checker only supports unary calls with pre-encoded request payloads. This keeps the implementation simple and predictable for health-check purposes.

## Architecture

```mermaid
graph TD
    A[Scheduler] -->|dispatches| B[Registry.Get"grpc"]
    B --> C[GRPCChecker.Check]
    C --> D{Parse Settings}
    D --> E[Build Dial Options]
    E --> F{TLS Mode}
    F -->|plaintext| G[grpc.WithTransportCredentials insecure]
    F -->|tls| H[credentials.NewTLS system CAs]
    F -->|tls_skip_verify| I[credentials.NewTLS InsecureSkipVerify]
    G --> J[grpc.NewClient]
    H --> J
    I --> J
    J --> K[Invoke Unary RPC]
    K --> L{Check Status Code}
    L -->|in expected set| M[State: up]
    L -->|not in expected set| N[State: down]
    J --> O[Extract TLS Peer Certs]
    O --> P[Compute SSL Days Remaining]
    P --> Q{Threshold Check}
    Q -->|within threshold| N
    Q -->|ok or no threshold| M
```

### Integration Points

| Component | Interaction |
|-----------|-------------|
| `Registry` | `GRPCChecker` registered under `"grpc"` in `DefaultRegistry()` |
| `Scheduler` | Dispatches via `checker.Check(ctx, target, settings)` with per-check timeout context |
| `TimescaleDB` | Check results (including `ssl_days_remaining`) persisted via existing `WriteCheckResult` |
| `WebSocket Hub` | Results broadcast as `monitor_status` patches |
| `Prometheus` | Existing `pulse_monitor_up` and `pulse_monitor_response_time_seconds` labels include `type=grpc` |
| `OpenAPI` | `grpc` added to type enums; `GRPCSettings` schema defined |
| `Database` | `monitors_type_check` constraint extended to include `grpc` |

## Components and Interfaces

### GRPCChecker

```go
// GRPCChecker implements the Checker interface for gRPC monitors.
type GRPCChecker struct{}

func (g *GRPCChecker) Check(ctx context.Context, target string, settings json.RawMessage) Result
```

The checker does NOT implement `AuthenticatedChecker` because gRPC authentication is handled via the `metadata` field in settings (gRPC metadata is the equivalent of HTTP headers). This aligns with how gRPC services expect credentials.

### GRPCSettings

```go
// GRPCSettings holds configuration for the gRPC checker.
type GRPCSettings struct {
    // ServiceMethod is the fully-qualified service/method in "package.Service/Method" format.
    // Default: "grpc.health.v1.Health/Check"
    ServiceMethod string `json:"service_method,omitempty"`

    // TLSMode controls connection security: "plaintext", "tls", or "tls_skip_verify".
    // Default: "tls"
    TLSMode string `json:"tls_mode,omitempty"`

    // SSLExpiryThreshold is the minimum acceptable days until certificate expiry.
    // If the cert expires within this many days, the check reports "down".
    // Range: 1–3650. Default: 0 (disabled).
    SSLExpiryThreshold int `json:"ssl_expiry_threshold,omitempty"`

    // Metadata are key-value pairs sent as gRPC request metadata.
    // Max 20 entries. Keys: lowercase alphanumeric, hyphen, underscore, dot.
    // Keys must not start with "grpc-". Values max 4096 chars.
    Metadata map[string]string `json:"metadata,omitempty"`

    // ExpectedStatuses is a list of gRPC status codes considered "up".
    // Values in range 0–16. Default: [0] (OK only).
    ExpectedStatuses []int `json:"expected_statuses,omitempty"`

    // RequestPayload is a base64-encoded protobuf message to send as the request body.
    // Max decoded size: 1MB. Default: empty (zero-length payload).
    RequestPayload string `json:"request_payload,omitempty"`
}
```

### Raw Codec

```go
// rawCodec is a gRPC codec that passes bytes through without protobuf marshaling.
type rawCodec struct{}

func (rawCodec) Marshal(v interface{}) ([]byte, error)   { return v.([]byte), nil }
func (rawCodec) Unmarshal(data []byte, v interface{}) error { *v.(*[]byte) = data; return nil }
func (rawCodec) Name() string                            { return "raw" }
```

This allows invoking any gRPC method without generated stubs.

### Settings Validation

Validation occurs at the start of `Check()` and returns immediate "down" results for invalid settings:

1. **service_method format** — must contain exactly one `/`, both segments non-empty, combined length ≤ 512
2. **tls_mode** — must be one of `plaintext`, `tls`, `tls_skip_verify`
3. **metadata keys** — lowercase ASCII alphanumeric + `-_\.`, no `grpc-` prefix, max 128 chars per key
4. **metadata binary values** — keys ending in `-bin` must have valid base64 values
5. **expected_statuses** — each value in range 0–16, max 17 entries
6. **request_payload** — valid base64, decoded size ≤ 1MB

### Check Flow

1. Parse and validate `GRPCSettings` from `json.RawMessage`
2. Record start time (monotonic clock)
3. Build gRPC dial options based on `tls_mode`
4. Create connection with `grpc.NewClient(target, opts...)`
5. If TLS, extract peer certificates from the connection state after dial
6. Compute `ssl_days_remaining` from leaf cert `NotAfter`
7. Check SSL expiry threshold (report "down" if within threshold)
8. Decode `request_payload` (base64 → bytes)
9. Invoke unary RPC using `conn.Invoke(ctx, "/"+fullMethod, reqBytes, &respBytes)` with raw codec
10. Record latency (time since start, truncated to ms)
11. Extract gRPC status code from response error
12. Compare status code against `expected_statuses`
13. Return `Result` with state, latency, ssl info, and error

## Data Models

### Database Migration (008_grpc_monitor_type)

**Up migration:**
```sql
-- 008_grpc_monitor_type.up.sql
-- Add 'grpc' to the monitors type constraint.

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'tcp', 'udp', 'websocket', 'grpc'));
```

**Down migration:**
```sql
-- 008_grpc_monitor_type.down.sql
-- Remove gRPC monitors and restore the previous type constraint.

DELETE FROM monitors WHERE type = 'grpc';

ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_type_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_type_check
    CHECK (type IN ('http', 'tcp', 'udp', 'websocket'));
```

### GRPCSettings JSON Schema (stored in `monitors.settings` JSONB column)

```json
{
  "service_method": "grpc.health.v1.Health/Check",
  "tls_mode": "tls",
  "ssl_expiry_threshold": 30,
  "metadata": {
    "authorization": "Bearer token123",
    "x-custom-header": "value"
  },
  "expected_statuses": [0],
  "request_payload": "CgRwdWxz"
}
```

### OpenAPI Schema Addition

```yaml
GRPCSettings:
  type: object
  required: [service_method, tls_mode]
  properties:
    service_method:
      type: string
      description: "Fully-qualified gRPC service and method in package.Service/Method format"
      example: "grpc.health.v1.Health/Check"
      maxLength: 512
    tls_mode:
      type: string
      enum: [plaintext, tls, tls_skip_verify]
      description: "Connection security mode"
    ssl_expiry_threshold:
      type: integer
      minimum: 1
      maximum: 3650
      description: "Minimum acceptable days until certificate expiry"
    metadata:
      type: object
      additionalProperties:
        type: string
        maxLength: 4096
      maxProperties: 20
      description: "Key-value pairs sent as gRPC request metadata"
    expected_statuses:
      type: array
      items:
        type: integer
        minimum: 0
        maximum: 16
      maxItems: 17
      description: "gRPC status codes considered healthy (default: [0] = OK)"
    request_payload:
      type: string
      format: byte
      maxLength: 65536
      description: "Base64-encoded protobuf request message"
```

### Check Result Fields Used

| Field | Usage |
|-------|-------|
| `State` | `"up"` or `"down"` |
| `LatencyMs` | Dial-to-response time in ms |
| `StatusCode` | Not used (HTTP-specific) |
| `SSLDaysRemaining` | Days until TLS cert expiry (when TLS mode is `tls` or `tls_skip_verify`) |
| `Error` | gRPC status message, connection error, or validation error |

### New Dependency

```
google.golang.org/grpc v1.72+
```

This is the standard Go gRPC library. It brings in `google.golang.org/protobuf` (already present) and `golang.org/x/net` (already present).



## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Status code determines up/down state

*For any* gRPC status code returned by the server and *for any* non-empty `expected_statuses` list containing valid codes (0–16), the checker SHALL report state `"up"` if and only if the returned code is present in the expected list; otherwise it SHALL report state `"down"`.

**Validates: Requirements 2.4, 2.6, 7.1**

### Property 2: Service method format validation

*For any* string value of `service_method`, the checker SHALL accept it (proceed to invoke the RPC) if and only if it contains exactly one `/` separator, both the service segment (before `/`) and method segment (after `/`) are non-empty, and the combined length is ≤ 512 characters. All other strings SHALL cause the checker to report state `"down"` with a format error.

**Validates: Requirements 3.2, 3.3**

### Property 3: Whitespace-only service method falls back to default

*For any* string composed entirely of whitespace characters (spaces, tabs, newlines), when provided as `service_method`, the checker SHALL treat it as unset and invoke the default method `grpc.health.v1.Health/Check`.

**Validates: Requirements 3.5**

### Property 4: Invalid TLS mode rejection

*For any* string value of `tls_mode` that is not one of `"plaintext"`, `"tls"`, or `"tls_skip_verify"`, the checker SHALL report state `"down"` with an error indicating the unrecognized TLS mode value.

**Validates: Requirements 4.6**

### Property 5: SSL days remaining calculation

*For any* certificate `NotAfter` timestamp in the future or past, the checker SHALL compute `ssl_days_remaining` as the number of hours between now and `NotAfter` divided by 24, truncated toward zero (producing negative values for already-expired certificates).

**Validates: Requirements 5.1, 5.2**

### Property 6: SSL expiry threshold triggers down state

*For any* `ssl_expiry_threshold` value in range 1–3650 and *for any* certificate with computed `days_remaining`, the checker SHALL report state `"down"` if and only if `days_remaining ≤ ssl_expiry_threshold`.

**Validates: Requirements 5.3**

### Property 7: Metadata key validation

*For any* metadata key string, the checker SHALL accept it if and only if: (a) it contains only lowercase ASCII letters, digits, hyphens, underscores, and dots; (b) it does not start with the prefix `grpc-`; (c) its length is ≤ 128 characters; and (d) if the key ends with `-bin`, its corresponding value is valid base64. The total number of metadata entries must be ≤ 20 and each value must be ≤ 4096 characters. Violation of any condition SHALL cause the checker to report state `"down"`.

**Validates: Requirements 6.2, 6.3, 6.4**

### Property 8: Expected statuses validation

*For any* list provided as `expected_statuses`, the checker SHALL accept it if and only if every element is an integer in the range 0–16 inclusive and the list contains at most 17 entries. Any list containing a value outside 0–16 SHALL cause the checker to report state `"down"`.

**Validates: Requirements 7.3, 7.4**

### Property 9: Request payload round-trip

*For any* byte sequence of length ≤ 1,048,576 bytes, base64-encoding it and providing the result as `request_payload` SHALL cause the checker to send those exact decoded bytes as the gRPC request body to the server.

**Validates: Requirements 8.1**

### Property 10: Invalid base64 payload rejection

*For any* string that is not valid standard base64 (RFC 4648 §4), when provided as `request_payload`, the checker SHALL report state `"down"` with an error indicating payload decode failure.

**Validates: Requirements 8.3**

## Error Handling

### Validation Errors (fail-fast before network I/O)

| Condition | Behavior |
|-----------|----------|
| Invalid `service_method` format | Return `down` with format error immediately |
| Invalid `tls_mode` value | Return `down` with unrecognized mode error |
| Metadata key validation failure | Return `down` with specific key name in error |
| Binary metadata value not base64 | Return `down` with key name in error |
| `expected_statuses` value out of range | Return `down` with invalid code error |
| `request_payload` not valid base64 | Return `down` with decode failure error |
| Decoded payload exceeds 1MB | Return `down` with size limit error |
| Metadata exceeds 20 entries | Return `down` with limit exceeded error |

### Network Errors

| Condition | Behavior |
|-----------|----------|
| DNS resolution failure | Return `down` with dial error, report elapsed latency |
| Connection refused | Return `down` with dial error, report elapsed latency |
| Context timeout | Return `down` with timeout error, report elapsed latency, close connection |
| Context cancelled | Return `down` with cancellation error, report elapsed latency, close connection |
| TLS handshake failure | Return `down` with TLS error details (expired, hostname mismatch, unknown CA) |

### gRPC Errors

| Condition | Behavior |
|-----------|----------|
| Unexpected status code | Return `down` with gRPC status code name + server message |
| UNIMPLEMENTED (12) | Return `down` — service or method does not exist |
| UNAVAILABLE (14) | Return `down` — server not ready |

### Resource Cleanup

- The gRPC connection (`*grpc.ClientConn`) MUST be closed via `defer conn.Close()` immediately after successful creation
- On timeout/cancellation, the context deadline propagates through the gRPC library to abort in-flight operations
- No goroutines are leaked — all operations are synchronous within the single `Check` call

## Testing Strategy

### Unit Tests (example-based)

| Test Case | What It Verifies |
|-----------|-----------------|
| Default settings parsing | Empty settings → tls_mode="tls", service_method="grpc.health.v1.Health/Check" |
| Settings JSON round-trip | Marshal → unmarshal preserves all fields |
| TLS mode default | Missing tls_mode defaults to "tls" |
| Expected statuses default | Missing expected_statuses defaults to [0] |
| Plaintext mode skips SSL | No ssl_days_remaining in result |
| No peer certs graceful handling | ssl_days_remaining is nil, no error |

### Property-Based Tests (using `pgregory.net/rapid`)

The project already uses `pgregory.net/rapid` (present in go.mod). Each property test runs a minimum of 100 iterations.

| Property | Generator Strategy |
|----------|-------------------|
| P1: Status code matching | Random code (0–16), random subset of [0..16] as expected list |
| P2: Service method validation | Random strings with varying `/` counts and segment lengths |
| P3: Whitespace fallback | Random whitespace-only strings (space, tab, newline, carriage return combos) |
| P4: Invalid TLS mode | Random strings excluding the three valid values |
| P5: SSL days calculation | Random time offsets (-365 to +365 days from now) |
| P6: SSL threshold | Random days_remaining (-30 to 3650) paired with random thresholds (1–3650) |
| P7: Metadata key validation | Random keys with valid/invalid characters, valid/invalid prefixes, -bin suffix with valid/invalid base64 |
| P8: Expected statuses validation | Random integer lists with values in [-10, 30] range and varying lengths |
| P9: Payload round-trip | Random byte slices (0 to 1MB) |
| P10: Invalid base64 rejection | Random strings with characters outside base64 alphabet |

**Tag format:** Each property test includes a comment:
```go
// Feature: grpc-monitor, Property N: <property title>
```

### Integration Tests

| Test | Setup |
|------|-------|
| Health check against mock server | Start a local gRPC server implementing Health service |
| Custom method invocation | Mock server with custom service |
| TLS connection (valid cert) | Mock server with self-signed cert + CA in system pool |
| TLS skip verify (self-signed) | Mock server with self-signed cert, tls_skip_verify mode |
| Plaintext connection | Mock server without TLS |
| Timeout handling | Mock server with artificial delay > timeout |
| Context cancellation | Cancel context mid-flight |
| Metadata propagation | Mock server that echoes received metadata in response |
| Full scheduler dispatch | Verify end-to-end flow through registry and scheduler |

### Test File Location

- Unit + property tests: `backend/internal/monitor/grpc_test.go`
- Integration tests: `backend/internal/monitor/grpc_integration_test.go` (build tag `integration`)
