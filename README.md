<p align="center">
  <img src="https://caido.io/images/logo.png" alt="Caido" width="120"/>
</p>

<h1 align="center">caido-mcp-server</h1>

<p align="center">
  MCP server and CLI for <a href="https://caido.io">Caido</a> web proxy - browse, replay, and analyze HTTP traffic from AI assistants or your terminal.
</p>

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/c0tton-fluff/caido-mcp-server/releases"><img src="https://img.shields.io/github/v/release/c0tton-fluff/caido-mcp-server" alt="Release"></a>
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-compatible-8A2BE2" alt="MCP"></a>
</p>

---

## What It Does

Two ways to interact with your Caido proxy:

- **MCP Server** - expose Caido tools to AI assistants (Claude Code, Cursor, etc.) via the Model Context Protocol
- **CLI** - standalone terminal client for pentesters who prefer the command line

Both share the same auth token, the same Go SDK, and the same codebase.

## Features

| Category | Capabilities |
|----------|-------------|
| **Proxy History** | Search requests with HTTPQL, get full request/response details |
| **Replay** | Send HTTP requests, get response inline (status, headers, body) |
| **Automate** | Access fuzzing sessions, results, and payloads |
| **Findings** | Create and list security findings linked to requests |
| **Sitemap** | Browse discovered endpoints |
| **Scopes** | Create and manage target scope definitions |
| **Projects** | List and switch between projects |
| **Workflows** | List automation workflows |
| **Intercept** | Check status, pause/resume the intercepting proxy |
| **Filters** | List saved HTTPQL filter presets |
| **Instance** | Get Caido version and platform info |

**Built-in optimizations:**

- Token auto-refresh - expired tokens refresh mid-session automatically
- Session reuse - single replay session per server lifetime, no sprawl
- Body limits - response bodies capped at 2KB by default to save LLM context
- Minimal tool descriptions - optimized for low token overhead per API call

---

## MCP Server

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/c0tton-fluff/caido-mcp-server/main/install.sh | bash
```

Or download a pre-built binary from [Releases](https://github.com/c0tton-fluff/caido-mcp-server/releases) (macOS, Linux, Windows - amd64/arm64).

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/c0tton-fluff/caido-mcp-server.git
cd caido-mcp-server
go build -ldflags "-X main.version=$(git describe --tags)" -o caido-mcp-server ./cmd/mcp
```

</details>

### Quick Start

**1. Authenticate**

```bash
CAIDO_URL=http://localhost:8080 caido-mcp-server login
```

This opens your browser for OAuth device-flow authentication and saves the token to `~/.caido-mcp/token.json`.

**2. Configure your MCP client**

Add to `~/.mcp.json` (Claude Code) or your editor's MCP config:

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

**3. Use it**

```
"List all POST requests to /api"
"Send this request with a modified user ID"
"Create a finding for this IDOR"
"Show fuzzing results from Automate session 1"
"What's in scope?"
```

### MCP Tools (20)

| Tool | Description |
|------|-------------|
| `caido_list_requests` | List requests with HTTPQL filter and pagination |
| `caido_get_request` | Get request details (metadata, headers, body). 2KB body limit default |
| `caido_send_request` | Send HTTP request via Replay, returns response inline. Polls up to 10s |
| `caido_list_replay_sessions` | List replay sessions |
| `caido_get_replay_entry` | Get replay entry with response. 2KB body limit default |
| `caido_list_automate_sessions` | List fuzzing sessions |
| `caido_get_automate_session` | Get session details with entry list |
| `caido_get_automate_entry` | Get fuzz results and payloads |
| `caido_list_findings` | List security findings |
| `caido_create_finding` | Create finding linked to a request |
| `caido_get_sitemap` | Browse sitemap hierarchy |
| `caido_list_scopes` | List target scopes |
| `caido_create_scope` | Create new scope with allow/deny lists |
| `caido_list_projects` | List projects, marks current |
| `caido_select_project` | Switch active project |
| `caido_list_workflows` | List automation workflows |
| `caido_get_instance` | Get Caido version and platform info |
| `caido_intercept_status` | Get intercept status (PAUSED/RUNNING) |
| `caido_intercept_control` | Pause or resume intercept |
| `caido_list_filters` | List saved HTTPQL filter presets |

<details>
<summary>Parameter reference</summary>

#### caido_list_requests

| Parameter | Type | Description |
|-----------|------|-------------|
| `httpql` | string | HTTPQL filter query |
| `limit` | int | Max requests (default 20, max 100) |
| `after` | string | Pagination cursor |

#### caido_get_request

| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Request IDs (required) |
| `include` | string[] | `requestHeaders`, `requestBody`, `responseHeaders`, `responseBody` |
| `bodyOffset` | int | Byte offset |
| `bodyLimit` | int | Byte limit (default 2000) |

