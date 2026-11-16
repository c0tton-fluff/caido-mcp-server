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

func newUpdateScopeEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterUpdateScopeTool(s, c)
	})
	// The queries are hand-written, so check them against Caido's schema.
	env.Mock.ValidateAgainstSchema()
	return env
}

func scopeFixture(name string, allow, deny []string) map[string]any {
	return map[string]any{"id": "7", "name": name, "allowlist": allow, "denylist": deny}
}

func toStrings(t *testing.T, v any) []string {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("want a JSON list, got %#v", v)
	}
	out := make([]string, len(raw))
	for i, e := range raw {
		out[i], _ = e.(string)
	}
	return out
}

// TestUpdateScope checks what is sent to Caido: updateScope replaces all
// three fields, so whatever the caller left out must be carried over from
// the stored scope, never dropped or sent as null.
func TestUpdateScope(t *testing.T) {
	stored := scopeFixture("prod", []string{"a.example", "b.example"}, []string{"admin.a.example"})
	tests := []struct {
		name      string
		args      map[string]any
		wantName  string
		wantAllow []string
		wantDeny  []string
	}{
		{
			name:      "addAllow appends and keeps the rest",
			args:      map[string]any{"id": "7", "addAllow": []string{"c.example"}},
			wantName:  "prod",
			wantAllow: []string{"a.example", "b.example", "c.example"},
			wantDeny:  []string{"admin.a.example"},
		},
		{
			name:      "addAllow skips an entry already present",
			args:      map[string]any{"id": "7", "addAllow": []string{"b.example", "c.example", "c.example"}},
			wantName:  "prod",
			wantAllow: []string{"a.example", "b.example", "c.example"},
			wantDeny:  []string{"admin.a.example"},
		},
		{
			name:      "allowlist replaces, denylist kept",
			args:      map[string]any{"id": "7", "allowlist": []string{"z.example"}},
			wantName:  "prod",
			wantAllow: []string{"z.example"},
			wantDeny:  []string{"admin.a.example"},
		},
		{
			name:      "empty denylist clears it",
			args:      map[string]any{"id": "7", "denylist": []string{}},
			wantName:  "prod",
			wantAllow: []string{"a.example", "b.example"},
			wantDeny:  []string{},
		},
		{
			name:      "rename only",
			args:      map[string]any{"id": "7", "name": "staging"},
			wantName:  "staging",
			wantAllow: []string{"a.example", "b.example"},
			wantDeny:  []string{"admin.a.example"},
		},
		{
			name:      "replace then append",
			args:      map[string]any{"id": "7", "denylist": []string{"x.example"}, "addDeny": []string{"y.example"}},
			wantName:  "prod",
			wantAllow: []string{"a.example", "b.example"},
			wantDeny:  []string{"x.example", "y.example"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newUpdateScopeEnv(t)
			env.Mock.On("GetScope", map[string]any{"scope": stored})
			env.Mock.On("UpdateScope", map[string]any{"updateScope": map[string]any{
				"scope": scopeFixture(tt.wantName, tt.wantAllow, tt.wantDeny),
				"error": nil,
			}})

			result := env.CallTool(t, "caido_update_scope", tt.args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}
			vars := env.Mock.LastVariables("UpdateScope")
			if vars == nil {
				t.Fatal("UpdateScope was not called")
			}
			if vars["id"] != "7" {
				t.Fatalf("id variable = %#v, want 7", vars["id"])
			}
			in, _ := vars["input"].(map[string]any)
			if in["name"] != tt.wantName {
				t.Errorf("sent name = %#v, want %q", in["name"], tt.wantName)
			}
			if got := toStrings(t, in["allowlist"]); !reflect.DeepEqual(got, tt.wantAllow) {
				t.Errorf("sent allowlist = %v, want %v", got, tt.wantAllow)
			}
			if got := toStrings(t, in["denylist"]); !reflect.DeepEqual(got, tt.wantDeny) {
				t.Errorf("sent denylist = %v, want %v", got, tt.wantDeny)
			}
			out := testutil.UnmarshalToolResult[tools.UpdateScopeOutput](t, result)
			if out.ID != "7" || out.Name != tt.wantName || !reflect.DeepEqual(out.Allowlist, tt.wantAllow) {
				t.Errorf("output = %+v", out)
			}
		})
	}
}

func TestUpdateScope_Errors(t *testing.T) {
	stored := scopeFixture("prod", []string{"a.example"}, []string{})
	tests := []struct {
		name    string
		args    map[string]any
		scope   any // GetScope result; nil = scope not found
		payload map[string]any
		want    string
	}{
		{
			name: "nothing to update",
			args: map[string]any{"id": "7"},
			want: "nothing to update",
		},
		{
			name:  "unknown scope",
			args:  map[string]any{"id": "7", "name": "x"},
			scope: nil,
			want:  "scope 7 not found",
		},
		{
			name:  "allowlist emptied",
			args:  map[string]any{"id": "7", "allowlist": []string{}},
			scope: stored,
			want:  "allowlist would be empty",
		},
		{
			name:  "invalid glob reported with a hint",
			args:  map[string]any{"id": "7", "addAllow": []string{"https://a.example/x"}},
			scope: stored,
			payload: map[string]any{"scope": nil, "error": map[string]any{
				"__typename": "InvalidGlobTermsUserError", "code": "INVALID_GLOB_TERMS",
			}},
			want: "InvalidGlobTermsUserError: INVALID_GLOB_TERMS; entries are host patterns",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newUpdateScopeEnv(t)
			env.Mock.On("GetScope", map[string]any{"scope": tt.scope})
			if tt.payload != nil {
				env.Mock.On("UpdateScope", map[string]any{"updateScope": tt.payload})
			}
			result := env.CallTool(t, "caido_update_scope", tt.args)
			if !result.IsError {
				t.Fatalf("want error containing %q, got success", tt.want)
			}
			if got := contentText(result); !strings.Contains(got, tt.want) {
				t.Fatalf("error = %q, want it to contain %q", got, tt.want)
			}
			if tt.payload == nil && env.Mock.LastVariables("UpdateScope") != nil {
				t.Fatal("UpdateScope must not be called when validation fails")
			}
		})
	}
}
