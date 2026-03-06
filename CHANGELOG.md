# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-03-06

### Added
- `send_request` returns response inline (status code, headers, body) - no extra tool calls needed
- Response body polling with 10s timeout and fallback to `get_replay_entry`
- `get_replay_entry` now supports `bodyLimit` and `bodyOffset` parameters
- Token auto-refresh mid-session via callback (no more expired token failures)
- Replay session reuse - single session per server lifetime with automatic fallback
- IPv6 host support (`[::1]:8080`)

### Changed
- `send_request` output now includes `requestId`, `entryId`, `statusCode`, `roundtripMs`, parsed `request`/`response`
- `get_replay_entry` defaults to 2KB body limit (matching `get_request`)
- `ParsedHTTPMessage` and `parseHTTPMessage` extracted to shared `http_utils.go`

### Removed
- Unused `urlEncode` function from send_request
- Unused `RequestSummary` struct from types
- `TaskID` field from send_request output (not useful to LLM callers)

## [1.0.0] - 2026-01-30

### Added
- Initial release
- OAuth authentication with automatic token refresh
- 14 MCP tools for Caido integration:
  - `caido_list_requests` - List proxied requests with HTTPQL filtering
  - `caido_get_request` - Get request details with field selection
  - `caido_send_request` - Send HTTP requests via Replay
  - `caido_list_replay_sessions` - List Replay sessions
  - `caido_get_replay_entry` - Get Replay entry details
  - `caido_list_automate_sessions` - List Automate fuzzing sessions
  - `caido_get_automate_session` - Get Automate session details
  - `caido_get_automate_entry` - Get fuzzing results
  - `caido_list_findings` - List security findings
  - `caido_create_finding` - Create new findings
  - `caido_get_sitemap` - Browse sitemap hierarchy
  - `caido_list_scopes` - List target scopes
  - `caido_create_scope` - Create new scopes
- Pre-built binaries for macOS, Linux, Windows (amd64/arm64)
