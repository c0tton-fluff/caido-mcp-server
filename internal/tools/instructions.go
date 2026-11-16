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

// InstructionsMinimal replaces the workflow text when toolsets restrict
// the server (Options.Toolsets): which tools exist is then the operator's
// choice, so it names none and keeps only the contract every toolset shares.
const InstructionsMinimal = `Caido MCP server: drive the Caido web proxy for authorized security testing. This server exposes a selected subset of its tools; use the ones listed and do not assume others exist. Everything sent through these tools is proxied by Caido and visible to the operator in its UI.

Credentials
- Values of Authorization, Cookie, Set-Cookie and similar headers, and of SECRET environment variables, come back as [REDACTED]. Redacted means present, not missing. Never put the literal [REDACTED] into a request you send.
- Requests sent on the same Replay session share a cookie jar: Set-Cookie values from a response are stored and injected into later requests on that session automatically.`

// InstructionsScopeEnforced is appended to the instructions when scope is
// enforced (Options.EnforceScope). It names no tool, so it holds for every
// toolset selection.
const InstructionsScopeEnforced = `

Scope is enforced: a request, batch, race or WebSocket connection to a host outside the project scope is refused, and the tools that create, change or delete scopes or switch projects are not available. If a target you need is out of scope, ask the operator to add it in Caido.`

// InstructionsFor returns the server instructions matching opts, so the
// text never points an agent at a tool that is not registered.
func InstructionsFor(opts Options) string {
	var text string
	switch {
	case len(opts.Toolsets) > 0:
		text = InstructionsMinimal
	case opts.ReadOnly:
		// Read-only mode has no tool that sends, so the scope note below
		// would describe refusals that cannot happen.
		return InstructionsReadOnly
	default:
		text = Instructions
	}
	if opts.EnforceScope && !opts.ReadOnly {
		text += InstructionsScopeEnforced
	}
	return text
}
