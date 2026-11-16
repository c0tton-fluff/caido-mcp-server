package tools_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newUpdateEnvironmentEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterUpdateEnvironmentTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	return env
}

func envVar(name, value, kind string) map[string]any {
	return map[string]any{"name": name, "value": value, "kind": kind}
}

func storedEnvironment() map[string]any {
	return map[string]any{"environment": map[string]any{
		"id": "3", "name": "target", "version": 4,
		"variables": []map[string]any{
			envVar("token", "old-secret", "SECRET"),
			envVar("userId", "41", "PLAIN"),
		},
	}}
}

func updatedEnvironment(name string, vars ...[2]string) map[string]any {
	// The mock answers with values too, as a server asked for them would:
	// the tool's output type is what must keep them out, and the leak check
	// in TestUpdateEnvironment only means something if they are on offer.
	refs := make([]map[string]any, len(vars))
	for i, v := range vars {
		refs[i] = map[string]any{"name": v[0], "kind": v[1], "value": "leaked-" + v[0]}
	}
	return map[string]any{"updateEnvironment": map[string]any{
		"environment": map[string]any{"id": "3", "name": name, "version": 5, "variables": refs},
		"error":       nil,
	}}
}

// sentVariables returns the variables list sent to updateEnvironment as
// name/value/kind triples.
func sentVariables(t *testing.T, env *testutil.MCPTestEnv) (map[string]any, [][3]string) {
	t.Helper()
	vars := env.Mock.LastVariables("UpdateEnvironment")
	if vars == nil {
		t.Fatal("UpdateEnvironment was not called")
	}
	in, _ := vars["input"].(map[string]any)
	list, ok := in["variables"].([]any)
	if !ok {
		t.Fatalf("variables must be a JSON list, got %#v", in["variables"])
	}
	out := make([][3]string, len(list))
	for i, e := range list {
		m, _ := e.(map[string]any)
		out[i] = [3]string{str(m["name"]), str(m["value"]), str(m["kind"])}
	}
	return in, out
}

func str(v any) string { s, _ := v.(string); return s }

// TestUpdateEnvironment checks the merge sent to Caido. updateEnvironment
// replaces the whole list, so an untouched variable must go back with its
// stored value and kind, and the version read must be the version written.
func TestUpdateEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantName string
		want     [][3]string
	}{
		{
			name: "overwrite keeps position and stored kind",
			args: map[string]any{"id": "3", "set": []map[string]any{
				{"name": "token", "value": "new-secret"},
			}},
			wantName: "target",
			want:     [][3]string{{"token", "new-secret", "SECRET"}, {"userId", "41", "PLAIN"}},
		},
		{
			name: "new variable appended, PLAIN by default",
			args: map[string]any{"id": "3", "set": []map[string]any{
				{"name": "csrf", "value": "abc"},
			}},
			wantName: "target",
			want: [][3]string{
				{"token", "old-secret", "SECRET"}, {"userId", "41", "PLAIN"}, {"csrf", "abc", "PLAIN"},
			},
		},
		{
			name: "explicit kind changes the stored kind",
			args: map[string]any{"id": "3", "set": []map[string]any{
				{"name": "userId", "value": "42", "kind": "SECRET"},
			}},
			wantName: "target",
			want:     [][3]string{{"token", "old-secret", "SECRET"}, {"userId", "42", "SECRET"}},
		},
		{
			name:     "remove",
			args:     map[string]any{"id": "3", "remove": []string{"token"}},
			wantName: "target",
			want:     [][3]string{{"userId", "41", "PLAIN"}},
		},
		{
			name:     "remove everything still sends a list",
			args:     map[string]any{"id": "3", "remove": []string{"token", "userId"}},
			wantName: "target",
			want:     [][3]string{},
		},
		{
			name:     "rename keeps the variables",
			args:     map[string]any{"id": "3", "name": "staging"},
			wantName: "staging",
			want:     [][3]string{{"token", "old-secret", "SECRET"}, {"userId", "41", "PLAIN"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newUpdateEnvironmentEnv(t)
			env.Mock.On("GetEnvironment", storedEnvironment())
			refs := make([][2]string, len(tt.want))
			for i, v := range tt.want {
				refs[i] = [2]string{v[0], v[2]}
			}
			env.Mock.On("UpdateEnvironment", updatedEnvironment(tt.wantName, refs...))

			result := env.CallTool(t, "caido_update_environment", tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}
			in, got := sentVariables(t, env)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sent variables = %v, want %v", got, tt.want)
			}
			if in["name"] != tt.wantName {
				t.Errorf("sent name = %#v, want %q", in["name"], tt.wantName)
			}
			if v, _ := in["version"].(float64); v != 4 {
				t.Errorf("sent version = %#v, want the version read (4)", in["version"])
			}

			out := testutil.UnmarshalToolResult[tools.UpdateEnvironmentOutput](t, result)
			if out.Version != 5 || out.Name != tt.wantName || len(out.Variables) != len(tt.want) {
				t.Errorf("output = %+v", out)
			}
			// The output names variables; it must never carry a value.
			if text := contentText(result); strings.Contains(text, "leaked-") ||
				strings.Contains(text, "old-secret") || strings.Contains(text, "new-secret") {
				t.Errorf("output leaks a variable value: %s", text)
			}
		})
	}
}

