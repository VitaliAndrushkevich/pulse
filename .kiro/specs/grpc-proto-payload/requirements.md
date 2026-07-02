# Requirements Document

## Introduction

This feature replaces the raw base64 textarea for gRPC request payloads with a schema-aware payload editor. Users can upload `.proto` files or compiled FileDescriptorSet (`.desc`) files, or use gRPC Server Reflection to auto-discover service schemas. The UI then presents a structured JSON editor that understands the protobuf message types, enabling human-readable payload authoring without manual base64 encoding.

## Glossary

- **Proto_Source**: A mechanism that provides protobuf type information to the system — either an uploaded `.proto` file set, a compiled FileDescriptorSet binary, or a Server Reflection connection.
- **Schema_Registry**: The backend component that stores, parses, and resolves protobuf type definitions from uploaded Proto_Sources.
- **Payload_Editor**: The frontend component that renders a structured JSON editor informed by protobuf message field definitions.
- **FileDescriptorSet**: A pre-compiled binary file produced by `protoc --descriptor_set_out` containing all type information for one or more `.proto` files.
- **Server_Reflection**: A gRPC protocol extension that allows clients to query a server for its available services and message types at runtime without pre-existing `.proto` files.
- **Proto_JSON**: The canonical protobuf JSON mapping format (as defined by the Protocol Buffers JSON specification) used for human-readable payload input.
- **Monitor_Settings**: The JSON configuration object stored per monitor containing gRPC connection and request parameters.

## Requirements

### Requirement 1: Upload Proto Source Files

**User Story:** As a monitor operator, I want to upload `.proto` files or a compiled FileDescriptorSet so that the system knows the message schema for my gRPC service.

#### Acceptance Criteria

1. WHEN a user uploads one or more `.proto` files (up to 50 files per request) via the API, THE Schema_Registry SHALL parse them and store the extracted type definitions associated with the monitor, replacing any previously stored proto sources for that monitor.
2. WHEN a user uploads a compiled FileDescriptorSet binary via the API, THE Schema_Registry SHALL parse it and store the extracted type definitions associated with the monitor, replacing any previously stored proto sources for that monitor.
3. IF an uploaded `.proto` file has unresolved imports, THEN THE Schema_Registry SHALL reject the entire upload and return an error listing each unresolved import path.
4. IF an uploaded file is not a valid protobuf source or FileDescriptorSet, THEN THE Schema_Registry SHALL reject the upload and return an error indicating the parse failure reason and the name of the invalid file.
5. IF an upload would cause the total stored proto source size for a monitor to exceed 5 MB, THEN THE Schema_Registry SHALL reject the upload and return an error indicating the size limit has been exceeded.
6. WHEN proto sources are stored for a monitor, THE Schema_Registry SHALL expose a list of all available service methods and their request/response message types.

### Requirement 2: Server Reflection Schema Discovery

**User Story:** As a monitor operator, I want to use gRPC Server Reflection to auto-discover available services and message types so that I do not need to upload `.proto` files manually.

#### Acceptance Criteria

1. WHEN a user requests schema discovery via Server Reflection for a monitor target, THE Schema_Registry SHALL connect to the gRPC server, retrieve the full list of available services and their method request/response message types (including nested message definitions), and return the discovered service and method list to the caller.
2. WHEN Server Reflection succeeds, THE Schema_Registry SHALL store the discovered type definitions in FileDescriptorSet format associated with the monitor, replacing any previously stored proto source for that monitor.
3. IF the target gRPC server does not support Server Reflection, THEN THE Schema_Registry SHALL return an error indicating reflection is not available and SHALL NOT modify any existing stored proto source for the monitor.
4. IF the Server Reflection operation (connection establishment plus schema retrieval) exceeds 10 seconds total elapsed time, THEN THE Schema_Registry SHALL abort the operation and return a timeout error.
5. WHILE a monitor has TLS settings configured, THE Schema_Registry SHALL use those same TLS settings when connecting for Server Reflection.
6. IF the target gRPC server is unreachable (connection refused, DNS resolution failure, or network error other than timeout), THEN THE Schema_Registry SHALL return an error indicating the connection failure reason and SHALL NOT modify any existing stored proto source for the monitor.
7. IF Server Reflection succeeds but the server exposes zero application services (only the reflection service itself), THEN THE Schema_Registry SHALL return an error indicating no discoverable services were found.

### Requirement 3: Proto JSON Payload Input

**User Story:** As a monitor operator, I want to write my gRPC request payload as human-readable JSON instead of base64-encoded protobuf bytes so that I can easily understand and edit the payload.

