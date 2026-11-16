package tools

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestIsInScope covers caido_is_in_scope (op ListScopes, field "scopes").
func TestIsInScope(t *testing.T) {
	prodScope := map[string]any{
		"id":        "s1",
		"name":      "Prod",
		"allowlist": []string{"*.example.com"},
		"denylist":  []string{"admin.example.com"},
		"indexed":   true,
	}
	openScope := map[string]any{
		"id":        "s2",
		"name":      "Open",
		"allowlist": []string{},
		"denylist":  []string{},
		"indexed":   false,
	}

	tests := []struct {
		name            string
		target          string
		scopes          []map[string]any
		doMock          bool
		wantErr         bool
		wantInScope     bool
		wantScopeID     string
		wantMatchedRule string
	}{
		{
			name:            "subdomain allowed and not denied is in scope",
			target:          "api.example.com",
			scopes:          []map[string]any{prodScope},
			doMock:          true,
			wantInScope:     true,
			wantScopeID:     "s1",
			wantMatchedRule: "*.example.com",
		},
		{
			name:            "denied host is not in scope",
			target:          "admin.example.com",
			scopes:          []map[string]any{prodScope},
			doMock:          true,
			wantInScope:     false,
			wantScopeID:     "s1",
			wantMatchedRule: "admin.example.com",
		},
		{
			name:        "unrelated host is not in scope",
			target:      "other.com",
			scopes:      []map[string]any{prodScope},
			doMock:      true,
			wantInScope: false,
			wantScopeID: "",
		},
		{
			name:        "empty allowlist matches all hosts",
			target:      "anything.test",
			scopes:      []map[string]any{openScope},
			doMock:      true,
			wantInScope: true,
			wantScopeID: "s2",
		},
		{
			name:            "full url input extracts host",
			target:          "https://api.example.com:8443/some/path",
			scopes:          []map[string]any{prodScope},
			doMock:          true,
			wantInScope:     true,
			wantScopeID:     "s1",
			wantMatchedRule: "*.example.com",
		},
		{
			name:    "empty target is rejected",
			target:  "",
			doMock:  false,
			wantErr: true,
		},
		{
			name:    "graphql error when op unmocked",
			target:  "example.com",
			doMock:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				RegisterIsInScopeTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("ListScopes", map[string]any{"scopes": tt.scopes})
			}

			result := env.CallTool(t, "caido_is_in_scope", map[string]any{"target": tt.target})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[IsInScopeOutput](t, result)
			if out.InScope != tt.wantInScope {
				t.Errorf("InScope = %v, want %v (reason: %s)", out.InScope, tt.wantInScope, out.Reason)
			}
			gotScopeID := ""
			if out.MatchedScope != nil {
				gotScopeID = out.MatchedScope.ID
			}
			if gotScopeID != tt.wantScopeID {
				t.Errorf("matched scope ID = %q, want %q", gotScopeID, tt.wantScopeID)
			}
			if tt.wantMatchedRule != "" && out.MatchedRule != tt.wantMatchedRule {
				t.Errorf("MatchedRule = %q, want %q", out.MatchedRule, tt.wantMatchedRule)
			}
			if out.Reason == "" {
				t.Error("expected a non-empty reason")
			}
			if out.Host == "" {
				t.Error("expected a non-empty resolved host")
			}
		})
	}
}
