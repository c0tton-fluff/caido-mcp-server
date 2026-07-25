package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- environments: list ----

func TestListEnvironments(t *testing.T) {
	t.Run("returns environments with context", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterListEnvironmentsTool(s, c)
		})
		// list_environments calls two GraphQL ops: ListEnvironments + GetEnvironmentContext.
		env.Mock.On("ListEnvironments", map[string]any{
			"environments": []map[string]any{
				{
					"id":      "2",
					"name":    "staging",
					"version": 1,
					"variables": []map[string]any{
						{"name": "token", "value": "abc", "kind": "SECRET"},
						{"name": "host", "value": "example.com", "kind": "PLAIN"},
					},
				},
			},
		})
		env.Mock.On("GetEnvironmentContext", map[string]any{
			"environmentContext": map[string]any{
				"global":   map[string]any{"id": "1", "name": "Global"},
				"selected": map[string]any{"id": "2", "name": "staging"},
			},
		})

		result := env.CallTool(t, "caido_list_environments", map[string]any{})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}

		out := testutil.UnmarshalToolResult[tools.ListEnvironmentsOutput](t, result)
		if len(out.Environments) != 1 {
			t.Fatalf("got %d environments, want 1", len(out.Environments))
		}
		if out.Environments[0].ID != "2" || out.Environments[0].Name != "staging" {
			t.Errorf("unexpected environment summary: %+v", out.Environments[0])
		}
		if len(out.Environments[0].Variables) != 2 {
			t.Fatalf("got %d variables, want 2", len(out.Environments[0].Variables))
		}
		if out.Environments[0].Variables[0].Kind != "SECRET" {
			t.Errorf("got kind %q, want SECRET", out.Environments[0].Variables[0].Kind)
		}
		if out.GlobalID != "1" {
			t.Errorf("got GlobalID %q, want 1", out.GlobalID)
		}
		if out.SelectedID != "2" {
			t.Errorf("got SelectedID %q, want 2", out.SelectedID)
		}
	})

	t.Run("graphql error when list op unmocked", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterListEnvironmentsTool(s, c)
		})
		// No mock registered -> mock handler returns a GraphQL error for ListEnvironments.
		result := env.CallTool(t, "caido_list_environments", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error, got success")
		}
	})
}

// ---- environments: create ----

func TestCreateEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		mock     bool
		wantErr  bool
		wantID   string
		wantName string
	}{
		{
			name:  "success",
			input: map[string]any{"name": "prod"},
			mock:  true,
			mockData: map[string]any{
				"createEnvironment": map[string]any{
					"environment": map[string]any{"id": "3", "name": "prod"},
					"error":       nil,
				},
			},
			wantID:   "3",
			wantName: "prod",
		},
		{
			name:    "rejects missing name",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "rejects name over 200 chars",
			input:   map[string]any{"name": strings.Repeat("a", 201)},
			wantErr: true,
		},
		{
			name:  "graphql user error",
			input: map[string]any{"name": "dup"},
			mock:  true,
			mockData: map[string]any{
				"createEnvironment": map[string]any{
					"environment": nil,
					"error":       map[string]any{"__typename": "NameTakenUserError"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateEnvironmentTool(s, c)
			})
			if tt.mock {
				env.Mock.On("CreateEnvironment", tt.mockData)
			}

			result := env.CallTool(t, "caido_create_environment", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateEnvironmentOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("got ID %q, want %q", out.ID, tt.wantID)
			}
			if out.Name != tt.wantName {
				t.Errorf("got Name %q, want %q", out.Name, tt.wantName)
			}
		})
	}
}

// ---- environments: select ----

func TestSelectEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		wantErr  bool
		wantID   string
		wantName string
	}{
		{
			name:  "selects environment by id",
			input: map[string]any{"id": "2"},
			mockData: map[string]any{
				"selectEnvironment": map[string]any{
					"environment": map[string]any{"id": "2", "name": "staging"},
					"error":       nil,
				},
			},
			wantID:   "2",
			wantName: "staging",
		},
		{
			name:  "deselect with empty id returns env payload",
			input: map[string]any{"id": ""},
			mockData: map[string]any{
				"selectEnvironment": map[string]any{
					"environment": map[string]any{"id": "1", "name": "Global"},
					"error":       nil,
				},
			},
			wantID:   "1",
			wantName: "Global",
		},
		{
			name:  "graphql user error",
			input: map[string]any{"id": "999"},
			mockData: map[string]any{
				"selectEnvironment": map[string]any{
					"environment": nil,
					"error": map[string]any{
						"__typename": "OtherUserError",
						"code":       "not_found",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterSelectEnvironmentTool(s, c)
			})
			env.Mock.On("SelectEnvironment", tt.mockData)

			result := env.CallTool(t, "caido_select_environment", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.SelectEnvironmentOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("got ID %q, want %q", out.ID, tt.wantID)
			}
			if out.Name != tt.wantName {
				t.Errorf("got Name %q, want %q", out.Name, tt.wantName)
			}
		})
	}
}

