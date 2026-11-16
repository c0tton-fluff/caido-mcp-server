package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// pluginPackagesFixture mirrors a GraphQL pluginPackages response with two
// packages that have backends and one frontend-only package.
func pluginPackagesFixture() map[string]any {
	return map[string]any{
		"pluginPackages": []map[string]any{
			{
				"id":          "pkg-1",
				"manifestId":  "quickssrf",
				"name":        "QuickSSRF",
				"description": "SSRF and OAST",
				"version":     "1.0.4",
				"plugins": []map[string]any{
					{
						"__typename": "PluginBackend",
						"id":         "backend-1",
						"enabled":    true,
						"manifestId": "quickssrf-backend",
						"name":       "backend",
						"state": map[string]any{
							"running": true,
							"error":   nil,
						},
					},
					{
						"__typename": "PluginFrontend",
						"id":         "frontend-1",
						"enabled":    true,
					},
				},
			},
			{
				"id":          "pkg-2",
				"manifestId":  "scanner",
				"name":        "Scanner",
				"description": "Active scanning",
				"version":     "1.0.10",
				"plugins": []map[string]any{
					{
						"__typename": "PluginBackend",
						"id":         "backend-2",
						"enabled":    true,
						"manifestId": "backend",
						"name":       "backend",
						"state": map[string]any{
							"running": true,
							"error":   nil,
						},
					},
				},
			},
			{
				"id":         "pkg-3",
				"manifestId": "ui-only",
				"name":       "UI Only",
				"version":    "0.1.0",
				"plugins": []map[string]any{
					{
						"__typename": "PluginFrontend",
						"id":         "frontend-3",
						"enabled":    true,
					},
				},
			},
		},
	}
}

// newPluginCallEnv registers caido_call_plugin against the mock GraphQL server
// and the mock plugin RPC route, and configures a static access token.
func newPluginCallEnv(
	t *testing.T,
	packages map[string]any,
	callStatus int,
	callBody string,
) *testutil.MCPTestEnv {
	t.Helper()
	t.Setenv("CAIDO_ACCESS_TOKEN", "test-access-token")
	t.Setenv("CAIDO_PAT", "")

	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterCallPluginTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("PluginPackagesWithBackends", packages)
	if callStatus != 0 {
		env.Mock.OnPluginCall(callStatus, callBody)
	}
	return env
}

func successEnvelope(returns string) string {
	body, _ := json.Marshal(map[string]any{
		"kind":    "success",
		"returns": returns,
	})
	return string(body)
}

// TestCallPluginSuccess asserts the resolved plugin, the parsed returns value
// and the token on the REST call.
func TestCallPluginSuccess(t *testing.T) {
	returns := `{"kind":"Ok","value":{"id":"session-1","url":"http://oast.test/x"}}`
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 200, successEnvelope(returns),
	)

	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "QuickSSRF",
		"function": "createSession",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	out := testutil.UnmarshalToolResult[tools.PluginCallOutput](t, result)
	if out.Plugin != "QuickSSRF" {
		t.Errorf("plugin = %q", out.Plugin)
	}
	if out.ManifestID != "quickssrf" {
		t.Errorf("manifestId = %q", out.ManifestID)
	}
	if out.BackendID != "backend-1" {
		t.Errorf("backendId = %q", out.BackendID)
	}
	if out.Function != "createSession" {
		t.Errorf("function = %q", out.Function)
	}
	if out.TotalBytes != len(returns) {
		t.Errorf("totalBytes = %d, want %d", out.TotalBytes, len(returns))
	}
	parsed, ok := out.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is %T, want an object", out.Result)
	}
	if parsed["kind"] != "Ok" {
		t.Errorf("result.kind = %v", parsed["kind"])
	}

	call := env.Mock.LastPluginCall()
	if call == nil {
		t.Fatal("the plugin route was never called")
	}
	if call.BackendID != "backend-1" {
		t.Errorf("backend path = %q", call.BackendID)
	}
	if call.Name != "createSession" {
		t.Errorf("function sent = %q", call.Name)
	}
	if call.Authorization != "Bearer test-access-token" {
		t.Errorf("authorization = %q", call.Authorization)
	}
	if call.Args == nil || len(call.Args) != 0 {
		t.Errorf("args = %v, want an empty array", call.Args)
	}
}

// TestCallPluginArgsEncoding asserts every argument is JSON-encoded into its
// own string, the wire shape the plugin route expects.
func TestCallPluginArgsEncoding(t *testing.T) {
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 200, successEnvelope(`{}`),
	)

	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "updateSessionTitle",
		"args": []any{
			"session-1", 42, map[string]any{"a": true}, true, nil,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	call := env.Mock.LastPluginCall()
	if call == nil {
		t.Fatal("the plugin route was never called")
	}
	want := []string{`"session-1"`, "42", `{"a":true}`, "true", "null"}
	if len(call.Args) != len(want) {
		t.Fatalf("args = %v, want %v", call.Args, want)
	}
	for i := range want {
		if call.Args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, call.Args[i], want[i])
		}
	}
}

// TestCallPluginNullReturns asserts a JSON null returns value is reported as
// no result rather than the string "null".
func TestCallPluginNullReturns(t *testing.T) {
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 200, successEnvelope("null"),
	)
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "startPolling",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.PluginCallOutput](t, result)
	if out.Result != nil {
		t.Errorf("result = %v, want nil", out.Result)
	}
	if out.TotalBytes != 0 {
		t.Errorf("totalBytes = %d, want 0", out.TotalBytes)
	}
}

