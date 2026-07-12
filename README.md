<p align="center">
  <img src="https://raw.githubusercontent.com/caido/caido/main/brand/png/logo.png" alt="Caido" width="120"/>
</p>

<h1 align="center">caido-mcp-server</h1>

<p align="center">
  MCP server and CLI for <a href="https://caido.io">Caido</a> web proxy - browse, replay, and analyze HTTP traffic from AI assistants or your terminal.
</p>

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/c0tton-fluff/caido-mcp-server/releases"><img src="https://img.shields.io/github/v/release/c0tton-fluff/caido-mcp-server" alt="Release"></a>
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-compatible-8A2BE2" alt="MCP"></a>
  <a href="https://github.com/c0tton-fluff/caido-mcp-server/actions/workflows/ci.yml"><img src="https://github.com/c0tton-fluff/caido-mcp-server/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

---

## What It Does

Two ways to interact with your Caido proxy:

- **MCP Server** - expose 66 tools and 6 read-only resources to AI assistants (Claude Code, Cursor, etc.) via the Model Context Protocol
- **CLI** - standalone terminal client for pentesters who prefer the command line

Both share the same auth token, the same Go SDK, and the same codebase.

## Features

| Category | Capabilities |
|----------|-------------|
| **Proxy History** | Search requests with HTTPQL, get full request/response details, diff two responses |
| **Replay** | Send HTTP requests, get response inline (status, headers, body). Per-session cookie jar auto-persists `Set-Cookie` between calls |
| **Automate** | Access fuzzing sessions, results, and payloads. Start/pause/resume/cancel tasks |
| **Findings** | Create, list, delete, and export security findings |
| **Sitemap** | Browse discovered endpoints |
| **Scopes** | Full lifecycle: create, rename, delete target scope definitions; check if a host/URL is in scope |
| **Projects** | Full lifecycle: create, rename, select, delete projects |
| **Workflows** | List, run, and toggle automation workflows |
| **Tamper** | List, create, toggle, and delete Match & Replace rules |
| **Intercept** | Check status, pause/resume, list/forward/drop intercepted requests |
| **Environments** | Create, select, delete variable environments (tokens, keys) |
| **Filters** | Create, list, and delete saved HTTPQL filter presets |
| **Hosted Files** | List payload files served by Caido |
| **Tasks** | List and cancel running background tasks |
| **Plugins** | List installed plugin packages |
| **Instance** | Get Caido version and platform info |

**Built-in security and performance:**