func TestUpdateEnvironment_Errors(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		stored  map[string]any
		payload map[string]any
		want    string
	}{
		{
			name: "nothing to update",
			args: map[string]any{"id": "3"},
			want: "nothing to update",
		},
		{
			name:   "unknown environment",
			args:   map[string]any{"id": "3", "name": "x"},
			stored: map[string]any{"environment": nil},
			want:   "environment 3 not found",
		},
		{
			name:   "remove of a missing variable",
			args:   map[string]any{"id": "3", "remove": []string{"nope"}},
			stored: storedEnvironment(),
			want:   `no variable named "nope"`,
		},
		{
			name: "same variable set and removed",
			args: map[string]any{
				"id": "3", "remove": []string{"token"},
				"set": []map[string]any{{"name": "token", "value": "v"}},
			},
			stored: storedEnvironment(),
			want:   "both set and remove",
		},
		{
			name:   "concurrent change explained",
			args:   map[string]any{"id": "3", "name": "x"},
			stored: storedEnvironment(),
			payload: map[string]any{"environment": nil, "error": map[string]any{
				"__typename": "NewerVersionUserError", "code": "NEWER_VERSION",
			}},
			want: "NewerVersionUserError: NEWER_VERSION; the environment changed since it was read",
		},
		{
			name:   "permission reason reported",
			args:   map[string]any{"id": "3", "name": "x"},
			stored: storedEnvironment(),
			payload: map[string]any{"environment": nil, "error": map[string]any{
				"__typename": "PermissionDeniedUserError", "code": "PERMISSION_DENIED", "reason": "ENTITLEMENT",
			}},
			want: "PermissionDeniedUserError: PERMISSION_DENIED (ENTITLEMENT)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newUpdateEnvironmentEnv(t)
			if tt.stored != nil {
				env.Mock.On("GetEnvironment", tt.stored)
			}
			if tt.payload != nil {
				env.Mock.On("UpdateEnvironment", map[string]any{"updateEnvironment": tt.payload})
			}
			result := env.CallTool(t, "caido_update_environment", tt.args)
			if !result.IsError {
				t.Fatalf("want error containing %q, got success", tt.want)
			}
			if got := contentText(result); !strings.Contains(got, tt.want) {
				t.Fatalf("error = %q, want it to contain %q", got, tt.want)
			}
			if tt.payload == nil && env.Mock.LastVariables("UpdateEnvironment") != nil {
				t.Fatal("UpdateEnvironment must not be called when validation fails")
			}
		})
	}
}
