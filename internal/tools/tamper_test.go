package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listTamperCollectionsData builds the GraphQL "data" object for the
// ListTamperRuleCollections query. Top-level key is the GraphQL field
// name "tamperRuleCollections". One rule carries an HTTPQL condition
// (enabled), the other has no condition and no enable (disabled).
func listTamperCollectionsData() map[string]any {
	return map[string]any{
		"tamperRuleCollections": []map[string]any{
			{
				"id":   "col-1",
				"name": "Default Collection",
				"rules": []map[string]any{
					{
						"id":   "rule-1",
						"name": "Rewrite Host",
						"condition": map[string]any{
							"__typename": "HTTPQL",
							"code":       "req.host.eq:\"a.com\"",
						},
						"sources": []string{"INTERCEPT", "REPLAY"},
						"enable":  map[string]any{"rank": "a0"},
					},
					{
						"id":        "rule-2",
						"name":      "Disabled Rule",
						"sources":   []string{},
						"enable":    nil,
						"condition": nil,
					},
				},
			},
		},
	}
}

func TestListTamperRulesTool(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testutil.MockHandler)
		wantErr     bool
		wantCols    int
		wantRules   int
		wantRule1On bool
	}{
		{
			name: "returns collections with nested rules",
			setup: func(m *testutil.MockHandler) {
				m.On("ListTamperRuleCollections", listTamperCollectionsData())
			},
			wantCols:    1,
			wantRules:   2,
			wantRule1On: true,
		},
		{
			name:    "graphql error when no mock registered",
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListTamperRulesTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_list_tamper_rules", map[string]any{})

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListTamperRulesOutput](t, result)
			if len(out.Collections) != tt.wantCols {
				t.Fatalf("want %d collections, got %d", tt.wantCols, len(out.Collections))
			}
			col := out.Collections[0]
			if col.ID != "col-1" || col.Name != "Default Collection" {
				t.Fatalf("unexpected collection: %+v", col)
			}
			if len(col.Rules) != tt.wantRules {
				t.Fatalf("want %d rules, got %d", tt.wantRules, len(col.Rules))
			}
			if col.Rules[0].Enabled != tt.wantRule1On {
				t.Fatalf("rule[0] enabled = %v, want %v", col.Rules[0].Enabled, tt.wantRule1On)
			}
			if col.Rules[0].Condition == nil || *col.Rules[0].Condition != "req.host.eq:\"a.com\"" {
				t.Fatalf("rule[0] condition = %v, want HTTPQL code", col.Rules[0].Condition)
			}
			if col.Rules[1].Enabled {
				t.Fatal("rule[1] should be disabled (no enable)")
			}
			if col.Rules[1].Condition != nil {
				t.Fatalf("rule[1] condition should be nil, got %v", *col.Rules[1].Condition)
			}
		})
	}
}

func TestCreateTamperRuleTool(t *testing.T) {
	// createTamperRuleData: top-level key is the mutation field
	// "createTamperRule" with the rule/error payload shape declared
	// in create_tamper_rule.go's createTamperRuleResp.
	okData := map[string]any{
		"createTamperRule": map[string]any{
			"error": nil,
			"rule":  map[string]any{"id": "rule-99", "name": "New Rule"},
		},
	}
	errData := map[string]any{
		"createTamperRule": map[string]any{
			"error": map[string]any{"__typename": "InvalidRegexUserError"},
			"rule":  nil,
		},
	}

	tests := []struct {
		name    string
		args    map[string]any
		setup   func(*testutil.MockHandler)
		wantErr bool
		wantID  string
	}{
		{
			name: "creates rule and returns id/name",
			args: map[string]any{
				"collection_id": "col-1",
				"name":          "New Rule",
				"section":       "requestHeader",
				"match":         "X-Foo",
				"replace":       "X-Bar",
			},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateTamperRule", okData)
			},
			wantID: "rule-99",
		},
		{
			name: "rejects name over 200 chars",
			args: map[string]any{
				"collection_id": "col-1",
				"name":          strings.Repeat("a", 201),
				"section":       "requestHeader",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "rejects unknown section",
			args: map[string]any{
				"collection_id": "col-1",
				"name":          "Bad Section",
				"section":       "notASection",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "graphql payload error surfaces",
			args: map[string]any{
				"collection_id": "col-1",
				"name":          "Bad Regex",
				"section":       "requestHeader",
				"match":         "[",
			},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateTamperRule", errData)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateTamperRuleTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_create_tamper_rule", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateTamperRuleOutput](t, result)
			if out.ID != tt.wantID {
				t.Fatalf("want id %q, got %q", tt.wantID, out.ID)
			}
			if out.Name != "New Rule" {
				t.Fatalf("want name %q, got %q", "New Rule", out.Name)
			}
		})
	}
}