- Credential redaction - Authorization, Cookie, and API key headers are redacted in tool output by default (including raw request/response dumps and the `caido://requests/{id}` resource); opt out with `CAIDO_ALLOW_SENSITIVE_HEADERS` (see [Revealing sensitive headers](#revealing-sensitive-headers))
- Tool annotations - every tool declares `readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint` so MCP clients can distinguish read-only, destructive, and external-network tools
- Session cookie jar - RFC 6265 jar per replay session; `Set-Cookie` from a response is auto-attached to the next `send_request` against the same session
- Response fingerprinting - auto-detects content kind (json/html/xml/text/binary) so agents know what they're dealing with
- Adaptive body limits - JSON gets 4KB, HTML 3KB, binary 200B (override with explicit `bodyLimit`)
- Response diff - repeated identical responses in the same session collapse to a one-line summary, saving tokens
- Input validation - length limits on all string inputs to prevent context flooding
- Token auto-refresh - expired OAuth tokens refresh mid-session automatically
- Session reuse - single replay session per server lifetime, no sprawl

### Session cookie jar

The `caido_send_request` tool maintains an in-memory `http.CookieJar` per replay session. Cookies set via `Set-Cookie` in any response are stored and auto-injected into subsequent requests targeting the same RFC 6265 domain/path. Pass `useCookieJar: false` to a single call to disable injection (useful for session-fixation testing or to verify auth gates). Use `caido_clear_session_cookies` to wipe a session jar between test runs and `caido_get_session_cookies` to introspect what is stored (cookie values are not returned, only metadata).

The output of `caido_send_request` includes a `cookieJar` block with `injectedCookies` (names sent on this call) and `storedCookies` (names captured from `Set-Cookie`), so the LLM can verify the chain stayed authenticated.

### Revealing sensitive headers

By default, sensitive headers (`Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization`, `X-Api-Key`, `X-Auth-Token`, `X-CSRF-Token`, `X-XSRF-Token`) are replaced with `[REDACTED]` in tool output to avoid leaking credentials into the model context. On an authorized engagement where you need the real values — to analyze or replay a captured authenticated request, or to produce a working `caido_export_curl` PoC — set `CAIDO_ALLOW_SENSITIVE_HEADERS` to a truthy value (`1`, `true`):

```json
{
  "mcpServers": {
    "caido": {
      "command": "caido-mcp-server",
      "args": ["serve"],
      "env": {
        "CAIDO_URL": "http://127.0.0.1:8080",
        "CAIDO_ALLOW_SENSITIVE_HEADERS": "true"
      }
    }
  }
}
```

When enabled, real credential values flow through tool output to the model; leave it unset to keep redaction. This toggle does not affect the session cookie jar, which only ever reports cookie names and metadata, never values.

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

**Option A: Static access token (recommended)**

This server talks to the **local Caido app's [GraphQL API](https://docs.caido.io/app/concepts/graphql#authentication)**, which authenticates with the **access token from your Caido login session** — *not* a Caido Cloud [Personal Access Token](https://docs.caido.io/dashboard/concepts/pat). A Cloud PAT (prefixed `caido_`) is for the cloud/dashboard API and will **not** authenticate against your local instance.

Grab the access token from the Caido GUI: open developer tools (`CTRL`+`SHIFT`+`I`) and run this in the Console tab:

```javascript
JSON.parse(localStorage.CAIDO_AUTHENTICATION).accessToken
```

Pass it via the `CAIDO_ACCESS_TOKEN` environment variable. No login command needed.

```json
{
  "mcpServers": {
    "caido": {
      "command": "caido-mcp-server",
      "args": ["serve"],
      "env": {
        "CAIDO_URL": "http://127.0.0.1:8080",
        "CAIDO_ACCESS_TOKEN": "your-caido-access-token"
      }
    }
  }
}
```

> **Note:** this token expires after ~7 days; for a long-lived setup use **Option B** (OAuth), which refreshes automatically. The older `CAIDO_PAT` variable is still accepted as a deprecated alias for `CAIDO_ACCESS_TOKEN`.

**Option B: OAuth device flow**

```bash
CAIDO_URL=http://localhost:8080 caido-mcp-server login
```

This opens your browser for OAuth authentication and saves the token to `~/.caido-mcp/token.json`. Then configure your MCP client:

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

### MCP Tools (66)

| Tool | Description |
|------|-------------|
| `caido_list_requests` | List requests with HTTPQL filter and pagination |
| `caido_get_request` | Get request details (metadata, headers, body). 2KB body limit default |
| `caido_diff_responses` | Structural diff of two responses by Caido request ID: status/size change flags and a compact body/header summary (never dumps full bodies) |
| `caido_send_request` | Send HTTP request via Replay, returns response inline. Polls up to 10s. Auto-injects session cookies and persists `Set-Cookie` (toggle with `useCookieJar`) |
| `caido_batch_send` | Send multiple requests in parallel (BAC sweeps, parameter fuzzing, endpoint sweeps). Max 50 per batch |
| `caido_edit_request` | Modify and resend an existing request. Preserves auth/cookies while changing method, path, headers, or body |
| `caido_export_curl` | Convert a request to an executable curl command for PoC reports |
| `caido_create_replay_session` | Create a named replay session, optionally seed with a request |
| `caido_list_replay_sessions` | List replay sessions |
| `caido_delete_replay_sessions` | Bulk delete replay sessions by ID |
| `caido_move_replay_session` | Move a session to a different collection |
| `caido_get_replay_entry` | Get replay entry with response. 2KB body limit default |
| `caido_clear_session_cookies` | Wipe the in-memory cookie jar for a replay session |
| `caido_get_session_cookies` | List metadata for cookies stored in a session jar matching a URL (values not returned) |
| `caido_list_replay_collections` | List replay session collections |
| `caido_create_replay_collection` | Create a named replay collection |
| `caido_rename_replay_collection` | Rename a replay collection |
| `caido_delete_replay_collection` | Delete a replay collection |
| `caido_list_automate_sessions` | List fuzzing sessions |
| `caido_get_automate_session` | Get session details with entry list |
| `caido_get_automate_entry` | Get fuzz results and payloads |
| `caido_automate_task_control` | Start/pause/resume/cancel fuzzing tasks |
| `caido_list_findings` | List security findings |
| `caido_create_finding` | Create finding linked to a request |
| `caido_delete_findings` | Delete findings by IDs or reporter name |
| `caido_export_findings` | Export findings for reporting |
| `caido_get_sitemap` | Browse sitemap hierarchy |
| `caido_list_scopes` | List target scopes |
| `caido_is_in_scope` | Check whether a host or URL is in the project scope; returns the matching scope and the allow/deny rule that decided it |
| `caido_create_scope` | Create new scope with allow/deny lists |
| `caido_rename_scope` | Rename a scope |
| `caido_delete_scope` | Delete a scope |
| `caido_list_projects` | List projects, marks current |
| `caido_select_project` | Switch active project |
| `caido_create_project` | Create a new project |
| `caido_rename_project` | Rename a project |
| `caido_delete_project` | Delete a project |
| `caido_list_workflows` | List automation workflows |
| `caido_run_workflow` | Execute an active or convert workflow |
| `caido_toggle_workflow` | Enable or disable a workflow |
| `caido_list_tamper_rules` | List Match & Replace rule collections |
| `caido_create_tamper_rule` | Create a tamper rule in a collection |
| `caido_update_tamper_rule` | Update an existing tamper rule |
| `caido_toggle_tamper_rule` | Enable or disable a tamper rule |
| `caido_delete_tamper_rule` | Delete a tamper rule |
| `caido_get_instance` | Get Caido version and platform info |
| `caido_intercept_status` | Get intercept status (PAUSED/RUNNING) |
| `caido_intercept_control` | Pause or resume intercept |
| `caido_list_intercept_entries` | List queued intercept entries with HTTPQL filtering |
| `caido_forward_intercept` | Forward intercepted request, optionally with modifications |
| `caido_drop_intercept` | Drop intercepted request |
| `caido_list_environments` | List environments and their variables |
| `caido_select_environment` | Switch active environment |
| `caido_create_environment` | Create a new environment |
| `caido_delete_environment` | Delete an environment |
| `caido_list_filters` | List saved HTTPQL filter presets |
| `caido_create_filter` | Save an HTTPQL query as a named filter preset |
| `caido_delete_filter` | Delete a filter preset |
| `caido_list_hosted_files` | List hosted payload files |
| `caido_list_tasks` | List running background tasks |
| `caido_cancel_task` | Cancel a running task by ID |
| `caido_list_plugins` | List installed plugin packages |
| `caido_list_ws_streams` | List WebSocket streams (connections) from the WebSocket tab |
| `caido_list_ws_messages` | List WebSocket frames for a stream (direction/format/decoded body) |
| `caido_convert_body` | Convert a request body between JSON, form-urlencoded, XML, and multipart |
| `caido_race_window_send` | Fire raw HTTP/1.1 requests with synchronized last-byte send for race-condition testing (bypasses Caido proxy) |

### MCP Resources (6)

Read-only data exposed via the MCP resources protocol. Agents can read these without consuming tool calls.

| URI | Description |
|-----|-------------|
| `caido://requests/{id}` | Full HTTP request and response for a given request ID |
| `caido://replay-sessions/{id}` | Replay session details with entry list |
| `caido://sitemap` | Root domains from the sitemap |
| `caido://findings` | Security finding summaries (up to 100) |
| `caido://scopes` | All target scopes with their allow/deny rules |
| `caido://project` | Current project, instance version, and connection status |

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
| `bodyLimit` | int | Response body byte limit (default 2000) |
| `bodyOffset` | int | Response body byte offset (default 0) |
| `useCookieJar` | bool | Auto-inject session cookies and persist `Set-Cookie` (default true); set false to disable for this call only |
| `includeBody` | bool | Include response body text (default true); the fingerprint is always populated |
| `marker` | string | String to search for in the response body; when set, `output.reflected` reports whether it was found |

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

#### caido_list_intercept_entries

| Parameter | Type | Description |
|-----------|------|-------------|
| `filter` | string | HTTPQL filter query |
| `limit` | int | Max entries (default 20, max 100) |
| `after` | string | Pagination cursor |

#### caido_forward_intercept

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Intercept entry ID (required) |
| `raw` | string | Modified raw HTTP request (base64-encoded, optional) |

#### caido_drop_intercept

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Intercept entry ID (required) |

#### caido_automate_task_control

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `start`, `pause`, `resume`, or `cancel` (required) |
| `session_id` | string | Automate session ID (required for start) |
| `task_id` | string | Automate task ID (required for pause/resume/cancel) |

#### caido_delete_findings

| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Finding IDs to delete |
| `reporter` | string | Delete all findings by this reporter |

#### caido_export_findings

| Parameter | Type | Description |
|-----------|------|-------------|
| `ids` | string[] | Finding IDs to export |
| `reporter` | string | Export all findings by this reporter |

#### caido_list_environments

No parameters required. Returns all environments with variables and selected/global context.

#### caido_select_environment

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Environment ID (required, empty string to deselect) |

#### caido_run_workflow

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Workflow ID (required) |
| `type` | string | `active` or `convert` (required) |
| `request_id` | string | Request ID (required for active workflows) |
| `input` | string | Input data (required for convert workflows) |

#### caido_toggle_workflow

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Workflow ID (required) |
| `enabled` | bool | Enable or disable (required) |

#### caido_list_tamper_rules

No parameters required. Returns all tamper rule collections with nested rules.

#### caido_create_tamper_rule

| Parameter | Type | Description |
|-----------|------|-------------|
| `collection_id` | string | Collection ID (required) |
| `name` | string | Rule name (required) |
| `condition` | string | HTTPQL filter condition |
| `sources` | string[] | Traffic sources: INTERCEPT, REPLAY, AUTOMATE, IMPORT, PLUGIN, WORKFLOW, SAMPLE |

#### caido_toggle_tamper_rule

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Tamper rule ID (required) |
| `enabled` | bool | Enable or disable (required) |

#### caido_delete_tamper_rule

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Tamper rule ID (required) |

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

Requires authentication - run `caido-mcp-server login` first to store a token. (The CLI reads the stored login token; it does not consume the `CAIDO_ACCESS_TOKEN` env var that the MCP server uses.)

```bash
# Check connection and auth
caido-cli status -u http://localhost:8080

# Send structured requests
caido-cli send GET https://target.com/api/users
caido-cli send POST https://target.com/api/login -j '{"user":"admin","pass":"test"}'
caido-cli send PUT https://target.com/api/profile -H "Authorization: Bearer tok" -j '{"role":"admin"}'

# Send raw HTTP requests
caido-cli raw 'GET /api/users HTTP/1.1\r\nHost: target.com\r\n\r\n'
caido-cli raw -f request.txt --host target.com --port 8443
echo -n 'GET / HTTP/1.1\r\nHost: example.com\r\n\r\n' | caido-cli raw -

# Parallel requests via Replay (BAC sweeps, param fuzzing, endpoint sweeps)
caido-cli batch sweep https://target.com/api/profile -t "owner=eyJ1...,cross=eyJ2...,noauth"
caido-cli batch fuzz "https://target.com/api/search?q=test" -p q -v "test,test',1 OR 1=1" -H "Authorization: Bearer eyJ..."
caido-cli batch ep -t eyJ... https://target.com/dashboard https://target.com/admin
caido-cli batch file batch.json

# Browse proxy history
caido-cli history
caido-cli history -f 'req.host.eq:"target.com"' -n 20

# Get full request/response details
caido-cli request 12345

# Encode/decode
caido-cli encode base64 "hello world"
caido-cli decode url "%3Cscript%3E"
caido-cli encode hex "test"
```

### Commands

| Command | Description |
|---------|-------------|
| `status` | Check Caido instance health and auth token |
| `send METHOD URL` | Send structured HTTP request via Replay API |
| `raw` | Send raw HTTP request (argument, file with `-f`, or stdin with `-`) |
| `batch MODE` | Parallel requests via Replay: `sweep` (N tokens), `fuzz` (N values), `ep` (N URLs), `file` (JSON spec) |
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
    auth/         OAuth device flow, static access token (CAIDO_ACCESS_TOKEN), token store, auto-refresh
    httputil/     HTTP parsing, fingerprinting, response diff, CRLF normalization
    replay/       Replay session management, cookie jar, response polling
    resources/    MCP read-only resources (requests, sessions, sitemap, findings)
    tools/        MCP tool definitions (one file per tool)
    testutil/     Mock GraphQL server, MCP test helpers, fixtures
```

Both `cmd/mcp` and `cmd/cli` share `internal/` packages. The project uses [caido-community/sdk-go](https://github.com/caido-community/sdk-go) for all GraphQL communication with Caido.

---

## Troubleshooting

| Error | Fix |
|-------|-----|
| `Invalid token` | `CAIDO_ACCESS_TOKEN` must be the local Caido **access token** (not a Cloud PAT) — re-grab it from the GUI console, or run `caido-mcp-server login` again |
| `token expired, no refresh token` | The static access token expires after ~7 days; re-grab it into `CAIDO_ACCESS_TOKEN`, or use `caido-mcp-server login` (OAuth auto-refreshes) |
| `poll failed: timed out` | Target server slow; use `get_replay_entry` with the returned `entryId` |
| `no authentication token found` | Set `CAIDO_ACCESS_TOKEN` env var or run `caido-mcp-server login` before `serve` |

MCP server logs: `~/.cache/claude-cli-nodejs/*/mcp-logs-caido/`

---

## Security

Sensitive HTTP headers (Authorization, Cookie, Set-Cookie, API keys) are redacted everywhere output leaves the server - structured tool output, raw request/response dumps, fuzz templates, and the `caido://requests/{id}` resource all pass through a single redaction choke-point to prevent credential leakage to LLM context. On an authorized engagement you can opt out with `CAIDO_ALLOW_SENSITIVE_HEADERS` (see [Revealing sensitive headers](#revealing-sensitive-headers)). All string inputs are length-validated server-side, and request batch sizes are capped.

Access tokens (via `CAIDO_ACCESS_TOKEN`) and OAuth tokens are stored with 0600 permissions and never appear in process arguments or log output.

To report a security issue, open a GitHub issue or contact the maintainer directly.

---

## Contributing

1. Fork the repo
2. Create a feature branch
3. `go build ./...` and `go test ./... -race`
4. Open a PR (CI runs build, test, vet, staticcheck)

### Local pre-push gate

A tracked `.githooks/pre-push` runs the same checks as CI's `lint` + `test` jobs (gofmt, `go vet`, `golangci-lint run ./...`, `go test ./... -race`) so failures are caught before they reach CI. Enable it once per clone:

```
git config core.hooksPath .githooks
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

Built with [caido-community/sdk-go](https://github.com/caido-community/sdk-go) and [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk).

## License

[MIT](LICENSE)
