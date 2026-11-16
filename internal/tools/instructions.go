package tools

// Instructions is the MCP server `instructions` string, sent once in the
// initialize result. Clients put it in the model's context for the whole
// session, so it holds only what no single tool description can say: how the
// tools chain, and the redaction and cookie-jar contract that spans them.
// Keep it short (TestInstructions caps the size) and name only real tools
// (the same test checks every caido_ name against the registered set).
const Instructions = `Caido MCP server: drive the Caido web proxy (HTTP history, Replay, WebSockets, Automate, intercept, findings, scopes) for authorized security testing. Everything sent through these tools is proxied by Caido and visible to the operator in its UI.

Workflow
- Find traffic with caido_list_requests: filter with httpql, order:"desc" returns the newest first, and totalCount counts matches without paging. Read one request with caido_get_request (metadata only by default; ask for headers and bodies with include).
- Send with caido_send_request and reuse the returned sessionId for follow-ups. Pass collectionId and name to file a request as its own titled Replay tab. Use caido_edit_request to resend a captured request with changes, caido_batch_send for sweeps, caido_race_pipeline for race conditions, and caido_ws_connect, caido_ws_send and caido_ws_read for WebSockets.
- Check a target with caido_is_in_scope before sending to a host you did not get from the project. Record results with caido_create_finding so they show up in Caido.

Credentials
- Values of Authorization, Cookie, Set-Cookie and similar headers come back as [REDACTED]. Redacted means present, not missing. Never put the literal [REDACTED] into a request you send.
- To act as a captured user, use caido_edit_request: it rebuilds the request inside the server with the original credentials. For a login flow, send the login request with caido_send_request and keep the same sessionId: Set-Cookie values are stored in a per-session cookie jar and injected into later requests on that session automatically (caido_get_session_cookies lists what is stored, without values).`

// InstructionsReadOnly replaces Instructions when the server runs
// read-only (Options.ReadOnly): it must not point an agent at tools that
// are not registered in that mode.
const InstructionsReadOnly = `Caido MCP server, read-only mode: inspect what the Caido web proxy has captured (HTTP history, WebSocket streams, Replay and Automate sessions, findings, scopes). Nothing here sends traffic or changes the Caido project; tools that do are not available in this mode.

Workflow
- Find traffic with caido_list_requests: filter with httpql, order:"desc" returns the newest first, scopeId restricts to one scope, and totalCount counts matches without paging. Read one request with caido_get_request (metadata only by default; ask for headers and bodies with include), and compare two responses with caido_diff_responses.
- caido_get_sitemap shows the discovered endpoints, caido_list_ws_streams and caido_list_ws_messages the WebSocket traffic, and caido_list_findings what has been recorded. caido_is_in_scope checks a host against the project scope.

Credentials
- Values of Authorization, Cookie, Set-Cookie and similar headers, and of SECRET environment variables, come back as [REDACTED]. Redacted means present, not missing.`

// InstructionsScopeEnforced is appended to the instructions when scope is
// enforced (Options.EnforceScope).
const InstructionsScopeEnforced = `

Scope is enforced: a request, batch, race or WebSocket connection to a host outside the project scope is refused, and the tools that create, change or delete scopes or switch projects are not available. Check a host with caido_is_in_scope; if a target you need is out of scope, ask the operator to add it in Caido.`
