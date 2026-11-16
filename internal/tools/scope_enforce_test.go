package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sendOps are the GraphQL operations that put a request on the wire or
// stage one in Caido. None may run for a refused target.
var sendOps = []string{
	"CreateReplaySession", "StartReplayTask", "UpdateReplayEntryDraft",
	"CreateReplayPipelineSession", "StartReplayPipelineTask",
	"StartReplayWsTask", "UpdateReplayWsHandshake",
}

func newEnforcedEnv(t *testing.T, scopes []map[string]any) *testutil.MCPTestEnv {
	t.Helper()
	// The guard is process-wide: reset it and the default session cache so
	// this test neither inherits nor leaks state.
	replay.ResetDefaultSession("")
	t.Cleanup(func() {
		replay.SetTargetGuard(nil)
		replay.ResetDefaultSession("")
	})
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		if _, err := tools.RegisterAllWith(
			context.Background(), s, c, tools.Options{EnforceScope: true},
		); err != nil {
			t.Fatalf("RegisterAllWith: %v", err)
		}
	})
	env.Mock.On("ListScopes", map[string]any{"scopes": scopes})
	return env
}

func scopeRow(id, name string, allow, deny []string) map[string]any {
	return map[string]any{"id": id, "name": name, "allowlist": allow, "denylist": deny, "indexed": true}
}

const outRaw = "GET / HTTP/1.1\r\nHost: out.example\r\n\r\n"

// TestScopeEnforcementRefusesEverySendPath drives each tool that contacts a
// target at an out-of-scope host and requires a scope refusal with nothing
// sent. A tool missing from this table is a tool that could go around the
// guard.
func TestScopeEnforcementRefusesEverySendPath(t *testing.T) {
	scopes := []map[string]any{scopeRow("1", "prod", []string{"*.in.example"}, []string{})}
	tests := []struct {
		tool string
		args map[string]any
		// perItem: the tool reports the refusal inside a successful result
		// (one entry per request) instead of failing the call.
		perItem bool
		setup   func(env *testutil.MCPTestEnv)
	}{
		{tool: "caido_send_request", args: map[string]any{"raw": outRaw, "host": "out.example"}},
		{tool: "caido_send_request", args: map[string]any{"raw": outRaw, "name": "titled"}},
		{tool: "caido_send_request", args: map[string]any{"curl": "curl https://out.example/x"}},
		{
			tool: "caido_edit_request", args: map[string]any{"requestId": "req-src"},
			setup: func(env *testutil.MCPTestEnv) {
				// The captured request's own host (example.com) is the target.
				env.Mock.On("GetRequest", testutil.GetRequestFullResponse("req-src", "body"))
			},
		},
		{
			tool: "caido_batch_send", perItem: true,
			args: map[string]any{"requests": []map[string]any{{"raw": outRaw, "label": "a"}}},
		},
		{
			tool: "caido_race_pipeline",
			args: map[string]any{"host": "out.example", "requests": []map[string]any{{"raw": outRaw}}},
		},
		{
			tool: "caido_race_window_send",
			args: map[string]any{"host": "out.example", "requests": []map[string]any{{"raw": outRaw}}},
		},
		{tool: "caido_ws_connect", args: map[string]any{"url": "wss://out.example/ws"}},
		{tool: "caido_ws_connect", args: map[string]any{"url": "wss://out.example/ws", "connect": false}},
		{tool: "caido_ws_connect", args: map[string]any{"raw": "GET /ws HTTP/1.1\r\nHost: out.example\r\n\r\n", "host": "out.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			env := newEnforcedEnv(t, scopes)
			if tt.setup != nil {
				tt.setup(env)
			}
			result := env.CallTool(t, tt.tool, tt.args)
			text := contentText(result)
			if !strings.Contains(text, "scope enforcement") {
				t.Fatalf("want a scope enforcement refusal, got isError=%v: %s", result.IsError, text)
			}
			if !tt.perItem && !result.IsError {
				t.Fatalf("refusal must be a tool error, got success: %s", text)
			}
			for _, op := range sendOps {
				if env.Mock.LastVariables(op) != nil {
					t.Errorf("%s ran for a refused target", op)
				}
			}
		})
	}
}

