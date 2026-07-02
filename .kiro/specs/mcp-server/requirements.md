# Requirements Document

## Introduction

This feature adds a Model Context Protocol (MCP) server for Pulse, enabling AI agents and
LLM-based clients to work with Pulse operational state through a standardized protocol. The
first version is intentionally small and covers a few high-value scenarios: check a monitor's
current status, find out whether a monitor had downtime in the last 24 hours, and create a
simple health-check monitor (HTTP, TCP, UDP, or ICMP) directly from an agent.

The MCP server runs as a small, separate service, independent from the main Pulse API binary,
reflecting the intent to keep the initial capability isolated and low-risk. It reads and writes
Pulse state exclusively through Pulse's existing REST API using a Pulse API token generated in
Pulse settings, so it never bypasses Pulse's security rules and never becomes a second source
of truth.

Because some scenarios modify Pulse state (creating monitors), the server's write capability is
configurable: an operator decides whether the server runs in read-only mode or read-write mode.
When write access is disabled, the server exposes only read tools.

This document defines WHAT the MCP server must do. Technical decisions (transport library,
whether it embeds a Pulse API client or calls over HTTP, deployment packaging) are deferred to
the design phase, except where captured here as explicit constraints.

## Glossary

- **MCP**: Model Context Protocol — an open protocol that lets LLM clients discover and invoke
  server-provided tools and read server-provided resources through a standardized message format.
- **Pulse_MCP_Server**: The new service that implements the MCP protocol and exposes Pulse
  capabilities to MCP clients. Referred to as "the server" in acceptance criteria.
- **MCP_Client**: Any MCP-compatible consumer (for example an AI agent or LLM desktop client)
  that connects to the Pulse_MCP_Server.
- **MCP_Tool**: A named, callable operation the Pulse_MCP_Server advertises to MCP_Clients,
  including its input schema and description.
- **Pulse_API**: The existing Pulse REST API under `/api/v1`, the authoritative source of
  monitor configuration and operational state.
- **API_Token**: A Pulse Bearer API token generated in Pulse settings (stored as a bcrypt hash,
  raw value shown only at creation) used for programmatic access to the Pulse_API.
- **Monitor**: A Pulse-configured health check with a stable UUID, type, target, interval,
  current status, and state.
- **Simple_Monitor_Type**: A monitor type supported for creation in the first version, limited
  to HTTP, TCP, UDP, and ICMP.
- **Incident**: A Pulse record of a monitor outage period, with open/resolved status and
  timestamps.
- **Check_History**: Time-series check results for a monitor, retained per the monitor's
  configured retention window (TimescaleDB-backed).
- **Downtime_Summary**: A derived answer describing whether, and for how long, a monitor was
  down within a requested recent window (default 24 hours).
- **Transport**: The channel over which MCP messages are exchanged (for example stdio or an
  HTTP-based streaming transport).
- **Access_Mode**: The configured capability level of the server — either read-only (read tools
  only) or read-write (read tools plus permitted write tools).

## Requirements

### Requirement 1: MCP Protocol Compliance and Tool Discovery

**User Story:** As an operator of an AI agent, I want the Pulse MCP server to speak the MCP
protocol correctly, so that any compatible MCP client can connect and discover what it can do.

#### Acceptance Criteria

