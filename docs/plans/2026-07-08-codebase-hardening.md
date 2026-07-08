# Codebase Hardening Plan - 2026-07-08

Source: a verified multi-agent audit of the whole repo (8 dimensions, 39 findings,
21 CONFIRMED against real source, 0 refuted). This plan tiers the confirmed work,
records what ships in this pass, and specifies the remaining roadmap functionality.

## Baseline (start of this work)

- Build / vet / staticcheck / tests: all green
- Total coverage 50.8% (internal/tools 71.8%, httputil 92.9%, raceattack 88.0%,
  resources 82.2%, replay 20.7%, auth 0%, cmd/cli 0%, cmd/mcp 0%, testutil 0%)
- 64 MCP tools, 4 resources
- v2 roadmap (docs/specs/2026-05-05-v2-roadmap-design.md): chunks 1-3 largely done,
  chunk 4 partial, chunks 5-8 mostly unstarted

## Design principles (unchanged - every item below respects these)

Thin, reliable Caido bridge. Surface Caido-native IDs only. No MCP-side persistence.
No encoded pentest methodology. Tools mutate/execute; resources inspect.
No MCP-side scope enforcement (Caido owns scope).

---

## Tier 1 - Correctness & security bugs (ship first)

### 1.1 Response-diff false negative (HIGH, bug)
`internal/httputil/diff.go` `GetAndSet` treats two responses as identical on
`BodyHash == && StatusCode ==` but omits `BodySize`. The hash is computed over the
TRUNCATED body (`send_request.go` builds `BodyHash` from `output.Response.Body`,
already cut to `bodyLimit`), while `BodySize` is the full pre-truncation length.
Two responses sharing the first `bodyLimit` bytes but differing after it are reported
"identical to previous response" and their body/headers are blanked - silently hiding
a real change in the exact BAC/fuzz sweeps the tool exists for.
Fix: add `&& prev.BodySize == current.BodySize` to the Same condition. Add a regression
test: two responses, same prefix, different totals -> `Same == false`.

### 1.2 Raw-request/response resource leaks credentials (HIGH, security)
`internal/resources/request.go` base64-decodes and emits the raw request AND response
with only length truncation, never calling the redaction in `httputil.ParseRaw`. The
`caido://requests/{id}` resource therefore returns `Authorization`, `Cookie`, and
`Set-Cookie` in cleartext - directly contradicting the README's "Credential redaction
... by default" promise.
Fix: add a single choke-point `httputil.RedactRawHeaders(raw string) string` that walks
header lines and redacts sensitive-header values (respecting `CAIDO_ALLOW_SENSITIVE_HEADERS`),
route the resource's request+response dumps through it (redact before truncate), and test
that an `Authorization`+`Cookie` request and `Set-Cookie` response render `[REDACTED]`.

