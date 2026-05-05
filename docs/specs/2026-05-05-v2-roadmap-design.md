# caido-mcp-server v2 Roadmap

## North Star

Make Caido's native project model legible and operable to agents with stable schemas, low token cost, and good tests. The MCP server is a thin, reliable Caido bridge -- not a scanner, state store, or safety framework.

## Design Principles

- Extend existing tools with parameters before creating new tools
- Surface Caido-native IDs only -- no MCP-side identity system
- Ephemeral server state (like cookie jars) dies with the process; persistent state lives in Caido
- Tools mutate or execute. Resources inspect.
- No pentest methodology encoded in Go -- the LLM composes workflows from primitives

## Explicitly Dropped

- MCP-generated IDs / parallel identity system
- SQLite or any persistence layer
- Prometheus / metrics endpoint
- Built-in vuln checks (IDOR, auth matrix, JWT replay)
- MCP-side scope safety rails duplicating Caido's scope system

---

## Chunk 1: Test Infra + CI

**Goal:** Protect against contributor regressions. Every PR gets automated verification.

**Deliverables:**
- GitHub Actions CI: `go build ./...`, `go test ./...`, `go vet ./...`, `staticcheck`
- Mock Caido client boundary (thin GraphQL mock, ~200 lines, stdlib only)
- Table-driven unit tests for core tools: `send_request`, `batch_send`, `get_request`, `list_requests`
- Schema contract tests for every MCP tool (validate input/output JSON schemas)
- Fixtures for raw HTTP parsing, cookie handling, body truncation, error responses
- Coverage badge in README

**Constraints:**
- stdlib `testing` + `net/http/httptest` only -- no testify, no gomock
- Mock server routes GraphQL operation names to fixture JSON files
- Integration tests behind `//go:build integration` tag (requires live Caido)
- Total shared test infra target: ~300-400 lines

**Done when:** CI runs on every PR, core tools >80% coverage.

---

## Chunk 2: Replay Session Support (Issue #12)

**Goal:** Let agents create named, organized replay sessions visible in Caido UI.

**Deliverables:**
- New tool: `caido_create_replay_session` -- creates session via `CreateReplaySession`, then renames via `RenameReplaySession(id, name)`. Takes `name` (string), returns Caido-native `sessionId`
- Surface Caido-native session/entry IDs in all replay-related responses
- Named replay entries: blocked on upstream Caido API. `StartReplayTaskInput` / `ReplayEntrySettingsInput` have no entry name field in sdk-go v0.5.0. Mark as best-effort -- implement only if raw GraphQL introspection confirms support, otherwise defer until Caido adds it.

**Non-goals:**
- No separate `create_replay_entry` / `send_replay_entry` tools (send_request already does this)
- No MCP-side session registry or persistence

**SDK dependency:** `CreateReplaySessionInput` takes `collectionId` and `requestSource`. `RenameReplaySession` takes session ID and new name. Both confirmed in sdk-go v0.5.0.

**Done when:** Agent can create a named session, send requests into it with labels, see everything organized in Caido Replay tab.

---

## Chunk 3: MCP Resources

**Goal:** Let agents browse Caido state without tool-call churn. Read-only inspection via MCP resource protocol.

**Resources to expose:**
- `caido://proxy/history` -- recent proxy history (paginated)
- `caido://sitemap` -- discovered endpoints
- `caido://findings` -- current findings list
- `caido://replay/sessions` -- replay sessions with entry counts
- `caido://project` -- current project name, Caido version, connection status

**Design:**
- Resources return compact summaries (IDs, paths, status codes, timestamps)
- Full request/response details still require tool calls (keeps resource payloads small)
- Resources support URI templates for filtering: `caido://proxy/history?host=example.com`

**Done when:** An MCP client can list and read all resources without any tool calls.

---

## Chunk 4: Smarter Responses

**Goal:** Reduce token burn by returning compact, useful response summaries by default.

**Deliverables:**
- Response fingerprint in every `send_request` / `batch_send` output:
  - status, content-length, content-type, title (extracted), redirect target, cookies set, word count
