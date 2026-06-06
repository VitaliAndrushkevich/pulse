# Requirements Document

## Introduction

Add gRPC health checking as a new monitor type in Pulse. The gRPC checker connects to a target host:port, invokes a user-specified service/method via unary RPC, and determines "up" or "down" based on gRPC response status codes. This mirrors the functionality of tools like `grpcurl` (e.g., `grpcurl grpc.server.com:443 my.custom.Service/Method`) but runs on a schedule inside the Pulse monitor engine.

## Glossary

- **GRPC_Checker**: The Go component implementing the Checker interface for gRPC monitors. Resides at `backend/internal/monitor/grpc.go`.
- **Target**: The `host:port` address of the gRPC server to probe (e.g., `grpc.server.com:443`).
- **Service_Method**: A fully-qualified gRPC service and method name in `package.Service/Method` format (e.g., `grpc.health.v1.Health/Check`).
- **GRPC_Settings**: The JSON configuration stored in the monitor's `settings` column, controlling service/method, TLS, metadata, and expected status codes.
- **GRPC_Status_Code**: A numeric gRPC status code as defined by the gRPC specification (0 = OK, 1 = CANCELLED, ... 16 = UNAUTHENTICATED).
- **Metadata**: Key-value pairs sent as gRPC request metadata (analogous to HTTP headers).
- **TLS_Mode**: The connection security mode — `plaintext`, `tls`, or `tls_skip_verify`.
- **SSL_Expiry_Threshold**: The minimum acceptable number of days until the TLS certificate expires. If the certificate expires within this threshold, the check reports state `down`.
- **Scheduler**: The bounded worker pool that polls due monitors and dispatches checks.
- **Registry**: The checker registry that maps monitor type strings to Checker implementations.

## Requirements

### Requirement 1: Register gRPC Checker in Registry

**User Story:** As a platform operator, I want the gRPC checker registered at startup, so that monitors with type `grpc` are dispatched to the correct checker implementation.

#### Acceptance Criteria

1. WHEN the application starts, THE Registry SHALL contain a checker registered under the type string `grpc` that implements the Checker interface.
2. WHEN the Scheduler encounters a monitor with type `grpc`, THE Scheduler SHALL resolve the checker from the Registry and invoke its Check method against the monitor's target.
3. IF no checker is registered for a monitor's type, THEN THE Scheduler SHALL persist a check result with state `down` and an error message indicating the unknown type, update the monitor's state to `down`, and set `next_check_at` to the current time plus the monitor's configured interval so that scheduling continues on the next cycle.

### Requirement 2: Basic gRPC Connectivity Check

**User Story:** As a user, I want to monitor gRPC endpoints, so that I am alerted when a gRPC service becomes unreachable.

#### Acceptance Criteria

1. WHEN a gRPC monitor is executed, THE GRPC_Checker SHALL establish a gRPC connection to the Target (formatted as `host:port`) within the timeout defined by the monitor's `timeout_seconds` (range: 1–300 seconds).
2. WHEN the connection is established and no `service_method` is specified in settings, THE GRPC_Checker SHALL invoke the standard gRPC health check method `grpc.health.v1.Health/Check`.
3. WHEN the connection is established and a `service_method` is specified in settings, THE GRPC_Checker SHALL invoke the specified method as a unary gRPC call with an empty request body.
4. WHEN the gRPC call returns a status code that is present in the monitor's `expected_status_codes` list (default: `[OK]` when not configured), THE GRPC_Checker SHALL report state `up` with the measured round-trip latency in milliseconds.
5. IF the gRPC connection fails (dial error, timeout, or connection refused), THEN THE GRPC_Checker SHALL report state `down` with an error message indicating the failure cause.
6. IF the gRPC call returns a status code that is not present in the monitor's `expected_status_codes` list, THEN THE GRPC_Checker SHALL report state `down` with the gRPC status code name and server-provided message.
7. IF `tls_enabled` is set to true in the monitor's settings, THEN THE GRPC_Checker SHALL establish the connection using TLS; otherwise THE GRPC_Checker SHALL connect using plaintext.

### Requirement 3: Custom Service and Method Invocation

**User Story:** As a user, I want to specify a custom gRPC service and method, so that I can health-check any RPC endpoint beyond the standard health service.

#### Acceptance Criteria

1. WHEN `service_method` is set to a non-empty value in GRPC_Settings, THE GRPC_Checker SHALL invoke that fully-qualified service and method on the target server.
2. THE GRPC_Checker SHALL accept Service_Method values in `package.Service/Method` format where both the service segment (before the `/`) and the method segment (after the `/`) are non-empty strings with a maximum combined length of 512 characters.
3. IF the Service_Method value does not contain exactly one `/` separator, or if either the service segment or method segment is empty, THEN THE GRPC_Checker SHALL report state `down` with an error indicating invalid method format.
4. WHEN the specified service or method does not exist on the server, THE GRPC_Checker SHALL report state `down` with the UNIMPLEMENTED (12) status code in the error.
5. IF `service_method` is set to an empty or whitespace-only string in GRPC_Settings, THEN THE GRPC_Checker SHALL treat it as unset and fall back to the default `grpc.health.v1.Health/Check` method.

