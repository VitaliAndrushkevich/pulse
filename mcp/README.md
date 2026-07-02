# Pulse MCP Server

MCP server exposing Pulse monitoring capabilities to AI clients. Implements the
[Model Context Protocol](https://modelcontextprotocol.io/) using the official Go SDK and
communicates with Pulse exclusively through its REST API (`/api/v1`).

The server runs as an independent binary — it never imports backend packages or touches the
database directly.

## Build

```bash
cd mcp
make build   # produces bin/pulse-mcp
make test    # run all tests
make lint    # requires golangci-lint
```

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PULSE_MCP_API_TOKEN` | **yes** | — | Pulse Bearer API token (generate in Pulse Settings → API Tokens) |
| `PULSE_MCP_API_BASE_URL` | no | `http://localhost:8080/api/v1` | Base URL of the Pulse REST API |
| `PULSE_MCP_ACCESS_MODE` | no | `read-only` | `read-only` or `read-write`. Unrecognized values default to `read-only` |
| `PULSE_MCP_TRANSPORT` | no | `stdio` | `stdio` (default, local single-client) or `http` (Streamable HTTP, networked) |
| `PULSE_MCP_HTTP_ADDR` | no | `:9090` | Bind address when transport is `http` |
| `PULSE_MCP_REQUEST_TIMEOUT` | no | `15s` | Per-request timeout for Pulse API calls (Go duration) |

The server aborts at startup if `PULSE_MCP_API_TOKEN` is absent, empty, or whitespace-only.

## Usage

### Claude Desktop

Add to your Claude Desktop MCP configuration (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "pulse": {
      "command": "/path/to/pulse-mcp",
      "env": {
        "PULSE_MCP_API_TOKEN": "your-pulse-api-token",
        "PULSE_MCP_API_BASE_URL": "http://localhost:8080/api/v1",
        "PULSE_MCP_ACCESS_MODE": "read-only"
      }
    }
  }
}
```

### Kiro

Add to your Kiro MCP configuration:

```json
{
  "mcpServers": {
    "pulse": {
      "command": "/path/to/pulse-mcp",
      "env": {
        "PULSE_MCP_API_TOKEN": "your-pulse-api-token",
        "PULSE_MCP_API_BASE_URL": "http://localhost:8080/api/v1",
        "PULSE_MCP_ACCESS_MODE": "read-only"
      }
    }
  }
}
```

### Cursor

Add to your Cursor MCP settings (`.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "pulse": {
      "command": "/path/to/pulse-mcp",
      "env": {
        "PULSE_MCP_API_TOKEN": "your-pulse-api-token",
        "PULSE_MCP_API_BASE_URL": "http://localhost:8080/api/v1",
        "PULSE_MCP_ACCESS_MODE": "read-write"
      }
    }
  }
}
```

Replace `/path/to/pulse-mcp` with the absolute path to the built binary
(e.g. `./mcp/bin/pulse-mcp` from the project root).

## Available Tools

The server exposes the following tools depending on the configured access mode:

### Read Tools (always available)

| Tool | Description |
|------|-------------|
| `list-monitors` | List monitors with optional type/tag filters and pagination |
| `get-monitor` | Get a single monitor's configuration and current status by ID or name |
| `monitor-stats` | Get uptime percentage, last error, and SSL certificate expiry for a monitor |
| `monitor-history` | Query check history points for a monitor over a time range |
| `downtime-summary` | Summarize downtime periods within a rolling window (default 24h) |
| `list-incidents` | List incidents globally or per monitor with pagination and open-only filter |

### Write Tools (read-write mode only)

| Tool | Description |
|------|-------------|
| `create-monitor` | Create a simple health-check monitor (HTTP, TCP, UDP, or ICMP) |

## Architecture

```
MCP Client ──(stdio/HTTP)──▶ pulse-mcp ──(REST + Bearer)──▶ Pulse API /api/v1
```

The server is a thin adapter: it validates inputs, resolves monitor names to IDs,
calls the Pulse API, and projects results into MCP tool responses. All state lives in
Pulse — the MCP server is stateless.