#### Acceptance Criteria

1. WHEN a monitor has a Proto_Source configured and a request payload is provided in Proto_JSON format, THE Backend SHALL convert the Proto_JSON to binary protobuf bytes using the canonical protobuf JSON mapping before sending the gRPC request.
2. IF the Proto_JSON payload does not conform to the request message schema, THEN THE Backend SHALL return a validation error within the standard error envelope listing each field that fails validation, indicating for each whether the issue is an unknown field name, a type mismatch, or a malformed value.
3. THE Backend SHALL support both legacy base64-encoded payloads and Proto_JSON payloads, selecting the format based on a `payload_format` field in Monitor_Settings.
4. WHEN `payload_format` is set to `proto_json`, THE Backend SHALL validate the payload against the stored schema — checking JSON syntax, field name existence in the message definition, and value type compatibility — before executing the monitor check.
5. WHEN `payload_format` is set to `raw` or is absent, THE Backend SHALL treat the payload as base64-encoded bytes (existing behavior).
6. IF `payload_format` is set to `proto_json` and no Proto_Source is configured for the monitor, THEN THE Backend SHALL reject the monitor check with an error indicating that a Proto_Source is required for proto_json payload format, and SHALL NOT execute the gRPC request.
7. IF the Proto_JSON payload exceeds 1 MB in size, THEN THE Backend SHALL reject the payload with an error indicating the maximum allowed payload size.

### Requirement 4: Schema-Aware Payload Editor UI

**User Story:** As a monitor operator, I want the payload editor to show me the message structure with field names, types, and documentation so that I can compose valid payloads without referencing external documentation.

#### Acceptance Criteria

1. WHEN a monitor has a Proto_Source configured, THE Payload_Editor SHALL display a JSON editor pre-populated with the request message field names and their protobuf-defined default values (0 for numeric types, empty string for string, false for bool, first declared value for enum).
2. WHEN a user types in the Payload_Editor, THE Payload_Editor SHALL display field-level autocompletion suggestions based on the message schema within 200ms of the last keystroke, showing at most 20 suggestions at a time.
3. WHEN the user modifies content in the Payload_Editor, THE Payload_Editor SHALL display inline validation errors adjacent to each non-conforming field within 500ms of the last keystroke, indicating which schema constraint is violated.
4. WHEN the user's cursor is positioned at an enum field's value, THE Payload_Editor SHALL display the list of valid enum values with their numeric identifiers as autocompletion options.
5. WHEN the user hovers over or focuses a message field that has a documentation comment in the `.proto` source, THE Payload_Editor SHALL display that comment as a tooltip, truncated to 256 characters with an ellipsis if longer.
6. IF no Proto_Source is configured for a monitor, THEN THE Payload_Editor SHALL fall back to a plain textarea accepting base64-encoded payloads.
7. IF the Proto_Source for a monitor fails to parse or contains unresolvable imports, THEN THE Payload_Editor SHALL display an error message indicating the parse failure reason and fall back to the plain textarea mode.

### Requirement 5: Proto Source Management API

**User Story:** As a monitor operator, I want to manage proto sources through the REST API so that I can automate schema configuration programmatically.

#### Acceptance Criteria

1. THE API SHALL expose a `POST /api/v1/monitors/{id}/proto-source` endpoint that accepts file uploads (multipart/form-data) for `.proto` files or FileDescriptorSet binaries, and on success SHALL return the parsed proto source metadata including source type, discovered services, methods, and message type names.
2. THE API SHALL expose a `POST /api/v1/monitors/{id}/proto-source/reflect` endpoint that triggers Server Reflection schema discovery against the monitor's configured target, and on success SHALL return the discovered proto source metadata including services, methods, and message type names.
3. THE API SHALL expose a `GET /api/v1/monitors/{id}/proto-source` endpoint that returns the current proto source metadata including source type, available services, methods, and message type names.
4. IF no proto source is configured for the specified monitor, THEN THE API SHALL return a not-found error response on `GET /api/v1/monitors/{id}/proto-source`.
5. THE API SHALL expose a `DELETE /api/v1/monitors/{id}/proto-source` endpoint that removes the stored proto source and reverts the monitor to raw payload mode; if no proto source exists for the monitor, the endpoint SHALL return a success response (idempotent).
6. WHEN a proto source is deleted, THE Backend SHALL set `payload_format` to `raw` if it was previously `proto_json`.
7. IF the monitor ID in the request path does not reference an existing monitor, THEN THE API SHALL return a not-found error response for any proto-source endpoint.