### Requirement 4: TLS Configuration

**User Story:** As a user, I want to control TLS behavior for gRPC connections, so that I can monitor both plaintext and TLS-secured gRPC services.

#### Acceptance Criteria

1. WHEN `tls_mode` is set to `tls` in GRPC_Settings, THE GRPC_Checker SHALL establish a TLS-secured connection with certificate verification enabled against the system certificate authority pool.
2. WHEN `tls_mode` is set to `plaintext` in GRPC_Settings, THE GRPC_Checker SHALL establish an insecure plaintext connection.
3. WHEN `tls_mode` is set to `tls_skip_verify` in GRPC_Settings, THE GRPC_Checker SHALL establish a TLS connection with certificate verification disabled.
4. WHEN `tls_mode` is not specified in GRPC_Settings, THE GRPC_Checker SHALL default to `tls`.
5. IF TLS handshake fails due to certificate validation error, THEN THE GRPC_Checker SHALL report state `down` with an error message that includes the failure reason (expired certificate, hostname mismatch, or unknown certificate authority).
6. IF `tls_mode` is set to a value other than `tls`, `plaintext`, or `tls_skip_verify`, THEN THE GRPC_Checker SHALL report state `down` with an error message indicating the unrecognized TLS mode value.

### Requirement 5: TLS Certificate Monitoring

**User Story:** As a user, I want to track certificate expiry for gRPC endpoints, so that I am warned before a certificate expires and causes service disruption.

#### Acceptance Criteria

1. WHEN TLS_Mode is `tls` or `tls_skip_verify`, THE GRPC_Checker SHALL extract the leaf certificate from the TLS connection and calculate the number of days until expiry as a truncated integer (hours until `NotAfter` divided by 24, rounded toward zero).
2. WHEN TLS_Mode is `tls` or `tls_skip_verify` and the TLS connection provides peer certificates, THE GRPC_Checker SHALL report the certificate days remaining in the check result `ssl_days_remaining` field as an int32 value (negative values indicate an already-expired certificate).
3. WHEN `ssl_expiry_threshold` is set in GRPC_Settings to a value between 1 and 3650 (inclusive) and the certificate expires within that number of days (days remaining is less than or equal to the threshold), THE GRPC_Checker SHALL report state `down` with an error message indicating the days remaining and the configured threshold.
4. WHEN `ssl_expiry_threshold` is not set in GRPC_Settings, THE GRPC_Checker SHALL report certificate days remaining without failing the check based on expiry.
5. WHEN TLS_Mode is `plaintext`, THE GRPC_Checker SHALL not attempt certificate extraction and SHALL leave `ssl_days_remaining` unset.
6. IF TLS_Mode is `tls` or `tls_skip_verify` and the TLS connection provides no peer certificates, THEN THE GRPC_Checker SHALL leave `ssl_days_remaining` unset and SHALL not fail the check due to missing certificate information.

### Requirement 6: Request Metadata

**User Story:** As a user, I want to send custom metadata with gRPC calls, so that I can authenticate or route requests to services that require specific headers.

#### Acceptance Criteria

1. WHEN `metadata` is set in GRPC_Settings as a map of string keys to string values, THE GRPC_Checker SHALL attach all key-value pairs as outgoing gRPC metadata on the unary call.
2. THE GRPC_Checker SHALL support up to 20 metadata entries per request, where each key is at most 128 characters and each value is at most 4096 characters.
3. IF a metadata key contains invalid characters (non-lowercase-ASCII-alphanumeric, non-hyphen, non-underscore, non-dot characters) or starts with the reserved `grpc-` prefix, THEN THE GRPC_Checker SHALL report state `down` with an error message indicating which key failed validation.
4. IF a metadata key ends with the `-bin` suffix and its value is not valid base64 encoding, THEN THE GRPC_Checker SHALL report state `down` with an error message indicating the invalid binary metadata value.
5. WHEN multiple values are provided for the same metadata key, THE GRPC_Checker SHALL send all values for that key as a multi-value metadata entry in the order they appear.

### Requirement 7: Expected Status Codes

**User Story:** As a user, I want to define which gRPC status codes mean "up", so that I can monitor services that intentionally return non-OK codes for health probes.

#### Acceptance Criteria

1. WHEN `expected_statuses` is set in GRPC_Settings with at least one entry, THE GRPC_Checker SHALL consider the service `up` if the returned GRPC_Status_Code is in the expected list.
2. WHEN `expected_statuses` is not set or is empty in GRPC_Settings, THE GRPC_Checker SHALL consider only status code OK (0) as `up`.
3. THE GRPC_Checker SHALL accept expected status codes as a list of integers with values in the range 0–16 inclusive and a maximum of 17 entries.
4. IF any value in `expected_statuses` is outside the range 0–16, THEN THE GRPC_Checker SHALL report state `down` with an error message indicating an invalid expected status code value.

