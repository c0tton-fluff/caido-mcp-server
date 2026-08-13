# PR 0 — MCP 2026-07-28 Conformance Evidence

Date: 2026-07-31
Repo state: `origin/main` @ 97925f6 (go-sdk v1.7.0)

## 1. Protocol version negotiation → 2026-07-28 (PASS)

- go-sdk `mcp/shared.go`: `latestProtocolVersion = protocolVersion20260728 = "2026-07-28"`;
  `supportedProtocolVersions = ["2026-07-28", "2025-11-25"]` (newest-first, preferred).
- The server is built with `mcp.NewServer(impl, nil)` (`cmd/caido-mcp-server/serve.go:81`) —
  **no `ServerOptions`, so no version pin**; it defaults to `latestProtocolVersion`.
- Durable regression guard added: `cmd/caido-mcp-server/protocol_version_test.go`
  (`TestProtocolVersionNegotiation`) builds the server the same way `serve.go` does, runs an
  in-memory client/server handshake, and asserts the negotiated
  `InitializeResult().ProtocolVersion == "2026-07-28"`. **PASS.**

## 2. Deterministic tools/list ordering (PASS — spec SHOULD)

`internal/tools/register.go` registers tools by iterating the fixed-order slice `allTools`
(not a map), so `tools/list` order is stable across processes — good for client prompt-cache.
No change needed.

## 3. Tool annotations correct (PASS)

- All 8 `delete_*` tools: `writeAnn(true, true, false)` → `destructiveHint: true`.
- All 4 external-contact tools (`send_request`, `batch_send`, `race_window_send`,
  `run_workflow`): `writeAnn(false, false, true)` → `openWorldHint: true`.
- Every registered tool sets `Annotations`. (The lone grep hit `schema_middleware.go`
  references `mcp.AddTool` only in comments — not a tool.)
- Read-only tools use `readOnlyAnn()` (`readOnlyHint: true`, `openWorldHint: false`).

## 4. No deprecated features (PASS)

grep of `internal/` `cmd/` for `CreateMessage` / `Sampling` / `ListRoots` / `SetLevel` /
`LoggingLevel` → none. The server does not use Sampling, Roots, or Logging (deprecated in
2026-07-28 per SEP-2577).

## 5. Discovery inventories (drive PR 2-4)

### PR 2 — tools missing `Title` (64)
All registered tools except `create_finding` and `list_findings` lack a `Title`.

### PR 3 — handler-enforced bounds a schema can mirror exactly (no new rejection)
| Tool | Param | Handler bound | Schema field |
|------|-------|---------------|--------------|
| create_finding | title | > 500 | maxLength 500 |
| create_finding | description | > 10000 | maxLength 10000 |
| create_tamper_rule | name | > 200 | maxLength 200 |
| create_tamper_rule | condition | > 10000 | maxLength 10000 |
| update_tamper_rule | name | > 200 | maxLength 200 |
| update_tamper_rule | condition | > 10000 | maxLength 10000 |
| create_scope | name | > 200 | maxLength 200 |
| create_scope | allowlist | > 100 items | maxItems 100 |
| create_environment | name | > 200 | maxLength 200 |
| list_requests | httpql | > 10000 | maxLength 10000 |
| list_intercept_entries | filter | > 10000 | maxLength 10000 |
| convert_body | body | > maxRawRequestBytes | maxLength (that const) |
| batch_send | requests | n > 50 | maxItems 50 |
| race_window_send | host | > 200 | maxLength 200 |
| race_window_send | count | 0 or > 50 | minimum 1 / maximum 50 |
| race_window_send | requests[].raw | > 1MB | maxLength 1048576 |
| (shared) helpers.go raw | > 1MB | send_request/edit_request raw via ParseRaw |

### PR 4 — closed-set params (enum only where handler default-rejects unknowns)
| Tool | Param | Accepted set | Notes |
|------|-------|--------------|-------|
| automate_task_control | action | start, pause, resume, cancel | verify default-error |
| intercept_control | action | pause, resume | verify default-error |
| create_tamper_rule | section | requestAll…responseStatusCode (13) | verify default-error |
| run_workflow | type | active, convert | verify default-error |
| create_replay_session | kind | HTTP, WS | **EXCLUDE**: handler `ToUpper`+`TrimSpace` normalizes case → enum would reject lowercase/whitespace input |
| edit_request | method | (free string) | **EXCLUDE**: `replaceMethod` substitutes verbatim, no validation → enum would newly reject |

Per-candidate handler verification (default-error + casing) happens in PR 4 before each enum.

## Verdict

The server is **already conformant** with MCP 2026-07-28 (negotiation verified by test,
deterministic ordering, correct annotations, no deprecated features). PR 0 adds only the
durable conformance test. PR 1-4 are additive tool-authoring polish.