1. WHEN an MCP_Client completes the MCP initialization handshake, THE Pulse_MCP_Server SHALL respond within 5 seconds with a protocol version identifier and a capabilities declaration listing its supported features.
2. WHEN an MCP_Client requests the list of available tools, THE Pulse_MCP_Server SHALL respond within 5 seconds with each MCP_Tool represented by a name that is unique within the response and 1 to 128 characters long, a human-readable description 1 to 1024 characters long, and an input schema.
3. WHEN an MCP_Client invokes an MCP_Tool with inputs that satisfy the tool's input schema, THE Pulse_MCP_Server SHALL return within 30 seconds a result that conforms to the MCP tool-result format.
4. IF an MCP_Client invokes a tool name that the Pulse_MCP_Server does not advertise, THEN THE Pulse_MCP_Server SHALL return an MCP error identifying the tool as unknown, SHALL NOT execute any tool, and SHALL leave server state unchanged.
5. IF an MCP_Client sends inputs that do not satisfy a tool's input schema, THEN THE Pulse_MCP_Server SHALL return an MCP error describing the validation failure, SHALL NOT execute the tool, and SHALL leave server state unchanged.
6. IF an MCP_Client requests a protocol version that the Pulse_MCP_Server does not support during the MCP initialization handshake, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the protocol version is unsupported and SHALL NOT complete the handshake.
7. IF the Pulse_MCP_Server cannot complete a tool invocation because the Pulse_API is unreachable, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the operation could not be completed and SHALL leave server state unchanged.

### Requirement 2: Separate, Independent Service

**User Story:** As the maintainer of Pulse, I want the MCP server to run as a separate service
independent from the main API, so that adding AI access does not increase risk or coupling in
the core monitoring binary.

#### Acceptance Criteria

1. THE Pulse_MCP_Server SHALL reach a ready-to-serve state and accept MCP_Client connections without requiring the main Pulse API process to be running.
2. THE Pulse_MCP_Server SHALL access Pulse operational state exclusively through the Pulse_API REST surface under `/api/v1`, and SHALL NOT read from or write to the Pulse databases directly.
3. WHILE the Pulse_MCP_Server is stopped, THE Pulse_API and monitoring engine SHALL continue operating with no interruption to scheduled check execution and no reduction in Pulse_API availability.
4. IF the Pulse_API does not accept a connection or does not respond within the configured request timeout (default 15 seconds) for a tool call, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the Pulse_API is unreachable.
5. WHILE the Pulse_API is unreachable, THE Pulse_MCP_Server SHALL continue accepting new MCP_Client connections without terminating the Pulse_MCP_Server process.

### Requirement 3: Authentication Using a Pulse Settings Token

**User Story:** As a Pulse administrator, I want the MCP server to access Pulse using an API
token I generate in Pulse settings, so that AI access is authenticated, auditable, and
revocable with the mechanism I already use.

#### Acceptance Criteria

1. THE Pulse_MCP_Server SHALL authenticate to the Pulse_API using a Pulse API_Token supplied through server configuration.
2. THE Pulse_MCP_Server SHALL accept an API_Token generated through the existing Pulse settings token-creation flow without requiring a new credential type.
3. IF no API_Token is configured at startup (the value is absent, empty, or whitespace-only), THEN THE Pulse_MCP_Server SHALL abort startup before opening its transport listener and SHALL return an error indicating that a Pulse API_Token is required.
4. WHILE no valid API_Token is configured, THE Pulse_MCP_Server SHALL NOT serve any MCP request.
5. WHEN the configured API_Token is rejected by the Pulse_API as invalid or revoked, THE Pulse_MCP_Server SHALL return an MCP error indicating that Pulse access is unauthorized and SHALL NOT retry the same request with the same API_Token.
6. THE Pulse_MCP_Server SHALL exclude the API_Token value from all log output, error messages, and MCP responses.

### Requirement 4: List Monitors

**User Story:** As an AI agent, I want to list Pulse monitors with their current status, so that
I can answer questions about what is being monitored and what is healthy.

#### Acceptance Criteria

