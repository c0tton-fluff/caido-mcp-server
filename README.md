# caido-mcp-server

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/c0tton-fluff/caido-mcp-server)](https://github.com/c0tton-fluff/caido-mcp-server/releases)

MCP server for [Caido](https://caido.io/) proxy. Lets AI assistants browse, replay, and analyze HTTP traffic.

## Features

- **Proxy history** - Search requests with HTTPQL filtering
- **Replay** - Send HTTP requests, get response inline (status, headers, body)
- **Automate** - Access fuzzing sessions, results, and payloads
- **Findings** - Create and list security findings
- **Sitemap** - Browse discovered endpoints
- **Scopes** - Manage target definitions
- **Token auto-refresh** - Expired tokens refresh automatically mid-session
- **Session reuse** - Single replay session per server lifetime, no sprawl
- **Body limits** - Response bodies capped at 2KB by default to save context

## Install

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

**2. Configure MCP client** (`~/.mcp.json`)

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

**3. Use**

```
"List all POST requests to /api"
"Send this request with a modified user ID"
"Create a finding for this IDOR"
"Show fuzzing results from Automate session 1"
```

## Tools

| Tool | Description |
|------|-------------|
| `caido_list_requests` | List requests with HTTPQL filter, pagination |
| `caido_get_request` | Get request details (metadata, headers, body). 2KB body limit default |
| `caido_send_request` | Send HTTP request, returns response inline (status, headers, 2KB body). Polls up to 10s |
| `caido_list_replay_sessions` | List replay sessions |
| `caido_get_replay_entry` | Get replay entry with response. 2KB body limit default |
| `caido_list_automate_sessions` | List fuzzing sessions |
| `caido_get_automate_session` | Get session with entry list |
| `caido_get_automate_entry` | Get fuzz results and payloads |
| `caido_list_findings` | List security findings |
| `caido_create_finding` | Create finding linked to a request |
| `caido_get_sitemap` | Browse sitemap hierarchy |
| `caido_list_scopes` | List target scopes |
| `caido_create_scope` | Create new scope |

<details>
<summary>Parameter reference</summary>

### caido_list_requests
| Parameter | Type | Description |
|-----------|------|-------------|
| `httpql` | string | HTTPQL filter query |
| `limit` | int | Max requests (default 20, max 100) |
| `after` | string | Pagination cursor |

### caido_get_request
| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Request IDs (required) |
| `include` | string[] | `requestHeaders`, `requestBody`, `responseHeaders`, `responseBody` |
| `bodyOffset` | int | Byte offset |
| `bodyLimit` | int | Byte limit (default 2000) |

### caido_send_request
| Parameter | Type | Description |
|-----------|------|-------------|
| `raw` | string | Full HTTP request (required) |
| `host` | string | Target host (overrides Host header) |
| `port` | int | Target port |
| `tls` | bool | Use HTTPS (default true) |
| `sessionId` | string | Replay session (auto-managed if omitted) |

### caido_get_replay_entry
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Replay entry ID (required) |
| `bodyOffset` | int | Byte offset |
| `bodyLimit` | int | Byte limit (default 2000) |

### caido_get_automate_entry
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Entry ID (required) |
| `limit` | int | Max results |
| `after` | string | Pagination cursor |

### caido_create_finding
| Parameter | Type | Description |
|-----------|------|-------------|
| `requestId` | string | Associated request (required) |
| `title` | string | Finding title (required) |
| `description` | string | Finding description |

### caido_create_scope
| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Scope name (required) |
| `allowlist` | string[] | URL patterns to include (required) |
| `denylist` | string[] | URL patterns to exclude |

</details>

## Troubleshooting

| Error | Fix |
|-------|-----|
| `Invalid token` | Run `caido-mcp-server login` again |
| `token expired, no refresh token` | Re-login: token store has no refresh token |
| `poll failed: timed out` | Target server slow; use `get_replay_entry` with the returned `entryId` |

MCP logs: `~/.cache/claude-cli-nodejs/*/mcp-logs-caido/`

## License

MIT
