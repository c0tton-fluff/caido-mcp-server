package tools

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Mock payloads follow what a live Caido 0.59.1 returned for the same
// operations (see internal/replay/ws.go for the observed behaviour).

func newWsEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		RegisterWsConnectTool(s, c)
		RegisterWsSendTool(s, c)
		RegisterWsReadTool(s, c)
		RegisterWsCloseTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	return env
}

func wsTasks(sessionID, taskID, entryID, streamID string) map[string]any {
	tasks := []any{}
	if taskID != "" {
		tasks = append(tasks, map[string]any{
			"__typename": "ReplayTask", "id": taskID, "sessionKind": "WS",
			"replayEntry": map[string]any{
				"id": entryID, "session": map[string]any{"id": sessionID},
				"stream": map[string]any{"id": streamID},
			},
		})
	}
	return map[string]any{"tasks": tasks}
}

func wsSession(id, httpID, entryID, streamID string) map[string]any {
	entry := map[string]any{
		"id": entryID, "http": map[string]any{"id": httpID},
		"stream": map[string]any{"id": streamID},
	}
	return map[string]any{"replaySession": map[string]any{
		"__typename": "ReplaySessionWs", "id": id, "activeEntry": entry,
	}}
}

// wsEntry is a GetReplayWsEntry result: the handshake state (status 0 =
// no response yet) and the IDs of the tasks still listed.
func wsEntry(streamID string, status int, raw string, liveTasks ...string) map[string]any {
	var resp any
	if status != 0 {
		resp = map[string]any{"statusCode": status, "raw": testutil.B64(raw)}
	}
	var stream any
	if streamID != "" {
		stream = map[string]any{"id": streamID}
	}
	tasks := make([]any, len(liveTasks))
	for i, id := range liveTasks {
		tasks[i] = map[string]any{"id": id}
	}
	return map[string]any{
		"replayEntry": map[string]any{
			"__typename": "ReplayEntryWs", "error": nil, "stream": stream,
			"http": map[string]any{"request": map[string]any{"response": resp}},
		},
		"tasks": tasks,
	}
}

func wsDrafted() map[string]any {
	return map[string]any{"updateReplayEntryDraft": map[string]any{"entry": map[string]any{"id": "h1"}}}
}

func wsStopped() map[string]any {
	return map[string]any{"stopReplayWsTasks": map[string]any{"error": nil}}
}

func wsSent(id string) map[string]any {
	return map[string]any{"sendReplayTaskMessage": map[string]any{
		"message": map[string]any{"id": id}, "error": nil,
	}}
}

// wsGone is the error union Caido returns for a task that no longer runs.
func wsGone(field string) map[string]any {
	return map[string]any{field: map[string]any{
		"message": nil, "error": map[string]any{"__typename": "UnknownIdUserError", "code": "UNKNOWN_ID"},
	}}
}

// mockReseed mocks the reads and the draft update of reusing session s1.
func mockReseed(env *testutil.MCPTestEnv) {
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	env.Mock.On("UpdateReplayWsHandshake", wsDrafted())
}

// toolErrCase is one call expected to fail with an error containing want.
type toolErrCase struct {
	name string
	args map[string]any
	mock func(env *testutil.MCPTestEnv)
	want string
}

func runToolErrCases(t *testing.T, tool string, cases []toolErrCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := newWsEnv(t)
			if tt.mock != nil {
				tt.mock(env)
			}
			result := env.CallTool(t, tool, tt.args)
			if !result.IsError || !strings.Contains(toolText(result), tt.want) {
				t.Fatalf("want error containing %q, got %s", tt.want, toolText(result))
			}
		})
	}
}

func wsStarted(taskID, entryID string) map[string]any {
	return map[string]any{"startReplayTask": map[string]any{
		"task": map[string]any{"id": taskID, "replayEntry": map[string]any{"id": entryID}}, "error": nil,
	}}
}

type wsMsg struct{ id, dir, format, body string }

