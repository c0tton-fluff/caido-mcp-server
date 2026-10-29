package tools_test

import (
	"reflect"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func passthroughWire(capture, pass []string, outOfScope bool) map[string]any {
	return map[string]any{
		"captureList": capture, "passthroughList": pass, "outOfScope": outOfScope,
	}
}

func newPassthroughEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterPassthroughTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	return env
}

func TestPassthroughOptions_ReadOnly(t *testing.T) {
	env := newPassthroughEnv(t)
	env.Mock.On("GetPassthroughOptions", map[string]any{
		"passthroughOptions": passthroughWire([]string{}, []string{"*.okta.com"}, false),
	})
	result := env.CallTool(t, "caido_passthrough_options", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.PassthroughOutput](t, result)
	if out.Changed || out.Options.OutOfScope ||
		!reflect.DeepEqual(out.Options.PassthroughList, []string{"*.okta.com"}) {
		t.Fatalf("unexpected read %+v", out)
	}
	if env.Mock.LastVariables("SetPassthroughOptions") != nil {
		t.Fatal("a call with no fields must not write")
	}
}

// The set mutation requires every field, so an update must send back the
// fields the caller omitted, captureList included.
func TestPassthroughOptions_UpdateKeepsOmittedFields(t *testing.T) {
	env := newPassthroughEnv(t)
	env.Mock.On("GetPassthroughOptions", map[string]any{
		"passthroughOptions": passthroughWire([]string{"api.test"}, []string{"*.okta.com"}, false),
	})
	env.Mock.On("SetPassthroughOptions", map[string]any{"setPassthroughOptions": map[string]any{
		"options": passthroughWire([]string{"api.test"}, []string{"*.okta.com"}, true),
	}})
	result := env.CallTool(t, "caido_passthrough_options", map[string]any{"outOfScope": true})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.PassthroughOutput](t, result)
	if !out.Changed || !out.Options.OutOfScope {
		t.Fatalf("want changed outOfScope=true, got %+v", out)
	}
	input, _ := env.Mock.LastVariables("SetPassthroughOptions")["input"].(map[string]any)
	if input["outOfScope"] != true ||
		!reflect.DeepEqual(input["passthroughList"], []any{"*.okta.com"}) ||
		!reflect.DeepEqual(input["captureList"], []any{"api.test"}) {
		t.Fatalf("omitted fields must be sent back unchanged, got %v", input)
	}
}

func TestPassthroughOptions_EmptyListClears(t *testing.T) {
	env := newPassthroughEnv(t)
	env.Mock.On("GetPassthroughOptions", map[string]any{
		"passthroughOptions": passthroughWire(nil, []string{"*.okta.com"}, true),
	})
	env.Mock.On("SetPassthroughOptions", map[string]any{"setPassthroughOptions": map[string]any{
		"options": passthroughWire([]string{}, []string{}, true),
	}})
	result := env.CallTool(t, "caido_passthrough_options", map[string]any{"passthroughList": []string{}})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	input, _ := env.Mock.LastVariables("SetPassthroughOptions")["input"].(map[string]any)
	if got, ok := input["passthroughList"].([]any); !ok || len(got) != 0 {
		t.Fatalf("want passthroughList cleared to [], got %#v", input["passthroughList"])
	}
	// A null captureList read back must go out as [], not null: the
	// input field is a non-null list.
	if got, ok := input["captureList"].([]any); !ok || len(got) != 0 {
		t.Fatalf("want captureList sent as [], got %#v", input["captureList"])
	}
}