1. WHEN an MCP_Client invokes the list-monitors tool, THE Pulse_MCP_Server SHALL return the set of monitors and, for each monitor, its identifier, name, type, target, current status (one of: up, down, pending), and current state (one of: active, paused).
2. WHERE the list-monitors tool receives a monitor-type filter, THE Pulse_MCP_Server SHALL return only monitors whose type matches the specified type using a case-insensitive comparison against the recognized types (HTTP, HTTPS, HTTP/3, TCP, UDP, WebSocket, gRPC, DNS, ICMP, SMTP).
3. WHERE the list-monitors tool receives a tag filter, THE Pulse_MCP_Server SHALL return only monitors that carry all specified tags.
4. WHEN the list-monitors tool is invoked, THE Pulse_MCP_Server SHALL accept a page parameter (minimum 1, default 1) and a limit parameter (minimum 1, maximum 100, default 50), and WHEN the number of matching monitors exceeds one page, THE Pulse_MCP_Server SHALL return paginated results and indicate how to request the next page.
5. IF a monitor-type filter value is not a recognized Pulse monitor type, THEN THE Pulse_MCP_Server SHALL return an MCP error listing the recognized types and SHALL NOT return any monitor set.
6. WHEN no monitors match the request, THE Pulse_MCP_Server SHALL return an empty monitor set with a count of 0 rather than an error.
7. WHERE the list-monitors tool receives both a monitor-type filter and a tag filter, THE Pulse_MCP_Server SHALL return only monitors that match the type and carry all specified tags.
8. IF the list-monitors tool receives a page or limit value outside its accepted range, THEN THE Pulse_MCP_Server SHALL return an MCP error stating the accepted range.

### Requirement 5: Check Monitor Status and Details

**User Story:** As an AI agent, I want to check a single monitor's current status and details, so
that I can report on a specific service on demand.

#### Acceptance Criteria

1. WHEN an MCP_Client invokes the get-monitor tool with a monitor identifier that matches exactly one existing monitor, THE Pulse_MCP_Server SHALL return that monitor's configuration and current status, including its latest check state and the timestamp of the most recent check, within 2 seconds.
2. WHEN an MCP_Client invokes the monitor-stats tool with a monitor identifier that matches exactly one existing monitor, THE Pulse_MCP_Server SHALL return the monitor's uptime percentage over the trailing 7-day window and its most recent recorded error within 2 seconds.
3. WHERE a monitor performs TLS-based checks, WHEN an MCP_Client invokes the monitor-stats tool for that monitor, THE Pulse_MCP_Server SHALL include the certificate expiry date in the response; WHERE a monitor does not perform TLS-based checks, THE Pulse_MCP_Server SHALL omit certificate expiry information from the response.
4. WHEN an MCP_Client invokes a monitor tool with a name that matches exactly one existing monitor using a case-sensitive exact string comparison, THE Pulse_MCP_Server SHALL resolve the name to that monitor's identifier and process the request.
5. IF a monitor name matches more than one existing monitor, THEN THE Pulse_MCP_Server SHALL return an MCP error that reports the ambiguity and lists the identifiers of all matching monitors, and SHALL NOT return any monitor data.
6. IF an MCP_Client provides a monitor identifier that matches no existing monitor, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the monitor was not found, and SHALL NOT return any monitor data.
7. IF a monitor name matches no existing monitor, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the monitor was not found, and SHALL NOT return any monitor data.

### Requirement 6: Query Monitor Check History

**User Story:** As an AI agent, I want to query a monitor's recent check history over a time
range, so that I can describe trends and recent behavior.

#### Acceptance Criteria

1. WHEN an MCP_Client invokes the monitor-history tool with an identifier that matches an existing monitor and a time range whose start is earlier than or equal to its end, THE Pulse_MCP_Server SHALL return the check history points recorded for that monitor whose timestamps fall within the requested time range.
2. WHERE the monitor-history tool receives no time range, THE Pulse_MCP_Server SHALL return check history for a default window covering the most recent 24 hours ending at the current server time.
3. WHEN the requested time range extends earlier than the 7-day history retention window, THE Pulse_MCP_Server SHALL return only the history points that fall within the retention window and SHALL include an indication that the requested range was truncated to the available retention window.
4. IF the requested time range has a start timestamp later than its end timestamp, THEN THE Pulse_MCP_Server SHALL reject the request and return an MCP error indicating that the time range is invalid, without returning any history points.
5. IF the monitor-history tool receives an identifier that does not match any existing monitor, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating that the monitor was not found, without returning any history points.
6. IF the monitor-history tool receives a monitor identifier that is not a well-formed identifier, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating that the monitor identifier is invalid, without returning any history points.
7. WHEN an existing monitor has no check history points within the requested time range, THE Pulse_MCP_Server SHALL return an empty history result rather than an error.