func wsMessages(msgs ...wsMsg) map[string]any {
	edges := make([]any, len(msgs))
	for i, m := range msgs {
		edges[i] = map[string]any{
			"cursor": "c" + m.id,
			"node": map[string]any{"id": m.id, "head": map[string]any{
				"direction": m.dir, "format": m.format, "length": len(m.body), "raw": testutil.B64(m.body),
			}},
		}
	}
	return map[string]any{"streamWsMessages": map[string]any{
		"pageInfo": map[string]any{"hasNextPage": false}, "edges": edges,
	}}
}

const upgraded = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nSec-WebSocket-Protocol: graphql-ws\r\n\r\n"

func TestBuildWsHandshake(t *testing.T) {
	raw, conn, err := buildWsHandshake(
		"wss://api.example:8443/graphql?x=1",
		map[string]string{"origin": "https://evil.example", "Cookie": "sid=1"},
		[]string{"graphql-ws", "graphql-transport-ws"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if conn != (caido.ReplayConnection{Host: "api.example", Port: 8443, IsTLS: true}) {
		t.Fatalf("unexpected connection %+v", conn)
	}
	for _, want := range []string{
		"GET /graphql?x=1 HTTP/1.1\r\n",
		"Host: api.example:8443\r\n",
		"Upgrade: websocket\r\n",
		"Sec-WebSocket-Version: 13\r\n",
		"Sec-WebSocket-Protocol: graphql-ws, graphql-transport-ws\r\n",
		"origin: https://evil.example\r\n",
		"Cookie: sid=1\r\n",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("handshake missing %q:\n%s", want, raw)
		}
	}
	if !strings.HasSuffix(raw, "\r\n\r\n") {
		t.Error("handshake must end with an empty line")
	}

	// A user header replaces the generated one of the same name.
	raw, _, err = buildWsHandshake("ws://h/", map[string]string{"sec-websocket-version": "8"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "Sec-WebSocket-Version: 13") || !strings.Contains(raw, "sec-websocket-version: 8") {
		t.Fatalf("override not applied:\n%s", raw)
	}
	if _, conn, _ := buildWsHandshake("ws://h", nil, nil); conn.Port != 80 || conn.IsTLS {
		t.Fatalf("ws:// should default to port 80 without TLS, got %+v", conn)
	}

	for _, bad := range []struct {
		url     string
		headers map[string]string
	}{
		{"ftp://h/", nil},
		{"ws:///nohost", nil},
		{"ws://h:99999/", nil},
		{"ws://h/", map[string]string{"X": "a\r\nInjected: 1"}},
		{"ws://h/", map[string]string{"Bad:Name": "v"}},
		{"ws://h/", map[string]string{"Bad Name": "v"}},
		{"ws://h/", map[string]string{"Cookie": "a=1", "cookie": "b=2"}},
	} {
		if _, _, err := buildWsHandshake(bad.url, bad.headers, nil); err == nil {
			t.Errorf("want error for %q %v", bad.url, bad.headers)
		}
	}
	for _, proto := range []string{"graphql-ws\r\nX-Injected: 1", "", "a,b", "a b"} {
		if _, _, err := buildWsHandshake("ws://h/", nil, []string{proto}); err == nil {
			t.Errorf("want error for subprotocol %q", proto)
		}
	}
}

func TestWsConnect_CreatesNamedSession(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("CreateReplaySession", map[string]any{"createReplaySession": map[string]any{
		"session": map[string]any{"__typename": "ReplaySessionWs", "id": "s1", "name": "s1"}, "error": nil,
	}})
	env.Mock.On("RenameReplaySession", testutil.RenameReplaySessionResponse("s1", "WS - valid"))
	env.Mock.On("StartReplayWsTask", wsStarted("t1", "e1"))
	env.Mock.On("GetReplayWsEntry", wsEntry("st1", 101, upgraded, "t1"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{
		"url": "wss://api.example/graphql", "subprotocols": []string{"graphql-ws"},
		"headers": map[string]string{"Origin": "https://app.example"},
		"name":    "WS - valid", "collectionId": "col-1",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.SessionID != "s1" || !out.Created || !out.Open || out.StatusCode != 101 || out.StreamID != "st1" {
		t.Fatalf("unexpected output %+v", out)
	}
	if out.Body != "" || len(out.ResponseHeaders) == 0 {
		t.Fatalf("want headers and no body for a 101, got %+v", out)
	}

	input, _ := env.Mock.LastVariables("CreateReplaySession")["input"].(map[string]any)
	if input["kind"] != "WS" || input["collectionId"] != "col-1" {
		t.Fatalf("unexpected create input %v", input)
	}
	src, _ := input["requestSource"].(map[string]any)
	rawSrc, _ := src["raw"].(map[string]any)
	ci, _ := rawSrc["connectionInfo"].(map[string]any)
	if ci["host"] != "api.example" || ci["port"] != float64(443) || ci["isTLS"] != true {
		t.Fatalf("unexpected connectionInfo %v", ci)
	}
	hs, _ := base64.StdEncoding.DecodeString(dig[string](t, rawSrc, "raw"))
	if !strings.Contains(string(hs), "Origin: https://app.example\r\n") {
		t.Fatalf("handshake missing Origin:\n%s", hs)
	}
}

// requestId seeds the handshake from a captured upgrade in history.
func TestWsConnect_FromHistoryRequest(t *testing.T) {
	env := newWsEnv(t)
	hs := "GET /live HTTP/1.1\r\nHost: app.example\r\nUpgrade: websocket\r\n\r\n"
	env.Mock.On("GetWsHandshakeRequest", map[string]any{"request": map[string]any{
		"host": "app.example", "port": 8443, "isTls": true, "raw": testutil.B64(hs),
	}})
	env.Mock.On("CreateReplaySession", map[string]any{"createReplaySession": map[string]any{
		"session": map[string]any{"__typename": "ReplaySessionWs", "id": "s1", "name": "s1"}, "error": nil,
	}})
	env.Mock.On("StartReplayWsTask", wsStarted("t1", "e1"))
	env.Mock.On("GetReplayWsEntry", wsEntry("st1", 101, upgraded, "t1"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{"requestId": "42"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", toolText(result))
	}
	src := dig[map[string]any](t, env.Mock.LastVariables("CreateReplaySession"), "input", "requestSource", "raw")
	raw, _ := base64.StdEncoding.DecodeString(dig[string](t, src, "raw"))
	ci := dig[map[string]any](t, src, "connectionInfo")
	if string(raw) != hs || ci["host"] != "app.example" || ci["port"] != float64(8443) || ci["isTLS"] != true {
		t.Fatalf("want the history request replayed as is, got raw=%q conn=%v", raw, ci)
	}
}

func TestWsConnect_HistoryRequestNotFound(t *testing.T) {
	runToolErrCases(t, "caido_ws_connect", []toolErrCase{{
		name: "unknown request id",
		args: map[string]any{"requestId": "404"},
		mock: func(env *testutil.MCPTestEnv) {
			env.Mock.On("GetWsHandshakeRequest", map[string]any{"request": nil})
		},
		want: "request 404 not found",
	}})
}

func TestWsConnect_ReusesSessionClosingOpenTask(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t-old", "e1", "st1"))
	env.Mock.On("StopReplayWsTasks", wsStopped())
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("st2", 101, upgraded, "t2"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{
		"sessionId": "s1", "raw": "GET /ws HTTP/1.1\nHost: h\n\n", "host": "h", "tls": false,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.Created || !out.ClosedPrevious || !out.Open || out.StreamID != "st2" {
		t.Fatalf("unexpected output %+v", out)
	}
	if ids := dig[[]any](t, env.Mock.LastVariables("StopReplayWsTasks"), "taskIds"); ids[0] != "t-old" {
		t.Fatalf("want old task stopped, got %v", ids)
	}
	vars := env.Mock.LastVariables("UpdateReplayWsHandshake")
	httpIn := dig[map[string]any](t, vars, "input", "http")
	raw, _ := base64.StdEncoding.DecodeString(dig[string](t, httpIn, "raw"))
	if vars["id"] != "h1" || string(raw) != "GET /ws HTTP/1.1\r\nHost: h\r\n\r\n" {
		t.Fatalf("want CRLF handshake drafted on http entry h1, got id=%v raw=%q", vars["id"], raw)
	}
	if conn := dig[map[string]any](t, httpIn, "connection"); conn["port"] != float64(80) {
		t.Fatalf("want port 80 without TLS, got %v", conn)
	}
}

// Steps that can fail run before the open connection is closed, so a
// failed reconnect leaves a working socket alone.
func TestWsConnect_FailedReseedKeepsOpenConnection(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	// No MoveReplaySession mock: the move fails.
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t-old", "e1", "st1"))
	env.Mock.On("StopReplayWsTasks", wsStopped())

	result := env.CallTool(t, "caido_ws_connect", map[string]any{
		"sessionId": "s1", "url": "ws://h/", "collectionId": "bad",
	})
	if !result.IsError || !strings.Contains(toolText(result), "move session") {
		t.Fatalf("want the move error, got %s", toolText(result))
	}
	if env.Mock.LastVariables("StopReplayWsTasks") != nil {
		t.Fatal("the open connection must not be closed when an earlier step failed")
	}
}

// A task that ends on its own between the lookup and the stop must not
// fail the connect.
func TestWsConnect_TolerantOfTaskEndingDuringClose(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t-old", "e1", "st1"))
	env.Mock.On("StopReplayWsTasks", map[string]any{"stopReplayWsTasks": map[string]any{
		"error": map[string]any{"__typename": "UnknownIdUserError", "code": "UNKNOWN_ID"},
	}})
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("st2", 101, upgraded, "t2"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{"sessionId": "s1", "url": "ws://h/"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", toolText(result))
	}
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.ClosedPrevious || !out.Open {
		t.Fatalf("want open with closedPrevious=false, got %+v", out)
	}
}

// Live 0.59.1: right after a task ends Caido can briefly answer
// TASK_IN_PROGRESS; a reused session retries it.
func TestWsConnect_RetriesBusyStartOnReusedSession(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("StartReplayWsTask", map[string]any{"startReplayTask": map[string]any{
		"task": nil, "error": map[string]any{"__typename": "TaskInProgressUserError", "code": "TASK_IN_PROGRESS"},
	}})
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("st2", 101, upgraded, "t2"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{"sessionId": "s1", "url": "ws://h/"})
	if result.IsError {
		t.Fatalf("want the busy start retried, got %s", toolText(result))
	}
	if out := testutil.UnmarshalToolResult[WsConnectOutput](t, result); !out.Open {
		t.Fatalf("want open after the retry, got %+v", out)
	}
}

// A new session has no previous task, so a busy start is an error at once.
func TestWsConnect_BusyStartOnNewSessionFails(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("CreateReplaySession", map[string]any{"createReplaySession": map[string]any{
		"session": map[string]any{"__typename": "ReplaySessionWs", "id": "s1", "name": "s1"}, "error": nil,
	}})
	env.Mock.On("StartReplayWsTask", map[string]any{"startReplayTask": map[string]any{
		"task": nil, "error": map[string]any{"__typename": "TaskInProgressUserError", "code": "TASK_IN_PROGRESS"},
	}})
	start := time.Now()
	result := env.CallTool(t, "caido_ws_connect", map[string]any{"url": "ws://h/"})
	if !result.IsError || !strings.Contains(toolText(result), "TaskInProgressUserError") {
		t.Fatalf("want the busy start reported, got %s", toolText(result))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("a new session must not retry, took %v", elapsed)
	}
}

// connect=false stores the handshake for a human to click Connect, and
// starts nothing.
func TestWsConnect_PrepareOnly(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	result := env.CallTool(t, "caido_ws_connect", map[string]any{
		"sessionId": "s1", "url": "ws://h/", "connect": false,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", toolText(result))
	}
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.Open || out.Pending || out.StatusCode != 0 {
		t.Fatalf("want a prepared, unopened tab, got %+v", out)
	}
	if env.Mock.LastVariables("UpdateReplayWsHandshake") == nil || env.Mock.LastVariables("StartReplayWsTask") != nil {
		t.Fatal("want the handshake drafted and no task started")
	}
}

func TestWsConnect_RejectedHandshakeReturnsBody(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("UpdateReplayWsHandshake", map[string]any{"updateReplayEntryDraft": map[string]any{
		"entry": map[string]any{"id": "h1"},
	}})
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("", 401,
		"HTTP/1.1 401 Unauthorized\r\nContent-Length: 12\r\n\r\nbad token!!!"))

	result := env.CallTool(t, "caido_ws_connect", map[string]any{"sessionId": "s1", "url": "ws://h/"})
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.Open || out.StatusCode != 401 || out.Body != "bad token!!!" {
		t.Fatalf("want closed 401 with body, got %+v", out)
	}
}

// Live 0.59.1: a refused connection leaves no response and no entry error;
// the task just ends.
func TestWsConnect_RefusedReportsError(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("UpdateReplayWsHandshake", map[string]any{"updateReplayEntryDraft": map[string]any{
		"entry": map[string]any{"id": "h1"},
	}})
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("", 0, "")) // no response, task gone

	result := env.CallTool(t, "caido_ws_connect", map[string]any{
		"sessionId": "s1", "url": "ws://h:1/", "timeoutSec": 30,
	})
	out := testutil.UnmarshalToolResult[WsConnectOutput](t, result)
	if out.Open || out.Pending || !strings.Contains(out.Error, "before any handshake response") {
		t.Fatalf("want a refused-connection error, got %+v", out)
	}
}

// A handshake still unanswered at the deadline, with the task alive, is
// pending rather than failed.
func TestWsConnect_TimeoutWhileTaskAliveIsPending(t *testing.T) {
	env := newWsEnv(t)
	mockReseed(env)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("StartReplayWsTask", wsStarted("t2", "e2"))
	env.Mock.On("GetReplayWsEntry", wsEntry("", 0, "", "t2"))

	out := testutil.UnmarshalToolResult[WsConnectOutput](t, env.CallTool(t, "caido_ws_connect", map[string]any{
		"sessionId": "s1", "url": "ws://h/", "timeoutSec": 1,
	}))
	if !out.Pending || !out.Open || out.Error != "" {
		t.Fatalf("want pending and open with no error, got %+v", out)
	}
}

func TestWsConnect_Errors(t *testing.T) {
	runToolErrCases(t, "caido_ws_connect", []toolErrCase{
		{name: "no source", args: map[string]any{}, want: "exactly one of url, raw or requestId"},
		{
			name: "two sources",
			args: map[string]any{"url": "ws://h/", "requestId": "9"},
			want: "exactly one of url, raw or requestId",
		},
		{name: "raw without host", args: map[string]any{"raw": "GET / HTTP/1.1\r\n\r\n"}, want: "host is required"},
		{
			name: "headers without url",
			args: map[string]any{"raw": "GET / HTTP/1.1\r\n\r\n", "host": "h", "headers": map[string]string{"A": "b"}},
			want: "only with url",
		},
		{
			name: "plan cap explained",
			args: map[string]any{"url": "ws://h/"},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("CreateReplaySession", map[string]any{"createReplaySession": map[string]any{
					"session": nil,
					"error": map[string]any{
						"__typename": "PermissionDeniedUserError", "code": "PERMISSION_DENIED",
						"permissionReason": "ENTITLEMENT",
					},
				}})
			},
			want: "limit on WebSocket sessions",
		},
		{
			name: "not a ws session",
			args: map[string]any{"sessionId": "s9", "url": "ws://h/"},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("GetReplayWsSession", map[string]any{"replaySession": map[string]any{
					"__typename": "ReplaySessionHttp", "id": "s9",
				}})
			},
			want: "is a http session, not a WebSocket one",
		},
		{
			name: "empty ws session",
			args: map[string]any{"sessionId": "s9", "url": "ws://h/"},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("GetReplayWsSession", map[string]any{"replaySession": map[string]any{
					"__typename": "ReplaySessionWs", "id": "s9", "activeEntry": nil,
				}})
			},
			want: "no handshake to replace",
		},
	})
}

func TestWsSend(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
	env.Mock.On("SendReplayWsMessage", wsSent("m1"))
	env.Mock.On("SendReplayWsMessage", wsSent("m2"))

	result := env.CallTool(t, "caido_ws_send", map[string]any{
		"sessionId": "s1",
		"frames": []map[string]any{
			{"body": `{"type":"connection_init"}`},
			{"body": testutil.B64("\x00\x01"), "format": "BINARY", "base64": true},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", toolText(result))
	}
	out := testutil.UnmarshalToolResult[WsSendOutput](t, result)
	if out.Sent != 2 || out.MessageIDs[1] != "m2" {
		t.Fatalf("unexpected output %+v", out)
	}
	vars := env.Mock.LastVariables("SendReplayWsMessage")
	ws := dig[map[string]any](t, vars, "input", "ws")
	if vars["task"] != "t1" || ws["format"] != "BINARY" || ws["direction"] != "SERVER" || ws["data"] != testutil.B64("\x00\x01") {
		t.Fatalf("unexpected send variables %v", vars)
	}
}

func TestWsSend_Errors(t *testing.T) {
	runToolErrCases(t, "caido_ws_send", []toolErrCase{
		// The input schema (minItems, enum) rejects these before the handler.
		{name: "no frames", args: map[string]any{"sessionId": "s1", "frames": []any{}}, want: "frames"},
		{
			name: "bad format",
			args: map[string]any{"sessionId": "s1", "frames": []map[string]any{{"body": "x", "format": "PING"}}},
			want: "format",
		},
		{
			name: "bad base64",
			args: map[string]any{"sessionId": "s1", "frames": []map[string]any{{"body": "!!", "base64": true}}},
			want: "invalid base64",
		},
		{
			name: "not open",
			args: map[string]any{"sessionId": "s1", "frames": []map[string]any{{"body": "x"}}},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
				env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
			},
			want: "no open WebSocket connection",
		},
		{
			name: "partial send names the frames that went out",
			args: map[string]any{"sessionId": "s1", "frames": []map[string]any{{"body": "a"}, {"body": "b"}}},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
				env.Mock.On("SendReplayWsMessage", wsSent("m1"))
				env.Mock.On("SendReplayWsMessage", wsGone("sendReplayTaskMessage"))
			},
			want: "sent 1 of 2 frames (message ids [m1])",
		},
		{
			name: "closed mid-send",
			args: map[string]any{"sessionId": "s1", "frames": []map[string]any{{"body": "x"}}},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
				env.Mock.On("SendReplayWsMessage", wsGone("sendReplayTaskMessage"))
			},
			want: "no open WebSocket connection",
		},
	})
}

func TestWsFrameBody(t *testing.T) {
	if b, enc, tr := wsFrameBody(replay.WSMessage{Format: "TEXT", Data: []byte("héllo")}, 2); b != "h" || enc != "" || !tr {
		t.Fatalf("want the split rune dropped, got %q %q %v", b, enc, tr)
	}
	if _, enc, _ := wsFrameBody(replay.WSMessage{Format: "TEXT", Data: []byte("a\xffbcdef")}, 4); enc != "base64" {
		t.Fatalf("want invalid UTF-8 inside the kept part as base64, got %q", enc)
	}
	if b, _, _ := wsFrameBody(replay.WSMessage{Format: "CLOSE", Data: []byte("\x03\xe8bye")}, 100); b != "1000 bye" {
		t.Fatalf("want close code and reason, got %q", b)
	}
}

func TestWsRead_FiltersAndTranslatesDirection(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
	env.Mock.On("ReadReplayWsMessages", wsMessages(
		wsMsg{"1", "SERVER", "TEXT", `{"type":"connection_init"}`}, // sent by us
		wsMsg{"2", "CLIENT", "TEXT", `{"type":"connection_ack"}`},
		wsMsg{"3", "CLIENT", "TEXT", `{"type":"ka"}`},
		wsMsg{"4", "CLIENT", "PING", ""},
		wsMsg{"5", "CLIENT", "BINARY", "\xff\x00"},
	))

	result := env.CallTool(t, "caido_ws_read", map[string]any{
		"sessionId": "s1", "skipBodies": []string{`{"type":"ka"}`},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[WsReadOutput](t, result)
	if len(out.Frames) != 2 || out.Skipped != 3 || out.Cursor != "c5" || !out.Matched {
		t.Fatalf("want 2 frames (ack, binary) and 3 skipped, got %+v", out)
	}
	if f := out.Frames[0]; f.Direction != "received" || f.Body != `{"type":"connection_ack"}` {
		t.Fatalf("unexpected first frame %+v", f)
	}
	if f := out.Frames[1]; f.Encoding != "base64" || f.Body != testutil.B64("\xff\x00") {
		t.Fatalf("want binary frame as base64, got %+v", f)
	}
	if out.Open == nil || !*out.Open {
		t.Fatalf("want open=true, got %v", out.Open)
	}
	if v := env.Mock.LastVariables("ReadReplayWsMessages"); v["streamId"] != "st1" {
		t.Fatalf("want stream st1 read, got %v", v)
	}
}

func TestWsRead_UntilContainsAndIncludeSent(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ReadReplayWsMessages", wsMessages(
		wsMsg{"1", "SERVER", "TEXT", "hello"},
		wsMsg{"2", "CLIENT", "TEXT", `{"type":"data","id":"1"}`},
		wsMsg{"3", "CLIENT", "TEXT", `{"type":"data","id":"2"}`},
	))
	result := env.CallTool(t, "caido_ws_read", map[string]any{
		"streamId": "st9", "after": "c0", "includeSent": true, "untilContains": `"id":"1"`,
	})
	out := testutil.UnmarshalToolResult[WsReadOutput](t, result)
	if len(out.Frames) != 2 || out.Frames[0].Direction != "sent" || out.Cursor != "c2" || !out.Matched {
		t.Fatalf("want stop at the matching frame with cursor c2, got %+v", out)
	}
	if out.Open != nil {
		t.Fatalf("open must be absent when reading by streamId, got %v", *out.Open)
	}
	if v := env.Mock.LastVariables("ReadReplayWsMessages"); v["after"] != "c0" {
		t.Fatalf("want after cursor passed through, got %v", v)
	}
}

func TestWsRead_RejectsBothUntilRules(t *testing.T) {
	env := newWsEnv(t)
	result := env.CallTool(t, "caido_ws_read", map[string]any{
		"streamId": "st1", "untilCount": 2, "untilContains": "x",
	})
	if !result.IsError || !strings.Contains(toolText(result), "at most one") {
		t.Fatalf("want a conflict error, got %s", toolText(result))
	}
}

func TestWsRead_DrainsAvailableFramesWithoutRule(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ReadReplayWsMessages", wsMessages(
		wsMsg{"1", "CLIENT", "TEXT", "a"}, wsMsg{"2", "CLIENT", "TEXT", "b"}, wsMsg{"3", "CLIENT", "TEXT", "c"},
	))
	out := testutil.UnmarshalToolResult[WsReadOutput](t,
		env.CallTool(t, "caido_ws_read", map[string]any{"streamId": "st1"}))
	if len(out.Frames) != 3 || !out.Matched || out.Cursor != "c3" {
		t.Fatalf("want all 3 stored frames returned, got %+v", out)
	}
}

// A frame larger than bodyLimit is cut, but untilContains still sees the
// whole body.
func TestWsRead_CapsBodyAfterMatching(t *testing.T) {
	env := newWsEnv(t)
	big := strings.Repeat("x", 100) + "needle"
	env.Mock.On("ReadReplayWsMessages", wsMessages(wsMsg{"1", "CLIENT", "TEXT", big}))
	out := testutil.UnmarshalToolResult[WsReadOutput](t, env.CallTool(t, "caido_ws_read", map[string]any{
		"streamId": "st1", "untilContains": "needle", "bodyLimit": 10,
	}))
	if !out.Matched || len(out.Frames) != 1 {
		t.Fatalf("want the match found past the cap, got %+v", out)
	}
	if f := out.Frames[0]; !f.Truncated || f.Body != strings.Repeat("x", 10) || f.Length != len(big) {
		t.Fatalf("want a 10-byte truncated body with the full length, got %+v", f)
	}
}

// A stream of nothing but skipped frames stops at the scan cap with a
// resumable cursor instead of scanning without end.
func TestWsRead_StopsAtScanCap(t *testing.T) {
	env := newWsEnv(t)
	msgs := make([]wsMsg, 100)
	for i := range msgs {
		msgs[i] = wsMsg{strconv.Itoa(i), "CLIENT", "PING", ""}
	}
	page := wsMessages(msgs...)
	dig[map[string]any](t, page, "streamWsMessages")["pageInfo"] = map[string]any{"hasNextPage": true}
	env.Mock.On("ReadReplayWsMessages", page) // served for every page
	out := testutil.UnmarshalToolResult[WsReadOutput](t, env.CallTool(t, "caido_ws_read", map[string]any{
		"streamId": "st1",
	}))
	if !out.More || out.Matched || out.Skipped != 10000 || out.Cursor == "" {
		t.Fatalf("want more=true after 10000 skipped frames, got %+v", out)
	}
}

func TestWsRead_HandshakePending(t *testing.T) {
	runToolErrCases(t, "caido_ws_read", []toolErrCase{{
		name: "open task without a stream",
		args: map[string]any{"sessionId": "s1"},
		mock: func(env *testutil.MCPTestEnv) {
			env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", ""))
		},
		want: "handshake still pending",
	}})
}

func TestWsRead_ClosedSessionDoesNotWait(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	env.Mock.On("ReadReplayWsMessages", wsMessages())
	result := env.CallTool(t, "caido_ws_read", map[string]any{"sessionId": "s1", "waitMs": 120000})
	out := testutil.UnmarshalToolResult[WsReadOutput](t, result)
	if out.Open == nil || *out.Open || out.Matched || len(out.Frames) != 0 {
		t.Fatalf("want an immediate empty read with open=false, got %+v", out)
	}
}

func TestWsRead_NoticesClose(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	env.Mock.On("ReadReplayWsMessages", wsMessages())
	result := env.CallTool(t, "caido_ws_read", map[string]any{"sessionId": "s1", "waitMs": 120000})
	out := testutil.UnmarshalToolResult[WsReadOutput](t, result)
	if out.Open == nil || *out.Open {
		t.Fatalf("want open=false once the task ended, got %+v", out)
	}
}

func TestWsClose(t *testing.T) {
	env := newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "t1", "e1", "st1"))
	env.Mock.On("StopReplayWsTasks", wsStopped())
	out := testutil.UnmarshalToolResult[WsCloseOutput](t,
		env.CallTool(t, "caido_ws_close", map[string]any{"sessionId": "s1"}))
	if !out.Closed {
		t.Fatalf("want closed=true, got %+v", out)
	}

	env = newWsEnv(t)
	env.Mock.On("ListReplayWsTasks", wsTasks("s1", "", "", ""))
	env.Mock.On("GetReplayWsSession", wsSession("s1", "h1", "e1", "st1"))
	out = testutil.UnmarshalToolResult[WsCloseOutput](t,
		env.CallTool(t, "caido_ws_close", map[string]any{"sessionId": "s1"}))
	if out.Closed {
		t.Fatalf("want closed=false with nothing open, got %+v", out)
	}
}

func TestWsClose_RejectsNonWsSession(t *testing.T) {
	runToolErrCases(t, "caido_ws_close", []toolErrCase{{
		name: "http session",
		args: map[string]any{"sessionId": "s9"},
		mock: func(env *testutil.MCPTestEnv) {
			env.Mock.On("ListReplayWsTasks", wsTasks("s9", "", "", ""))
			env.Mock.On("GetReplayWsSession", map[string]any{"replaySession": map[string]any{
				"__typename": "ReplaySessionHttp", "id": "s9",
			}})
		},
		want: "not a WebSocket one",
	}})
}

// dig walks nested JSON objects along keys and returns the value as T,
// failing the test on a missing key or a wrong type.
func dig[T any](t *testing.T, v any, keys ...string) T {
	t.Helper()
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("want an object holding %q, got %T", k, v)
		}
		v = m[k]
	}
	out, ok := v.(T)
	if !ok {
		t.Fatalf("want %T, got %T (%v)", out, v, v)
	}
	return out
}

func toolText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
