# Changelog

All notable changes to this project will be documented in this file.

## [4.2.1] - 2026-07-08

### Changed
- Bump `modelcontextprotocol/go-sdk` v1.4.1 -> v1.6.1. Tool and input-validation errors now surface as spec-compliant MCP `isError` results instead of transport (Go) errors; server-side required-field enforcement is unchanged (still validated against the tool's input schema). No API changes to this server's tools.

## [4.2.0] - 2026-07-08

### Security
- **Bumped `modelcontextprotocol/go-sdk` v1.2.0 -> v1.4.1**, fixing 4 govulncheck-reported advisories (GO-2026-5771, GO-2026-4773, GO-2026-4770, GO-2026-4569). Requires Go 1.25+.
- **Raw-HTTP credential redaction is now enforced everywhere output leaves the server.** The `caido://requests/{id}` resource and `caido_get_automate_session` previously emitted base64-decoded raw requests/responses (and fuzz templates) with `Authorization`, `Cookie`, and `Set-Cookie` in cleartext, bypassing the redaction that only lived inside `ParseRaw`. A new `httputil.RedactRawHeaders` choke-point redacts sensitive header values in any raw dump (still honoring the `CAIDO_ALLOW_SENSITIVE_HEADERS` opt-out).

### Fixed
- **Response diff no longer hides real changes.** Two responses that shared the first `bodyLimit` bytes but differed in total size were reported "identical to previous response" (the hash is computed over the truncated body); the diff now also compares full body size.
- **Replay `send_request` no longer orphans an empty session.** The first send of each process auto-created a default replay session that was replaced but never deleted; the fallback now best-effort deletes it (and pool cleanup logs, rather than discards, delete errors).
- **OAuth login is context-cancellable.** The WebSocket token read could hang past a cancelled/expired context; a watcher now closes the connection on cancellation.
- **`caido_race_window_send` no longer stalls on keep-alive targets.** The response read was `io.ReadFull` on a fixed buffer with a 10s deadline, blocking ~10s per request against keep-alive servers; it is now EOF/idle-aware.

### Added
- **MCP tool annotations** on all tools (`readOnlyHint` / `destructiveHint` / `idempotentHint` / `openWorldHint`), so clients can distinguish read-only, destructive, and external-network tools (roadmap Chunk 5).
- **`caido_is_in_scope`** - check whether a host/URL is in the project scope, returning the matching rule (roadmap Chunk 8).
- **`caido_diff_responses`** - structural diff (status/size/body) of two Caido-native request IDs (roadmap Chunk 4).
- **`caido://scopes`** and **`caido://project`** read-only resources (roadmap Chunks 8 and 3). Tool/resource totals: 64 -> 66 tools, 4 -> 6 resources.
- **Richer response fingerprint** - `send_request`/`batch_send` now surface status, title, redirect target, Set-Cookie names, and word count; `includeBody` (default true single / false batch) and a `marker` reflected-in-response check (roadmap Chunk 4).
- Opt out of sensitive-header redaction via the `CAIDO_ALLOW_SENSITIVE_HEADERS` env var (#29).

### Changed
- `caido_create_replay_session` exposes the required `kind` field (#28).
- Internal DRY refactors (shared `clampLimit`/`pageCursor`/`DefaultPort` helpers, one `maxRawRequestBytes` constant) and new test coverage for `edit_request`, the replay-session `kind` GraphQL contract, the CLI token/request builders, and the `auth` package.

## [4.1.0] - 2026-06-16

### Fixed
- **Caido 0.57.0 compatibility** - 0.57.0 reshaped the replay and filter GraphQL contracts, breaking `caido_send_request`, `caido_edit_request`, batch send, and `caido_create_filter`. Adapted to the new API:
  - **Replay is now a draft-then-start flow.** `startReplayTask` dropped its `input` arg. Sending updates the active entry's draft on an existing session, or seeds a fresh session via `requestSource.raw` and then starts the task. A 0.57 session created with no request source has no entry, so an empty cached session transparently falls back to a seeded one.
  - **`ReplaySession` / `ReplayEntry` are now GraphQL interfaces** (HTTP/WS variants). The SDK unwraps them into stable domain structs, so tool/resource code reads plain fields.
  - **`@oneOf` inputs are strictly enforced** ("exactly one field"). This also broke `caido_create_filter` since `QueryInput` is `@oneOf`; fixed via hand-written `MarshalJSON` on the `@oneOf` inputs in sdk-go.
  - `streams` swapped its `protocol` arg for a StreamQL `filter`, so `caido_list_ws_streams` now filters to WS client-side.

### Changed (SDK alignment)
- Bumped `caido-community/sdk-go` to the 0.57-aware build (`f03a805`): regenerated against the 0.57 schema, replay SDK unwraps the interface types into stable domain structs, added the draft/start methods, and the `@oneOf` marshaling fix.

### Tests
- Test fixtures updated to the 0.57 interface shape (`__typename` on session/entry, seeded-create, update-draft). Added a live MCP-level send test (gated on `CAIDO_IT_URL`) verified end-to-end against a `caido/caido:0.57.0` instance.

## [4.0.0] - 2026-06-02

### Added
- **WebSocket history (read)** - exposes the WebSocket tab (the Go SDK did not wrap stream queries; see SDK alignment below):
  - `caido_list_ws_streams` - list WebSocket streams (connections), filterable by scope
  - `caido_list_ws_messages` - list frames of a stream with direction (CLIENT/SERVER), format (TEXT/BINARY), and decoded body (base64 `Blob` decoded, truncated to `body_limit`)
- **`caido_convert_body`** - convert a request body between JSON, form-urlencoded, XML, and multipart/form-data. Pure stdlib; flat objects are lossless, nested JSON uses bracket notation (`a[b]=c`).
- **`caido_race_window_send`** - fire multiple raw HTTP/1.1 requests with a synchronized last-byte send (single-packet / race-window style) for race-condition testing. Dials targets directly with raw sockets and parks all connections at a barrier before writing final bytes. NOTE: bypasses the Caido proxy, so these requests do not appear in Caido history.

### Changed
- Tool count 62 -> 64.

### CI
- **Release workflow** (`.github/workflows/release.yml`) - on a `v*` tag push, cross-compiles both binaries for 6 platforms via `scripts/build.sh`, generates `sha256sums.txt`, and publishes a GitHub Release with assets named to match `install.sh`.

### Tests
- Test coverage for the `internal/tools` package raised from ~23% to ~70%: new table-driven suites for Findings, Intercept, Workflows, Tamper, Projects/Scopes, Environments/Filters, Replay collections, Automate/Tasks, misc reads, and WebSocket tools. New `internal/httputil/bodyconvert` (92.9%) and `internal/raceattack` (88.0%) suites.

### Internal
- Documented the genqlient v0.8.1 oneof/omitempty limitation in `create_tamper_rule.go` as an upstream constraint (the `delete_findings`/`export_findings` tools use clean typed structs and need no workaround).

### Changed (SDK alignment)
- Added WebSocket/stream read queries to `caido-community/sdk-go` (new `StreamSDK`: `List`, `Get`, `ListWsMessages`) and bumped this server to that SDK build. `caido_list_ws_streams` and `caido_list_ws_messages` now use the typed SDK wrappers instead of hand-rolled raw GraphQL.

## [3.0.0] - 2026-05-21

### Added
- **`caido_edit_request` MCP tool** - modify an existing request (method, path, headers, body) while preserving all auth cookies and session state, then send it. The most-requested feature for AI-driven pentesting.
- **`caido_export_curl` MCP tool** - convert any request to a ready-to-use curl command for PoC reports.
- **Replay Collections** - full CRUD:
  - `caido_list_replay_collections` - list all collections
  - `caido_create_replay_collection` - create a named collection
  - `caido_rename_replay_collection` - rename a collection
  - `caido_delete_replay_collection` - delete a collection
- **`caido_delete_replay_sessions`** - bulk delete replay sessions by ID
- **`caido_move_replay_session`** - move a session between collections
- **Scope management** - full lifecycle:
  - `caido_rename_scope` - rename a scope
  - `caido_delete_scope` - delete a scope
- **Project management** - full CRUD:
  - `caido_create_project` - create a new project
  - `caido_rename_project` - rename a project
  - `caido_delete_project` - delete a project
- **Environment management**:
  - `caido_create_environment` - create a new environment
  - `caido_delete_environment` - delete an environment
- **Filter presets**:
  - `caido_create_filter` - save an HTTPQL query as a named preset
  - `caido_delete_filter` - delete a filter preset
- **Install script auto-version** - `install.sh` now auto-detects the latest release via GitHub API (fixes #19, #20)

### Changed
- Tool count: 42 -> 60 (18 new tools)
- Full CRUD coverage across all Caido resources (scopes, projects, environments, filters, replay collections)
- Every Caido SDK operation is now exposed as an MCP tool

## [2.0.0] - 2026-05-05

### Added
- **`caido_create_replay_session` MCP tool** - create named replay sessions with optional request seeding and collection assignment. Two-step flow: CreateSession then RenameSession.
- **MCP Resources** - expose Caido data as read-only resources for zero-tool-call context:
  - `caido://requests/{id}` - full request/response content
  - `caido://replay-sessions/{id}` - session details with entry list
  - `caido://sitemap` - root domains from sitemap
  - `caido://findings` - security finding summaries
- **Response fingerprinting** - responses include content kind (json/html/xml/text/binary) and content-type detection
- **Adaptive body limits** - auto-scale response body limits by content type (4KB JSON, 3KB HTML, 200B binary) when no explicit limit set
- **Response diff mode** - repeated identical responses in the same session return a compact diff summary instead of full body, saving agent tokens
- **`caido_list_hosted_files` MCP tool** - list hosted files for payload serving
- **`caido_list_tasks` MCP tool** - list running background tasks
- **`caido_cancel_task` MCP tool** - cancel a running task by ID
- **`caido_list_plugins` MCP tool** - list installed plugin packages
- **`caido_update_tamper_rule` MCP tool** - update an existing Match & Replace rule without deleting and recreating it
- **Windows binaries** - build pipeline now produces windows/amd64 and windows/arm64 .exe binaries
- **Test infrastructure** - 115 tests with race detection, mock GraphQL server, MCP in-memory transport testing, schema contract tests, GitHub Actions CI

### Changed
- Upgraded to sdk-go v0.5.0 (Caido 0.56 Query union types, HTTPQL/StreamQL split)
- Tool count: 37 -> 42 (5 new tools)
- Build targets: 8 -> 12 binaries (added Windows amd64/arm64)

## [1.5.0] - 2026-04-09

### Added
- **`caido_batch_send` MCP tool** - send multiple HTTP requests in parallel via session pool. Supports BAC token sweeps, parameter fuzzing, endpoint sweeps. Max 50 requests per batch, configurable concurrency (default 5, max 20). One tool call replaces N sequential `caido_send_request` calls.
- **`caido batch` CLI subcommand** - parallel HTTP through Caido Replay API. Four modes: `sweep` (same endpoint, N tokens), `fuzz` (same endpoint, N values), `ep` (N URLs, same auth), `file` (JSON batch spec). Drop-in replacement for `burp-batch` with identical interface.
- **Session pool** (`internal/replay/pool.go`) - manages N replay sessions for concurrent sends. Pre-creates sessions in parallel, acquire/release pattern with channel-based semaphore.
- **Batch engine** (`internal/replay/batch.go`) - shared by MCP tool and CLI. Handles session acquisition, CRLF normalization, host resolution, parallel polling.
- Auth mode support in CLI batch: `bearer` (default), `cookie:NAME`, `header:NAME` - matches `burp-batch` interface.

### Changed
- Upgraded sdk-go to v0.4.0 (`use_struct_references: false`) -- fixes 53 NON_NULL fields across generated types, removes manual GraphQL workarounds for `delete_findings`, `export_findings`, and `create_tamper_rule`.

### Fixed
- `caido_create_tamper_rule` -- nested oneof serialization for section, operation, matcher, and replacer fields now correct at all 4 levels of nesting.
- `caido_create_tamper_rule` -- `sources` field sent as `[]` (empty array) instead of omitted -- Caido API requires non-null.
- `caido_get_sitemap` -- null entries in sitemap response no longer cause a panic.
- `caido_run_workflow` -- Blob-encoded workflow output decoded correctly.
- `caido_list_environments` -- description field populated correctly.

## [1.4.0] - 2026-04-09

### Added
- **PAT authentication** - set `CAIDO_PAT` env var to skip OAuth device flow entirely (recommended for automation)
- **14 new MCP tools** (20 -> 34 total):
  - `caido_run_workflow` - execute active or convert workflows
  - `caido_toggle_workflow` - enable/disable automation workflows
  - `caido_list_tamper_rules` - list Match & Replace rule collections
  - `caido_create_tamper_rule` - create tamper rules with HTTPQL conditions
  - `caido_toggle_tamper_rule` - enable/disable tamper rules
  - `caido_delete_tamper_rule` - delete tamper rules
  - `caido_intercept_status` - get intercept status (PAUSED/RUNNING)
  - `caido_intercept_control` - pause or resume intercept
  - `caido_list_intercept_entries` - list queued intercept entries with HTTPQL filtering
  - `caido_forward_intercept` - forward intercepted request with optional modifications
  - `caido_drop_intercept` - drop intercepted request
  - `caido_list_environments` - list environments and variables
  - `caido_select_environment` - switch active environment
  - `caido_list_filters` - list saved HTTPQL filter presets
- Sensitive header redaction (Authorization, Cookie, Set-Cookie, API keys) in all tool output
- Input length validation on all string parameters
- Request ID batch cap (max 20 per call)
- Test coverage for header redaction

### Changed
- Bumped sdk-go to v0.3.0 (tamper rules SDK, workflow execution, WebSocket fix)
- Removed WebSocket endpoint workaround (fixed upstream in sdk-go)
- README rewritten with PAT auth as recommended setup, security section added

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
