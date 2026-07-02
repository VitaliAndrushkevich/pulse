# Implementation Plan: MCP Server for Pulse

## Overview

Implement the Pulse MCP server (`pulse-mcp`) as a separate binary that exposes Pulse operational state to MCP-compatible AI clients via the official Go MCP SDK. The implementation follows the design's package structure: config, pulseapi, mcperr, redact, resolve, downtime, tools, and server wiring. Each phase builds incrementally so that earlier phases are testable before later ones depend on them.

## Tasks

- [x] 1. Project structure and core interfaces
  - [x] 1.1 Create directory layout and Go entrypoint
    - Create `mcp/cmd/pulse-mcp/main.go` (stub main that exits with "not yet implemented")
    - Create package directories: `mcp/internal/config/`, `mcp/internal/pulseapi/`, `mcp/internal/mcperr/`, `mcp/internal/redact/`, `mcp/internal/resolve/`, `mcp/internal/downtime/`, `mcp/internal/tools/`, `mcp/internal/server/`
    - Create `mcp/go.mod` with module path (e.g. `github.com/vandrushkevich/pulse/mcp`) and Go 1.25
    - Add `github.com/modelcontextprotocol/go-sdk` dependency to `mcp/go.mod`
    - Add `pgregory.net/rapid` dependency to `mcp/go.mod`
    - Remove `backend/cmd/pulse-mcp/` if it was created previously
    - _Requirements: 2.1, 2.2_

  - [x] 1.2 Implement configuration model (`config` package)
    - Define `Config` struct with fields for all `PULSE_MCP_*` env vars
    - Define `AccessMode` type with `ReadOnly` and `ReadWrite` constants
    - Implement `Load()` that reads env vars, applies defaults (base URL `http://localhost:8080/api/v1`, access mode `read-only`, transport `stdio`, HTTP addr `:9090`, request timeout `15s`)
    - Abort with error if `PULSE_MCP_API_TOKEN` is absent/empty/whitespace-only
    - Parse `PULSE_MCP_ACCESS_MODE`: anything other than `read-write` → `ReadOnly`
    - _Requirements: 3.3, 3.4, 10.1, 10.2, 12.5_

  - [x] 1.3 Write property test for Access_Mode parsing
    - **Property 24: Access_Mode parsing defaults to read-only**
    - **Validates: Requirements 10.1, 10.2**

  - [x] 1.4 Define PulseClient interface and data models (`pulseapi` package)
    - Define `PulseClient` interface with methods: `ListMonitors`, `GetMonitor`, `GetMonitorStats`, `GetMonitorHistory`, `ListIncidents`, `CreateMonitor`
    - Define DTOs: `Monitor`, `MonitorStats`, `MonitorPage`, `HistoryPoint`, `History`, `Incident`, `IncidentPage`, `CreateMonitorInput`, `MonitorQuery`, `IncidentQuery`, `TimeRange`, `Tag`, `CheckError`, `SSLInfo`
    - Define error types: `PulseError` (Code, Message, RequestID, HTTPStatus) and `ConnectivityError` (Reason)
    - _Requirements: 2.2, 12.1, 12.4_

  - [x] 1.5 Implement MCP error mapping (`mcperr` package)
    - Define error codes: `VALIDATION_ERROR`, `INVALID_TYPE`, `AMBIGUOUS_NAME`, `NOT_FOUND`, `INVALID_RANGE`, `INVALID_IDENTIFIER`, `INVALID_WINDOW`, `WRITE_DISABLED`, `PULSE_UNREACHABLE`, `PULSE_TIMEOUT`, `PULSE_UNAUTHORIZED`
    - Implement `FromPulseError(*PulseError)` that preserves code, message, and X-Request-ID
    - Implement `FromConnectivityError(*ConnectivityError)` that maps to `PULSE_UNREACHABLE` or `PULSE_TIMEOUT`
    - Ensure every MCP error carries a non-empty code and message
    - _Requirements: 1.7, 2.4, 9.4, 12.1, 12.2, 12.3, 12.4_

  - [x] 1.6 Write property tests for error mapping
    - **Property 28: Pulse error envelope is preserved verbatim with request id**
    - **Validates: Requirements 9.4, 12.1, 12.3**
    - **Property 29: Connectivity/timeout failures map to a distinct error**
    - **Validates: Requirements 1.7, 2.4, 12.4**
    - **Property 30: Every MCP error carries a code and a message**
    - **Validates: Requirements 12.2**