### 1.3 Automate session template leaks credentials (HIGH, security)
`internal/tools/get_automate_session.go` returns the raw fuzz request template verbatim
(a full raw HTTP request carrying the session's `Authorization`/`Cookie`), bypassing
redaction. (Note: `get_automate_entry.go` payloads are position-inserted fuzz *values*,
not full requests - no header redaction needed there; the audit's entry claim was refuted.)
Fix: route the decoded template through `RedactRawHeaders`; test with an auth+cookie template.

### 1.4 Replay Send orphans the auto-created empty session (MEDIUM, bug)
`internal/replay/replay.go` `Send` fallback creates a seeded session and resets the
default cache but never deletes the empty session `GetOrCreateSession` made, leaking it
in Caido on the first send of every process.
Fix: in the fallback, best-effort `DeleteSessions` the failed session id ONLY when it was
the auto-created default (`cacheReplacement` true), so a user-supplied session is untouched.

### 1.5 OAuth WS read not context-cancellable (MEDIUM, bug)
`internal/auth/oauth.go` `readTokenFromWS` does a non-blocking `select {<-ctx.Done()}`
then a blocking `conn.ReadJSON` with no read deadline; a cancelled/expired auth ctx cannot
unblock the read. Fix: start a goroutine that closes the conn on `<-ctx.Done()` (or set a
ctx-derived read deadline) so the blocked read returns and the loop honors cancellation.

### 1.6 Race-window read stalls on keep-alive targets (MEDIUM, bug)
`internal/raceattack/raceattack.go` `readResponse` uses `io.ReadFull` on a fixed
`bodyLimit` buffer with a single 10s deadline and no Content-Length/chunked/Connection
parsing; against a keep-alive target returning a smaller body, it blocks until the 10s
deadline - stalling every request. Fix: read EOF/idle-aware
(`io.ReadAll(io.LimitReader(reader, bodyLimit))` with a short idle deadline); add a
keep-alive (no `Connection: close`) test asserting no multi-second stall.

### 1.7 Pool cleanup discards delete error (LOW, bug)
`internal/replay/pool.go:85` `_, _ = ...DeleteSessions` swallows the error so a batch's
sessions can leak silently. Fix: capture and log to stderr (best-effort, observable).

---

## Tier 2 - Refactors (mechanical, behavior-preserving, DRY)

New `internal/tools/helpers.go`:
- `clampLimit(v, def, max int) int` - replaces the identical limit-clamp block in 6 list
  tools (`list_requests`, `list_findings`, `list_intercept_entries`, `list_ws_messages`,
  `list_ws_streams`, `get_automate_entry`), each with its own default/max preserved.
- `pageCursor(hasNext bool, end *string) (bool, string)` - replaces the nil-guarded
  PageInfo/EndCursor extraction duplicated across the same 6 tools.
- `maxRawRequestBytes = 1 << 20` + `checkRawSize(name, raw string) error` - replaces the
  raw `1048576` literal in 4 files and two redundant local consts (`maxRaceRaw`,
  `maxConvertBodyBytes`), standardizing the drifted error messages.

`internal/httputil` `DefaultPort(useTLS bool) int` - replaces the 443/80 block duplicated
across `send_request.go`, `edit_request.go`, `replay/batch.go`, `export_curl.go`, and the
TLS-unaware `race_window_send.go` (flag the unconditional-443 divergence for a decision).

---

## Tier 3 - Tool annotations (roadmap Chunk 5, part 1)

All 64 tools ship with zero `ToolAnnotations`. The pinned go-sdk v1.2.0 supports
`Tool.Annotations *ToolAnnotations{ReadOnlyHint, DestructiveHint, IdempotentHint, OpenWorldHint, Title}`.
Classification (frozen contract):
- Read tools (all `list_*`, `get_*`, `intercept_status`, `get_sitemap`, `get_instance`,
  `get_session_cookies`): `ReadOnlyHint: true, IdempotentHint: true`.
- Delete tools (8 `delete_*`) + `drop_intercept`: `DestructiveHint: true`, idempotent by id.
- Create/rename/select/set-style: `ReadOnlyHint: false`, idempotent where applicable.
- `toggle_workflow`/`toggle_tamper_rule`: not idempotent (flip state).
- `send_request`/`batch_send`/`forward_intercept`/`race_window_send`/`run_workflow`:
  `OpenWorldHint: true` (hit external targets), not idempotent.
Applied per file (each `mcp.Tool` literal gets an `Annotations` field); resources get
`ReadOnlyHint: true`.

---

## Tier 4 - Test coverage (close the confirmed gaps)

- `internal/tools/edit_request_test.go`: table tests for `replaceMethod`/`replacePath`/
  `removeHeader`/`replaceBody` (3-token and 2-token request lines, missing CRLF, colon-less
  header, case-mismatched removal, Content-Length recompute) + a handler test.
- `internal/testutil/mock_caido.go`: record decoded GraphQL `variables` per request;
  `internal/tools/create_replay_session_test.go`: assert transmitted `kind`/`requestSource`/
  `collectionId` (currently vacuous - passes regardless of what is sent).
- `cmd/cli/batch_test.go` + `send_test.go`: `parseTokens`, `applyToken` (bearer/cookie/header
  modes), `buildRequest` (Host/Connection/Content-Length only-when-absent, default-port omit).
- `internal/auth/oauth_test.go` + `token_store_test.go` (white-box): `ParseExpiresAt`,
  `parseWSTokenPayload` (success/error/malformed), `IsExpired` skew, TokenStore roundtrip.

---

## Tier 5 - CI / supply-chain hardening

- Add `golangci-lint` (pinned) with `errcheck` enabled + a minimal `.golangci.yml`; annotate
  the ~7 intentional error discards with `//nolint:errcheck` justifications. This turns the
  pool.go class of bug into a CI failure.
- Add `govulncheck` (pinned) step - the canonical Go dep CVE audit (low-noise, reachability-based).
- Add `gofmt -l` gate (fail on unformatted).
- Pin `staticcheck` to a release (drop `@latest`) via a `tools` directive.
- Align `actions/checkout` + `actions/setup-go` majors between ci.yml and release.yml;
  SHA-pin the third-party `softprops/action-gh-release` (has `contents: write`).
- Add `.github/dependabot.yml` (gomod + github-actions).
- Upload `coverage.out` as a CI artifact.

---

## Tier 6 - Docs

- README: CLI examples call `caido` but the shipped binary is `caido-cli`
  (install.sh/build.sh). Align examples to `caido-cli`.
- CHANGELOG: add an `[Unreleased]` section for #29 (`CAIDO_ALLOW_SENSITIVE_HEADERS`) and
  #28 (replay `kind` field) - both shipped but undocumented.
- Add `// Package X` godoc comments to the 7 packages missing them (raceattack is the model).
- README: note redaction covers named sensitive HEADERS only (not query strings / bodies).

---

## Tier 7 - New roadmap functionality

### 7.1 Response fingerprint expansion (Chunk 4)
Extend `httputil.Fingerprint` with `StatusCode`, `Title` (extract `<title>`), `RedirectTarget`
(Location), `SetCookies []string` (names only), `WordCount`. Add `IncludeBody *bool` to
`SendRequestInput` (default true) and `batch_send` (default false). Add `Marker string` param;
report `reflected bool` when the marker appears in the response body.

### 7.2 `caido_validate_httpql` (Chunk 6)
New read-only tool. First probe whether sdk-go/Caido exposes an HTTPQL parse/validate query;
if yes, map it. Otherwise implement a local subset validator (balanced quotes/parens, known
`req.*`/`resp.*` fields, operator syntax) returning `{valid, errors:[{position,message,
suggestion}], normalized}`.

### 7.3 `caido_diff_responses` (Chunk 4)
New read-only tool taking two Caido-native request ids; fetch both via the existing
get_request path and reuse `DiffResult` (status/size/structural body diff) to compare them.

### 7.4 `caido_is_in_scope` + `caido://scopes` resource (Chunk 8)
Prefer a Caido-native scope-check API if sdk-go exposes one; else fetch scopes and match the
host against allow/deny glob rules, returning `{inScope, matchedRule}`. Add a compact
`caido://scopes` resource. (`list_scopes` already surfaces full rule detail - that sub-item
is done.)

### 7.5 `caido://project` resource (Chunk 3)
Register `caido://project` returning current project name, Caido version, connection status
(reuse the get_instance data path). Optionally add a `caido://proxy/history` list resource.

### 7.6 Structured logging (Chunk 5, part 2)
`--log-json` flag on the mcp command; per-tool-call JSON record (tool, target_host, request_id,
duration_ms, status, error_type) with a correlation id, redacting Cookie/Authorization/Set-Cookie.
Optional `--audit-log <path>` sink.

### 7.7 Progress notifications (Chunk 7)
Read `progressToken` from request meta in `batch_send` (and `list_requests` >100) and call the
SDK progress-notification method after each chunk with progress/total.

---

## Tier 8 - Tracked, not actionable here

- sdk-go pinned to a pseudo-version: an immutable go.sum-verified pin (not a supply-chain risk),
  blocked on an upstream tagged release (needs admin on caido-community/sdk-go). Keep the TODO.
- Coverage badge (roadmap Chunk 1 deliverable): add once a coverage endpoint/gist step exists.

---

## Execution order

1. Tier 1 + Tier 2 (shared Go files, one hand, verify green, commit).
2. Tier 3 annotations (parallel by file, verify, commit).
3. Tier 4 tests (parallel by file, verify, commit).
4. Tier 5 + Tier 6 CI/docs (parallel by file, verify, commit).
5. Tier 7 new tools/resources (parallel new files + orchestrator wires register.go/resources.go,
   verify, commit) - gated on sdk-go feasibility per item.

Verification gate after every wave: `go build ./... && go vet ./... && go test ./... -race && staticcheck ./...`
must be green before the next wave.