### Requirement 7: Downtime Summary for a Recent Window

**User Story:** As an AI agent, I want to ask whether a monitor had any downtime in the last 24
hours, so that I can give a direct yes/no answer with the outage details.

#### Acceptance Criteria

1. WHEN an MCP_Client invokes the downtime-summary tool with a monitor identifier that matches an existing monitor and no window, THE Pulse_MCP_Server SHALL return within 2 seconds a Downtime_Summary covering a rolling 24-hour window ending at the time the request is received.
2. THE Downtime_Summary SHALL state whether the monitor experienced downtime in the window (as a boolean), the number of downtime periods, and the total downtime duration in whole seconds, where a downtime period is one contiguous down-state interval bounded by an up-to-down and down-to-up transition or by a window edge.
3. WHERE the downtime-summary tool receives an explicit window, THE Pulse_MCP_Server SHALL compute the Downtime_Summary over that window, where the window duration is at least 60 seconds and at most the monitor's retention window.
4. WHEN the monitor had no downtime in the window, THE Pulse_MCP_Server SHALL return a Downtime_Summary with the downtime boolean set to false, zero downtime periods, and a total downtime duration of zero seconds.
5. WHEN the requested window extends earlier than the monitor's retention window, THE Pulse_MCP_Server SHALL compute the Downtime_Summary over the available retained data, set a truncation indicator, and report the effective start and end timestamps of the covered window.
6. IF the downtime-summary tool receives a monitor identifier that does not match any existing monitor, THEN THE Pulse_MCP_Server SHALL return an MCP error indicating the monitor was not found, without returning a Downtime_Summary.
7. IF the downtime-summary tool receives an explicit window that is malformed, non-positive, or shorter than 60 seconds, THEN THE Pulse_MCP_Server SHALL return an MCP error describing the invalid window, without returning a Downtime_Summary.

### Requirement 8: List Incidents

**User Story:** As an AI agent, I want to list incidents globally and per monitor, so that I can
report on outages and their resolution status.

#### Acceptance Criteria

1. WHEN an MCP_Client invokes the list-incidents tool, THE Pulse_MCP_Server SHALL return incidents ordered by start time descending and, for each incident, its identifier, associated monitor, start time (ISO 8601 UTC), resolution status (one of: open, resolved), and resolution time when resolved.
2. WHERE the list-incidents tool receives an open-only filter, THE Pulse_MCP_Server SHALL return only incidents whose status is open and SHALL exclude resolved incidents.
3. WHERE the list-incidents tool receives a valid monitor identifier, THE Pulse_MCP_Server SHALL return only incidents associated with that monitor, and WHERE both an open-only filter and a monitor identifier are provided, THE Pulse_MCP_Server SHALL apply both filters together.
4. WHEN the list-incidents tool is invoked, THE Pulse_MCP_Server SHALL accept a page parameter (minimum 1, default 1) and a limit parameter (minimum 1, maximum 100, default 20), and WHEN the number of matching incidents exceeds one page, THE Pulse_MCP_Server SHALL return paginated results with pagination metadata and indicate how to request the next page.
5. WHEN no incidents match the request, THE Pulse_MCP_Server SHALL return an empty incident collection with a total of 0 rather than an error.
6. WHEN an incident is open, THE Pulse_MCP_Server SHALL omit the resolution time; WHEN an incident is resolved, THE Pulse_MCP_Server SHALL include its resolution time.
7. IF the list-incidents tool receives a monitor identifier that is invalid or does not match any existing monitor, or a page or limit value outside its accepted range, THEN THE Pulse_MCP_Server SHALL return an MCP error stating the invalid parameter, without returning an incident collection.

