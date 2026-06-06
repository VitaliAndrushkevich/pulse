# Implementation Plan: gRPC Monitor

## Overview

Implement a gRPC health-check monitor type for the Pulse uptime monitoring platform. The implementation follows the same patterns as existing checkers (HTTP, TCP, UDP, WebSocket): a `GRPCChecker` struct implementing the `Checker` interface, registered in `DefaultRegistry()`, dispatched by the bounded worker pool scheduler. The checker uses raw codec invocation via `google.golang.org/grpc` to call arbitrary unary RPCs without generated stubs.

## Tasks

- [x] 1. Database migration and dependency setup
  - [x] 1.1 Add `google.golang.org/grpc` dependency to go.mod
    - Run `go get google.golang.org/grpc@latest` in the backend directory
    - Verify `google.golang.org/grpc` appears in `go.mod` with version v1.72+
    - _Requirements: Design — New Dependency_

  - [x] 1.2 Create database migration 008_grpc_monitor_type
    - Create `backend/migrations/008_grpc_monitor_type.up.sql`: drop existing `monitors_type_check` constraint and add new constraint including `grpc`
    - Create `backend/migrations/008_grpc_monitor_type.down.sql`: delete monitors with type `grpc`, drop constraint, re-add constraint without `grpc`
    - Follow the pattern of existing migrations (sequential numbering)
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

- [x] 2. Implement GRPCSettings parsing and validation
  - [x] 2.1 Create `backend/internal/monitor/grpc.go` with GRPCSettings struct and parsing
    - Define `GRPCSettings` struct with JSON tags: `service_method`, `tls_mode`, `ssl_expiry_threshold`, `metadata`, `expected_statuses`, `request_payload`
    - Implement `parseGRPCSettings(json.RawMessage) GRPCSettings` with defaults: `tls_mode="tls"`, `service_method=""` (falls back to health check), `expected_statuses=[0]`
    - Implement `rawCodec` struct for pass-through gRPC marshaling
    - _Requirements: 2.2, 4.4, 7.2_

  - [x] 2.2 Implement settings validation functions
    - `validateServiceMethod(string) error` — exactly one `/`, both segments non-empty, combined ≤ 512 chars, whitespace-only treated as unset
    - `validateTLSMode(string) error` — must be `plaintext`, `tls`, or `tls_skip_verify`
    - `validateMetadata(map[string]string) error` — max 20 entries, key max 128 chars (lowercase alphanum + `-_\.`), no `grpc-` prefix, `-bin` suffix requires valid base64 value, value max 4096 chars
    - `validateExpectedStatuses([]int) error` — each value 0–16, max 17 entries
    - `validateRequestPayload(string) ([]byte, error)` — valid base64, decoded ≤ 1MB
    - _Requirements: 3.2, 3.3, 3.5, 4.6, 6.2, 6.3, 6.4, 7.3, 7.4, 8.1, 8.3, 8.4_

  - [x] 2.3 Write property test for service method format validation
    - **Property 2: Service method format validation**
    - Generate random strings with varying `/` counts and segment lengths
    - Verify: accepted iff exactly one `/`, both segments non-empty, combined length ≤ 512
    - **Validates: Requirements 3.2, 3.3**

  - [x] 2.4 Write property test for whitespace-only service method fallback
    - **Property 3: Whitespace-only service method falls back to default**
    - Generate random whitespace-only strings (spaces, tabs, newlines)
    - Verify: all treated as unset, resulting in default `grpc.health.v1.Health/Check`
    - **Validates: Requirements 3.5**

  - [x] 2.5 Write property test for invalid TLS mode rejection
    - **Property 4: Invalid TLS mode rejection**
    - Generate random strings excluding `plaintext`, `tls`, `tls_skip_verify`
    - Verify: all produce validation error
    - **Validates: Requirements 4.6**

  - [x] 2.6 Write property test for metadata key validation
    - **Property 7: Metadata key validation**
    - Generate random keys with valid/invalid characters, `grpc-` prefixes, `-bin` suffixes with valid/invalid base64 values
    - Verify: accepted iff lowercase alphanum + `-_\.`, no `grpc-` prefix, ≤ 128 chars, `-bin` keys have valid base64 values, ≤ 20 entries, values ≤ 4096 chars
    - **Validates: Requirements 6.2, 6.3, 6.4**

  - [x] 2.7 Write property test for expected statuses validation
    - **Property 8: Expected statuses validation**
    - Generate random integer lists with values in [-10, 30] range and varying lengths
    - Verify: accepted iff all values in 0–16 and list length ≤ 17
    - **Validates: Requirements 7.3, 7.4**

  - [x] 2.8 Write property test for invalid base64 payload rejection
    - **Property 10: Invalid base64 payload rejection**
    - Generate random strings with characters outside the base64 alphabet
    - Verify: all produce payload decode error
    - **Validates: Requirements 8.3**