- [x] 2. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 3. Pulse API HTTP client implementation
  - [x] 3.1 Implement HTTP client (`pulseapi` package)
    - Implement `httpClient` struct satisfying `PulseClient` interface
    - Set `Authorization: Bearer <token>` on every request
    - Use per-request `context.WithTimeout` of configured timeout (default 15s)
    - Read `X-Request-ID` from Pulse responses and attach to results/errors
    - Parse error envelope `{ "error": { "code", "message" } }` on non-2xx into `*PulseError`
    - Map dial errors, connection refused, and context deadline exceeded to `*ConnectivityError`
    - On HTTP 401, surface as `PULSE_UNAUTHORIZED` and never retry
    - Implement all 6 client methods: `ListMonitors` (GET /monitors), `GetMonitor` (GET /monitors/{id}), `GetMonitorStats` (GET /monitors/{id}/stats), `GetMonitorHistory` (GET /monitors/{id}/history), `ListIncidents` (GET /incidents or GET /monitors/{id}/incidents), `CreateMonitor` (POST /monitors)
    - _Requirements: 2.2, 3.1, 3.2, 3.5, 12.3_

  - [x] 3.2 Write unit tests for HTTP client
    - Test error-envelope parsing with httptest server
    - Test connectivity error mapping (dial failure, timeout)
    - Test 401 handling
    - Test X-Request-ID propagation
    - _Requirements: 3.5, 12.1, 12.3, 12.4_

- [x] 4. Secret redaction and log sanitization
  - [x] 4.1 Implement redaction (`redact` package)
    - Implement `SanitizeLog(entry)` that replaces Authorization/Bearer headers and known secret fields with `[REDACTED]`
    - Implement `OmitSecretFields(resource)` that strips credential/secret/token/password/key fields from a struct
    - If a value cannot be safely redacted, suppress the entire log entry and emit a redaction-failure marker
    - Ensure the fixed placeholder shares no characters with any original value
    - Return `withheld_fields` list when secret fields are omitted from tool results
    - _Requirements: 3.6, 11.1, 11.2, 11.3, 11.4, 11.5_

  - [x] 4.2 Write property tests for redaction
    - **Property 26: Secrets and tokens never leak, and placeholders are disjoint**
    - **Validates: Requirements 3.6, 11.1, 11.2, 11.3, 11.4**
    - **Property 27: Secret-bearing fields are omitted with a withheld indication**
    - **Validates: Requirements 11.3, 11.5**