### Requirement 9: Create a Simple Health-Check Monitor

**User Story:** As an AI agent operator, I want to create a simple health-check monitor from an
agent, so that I can add monitoring for a service without switching to the Pulse UI.

#### Acceptance Criteria

1. WHILE the server is in read-write Access_Mode, WHEN an MCP_Client invokes the create-monitor tool with a Simple_Monitor_Type (one of: HTTP, TCP, UDP, ICMP), a name of 1 to 255 characters, and a non-empty target, THE Pulse_MCP_Server SHALL create the monitor through the Pulse_API and return the created monitor's identifier and current status.
2. WHERE the create-monitor tool omits optional check interval or timeout values, THE Pulse_MCP_Server SHALL apply Pulse default values for those fields and reflect the applied defaults in the returned monitor record.
3. IF the create-monitor tool receives a monitor type outside the Simple_Monitor_Type set (HTTP, TCP, UDP, ICMP), THEN THE Pulse_MCP_Server SHALL reject the request without calling the Pulse_API and return an MCP error listing the supported Simple_Monitor_Type values.
4. IF the create-monitor tool receives a target that the Pulse_API rejects as invalid for the requested type, THEN THE Pulse_MCP_Server SHALL return an MCP error preserving the Pulse validation code and message, and no monitor SHALL be created.
5. IF the create-monitor tool is invoked WHILE the server is in read-only Access_Mode, THEN THE Pulse_MCP_Server SHALL reject the request without calling the Pulse_API, return an MCP error stating that write access is disabled, and no monitor SHALL be created.
6. IF the create-monitor tool receives a name that is missing, empty, or longer than 255 characters, THEN THE Pulse_MCP_Server SHALL reject the request without calling the Pulse_API and return an MCP error describing the name validation failure.

### Requirement 10: Configurable Access Mode

**User Story:** As the maintainer of Pulse, I want to decide whether the MCP server can modify
Pulse state, so that I can start read-only and enable write actions only when I choose.

#### Acceptance Criteria

1. WHEN the Pulse_MCP_Server starts, THE Pulse_MCP_Server SHALL read its Access_Mode from configuration, where a valid Access_Mode is exactly one of two values: read-only or read-write.
2. IF the Access_Mode is unspecified, empty, or not one of the two recognized values (read-only or read-write) at startup, THEN THE Pulse_MCP_Server SHALL set the effective Access_Mode to read-only.
3. WHILE the server is in read-only Access_Mode, THE Pulse_MCP_Server SHALL advertise and make available for invocation only read tools, where a read tool is a tool that does not modify Pulse state.
4. WHILE the server is in read-only Access_Mode, IF an MCP_Client invokes a write tool, where a write tool is a tool that modifies Pulse state, THEN THE Pulse_MCP_Server SHALL reject the invocation, return an error indicating that write actions are disabled, and leave Pulse state unchanged.
5. WHILE the server is in read-write Access_Mode, THE Pulse_MCP_Server SHALL advertise and make available for invocation read tools and the permitted write tools.
6. WHEN an MCP_Client requests the tool list, THE Pulse_MCP_Server SHALL return exactly the set of tools permitted by the current Access_Mode.

### Requirement 11: Secret and Sensitive Data Protection

**User Story:** As a security-conscious operator, I want the MCP server to never expose secrets,
so that AI access cannot leak credentials or sensitive configuration.

#### Acceptance Criteria

