# caido-mcp-server

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/c0tton-fluff/caido-mcp-server)](https://github.com/c0tton-fluff/caido-mcp-server/releases)

MCP server for [Caido](https://caido.io/) proxy integration. Enables AI assistants like Claude Code to browse, analyze, and interact with HTTP traffic.

## Features

- **Proxy history** — List and search requests with HTTPQL filtering
- **Replay** — Send HTTP requests, manage sessions
- **Automate** — Access fuzzing results and payloads
- **Findings** — Create and list security findings
- **Sitemap** — Browse discovered endpoints
- **Scopes** — Manage target definitions
- **OAuth** — Automatic token refresh

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/c0tton-fluff/caido-mcp-server/main/install.sh | bash
```

Or download from [Releases](https://github.com/c0tton-fluff/caido-mcp-server/releases).

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/c0tton-fluff/caido-mcp-server.git
cd caido-mcp-server
go build -o caido-mcp-server .
```
</details>

## Quick Start

**1. Authenticate**

```bash
CAIDO_URL=http://localhost:8080 caido-mcp-server login
```

**2. Configure MCP client**

Add to `~/.mcp.json`:

```json
{
  "mcpServers": {
    "caido": {
      "command": "caido-mcp-server",
      "args": ["serve"],
      "env": {
        "CAIDO_URL": "http://127.0.0.1:8080"
      }
    }
  }
}
```

**3. Use with Claude Code**

```
"List all POST requests to /api"
"Send this request with a modified user ID"
"Create a finding for this IDOR"
"Show fuzzing results from Automate session 1"
```

## Tools Reference

### Proxy

| Tool | Description |
|------|-------------|
| `caido_list_requests` | List requests with HTTPQL filter, pagination |
| `caido_get_request` | Get request details (headers, body, response) |

### Replay

| Tool | Description |
|------|-------------|
| `caido_send_request` | Send raw HTTP request |
| `caido_list_replay_sessions` | List Replay sessions |
| `caido_get_replay_entry` | Get Replay entry details |

### Automate

| Tool | Description |
|------|-------------|
| `caido_list_automate_sessions` | List fuzzing sessions |
| `caido_get_automate_session` | Get session with entry list |
| `caido_get_automate_entry` | Get fuzz results and payloads |

### Findings & Scope

| Tool | Description |
|------|-------------|
| `caido_list_findings` | List security findings |
| `caido_create_finding` | Create finding for a request |
| `caido_get_sitemap` | Browse sitemap hierarchy |
| `caido_list_scopes` | List defined scopes |
| `caido_create_scope` | Create new scope |

<details>
<summary>Full parameter reference</summary>

### caido_list_requests
| Parameter | Type | Description |
|-----------|------|-------------|
| `httpql` | string | HTTPQL filter query |
| `limit` | int | Max requests (default 20, max 100) |
| `after` | string | Pagination cursor |

### caido_get_request
| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Request IDs |
| `include` | string[] | `requestHeaders`, `requestBody`, `responseHeaders`, `responseBody` |
| `bodyOffset` | int | Byte offset |
| `bodyLimit` | int | Byte limit |

### caido_send_request
| Parameter | Type | Description |
|-----------|------|-------------|
| `raw` | string | Full HTTP request |
| `host` | string | Target host |
| `port` | int | Target port |
| `tls` | bool | Use HTTPS (default: true) |
| `sessionId` | string | Replay session ID |

### caido_get_automate_entry
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Entry ID |
| `limit` | int | Max results |
| `after` | string | Pagination cursor |

### caido_create_finding
| Parameter | Type | Description |
|-----------|------|-------------|
| `requestId` | string | Associated request |
| `title` | string | Finding title |
| `description` | string | Finding description |

### caido_create_scope
| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Scope name |
| `allowlist` | string[] | URL patterns to include |
| `denylist` | string[] | URL patterns to exclude |

</details>

## Troubleshooting

| Error | Fix |
|-------|-----|
| `Invalid token` | Run `caido-mcp-server login` again |
| `sessionId required` | Use `sessionId` not `replaySessionId` |
| `depth required` | Add `depth: "DIRECT"` or `"ALL"` |

Check MCP logs: `~/.cache/claude-cli-nodejs/*/mcp-logs-caido/`

## License

MIT