#### caido_send_request

| Parameter | Type | Description |
|-----------|------|-------------|
| `raw` | string | Full HTTP request (required) |
| `host` | string | Target host (overrides Host header) |
| `port` | int | Target port |
| `tls` | bool | Use HTTPS (default true) |
| `sessionId` | string | Replay session (auto-managed if omitted) |

#### caido_get_replay_entry

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Replay entry ID (required) |
| `bodyOffset` | int | Byte offset |
| `bodyLimit` | int | Byte limit (default 2000) |

#### caido_get_automate_entry

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Entry ID (required) |
| `limit` | int | Max results |
| `after` | string | Pagination cursor |

#### caido_create_finding

| Parameter | Type | Description |
|-----------|------|-------------|
| `requestId` | string | Associated request (required) |
| `title` | string | Finding title (required) |
| `description` | string | Finding description |

#### caido_create_scope

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Scope name (required) |
| `allowlist` | string[] | Hostnames to include, e.g. `example.com`, `*.example.com` (required) |
| `denylist` | string[] | Hostnames to exclude |

#### caido_select_project

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Project ID to switch to (required) |

#### caido_intercept_control

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `pause` or `resume` (required) |

</details>

---

## CLI

Standalone terminal client for Caido. No MCP required - use it directly from your shell.

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/c0tton-fluff/caido-mcp-server/main/install.sh | TOOL=cli bash
```

Or download from [Releases](https://github.com/c0tton-fluff/caido-mcp-server/releases).

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/c0tton-fluff/caido-mcp-server.git
cd caido-mcp-server
go build -o caido-cli ./cmd/cli
```

</details>

### Usage

Requires the same auth token as the MCP server - run `caido-mcp-server login` first.

```bash
# Check connection and auth
caido status -u http://localhost:8080

# Send structured requests
caido send GET https://target.com/api/users
caido send POST https://target.com/api/login -j '{"user":"admin","pass":"test"}'
caido send PUT https://target.com/api/profile -H "Authorization: Bearer tok" -j '{"role":"admin"}'

# Send raw HTTP requests
caido raw 'GET /api/users HTTP/1.1\r\nHost: target.com\r\n\r\n'
caido raw -f request.txt --host target.com --port 8443
echo -n 'GET / HTTP/1.1\r\nHost: example.com\r\n\r\n' | caido raw -

# Browse proxy history
caido history
caido history -f 'req.host.eq:"target.com"' -n 20

# Get full request/response details
caido request 12345

# Encode/decode
caido encode base64 "hello world"
caido decode url "%3Cscript%3E"
caido encode hex "test"
```

### Commands

| Command | Description |
|---------|-------------|
| `status` | Check Caido instance health and auth token |
| `send METHOD URL` | Send structured HTTP request via Replay API |
| `raw` | Send raw HTTP request (argument, file with `-f`, or stdin with `-`) |
| `history` | List proxy history with HTTPQL filtering |
| `request ID` | Get full request/response by ID |
| `encode TYPE VALUE` | Encode value (`url`, `base64`, `hex`) |
| `decode TYPE VALUE` | Decode value (`url`, `base64`, `hex`) |

### Global Flags

| Flag | Description |
|------|-------------|
| `-u, --url` | Caido instance URL (or set `CAIDO_URL`) |
| `-b, --body-limit` | Response body byte limit (default 2000) |

---

## Architecture

```
caido-mcp-server/
  cmd/
    mcp/          MCP server (stdio transport)
    cli/          Standalone CLI
  internal/
    auth/         OAuth device flow, token store, auto-refresh
    httputil/     HTTP parsing, CRLF normalization, URL handling
    replay/       Replay session management, response polling
    tools/        MCP tool definitions (one file per tool)
```

Both `cmd/mcp` and `cmd/cli` share `internal/` packages. The project uses [caido-community/sdk-go](https://github.com/caido-community/sdk-go) for all GraphQL communication with Caido.

---

## Troubleshooting

| Error | Fix |
|-------|-----|
| `Invalid token` | Run `caido-mcp-server login` again |
| `token expired, no refresh token` | Re-login -- token store has no refresh token |
| `poll failed: timed out` | Target server slow; use `get_replay_entry` with the returned `entryId` |
| `no authentication token found` | Run `caido-mcp-server login` before `serve` |

MCP server logs: `~/.cache/claude-cli-nodejs/*/mcp-logs-caido/`

---

## Contributing

1. Fork the repo
2. Create a feature branch
3. `go build ./...` and `go test ./...`
4. Open a PR

Built with [caido-community/sdk-go](https://github.com/caido-community/sdk-go) and [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk).

## License

[MIT](LICENSE)