- [x] 5. Pure logic: name resolution and downtime derivation
  - [x] 5.1 Implement name-to-identifier resolution (`resolve` package)
    - Implement `Monitor(ctx, client, ref)` that:
      - If `ref` is a valid UUID → use directly
      - Otherwise page through `GET /monitors` collecting exact case-sensitive name matches
      - Exactly one match → return id
      - Zero matches → return `NOT_FOUND` error
      - Two+ matches → return `AMBIGUOUS_NAME` error listing all matching ids
    - _Requirements: 5.4, 5.5, 5.6, 5.7, 6.5_

  - [x] 5.2 Write property tests for name resolution
    - **Property 6: Case-sensitive exact name resolution**
    - **Validates: Requirements 5.4**
    - **Property 7: Ambiguous name reports all matching ids and no data**
    - **Validates: Requirements 5.5**
    - **Property 8: Unknown id or name yields not-found and no data**
    - **Validates: Requirements 5.6, 5.7, 6.5**

  - [x] 5.3 Implement downtime derivation algorithm (`downtime` package)
    - Implement `Summarize(points, windowStart, windowEnd, truncated)` as a pure function
    - Walk ordered points, tracking down intervals bounded by up→down / down→up transitions or window edges
    - Compute: `had_downtime`, `downtime_period_count`, `total_downtime_seconds`, `periods[]`
    - Treat `pending` as not-down
    - Enforce invariants: total ≥ 0 and ≤ window length; periods non-overlapping and ordered; count == 0 iff total == 0 iff had_downtime == false
    - _Requirements: 7.2, 7.4_

  - [x] 5.4 Write property tests for downtime derivation
    - **Property 14: Downtime summary invariants hold**
    - **Validates: Requirements 7.2, 7.4**
    - **Property 15: Window clamping sets truncation and reports the effective window**
    - **Validates: Requirements 7.3, 7.5**

- [x] 6. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Tool handlers: read tools
  - [x] 7.1 Implement `list-monitors` tool handler
    - Define input schema struct with `type`, `tags`, `page`, `limit` fields and jsonschema tags
    - Validate type against recognized set (case-insensitive), normalize to canonical wire form
    - Validate page (≥1) and limit (1–100)
    - Call `PulseClient.ListMonitors` and build pagination metadata (`total_pages`, `has_next_page`)
    - Return empty set with `total: 0` when no matches
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

  - [x] 7.2 Write property tests for `list-monitors`
    - **Property 1: Pagination arithmetic is consistent**
    - **Validates: Requirements 4.4, 8.4**
    - **Property 2: Out-of-range page/limit is rejected without calling Pulse**
    - **Validates: Requirements 4.8, 8.7**
    - **Property 3: Empty match returns an empty set, not an error**
    - **Validates: Requirements 4.6, 8.5**
    - **Property 4: Monitor-type filter normalizes case-insensitively**
    - **Validates: Requirements 4.2**
    - **Property 5: Unrecognized type filter is rejected without calling Pulse**
    - **Validates: Requirements 4.5**

  - [x] 7.3 Implement `get-monitor` tool handler
    - Define input schema: `{ monitor: string }`
    - Resolve monitor reference via `resolve.Monitor`
    - Call `PulseClient.GetMonitor` and project non-secret fields
    - Return full config + current status including last check timestamp
    - _Requirements: 5.1, 5.4, 5.5, 5.6, 5.7_

  - [x] 7.4 Write property test for `get-monitor`
    - **Property 9: Monitor projection preserves non-secret fields faithfully**
    - **Validates: Requirements 5.1**

  - [x] 7.5 Implement `monitor-stats` tool handler
    - Define input schema: `{ monitor: string }`
    - Resolve monitor reference, call `PulseClient.GetMonitorStats`
    - Include SSL info only when present in response; omit when absent
    - Compute 7-day uptime from history if stats endpoint lacks it
    - _Requirements: 5.2, 5.3_

  - [x] 7.6 Write property test for `monitor-stats`
    - **Property 10: SSL info is included exactly when the monitor is TLS-based**
    - **Validates: Requirements 5.3**

  - [x] 7.7 Implement `monitor-history` tool handler
    - Define input schema: `{ monitor: string, from?: RFC3339, to?: RFC3339 }`
    - Default window: trailing 24 hours
    - Validate: `from > to` → invalid-range error; malformed identifier → invalid-identifier error
    - Call `PulseClient.GetMonitorHistory` with the effective range
    - Pass through truncation flag from Pulse
    - Return empty list when no points in range
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7_

  - [x] 7.8 Write property tests for `monitor-history`
    - **Property 11: History returns exactly the points within the range**
    - **Validates: Requirements 6.1, 6.7**
    - **Property 12: from > to is rejected without calling Pulse**
    - **Validates: Requirements 6.4**
    - **Property 13: Malformed monitor identifier is rejected**
    - **Validates: Requirements 6.6**

  - [x] 7.9 Implement `downtime-summary` tool handler
    - Define input schema: `{ monitor: string, window_seconds?: int }`
    - Default window: 86400 (24h); validate ≥ 60 and ≤ retention
    - Resolve monitor, fetch history for the window, delegate to `downtime.Summarize`
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

  - [x] 7.10 Write property test for `downtime-summary` validation
    - **Property 16: Invalid window is rejected without calling Pulse**
    - **Validates: Requirements 7.7**

  - [x] 7.11 Implement `list-incidents` tool handler
    - Define input schema: `{ monitor?: string, open_only?: bool, page: int, limit: int }`
    - Validate page (≥1) and limit (1–100)
    - Route to global or per-monitor endpoint based on `monitor` presence
    - Apply `open_only` filter client-side for per-monitor endpoint
    - Order by `started_at` descending; include `resolved_at` only when resolved
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

  - [x] 7.12 Write property tests for `list-incidents`
    - **Property 17: Incidents are ordered by start time descending**
    - **Validates: Requirements 8.1**
    - **Property 18: Incident filters are applied conjunctively**
    - **Validates: Requirements 8.2, 8.3**
    - **Property 19: resolved_at present exactly when resolved**
    - **Validates: Requirements 8.6**