func TestUpdateTamperRuleTool(t *testing.T) {
	// updateTamperRuleData: top-level key is the mutation field
	// "updateTamperRule" matching update_tamper_rule.go's
	// updateTamperRuleResp.
	okData := map[string]any{
		"updateTamperRule": map[string]any{
			"error": nil,
			"rule":  map[string]any{"id": "rule-1", "name": "Renamed Rule"},
		},
	}
	errData := map[string]any{
		"updateTamperRule": map[string]any{
			"error": map[string]any{"__typename": "UnknownIdUserError"},
			"rule":  nil,
		},
	}

	tests := []struct {
		name    string
		args    map[string]any
		setup   func(*testutil.MockHandler)
		wantErr bool
	}{
		{
			name: "updates rule and returns id/name",
			args: map[string]any{
				"id":      "rule-1",
				"name":    "Renamed Rule",
				"section": "responseBody",
				"match":   "secret",
				"replace": "REDACTED",
			},
			setup: func(m *testutil.MockHandler) {
				m.On("UpdateTamperRule", okData)
			},
		},
		{
			name: "rejects name over 200 chars",
			args: map[string]any{
				"id":      "rule-1",
				"name":    strings.Repeat("b", 201),
				"section": "responseBody",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "graphql payload error surfaces",
			args: map[string]any{
				"id":      "missing-id",
				"name":    "Whatever",
				"section": "responseBody",
			},
			setup: func(m *testutil.MockHandler) {
				m.On("UpdateTamperRule", errData)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterUpdateTamperRuleTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_update_tamper_rule", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.UpdateTamperRuleOutput](t, result)
			if out.ID != "rule-1" || out.Name != "Renamed Rule" {
				t.Fatalf("unexpected output: %+v", out)
			}
		})
	}
}

func TestToggleTamperRuleTool(t *testing.T) {
	// toggleData: top-level key "toggleTamperRule". The SDK Error field
	// is json:"-" (custom oneof unmarshal), so the success mock only
	// provides "rule"; enable={rank} => enabled, enable=null => disabled.
	enabledData := map[string]any{
		"toggleTamperRule": map[string]any{
			"rule": map[string]any{
				"id":     "rule-1",
				"name":   "Rule One",
				"enable": map[string]any{"rank": "a0"},
			},
		},
	}
	disabledData := map[string]any{
		"toggleTamperRule": map[string]any{
			"rule": map[string]any{
				"id":     "rule-1",
				"name":   "Rule One",
				"enable": nil,
			},
		},
	}

	tests := []struct {
		name        string
		args        map[string]any
		setup       func(*testutil.MockHandler)
		wantErr     bool
		wantEnabled bool
	}{
		{
			name: "enables rule",
			args: map[string]any{"id": "rule-1", "enabled": true},
			setup: func(m *testutil.MockHandler) {
				m.On("ToggleTamperRule", enabledData)
			},
			wantEnabled: true,
		},
		{
			name: "disables rule",
			args: map[string]any{"id": "rule-1", "enabled": false},
			setup: func(m *testutil.MockHandler) {
				m.On("ToggleTamperRule", disabledData)
			},
			wantEnabled: false,
		},
		{
			name:    "graphql error when no mock registered",
			args:    map[string]any{"id": "rule-1", "enabled": true},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterToggleTamperRuleTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_toggle_tamper_rule", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ToggleTamperRuleOutput](t, result)
			if out.ID != "rule-1" || out.Name != "Rule One" {
				t.Fatalf("unexpected output: %+v", out)
			}
			if out.Enabled != tt.wantEnabled {
				t.Fatalf("enabled = %v, want %v", out.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestDeleteTamperRuleTool(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		setup     func(*testutil.MockHandler)
		wantErr   bool
		wantDelID string
	}{
		{
			name: "deletes rule and returns deleted id",
			args: map[string]any{"id": "rule-1"},
			setup: func(m *testutil.MockHandler) {
				// Top-level key "deleteTamperRule" with deletedId
				// (DeleteTamperRuleResponse / DeleteTamperRulePayload).
				m.On("DeleteTamperRule", map[string]any{
					"deleteTamperRule": map[string]any{"deletedId": "rule-1"},
				})
			},
			wantDelID: "rule-1",
		},
		{
			name: "errors when deletedId is null",
			args: map[string]any{"id": "rule-1"},
			setup: func(m *testutil.MockHandler) {
				m.On("DeleteTamperRule", map[string]any{
					"deleteTamperRule": map[string]any{"deletedId": nil},
				})
			},
			wantErr: true,
		},
		{
			name:    "graphql error when no mock registered",
			args:    map[string]any{"id": "rule-1"},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteTamperRuleTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_delete_tamper_rule", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteTamperRuleOutput](t, result)
			if out.DeletedID != tt.wantDelID {
				t.Fatalf("want deleted_id %q, got %q", tt.wantDelID, out.DeletedID)
			}
		})
	}
}