- `includeBody` param (bool, default true for single send to preserve compat, default false for batch)
- `bodyLimit` already exists -- make adaptive: JSON gets 4KB, HTML gets 1KB, binary gets 0
- Reflected marker detection: if caller provides `marker` param, report whether it appears in response
- Diff mode: new tool `caido_diff_responses` taking two Caido-native request IDs, returns structural diff

**Non-goals:**
- No vulnerability classification from response analysis
- No "similar response clustering" (LLM can do this from fingerprints)

**Done when:** A 20-request pentest chain uses <50% of the tokens it currently burns on response bodies.

---

## Chunk 5: Tool Annotations + Structured Logging

**Goal:** Help AI clients reason about tool safety; give operators visibility without noise.

**Tool annotations (MCP spec):**
- `readOnlyHint`: all list/get tools, resources
- `destructiveHint`: delete tools, drop_intercept
- `idempotentHint`: get/list tools only. send_request is NOT idempotent (creates new entry + sends HTTP request each call)
- `openWorldHint`: send_request, batch_send, forward_intercept (makes network calls to external targets)
- Every tool gets annotated

**Structured logging:**
- Behind `--verbose` / `--log-json` flags (no default output)
- JSON format: `tool`, `target_host`, `request_id`, `duration_ms`, `status`, `error_type`
- Correlation ID per MCP call
- Redact Cookie, Authorization, Set-Cookie values by default
- Optional `--audit-log <path>` for "what did the agent send?" forensics

**Done when:** Every tool has annotations; `--log-json` produces parseable structured logs.

---

## Chunk 6: HTTPQL Validation

**Goal:** Save tokens by letting agents validate and fix queries before expensive calls.

**Deliverables:**
- New tool: `caido_validate_httpql` -- takes query string, returns:
  - `valid` (bool)
  - `errors` (array of `{position, message, suggestion}`)
  - `normalized` (cleaned-up query if valid)
- Leverage Caido's own HTTPQL parser if exposed via API, otherwise implement subset validation locally

**Non-goals:**
- No saved query templates (add later if users ask)
- No query builder (LLM can construct from validated syntax)

**Done when:** Agent can validate HTTPQL before calling `list_requests`, reducing failed calls by >80%.

---

## Chunk 7: Progress Notifications / Streaming

**Goal:** Don't block on long operations; return progress updates.

**Applies to:**
- Large HTTPQL result sets (>100 results)
- Bulk replay / batch_send with many requests
- Active scan integration (if Caido exposes it cleanly)

**Design:**
- Use MCP progress notifications (`notifications/progress`)
- Return partial results as they arrive
- Final response includes summary + total count

**Done when:** A 200-request batch_send shows progress instead of hanging for 30s then dumping everything.

---

## Chunk 8: Expose Caido Scope

**Goal:** Let agents check scope without reimplementing scope logic.

**Deliverables:**
- Enhance existing `list_scopes` with full rule details
- New tool: `caido_is_in_scope` -- takes host/URL, returns bool + matching rule
- Expose scope as MCP resource: `caido://scopes`

**Non-goals:**
- No MCP-side allowlist/denylist enforcement
- No private IP blocking (Caido handles this if configured)

**Done when:** Agent can check any URL against project scope before sending requests.

---

## SDK Tracking

- Current: `caido-community/sdk-go v0.5.0` (latest as of 2026-05-05)
- Check for new releases before each chunk
- If a chunk needs API not in sdk-go, contribute upstream first or use raw GraphQL as fallback
- Pin exact versions in go.mod

## Ordering Rationale

Each chunk builds on the previous:
1. Tests give confidence to ship everything after
2. Named sessions are user-requested and simple -- quick win
3. Resources reduce tool-call volume, making all subsequent features cheaper to use
4. Smarter responses compound with resources for minimal token usage
5. Annotations + logging add operational maturity
6. HTTPQL validation reduces wasted calls (which now go through tested, annotated, resource-aware tools)
7. Streaming handles the scale edge cases
8. Scope exposure completes the "read Caido's model" story