- [x] 8. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Tool handler: write tool and access-mode gate
  - [x] 9.1 Implement `create-monitor` tool handler
    - Define input schema: `{ type: string, name: string, target: string, interval_seconds?: int, timeout_seconds?: int, http_expected_statuses?: int[] }`
    - Validation order: (1) read-only mode → WRITE_DISABLED, (2) type not in {HTTP, TCP, UDP, ICMP} → error listing types, (3) name missing/blank/>255 → name-validation error, (4) basic target shape check
    - Normalize type to canonical form; build `CreateMonitorInput` with settings map for HTTP
    - Call `PulseClient.CreateMonitor`; if Pulse rejects target, preserve error code/message
    - Return created monitor: id, name, type, target, status, state, interval, timeout
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6_

  - [x] 9.2 Write property tests for `create-monitor`
    - **Property 20: create-monitor builds the correct payload and reflects defaults**
    - **Validates: Requirements 9.1, 9.2**
    - **Property 21: Non-simple monitor type is rejected without calling Pulse**
    - **Validates: Requirements 9.3**
    - **Property 22: Access-mode gate blocks writes in read-only mode**
    - **Validates: Requirements 9.5, 10.4**
    - **Property 23: Invalid monitor name is rejected without calling Pulse**
    - **Validates: Requirements 9.6**

  - [x] 9.3 Implement tool registration with Access_Mode gate (`server` package)
    - Implement `Register(s *mcp.Server, deps Deps, mode AccessMode)` that registers all read tools always and `create-monitor` only in read-write mode
    - Add defense-in-depth mode check inside `create-monitor` handler itself
    - _Requirements: 10.3, 10.4, 10.5, 10.6_

  - [x] 9.4 Write property test for access-mode gating
    - **Property 25: Advertised tools match the Access_Mode exactly**
    - **Validates: Requirements 10.3, 10.5, 10.6**