// TestScopeEnforcementAllowsInScope is the positive control: with the same
// guard on, an in-scope host is sent. Without it the refusal tests would
// also pass against a guard that refuses everything.
func TestScopeEnforcementAllowsInScope(t *testing.T) {
	env := newEnforcedEnv(t, []map[string]any{
		scopeRow("1", "prod", []string{"*.in.example"}, []string{"admin.in.example"}),
	})
	env.Mock.On("CreateReplaySession", testutil.CreateReplaySessionSeededResponse("s-1", "entry-1"))
	env.Mock.On("RenameReplaySession", testutil.RenameReplaySessionResponse("s-1", "ok"))
	env.Mock.On("StartReplayTask", testutil.StartReplayTaskResponse())
	env.Mock.On("GetReplaySession", testutil.GetReplaySessionResponse("s-1", "entry-1"))
	env.Mock.On("GetReplayEntry", testutil.GetReplayEntryResponse("entry-1", "req-1", 200, "ok"))

	result := env.CallTool(t, "caido_send_request", map[string]any{
		"raw": "GET / HTTP/1.1\r\nHost: api.in.example\r\n\r\n", "name": "ok",
	})
	if result.IsError {
		t.Fatalf("in-scope host was refused: %s", contentText(result))
	}
	if env.Mock.LastVariables("StartReplayTask") == nil {
		t.Fatal("in-scope request was not sent")
	}

	// The denylist still wins inside an allowed pattern.
	denied := env.CallTool(t, "caido_send_request", map[string]any{
		"raw": "GET / HTTP/1.1\r\nHost: admin.in.example\r\n\r\n", "name": "no",
	})
	if !denied.IsError || !strings.Contains(contentText(denied), "excluded by denylist rule") {
		t.Fatalf("denylisted host must be refused with the rule, got: %s", contentText(denied))
	}
}

// TestScopeEnforcementFailsClosed: when the boundary cannot be established
// the answer is no.
func TestScopeEnforcementFailsClosed(t *testing.T) {
	args := map[string]any{"raw": "GET / HTTP/1.1\r\nHost: api.in.example\r\n\r\n", "name": "x"}
	tests := []struct {
		name   string
		scopes []map[string]any // nil = ListScopes is not answered at all
		want   string
	}{
		{"scopes cannot be read", nil, "cannot read scopes, refusing to send"},
		{"project has no scopes", []map[string]any{}, "no scope with an allowlist"},
		{
			"only an empty-allowlist scope",
			[]map[string]any{scopeRow("1", "open", []string{}, []string{})},
			"no scope with an allowlist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replay.ResetDefaultSession("")
			t.Cleanup(func() { replay.SetTargetGuard(nil); replay.ResetDefaultSession("") })
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				if _, err := tools.RegisterAllWith(
					context.Background(), s, c, tools.Options{EnforceScope: true},
				); err != nil {
					t.Fatalf("RegisterAllWith: %v", err)
				}
			})
			if tt.scopes != nil {
				env.Mock.On("ListScopes", map[string]any{"scopes": tt.scopes})
			}
			result := env.CallTool(t, "caido_send_request", args)
			if !result.IsError || !strings.Contains(contentText(result), tt.want) {
				t.Fatalf("want refusal containing %q, got isError=%v: %s", tt.want, result.IsError, contentText(result))
			}
			if env.Mock.LastVariables("StartReplayTask") != nil || env.Mock.LastVariables("CreateReplaySession") != nil {
				t.Fatal("request was sent although the scope could not be established")
			}
		})
	}
}

// TestScopeEnforcementLocksScopeTools: with enforcement on, the tools that
// could move the boundary are gone, and everything else is still there.
func TestScopeEnforcementLocksScopeTools(t *testing.T) {
	var all int
	testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		all, _ = tools.RegisterAllWith(context.Background(), s, c, tools.Options{})
	})
	var reported int
	t.Cleanup(func() { replay.SetTargetGuard(nil) })
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		reported, _ = tools.RegisterAllWith(context.Background(), s, c, tools.Options{EnforceScope: true})
	})
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	locked := []string{
		"caido_create_scope", "caido_update_scope", "caido_delete_scope",
		"caido_select_project", "caido_create_project", "caido_delete_project",
	}
	for _, name := range locked {
		if names[name] {
			t.Errorf("%s must be withheld while scope is enforced", name)
		}
	}
	for _, name := range []string{"caido_send_request", "caido_is_in_scope", "caido_list_scopes", "caido_create_finding"} {
		if !names[name] {
			t.Errorf("%s should stay available while scope is enforced", name)
		}
	}
	// Exactly the locked tools are missing: a name in the lock list that is
	// not a real tool would make these counts disagree.
	if len(listed.Tools) != all-len(locked) || reported != len(listed.Tools) {
		t.Errorf("enforced server lists %d tools (reported %d), want %d", len(listed.Tools), reported, all-len(locked))
	}
}

// TestNoEnforcementByDefault: without the option an out-of-scope host is not
// refused by the guard, and registering a default server clears a guard an
// earlier server installed.
func TestNoEnforcementByDefault(t *testing.T) {
	replay.SetTargetGuard(func(context.Context, *caido.Client, string) error {
		t.Error("stale guard consulted after a default server was registered")
		return nil
	})
	replay.ResetDefaultSession("")
	t.Cleanup(func() { replay.SetTargetGuard(nil); replay.ResetDefaultSession("") })
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})
	result := env.CallTool(t, "caido_send_request", map[string]any{"raw": outRaw, "name": "x"})
	if strings.Contains(contentText(result), "scope enforcement") {
		t.Fatalf("default server refused on scope: %s", contentText(result))
	}
	if env.Mock.LastVariables("ListScopes") != nil {
		t.Fatal("default server looked up scopes before sending")
	}
}