1. THE Pulse_MCP_Server SHALL exclude raw secret values, credential values, AES-256-GCM encryption keys, and raw API token values from every field of every MCP tool result.
2. THE Pulse_MCP_Server SHALL exclude authentication headers (including Authorization and Bearer token headers) and known secret fields (any field designated as a secret, credential, token, password, or encryption key) from all log output, replacing each excluded value with a fixed redaction placeholder that contains none of the original value's characters.
3. WHEN an MCP tool result includes a Pulse resource that carries credential metadata, THE Pulse_MCP_Server SHALL return only non-secret metadata for that resource (such as identifier, name, type, and creation timestamp) and SHALL omit every field holding a secret or credential value.
4. IF a value designated as a secret or credential cannot be redacted before being written to log output, THEN THE Pulse_MCP_Server SHALL suppress the entire log entry and record a redaction-failure indication that contains no secret value.
5. IF constructing an MCP tool result would require including a raw secret or credential value in any field, THEN THE Pulse_MCP_Server SHALL omit that field from the result, return the remaining non-secret fields, and include a response indication that the secret field was withheld.

### Requirement 12: Error Reporting and Diagnostics

**User Story:** As an operator running an MCP client, I want clear errors and basic diagnostics,
so that I can tell whether a failure is in Pulse, the MCP server, or my request.

#### Acceptance Criteria

1. IF a tool call fails because the Pulse_API returns an error envelope, THEN THE Pulse_MCP_Server SHALL return an MCP error whose code equals the Pulse error code and whose message equals the Pulse error message, without alteration or truncation.
2. WHEN the Pulse_MCP_Server returns an MCP error, THE Pulse_MCP_Server SHALL include a machine-readable code field and a human-readable message field.
3. WHEN the Pulse_MCP_Server returns an MCP error that originated from a Pulse_API response, THE Pulse_MCP_Server SHALL include the X-Request-ID value from that Pulse_API response so the caller can correlate the failure with the corresponding Pulse request.
4. IF a tool call fails because the Pulse_API is unreachable or does not respond within the configured request timeout, THEN THE Pulse_MCP_Server SHALL return an MCP error that includes a machine-readable code and a human-readable message indicating a Pulse_MCP_Server-to-Pulse_API connectivity or timeout failure, distinct from a Pulse_API-originated error.
5. WHEN the Pulse_MCP_Server completes startup and becomes ready to accept MCP_Client connections, THE Pulse_MCP_Server SHALL emit exactly one startup log entry stating the configured transport and the configured Access_Mode.

## Non-Goals (First Version)

The following are explicitly out of scope for the first version and recorded as future scope:

- Updating or deleting existing monitors (only creation of Simple_Monitor_Type monitors is in scope).
- Creating monitor types beyond HTTP, TCP, UDP, and ICMP (for example gRPC, DNS, SMTP, HTTP/3, WebSocket).
- Incident actions: acknowledging or resolving incidents.
- Managing Pulse tokens or credentials through MCP tools.
- Real-time streaming of status changes to MCP clients (the WebSocket hub remains the realtime channel for the frontend).
- Multi-tenant or per-user scoping of MCP access beyond the single configured API_Token.
- Exposing Prometheus metrics or dashboard-summary aggregates as MCP tools.

## Assumptions and Open Questions

Resolved during requirements review:

- **Access model**: The server reads and writes through the existing Pulse_API using an API_Token
  generated in Pulse settings (Requirements 2, 3). Confirmed.
- **Read vs. write**: V1 includes one write action — creating a simple health-check monitor —
  and write capability is configurable via Access_Mode (Requirements 9, 10). Confirmed.
- **First-version scenarios**: Check monitor status, downtime-in-last-24h summary, and create a
  simple monitor are the anchor scenarios; list/history/incidents tools support them
  (Requirements 4–9). Confirmed.

Still open for the design phase:

1. **Transport**: The intent is "very simple to start." The primary usage pattern (local
   single-client agent via stdio vs. a shared networked service via an HTTP-based transport)
   still needs confirmation, because it affects how MCP_Clients connect and how many connect at
   once.
2. **Simple_Monitor_Type creation inputs**: The minimal required inputs per type (for example
   whether HTTP requires only a URL, or also an expected status) will be finalized in design
   against the Pulse create-monitor contract.