### Requirement 6: Proto Source File Upload UI

**User Story:** As a monitor operator, I want a file upload interface in the monitor form so that I can easily provide `.proto` files or a FileDescriptorSet without using the API directly.

#### Acceptance Criteria

1. THE Payload_Editor SHALL display a file upload area that accepts files with `.proto` and `.desc` extensions, allowing the user to select up to 20 files per upload operation.
2. WHEN files are uploaded, THE Payload_Editor SHALL display a progress indicator until the backend responds with a success or error result.
3. WHEN upload and parsing succeed, THE Payload_Editor SHALL display the list of discovered fully-qualified service names and method names, and allow the user to select exactly one service method to associate with the monitor.
4. WHEN the user confirms a service method selection, THE Payload_Editor SHALL save the proto source and selected method to the monitor configuration and transition the editor to schema-aware mode.
5. IF file upload or parsing fails, THEN THE Payload_Editor SHALL display an error message indicating the failure reason returned by the backend, and preserve any previously configured proto source unchanged.
6. IF the monitor target address is configured, THEN THE Payload_Editor SHALL display an enabled "Use Server Reflection" button as an alternative to file upload.
7. IF the monitor target address is not configured, THEN THE Payload_Editor SHALL display a disabled "Use Server Reflection" button with a hint indicating that a target address is required.
8. WHEN Server Reflection is triggered and succeeds, THE Payload_Editor SHALL display the discovered services and methods using the same selection interface as file upload results.
9. IF Server Reflection fails, THEN THE Payload_Editor SHALL display an error message indicating the failure reason returned by the backend, and preserve any previously configured proto source unchanged.
10. IF a proto source already exists for the monitor, THEN THE Payload_Editor SHALL display the source type, original filenames, and discovered service names, with options to replace or remove the proto source.

### Requirement 7: Proto JSON Serialization Round-Trip

**User Story:** As a developer, I want the Proto_JSON-to-binary conversion to be lossless so that payloads are transmitted exactly as specified.

#### Acceptance Criteria

1. THE Schema_Registry SHALL serialize Proto_JSON to binary protobuf using the canonical protobuf JSON mapping rules (proto3 JSON specification: camelCase field names accepted on input, enum values as strings, 64-bit integers as strings, bytes as base64).
2. THE Schema_Registry SHALL deserialize binary protobuf to Proto_JSON using the canonical protobuf JSON mapping rules, emitting camelCase field names, enum values as strings, and 64-bit integers as decimal strings.
3. THE Schema_Registry SHALL treat two Proto_JSON objects as semantically equivalent when they contain the same field values after type normalization regardless of key ordering, whitespace differences, or explicit-zero vs. omitted-default representation.
4. WHEN a valid Proto_JSON payload conforming to a message schema is serialized to binary and then deserialized back to Proto_JSON, THE Schema_Registry SHALL produce a semantically equivalent JSON object (round-trip property).
5. IF the Proto_JSON input contains a field name or value that does not match the message schema, THEN THE Schema_Registry SHALL return a serialization error indicating the first mismatched field name and expected type.
6. IF the binary protobuf input is malformed or does not conform to the expected message descriptor, THEN THE Schema_Registry SHALL return a deserialization error indicating the byte offset or field number where parsing failed.
7. THE Schema_Registry SHALL generate a Proto_JSON template from a stored message schema containing all scalar fields set to their zero values, nested message fields as empty objects, repeated fields as empty arrays, map fields as empty objects, and oneof fields represented by their first option at zero value.

### Requirement 8: Database Storage for Proto Sources

**User Story:** As a system operator, I want proto source data persisted in the database so that schema information survives restarts and is available for all monitor checks.

#### Acceptance Criteria

1. THE Backend SHALL store at most one proto source record per monitor, containing the binary data (FileDescriptorSet format) in the PostgreSQL database associated with the monitor ID.
2. THE Backend SHALL store proto source metadata (source type being one of "upload" or "reflection", original filenames, discovered services, message types) as a JSON column alongside the binary data.
3. WHEN a monitor is deleted, THE Backend SHALL cascade-delete its associated proto source data.
4. IF a proto source upload or reflection result exceeds 5MB of binary data for a given monitor, THEN THE Backend SHALL reject the operation and return an error indicating the size limit has been exceeded, without modifying the existing stored data.
5. WHEN a new proto source is stored for a monitor that already has an existing proto source, THE Backend SHALL replace the existing proto source binary and metadata with the new data.
