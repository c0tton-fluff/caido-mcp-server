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