- [x] 3. Checkpoint - Ensure validation tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Implement GRPCChecker core logic
  - [x] 4.1 Implement `GRPCChecker.Check` method
    - Implement the `Checker` interface: `Check(ctx context.Context, target string, settings json.RawMessage) Result`
    - Parse and validate settings (return immediate `down` for validation failures)
    - Build gRPC dial options based on `tls_mode` (plaintext → `insecure.NewCredentials()`, tls → `credentials.NewTLS(...)`, tls_skip_verify → `credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})`)
    - Create connection with `grpc.NewClient(target, opts...)`
    - Defer `conn.Close()` for resource cleanup
    - Extract TLS peer certificates from connection state after dial
    - Compute `ssl_days_remaining` from leaf cert NotAfter
    - Check SSL expiry threshold
    - Decode request payload from base64
    - Invoke unary RPC via `conn.Invoke(ctx, "/"+fullMethod, reqBytes, &respBytes)` with raw codec
    - Measure latency from start of dial to response (monotonic clock, truncated to ms)
    - Extract gRPC status code and compare against expected statuses
    - Return `Result` with state, latency, ssl_days_remaining, error
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 3.1, 4.1, 4.2, 4.3, 4.4, 4.5, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 6.1, 7.1, 8.1, 8.2, 11.1, 11.2, 11.3, 11.4, 12.1, 12.2, 12.3, 12.4, 12.5_

  - [x] 4.2 Register GRPCChecker in DefaultRegistry
    - Add `reg.Register("grpc", &GRPCChecker{})` in `DefaultRegistry()` in `backend/internal/monitor/checker.go`
    - _Requirements: 1.1, 1.2_

  - [x] 4.3 Write property test for status code determines up/down state
    - **Property 1: Status code determines up/down state**
    - Generate random gRPC status code (0–16) and random non-empty subset of [0..16] as expected list
    - Verify: state is `up` iff returned code is in expected list, `down` otherwise
    - **Validates: Requirements 2.4, 2.6, 7.1**

  - [x] 4.4 Write property test for SSL days remaining calculation
    - **Property 5: SSL days remaining calculation**
    - Generate random time offsets (-365 to +365 days from now) as certificate NotAfter
    - Verify: `ssl_days_remaining` equals hours-until-NotAfter / 24 truncated toward zero
    - **Validates: Requirements 5.1, 5.2**

  - [x] 4.5 Write property test for SSL expiry threshold triggers down state
    - **Property 6: SSL expiry threshold triggers down state**
    - Generate random `days_remaining` (-30 to 3650) paired with random threshold (1–3650)
    - Verify: state is `down` iff `days_remaining ≤ threshold`
    - **Validates: Requirements 5.3**

  - [x] 4.6 Write property test for request payload round-trip
    - **Property 9: Request payload round-trip**
    - Generate random byte slices (0 to 1MB), base64-encode them
    - Verify: decoded bytes sent to the gRPC server match the original byte slice
    - **Validates: Requirements 8.1**

  - [x] 4.7 Write unit tests for GRPCChecker
    - Test default settings parsing (empty settings → tls_mode="tls", default health check method)
    - Test settings JSON round-trip (marshal/unmarshal preserves all fields)
    - Test plaintext mode skips SSL (no ssl_days_remaining in result)
    - Test no peer certs graceful handling (ssl_days_remaining is nil, no error)
    - Test context timeout returns down with elapsed latency
    - Test context cancellation returns down with cancellation error
    - _Requirements: 2.2, 4.4, 5.5, 5.6, 11.3, 12.2, 12.3, 12.4_

- [x] 5. Checkpoint - Ensure core checker tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Update OpenAPI specification
  - [x] 6.1 Add `grpc` to type enums and define GRPCSettings schema in OpenAPI spec
    - Add `grpc` to the `type` enum in `CreateMonitorRequest`, `PutMonitorRequest`, and `Monitor` schemas
    - Define `GRPCSettings` schema object with all properties: `service_method`, `tls_mode`, `ssl_expiry_threshold`, `metadata`, `expected_statuses`, `request_payload`
    - Reference `GRPCSettings` from the `settings` property description on monitor schemas
    - Add request body example on `createMonitor` operation demonstrating gRPC monitor creation
    - _Requirements: 10.1, 10.2, 10.3, 10.4_

- [x] 7. Integration wiring and scheduler support
  - [x] 7.1 Ensure scheduler dispatches gRPC monitors correctly
    - Verify the scheduler's `executeCheck` handles `type=grpc` via registry lookup (no special-casing needed — registry dispatch is generic)
    - Verify gRPC results are persisted to TimescaleDB and broadcast via WebSocket hub
    - No code changes expected since the scheduler uses the registry generically, but verify the gRPC checker does not need `AuthenticatedChecker` support (it uses metadata instead)
    - _Requirements: 1.2, 1.3_

  - [x] 7.2 Write integration tests for GRPCChecker
    - Create `backend/internal/monitor/grpc_integration_test.go` with `//go:build integration` tag
    - Test: health check against mock gRPC server implementing Health service
    - Test: custom method invocation against mock server
    - Test: TLS connection with self-signed cert + CA
    - Test: plaintext connection
    - Test: timeout handling with artificial delay
    - Test: metadata propagation (mock server echoes received metadata)
    - Test: full scheduler dispatch end-to-end through registry
    - _Requirements: 2.1, 2.2, 2.3, 3.1, 3.4, 4.1, 4.2, 4.3, 6.1, 12.1, 12.4_

- [x] 8. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `pgregory.net/rapid`
- Unit tests validate specific examples and edge cases
- Integration tests use a mock gRPC server (no external dependencies)
- All test files are co-located: `backend/internal/monitor/grpc_test.go` for unit + property tests, `backend/internal/monitor/grpc_integration_test.go` for integration tests
- The scheduler requires no modifications — it dispatches `grpc` monitors generically via the registry
- The `GRPCChecker` does NOT implement `AuthenticatedChecker` — gRPC auth is handled via the `metadata` settings field

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["2.1"] },
    { "id": 2, "tasks": ["2.2"] },
    { "id": 3, "tasks": ["2.3", "2.4", "2.5", "2.6", "2.7", "2.8"] },
    { "id": 4, "tasks": ["4.1"] },
    { "id": 5, "tasks": ["4.2", "4.3", "4.4", "4.5", "4.6", "4.7"] },
    { "id": 6, "tasks": ["6.1", "7.1"] },
    { "id": 7, "tasks": ["7.2"] }
  ]
}
```