- [x] 10. Server wiring and transport
  - [x] 10.1 Implement server assembly (`server` package)
    - Wire `mcp.NewServer` with implementation name/version
    - Build `Deps` struct from `PulseClient` and config
    - Call `Register` with resolved Access_Mode
    - Implement transport selection: `stdio` (default) → `mcp.StdioTransport`; `http` → Streamable HTTP on configured addr
    - Emit exactly one startup log line stating transport and Access_Mode
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 12.5_

  - [x] 10.2 Complete `main.go` entrypoint
    - Load config (abort if token missing)
    - Create HTTP PulseClient with config
    - Build and start server with selected transport
    - Handle graceful shutdown on SIGINT/SIGTERM
    - _Requirements: 2.1, 2.5, 3.3, 3.4_

  - [x] 10.3 Write integration tests for server lifecycle
    - Test: startup with blank token aborts before opening transport
    - Test: server accepts MCP connections while Pulse is unreachable (returns connectivity errors)
    - Test: handshake returns protocol version and capabilities within 5s
    - Test: tools/list returns exactly the tools for the configured Access_Mode
    - Test: unknown tool invocation returns MCP error
    - _Requirements: 1.1, 1.2, 1.4, 1.6, 2.1, 2.5, 3.3, 3.4_

- [x] 11. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 12. Packaging and documentation
  - [x] 12.1 Add Makefile in `mcp/` directory
    - Add `build` target: `go build -o bin/pulse-mcp ./cmd/pulse-mcp`
    - Add `test` target: `go test ./...`
    - Add `lint` target if golangci-lint is available
    - _Requirements: 2.1_

  - [x] 12.2 Add setup instructions (`mcp/README.md`)
    - Document how to build: `cd mcp && make build`
    - Document required env vars (`PULSE_MCP_API_TOKEN`) and optional ones (`PULSE_MCP_API_BASE_URL`, `PULSE_MCP_ACCESS_MODE`, `PULSE_MCP_TRANSPORT`, `PULSE_MCP_REQUEST_TIMEOUT`)
    - Provide example MCP client config (JSON snippet for Claude Desktop / Kiro / Cursor)
    - Add `PULSE_MCP_*` variables to root `.env.example` with documentation comments
    - _Requirements: 2.1, 3.1_

- [x] 13. Documentation and co-change rules
  - [x] 13.1 Add MCP co-change rule to AGENTS.md
    - In a new "MCP Server" section, add a rule stating: when any endpoint used by the MCP server (`GET /monitors`, `GET /monitors/{id}`, `GET /monitors/{id}/stats`, `GET /monitors/{id}/history`, `GET /incidents`, `GET /monitors/{id}/incidents`, `POST /monitors`) is changed in `backend/api/openapi.yaml` or its handler, the corresponding MCP tool handler(s) in `mcp/internal/tools/` and PulseClient method(s) in `mcp/internal/pulseapi/` MUST be updated in the same commit.
    - Add a note that breaking response-shape changes require updating the relevant tool's output projection and property tests.
    - _Requirements: 2.2_

- [x] 14. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design's 30 properties
- Unit tests validate specific examples and edge cases
- The implementation language is Go (as specified in the design)
- All MCP server code lives under `mcp/` at the project root as a separate Go module with zero imports from `backend/` packages
- The fake `PulseClient` used in property tests enables asserting "zero Pulse calls" for pre-validation rejections

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.4"] },
    { "id": 2, "tasks": ["1.3", "1.5"] },
    { "id": 3, "tasks": ["1.6", "3.1", "4.1"] },
    { "id": 4, "tasks": ["3.2", "4.2", "5.1", "5.3"] },
    { "id": 5, "tasks": ["5.2", "5.4"] },
    { "id": 6, "tasks": ["7.1", "7.3", "7.5", "7.7", "7.9", "7.11"] },
    { "id": 7, "tasks": ["7.2", "7.4", "7.6", "7.8", "7.10", "7.12"] },
    { "id": 8, "tasks": ["9.1"] },
    { "id": 9, "tasks": ["9.2", "9.3"] },
    { "id": 10, "tasks": ["9.4", "10.1"] },
    { "id": 11, "tasks": ["10.2"] },
    { "id": 12, "tasks": ["10.3"] },
    { "id": 13, "tasks": ["12.1", "12.2"] },
    { "id": 14, "tasks": ["13.1"] }
  ]
}
```
