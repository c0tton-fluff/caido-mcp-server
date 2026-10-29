package tools_test

import (
	"reflect"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func passthroughWire(outOfScope bool, allow, deny []string) map[string]any {
	return map[string]any{"passthroughOptions": map[string]any{
		"allowlist": allow, "denylist": deny, "outOfScope": outOfScope,
	}}
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
		"passthroughOptions": passthroughWire(false, []string{"*.okta.com"}, nil),
	})
	result := env.CallTool(t, "caido_passthrough_options", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.PassthroughOutput](t, result)
	if out.Changed || out.Options.OutOfScope ||
		!reflect.DeepEqual(out.Options.Allowlist, []string{"*.okta.com"}) ||
		out.Options.Denylist == nil {
		t.Fatalf("unexpected read %+v", out)
	}
	if env.Mock.LastVariables("SetPassthroughOptions") != nil {
		t.Fatal("a call with no fields must not write")
	}
}

func TestPassthroughOptions_UpdateKeepsOmittedFields(t *testing.T) {
	env := newPassthroughEnv(t)
	env.Mock.On("GetPassthroughOptions", map[string]any{
		"passthroughOptions": passthroughWire(false, []string{"*.okta.com"}, []string{"api.test"}),
	})
	env.Mock.On("SetPassthroughOptions", map[string]any{"setPassthroughOptions": map[string]any{
		"options": passthroughWire(true, []string{"*.okta.com"}, []string{"api.test"}),
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
	targets, _ := input["targets"].(map[string]any)
	if input["outofscope"] != true ||
		!reflect.DeepEqual(targets["allowlist"], []any{"*.okta.com"}) ||
		!reflect.DeepEqual(targets["denylist"], []any{"api.test"}) {
		t.Fatalf("omitted lists must be sent back unchanged, got %v", input)
	}
}

func TestPassthroughOptions_EmptyListClears(t *testing.T) {
	env := newPassthroughEnv(t)
	env.Mock.On("GetPassthroughOptions", map[string]any{
		"passthroughOptions": passthroughWire(true, []string{"*.okta.com"}, nil),
	})
	env.Mock.On("SetPassthroughOptions", map[string]any{"setPassthroughOptions": map[string]any{
		"options": passthroughWire(true, nil, nil),
	}})
	result := env.CallTool(t, "caido_passthrough_options", map[string]any{"allowlist": []string{}})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	input, _ := env.Mock.LastVariables("SetPassthroughOptions")["input"].(map[string]any)
	targets, _ := input["targets"].(map[string]any)
	if got, ok := targets["allowlist"].([]any); !ok || len(got) != 0 {
		t.Fatalf("want allowlist cleared to [], got %#v", targets["allowlist"])
	}
}
