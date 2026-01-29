# caido-mcp-server

A Model Context Protocol (MCP) server that provides comprehensive access to [Caido](https://caido.io/) proxy features. This allows LLM-based tools like Claude Code to browse, analyze, and interact with HTTP requests captured by Caido.

## Features

- **List & get requests** with HTTPQL filtering and field selection
- **Send requests** via Replay functionality
- **Replay sessions** - list, get entries, send custom requests
- **Automate sessions** - access fuzzing results and payloads
- **Findings** - list and create security findings
- **Sitemap** - browse discovered endpoints hierarchically
- **Scopes** - manage target scopes
- **OAuth authentication** with automatic token refresh

## Installation

Build from source:

```bash
git clone https://github.com/c0tton-fluff/caido-mcp-server.git
cd caido-mcp-server
go build -o caido-mcp-server .
```

## Usage

### 1. Authenticate with Caido

```bash
env CAIDO_URL=http://localhost:8080 ./caido-mcp-server login
```

This will:
1. Open a browser to the Caido authentication page
2. Wait for you to complete authentication
3. Save the token to `~/.caido-mcp/token.json`

### 2. Configure MCP Client

Add to your `~/.mcp.json`:

```json
{
  "mcpServers": {
    "caido": {
      "command": "/path/to/caido-mcp-server",
      "args": ["serve"],
      "env": {
        "CAIDO_URL": "http://127.0.0.1:8080"
      }
    }
  }
}
```

### 3. Use with Claude Code

Once configured, you can ask Claude Code to:

- "List all POST requests to the API"
- "Send this request with modified parameters"
- "Show me the sitemap for target.com"
- "Create a finding for this IDOR vulnerability"
- "What fuzzing payloads were tried in Automate session 1?"

## MCP Tools

### Proxy History

#### caido_list_requests
List proxied HTTP requests with optional HTTPQL filtering.

| Parameter | Type | Description |
|-----------|------|-------------|
| `httpql` | string | HTTPQL filter query |
| `limit` | int | Max requests (default 20, max 100) |
| `after` | string | Cursor for pagination |

#### caido_get_request
Get detailed information about HTTP request(s).

| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Request IDs to retrieve |
| `include` | string[] | Fields: `metadata`, `requestHeaders`, `requestBody`, `responseHeaders`, `responseBody` |
| `bodyOffset` | int | Byte offset for body |
| `bodyLimit` | int | Byte limit for body |

### Replay

#### caido_list_replay_sessions
List all Replay sessions.

#### caido_get_replay_entry
Get a Replay entry with request/response details.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Replay entry ID |

#### caido_send_request
Send an HTTP request via Replay.

| Parameter | Type | Description |
|-----------|------|-------------|
| `raw` | string | Full HTTP request (headers + body) |
| `host` | string | Target host (optional if in Host header) |
| `port` | int | Target port (default: 443/80) |
| `tls` | bool | Use HTTPS (default: true) |
| `sessionId` | string | Replay session ID (default: "1") |

### Automate (Fuzzing)

#### caido_list_automate_sessions
List all Automate fuzzing sessions.

#### caido_get_automate_session
Get Automate session with entry list.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Automate session ID |

#### caido_get_automate_entry
Get Automate entry with fuzz results.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Automate entry ID |
| `first` | int | Max results (default 10) |
| `after` | string | Cursor for pagination |

### Findings

#### caido_list_findings
List security findings.

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | int | Max findings |
| `after` | string | Cursor for pagination |
| `filter` | string | HTTPQL filter |

#### caido_create_finding
Create a new finding.

| Parameter | Type | Description |
|-----------|------|-------------|
| `requestId` | string | Associated request ID |
| `title` | string | Finding title |
| `description` | string | Finding description |

### Sitemap

#### caido_get_sitemap
Get sitemap entries (root or children).

| Parameter | Type | Description |
|-----------|------|-------------|
| `parentId` | string | Parent entry ID (omit for root) |

### Scopes

#### caido_list_scopes
List all defined scopes.

#### caido_create_scope
Create a new scope.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Scope name |
| `allowlist` | string[] | Allowed URL patterns |
| `denylist` | string[] | Denied URL patterns |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CAIDO_URL` | Caido instance URL (e.g., `http://127.0.0.1:8080`) |

## Troubleshooting

### GraphQL Errors

If you see errors like `argument "X" is required but not provided`:

1. Check MCP logs: `~/.cache/claude-cli-nodejs/*/mcp-logs-caido/`
2. Verify parameter names match [Caido's GraphQL schema](https://github.com/caido/graphql-explorer)
3. Rebuild after fixes: `go build -o caido-mcp-server .`
4. Restart Claude Code to reload the MCP server

### Common Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `sessionId required` | Parameter name mismatch | Use `sessionId` not `replaySessionId` |
| `parentId required` | Parameter name mismatch | Use `parentId` not `id` for sitemap |
| `depth required` | Missing enum param | Add `depth: "DIRECT"` or `"ALL"` |
| `Invalid token` | Token expired | Run `./caido-mcp-server login` again |

## License

MIT