// ---- environments: delete ----

func TestDeleteEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		mockData    map[string]any
		mock        bool
		wantErr     bool
		wantSuccess bool
	}{
		{
			name:  "success",
			input: map[string]any{"id": "2"},
			mock:  true,
			mockData: map[string]any{
				"deleteEnvironment": map[string]any{
					"deletedId": "2",
					"error":     nil,
				},
			},
			wantSuccess: true,
		},
		{
			name:    "rejects missing id",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name:  "graphql user error",
			input: map[string]any{"id": "1"},
			mock:  true,
			mockData: map[string]any{
				"deleteEnvironment": map[string]any{
					"deletedId": nil,
					"error": map[string]any{
						"__typename": "OtherUserError",
						"code":       "cannot_delete_global",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteEnvironmentTool(s, c)
			})
			if tt.mock {
				env.Mock.On("DeleteEnvironment", tt.mockData)
			}

			result := env.CallTool(t, "caido_delete_environment", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteEnvironmentOutput](t, result)
			if out.Success != tt.wantSuccess {
				t.Errorf("got Success=%v, want %v", out.Success, tt.wantSuccess)
			}
		})
	}
}

// ---- filters: list ----

func TestListFilters(t *testing.T) {
	t.Run("returns httpql filter presets", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterListFiltersTool(s, c)
		})
		// clause is a union; __typename selects the concrete type (HTTPQL/StreamQL).
		env.Mock.On("ListFilterPresets", map[string]any{
			"filterPresets": []map[string]any{
				{
					"id":    "f1",
					"name":  "errors",
					"alias": "err",
					"clause": map[string]any{
						"__typename": "HTTPQL",
						"code":       `resp.code.gte:"500"`,
					},
				},
			},
		})

		result := env.CallTool(t, "caido_list_filters", map[string]any{})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}

		out := testutil.UnmarshalToolResult[tools.ListFiltersOutput](t, result)
		if len(out.Filters) != 1 {
			t.Fatalf("got %d filters, want 1", len(out.Filters))
		}
		f := out.Filters[0]
		if f.ID != "f1" || f.Name != "errors" || f.Alias != "err" {
			t.Errorf("unexpected filter summary: %+v", f)
		}
		if f.Clause != `resp.code.gte:"500"` {
			t.Errorf("got clause %q, want resp.code.gte:\"500\"", f.Clause)
		}
	})

	t.Run("graphql error when op unmocked", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterListFiltersTool(s, c)
		})
		result := env.CallTool(t, "caido_list_filters", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error, got success")
		}
	})
}

// ---- filters: create ----

func TestCreateFilter(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		mockData  map[string]any
		wantErr   bool
		wantID    string
		wantName  string
		wantAlias string
	}{
		{
			name: "success",
			input: map[string]any{
				"name":  "errors",
				"query": `resp.code.gte:"500"`,
				"alias": "err",
			},
			mockData: map[string]any{
				"createFilterPreset": map[string]any{
					"filter": map[string]any{
						"id": "f9", "name": "errors", "alias": "err",
					},
					"error": nil,
				},
			},
			wantID:    "f9",
			wantName:  "errors",
			wantAlias: "err",
		},
		{
			name: "graphql user error",
			input: map[string]any{
				"name": "dup", "query": `resp.code.gte:"500"`,
			},
			mockData: map[string]any{
				"createFilterPreset": map[string]any{
					"filter": nil,
					"error":  map[string]any{"__typename": "NameTakenUserError"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateFilterTool(s, c)
			})
			env.Mock.On("CreateFilterPreset", tt.mockData)

			result := env.CallTool(t, "caido_create_filter", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateFilterOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("got ID %q, want %q", out.ID, tt.wantID)
			}
			if out.Name != tt.wantName {
				t.Errorf("got Name %q, want %q", out.Name, tt.wantName)
			}
			if out.Alias != tt.wantAlias {
				t.Errorf("got Alias %q, want %q", out.Alias, tt.wantAlias)
			}
		})
	}
}

// ---- filters: delete ----

func TestDeleteFilter(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		mockData    map[string]any
		wantErr     bool
		wantSuccess bool
	}{
		{
			name:  "success",
			input: map[string]any{"id": "f1"},
			mockData: map[string]any{
				"deleteFilterPreset": map[string]any{"deletedId": "f1"},
			},
			wantSuccess: true,
		},
		{
			name:  "error when deletedId null",
			input: map[string]any{"id": "missing"},
			mockData: map[string]any{
				"deleteFilterPreset": map[string]any{"deletedId": nil},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteFilterTool(s, c)
			})
			env.Mock.On("DeleteFilterPreset", tt.mockData)

			result := env.CallTool(t, "caido_delete_filter", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteFilterOutput](t, result)
			if out.Success != tt.wantSuccess {
				t.Errorf("got Success=%v, want %v", out.Success, tt.wantSuccess)
			}
		})
	}
}