### Requirement 8: Request Body Payload

**User Story:** As a user, I want to send a request payload with the gRPC call, so that I can invoke methods that require input messages.

#### Acceptance Criteria

1. WHEN `request_payload` is set in GRPC_Settings as a non-empty string, THE GRPC_Checker SHALL decode the value using standard base64 (RFC 4648 §4) and send the decoded bytes as the raw request message body.
2. WHEN `request_payload` is not set or is an empty string, THE GRPC_Checker SHALL send a zero-length byte payload as the request message body.
3. IF `request_payload` is not valid standard base64, THEN THE GRPC_Checker SHALL report state `down` with an error indicating payload decode failure.
4. IF the decoded `request_payload` exceeds 1 MB (1,048,576 bytes), THEN THE GRPC_Checker SHALL report state `down` with an error indicating the payload size limit exceeded.

### Requirement 9: Database Schema Update

**User Story:** As a platform operator, I want the database to accept `grpc` as a valid monitor type, so that gRPC monitors can be persisted.

#### Acceptance Criteria

1. THE database migration SHALL replace the `monitors_type_check` constraint with one that allows the values `http`, `tcp`, `udp`, `websocket`, and `grpc`.
2. WHEN a monitor with type `grpc` is inserted, THE database SHALL accept the record without constraint violation.
3. WHEN the migration runs on a database containing existing monitors, THE migration SHALL preserve all existing rows unchanged and keep previously valid type values (`http`, `tcp`, `udp`, `websocket`) accepted by the new constraint.
4. IF the migration is rolled back while monitors with type `grpc` exist, THEN THE down migration SHALL delete monitors with type `grpc` before restoring the previous constraint that excludes `grpc`.

### Requirement 10: OpenAPI Specification Update

**User Story:** As an API consumer, I want the OpenAPI spec to document the `grpc` monitor type and its settings schema, so that I can create gRPC monitors programmatically.

#### Acceptance Criteria

1. THE OpenAPI specification SHALL include `grpc` in the `type` enum of the `CreateMonitorRequest`, `PutMonitorRequest`, and `Monitor` schemas.
2. THE OpenAPI specification SHALL define a `GRPCSettings` schema object with the following fields and types: `service_method` (string, required, format: `package.Service/Method`), `tls_mode` (string, required, enum: `plaintext`, `tls`, `tls_skip_verify`), `ssl_expiry_threshold` (integer, optional, days until certificate expiry that triggers warning, minimum 1, maximum 3650), `metadata` (object, optional, additionalProperties of type string representing key-value header pairs, maximum 20 keys), `expected_statuses` (array of integers, optional, each value in range 0–16 representing gRPC status codes, maximum 17 items), and `request_payload` (string, optional, base64-encoded protobuf message, maximum length 65536 characters).
3. THE OpenAPI specification SHALL reference the `GRPCSettings` schema from the `settings` property description or `oneOf`/`discriminator` on the `CreateMonitorRequest`, `PutMonitorRequest`, and `Monitor` schemas when `type` is `grpc`.
4. THE OpenAPI specification SHALL include a request body `example` on the `createMonitor` operation demonstrating gRPC monitor creation with at minimum the required fields (`name`, `type`, `target`, `settings.service_method`, `settings.tls_mode`) and at least one optional settings field populated.

### Requirement 11: Latency Measurement

**User Story:** As a user, I want accurate latency reporting for gRPC checks, so that I can track service performance over time.

#### Acceptance Criteria

1. THE GRPC_Checker SHALL measure latency from the start of the gRPC dial to the receipt of the unary response, using monotonic clock truncated to whole milliseconds.
2. THE GRPC_Checker SHALL report latency in whole milliseconds as an int32 value, where fractional milliseconds are truncated (floored) toward zero.
3. WHEN the check fails due to timeout, THE GRPC_Checker SHALL report the elapsed time up to the timeout as latency.
4. IF the check fails due to a non-timeout error (connection refused, DNS resolution failure, or TLS handshake error), THEN THE GRPC_Checker SHALL still report the elapsed time as latency.

### Requirement 12: Context and Timeout Compliance

**User Story:** As a platform operator, I want gRPC checks to respect timeout and cancellation, so that the scheduler can shut down gracefully and long-running checks do not block the worker pool.

#### Acceptance Criteria

1. THE GRPC_Checker SHALL use the context deadline provided by the Scheduler as the gRPC call timeout.
2. WHEN the context is cancelled, THE GRPC_Checker SHALL abort the in-progress connection or call and return within 500 milliseconds of the cancellation signal.
3. WHEN the context is cancelled, THE GRPC_Checker SHALL report state `down` with an error message indicating cancellation and SHALL include the elapsed time as latency.
4. IF the check exceeds the timeout, THEN THE GRPC_Checker SHALL report state `down` with an error message indicating timeout and SHALL include the elapsed time as latency.
5. WHEN the check completes due to context cancellation or timeout, THE GRPC_Checker SHALL close the underlying gRPC connection before returning.