// TestCallPluginInvalidProcedure asserts the error points at the plugin
// backend source for function names.
func TestCallPluginInvalidProcedure(t *testing.T) {
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 200,
		`{"kind":"error","error":{"kind":"invalid_procedure","name":"nope"}}`,
	)
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "nope",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		t.Fatal("expected text content")
	}
	if !strings.Contains(text.Text, "sdk.api.register") {
		t.Errorf("error does not point at the backend source: %s", text.Text)
	}
	if !strings.Contains(text.Text, `"nope"`) {
		t.Errorf("error does not name the function: %s", text.Text)
	}
}

// TestCallPluginHTTP500 asserts HTTP 500 maps to the restart hint.
func TestCallPluginHTTP500(t *testing.T) {
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 500,
		`{"kind":"error","error":{"kind":"internal_server_error"}}`,
	)
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "createSession",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "restart Caido") {
		t.Fatalf("error lacks the restart hint: %v", text)
	}
	if !strings.Contains(text.Text, "not callable until then") {
		t.Errorf("error lacks the explanation: %s", text.Text)
	}
}

// TestCallPluginHTTP401 asserts an invalid token is reported with the fix.
func TestCallPluginHTTP401(t *testing.T) {
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 401, `{"error":"INVALID_TOKEN"}`,
	)
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "createSession",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "CAIDO_ACCESS_TOKEN") {
		t.Fatalf("error lacks the token guidance: %v", text)
	}
}

// TestCallPluginUnknownPlugin asserts the error lists what is installed and
// that no REST call is made.
func TestCallPluginUnknownPlugin(t *testing.T) {
	env := newPluginCallEnv(t, pluginPackagesFixture(), 200, "")

	for _, name := range []string{"nope", "backend"} {
		result := env.CallTool(t, "caido_call_plugin", map[string]any{
			"plugin":   name,
			"function": "createSession",
		})
		if !result.IsError {
			t.Fatalf("%s: expected error", name)
		}
		text, _ := result.Content[0].(*mcp.TextContent)
		if text == nil || !strings.Contains(text.Text, "installed plugins:") {
			t.Fatalf("%s: error does not list installed plugins: %v", name, text)
		}
		for _, want := range []string{"quickssrf", "scanner"} {
			if !strings.Contains(text.Text, want) {
				t.Errorf("%s: error does not list %q: %s", name, want, text.Text)
			}
		}
	}
	if call := env.Mock.LastPluginCall(); call != nil {
		t.Errorf("an unknown plugin still reached the REST route: %+v", call)
	}
}

// TestCallPluginMatching asserts manifestId, name, package id, backend id and
// case are all accepted.
func TestCallPluginMatching(t *testing.T) {
	for _, name := range []string{
		"quickssrf", "QuickSSRF", "QUICKSSRF", "pkg-1", "backend-1",
	} {
		t.Run(name, func(t *testing.T) {
			env := newPluginCallEnv(
				t, pluginPackagesFixture(), 200, successEnvelope(`{"ok":true}`),
			)
			result := env.CallTool(t, "caido_call_plugin", map[string]any{
				"plugin":   name,
				"function": "createSession",
			})
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.PluginCallOutput](t, result)
			if out.ManifestID != "quickssrf" {
				t.Errorf("manifestId = %q, want quickssrf", out.ManifestID)
			}
		})
	}
}

// TestCallPluginNotRunning asserts a stopped backend is refused before the
// REST call and says how to fix it.
func TestCallPluginNotRunning(t *testing.T) {
	packages := pluginPackagesFixture()
	backend := packages["pluginPackages"].([]map[string]any)[0]["plugins"].([]map[string]any)[0]
	backend["state"] = map[string]any{
		"running": false,
		"error":   "boot failed",
	}
	env := newPluginCallEnv(t, packages, 200, "")

	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "createSession",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "restart Caido") {
		t.Fatalf("error lacks the restart hint: %v", text)
	}
	if !strings.Contains(text.Text, "boot failed") {
		t.Errorf("error does not include the backend error: %s", text.Text)
	}
	if call := env.Mock.LastPluginCall(); call != nil {
		t.Errorf("a stopped backend still reached the REST route: %+v", call)
	}
}

// TestCallPluginNoBackend asserts a frontend-only package is rejected.
func TestCallPluginNoBackend(t *testing.T) {
	env := newPluginCallEnv(t, pluginPackagesFixture(), 200, "")
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "ui-only",
		"function": "anything",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "has no backend") {
		t.Fatalf("unexpected error: %v", text)
	}
}

// TestCallPluginTruncation asserts maxBytes caps the returned text, keeps the
// total size and flags the cut.
func TestCallPluginTruncation(t *testing.T) {
	returns := strings.Repeat("x", 100)
	env := newPluginCallEnv(
		t, pluginPackagesFixture(), 200, successEnvelope(returns),
	)
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin":   "quickssrf",
		"function": "getConfig",
		"maxBytes": 10,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.PluginCallOutput](t, result)
	if !out.Truncated {
		t.Error("expected truncated")
	}
	if out.TotalBytes != len(returns) {
		t.Errorf("totalBytes = %d, want %d", out.TotalBytes, len(returns))
	}
	text, ok := out.Result.(string)
	if !ok {
		t.Fatalf("result is %T, want the truncated string", out.Result)
	}
	if len(text) != 10 {
		t.Errorf("result length = %d, want 10", len(text))
	}
}

// TestCallPluginRequiresFunction asserts a blank function fails locally.
func TestCallPluginRequiresFunction(t *testing.T) {
	env := newPluginCallEnv(t, pluginPackagesFixture(), 200, "")
	result := env.CallTool(t, "caido_call_plugin", map[string]any{
		"plugin": "quickssrf",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if call := env.Mock.LastPluginCall(); call != nil {
		t.Errorf("a blank function reached the REST route: %+v", call)
	}
}
