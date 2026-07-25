package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestListProjects covers caido_list_projects. The handler issues two GraphQL
// operations: ListProjects (field "projects") and GetCurrentProject
// (field "currentProject"), so both must be mocked for the success path.
func TestListProjects(t *testing.T) {
	tests := []struct {
		name        string
		projects    map[string]any
		current     map[string]any
		mockCurrent bool
		wantCount   int
		wantCurrent string // ID expected to have isCurrent=true ("" = none)
		wantErr     bool
	}{
		{
			name: "lists projects and marks current",
			projects: map[string]any{
				"projects": []map[string]any{
					{"id": "p1", "name": "Alpha", "status": "READY", "version": "1.0"},
					{"id": "p2", "name": "Beta", "status": "READY", "version": "1.0"},
				},
			},
			current: map[string]any{
				"currentProject": map[string]any{
					"project":  map[string]any{"id": "p2", "name": "Beta"},
					"readOnly": false,
				},
			},
			mockCurrent: true,
			wantCount:   2,
			wantCurrent: "p2",
		},
		{
			name:        "empty project list",
			projects:    map[string]any{"projects": []map[string]any{}},
			current:     map[string]any{"currentProject": nil},
			mockCurrent: true,
			wantCount:   0,
			wantCurrent: "",
		},
		{
			name:        "graphql error when current project op fails",
			projects:    map[string]any{"projects": []map[string]any{}},
			mockCurrent: false, // unmocked GetCurrentProject -> GraphQL error
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListProjectsTool(s, c)
			})

			env.Mock.On("ListProjects", tt.projects)
			if tt.mockCurrent {
				env.Mock.On("GetCurrentProject", tt.current)
			}

			result := env.CallTool(t, "caido_list_projects", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListProjectsOutput](t, result)
			if len(out.Projects) != tt.wantCount {
				t.Fatalf("got %d projects, want %d", len(out.Projects), tt.wantCount)
			}
			for _, p := range out.Projects {
				if p.IsCurrent && p.ID != tt.wantCurrent {
					t.Errorf("project %s marked current, want %s", p.ID, tt.wantCurrent)
				}
				if !p.IsCurrent && p.ID == tt.wantCurrent && tt.wantCurrent != "" {
					t.Errorf("project %s should be current", p.ID)
				}
			}
		})
	}
}

// TestCreateProject covers caido_create_project (op CreateProject, field
// "createProject" with nested "project").
func TestCreateProject(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name:  "creates project",
			input: map[string]any{"name": "NewProj"},
			mockData: map[string]any{
				"createProject": map[string]any{
					"project": map[string]any{"id": "p9", "name": "NewProj"},
				},
			},
			doMock:   true,
			wantID:   "p9",
			wantName: "NewProj",
		},
		{
			name:    "rejects empty name",
			input:   map[string]any{"name": ""},
			wantErr: true,
		},
		{
			name:    "graphql error when create op unmocked",
			input:   map[string]any{"name": "Boom"},
			doMock:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateProjectTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("CreateProject", tt.mockData)
			}

			result := env.CallTool(t, "caido_create_project", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateProjectOutput](t, result)
			if out.ID != tt.wantID || out.Name != tt.wantName {
				t.Errorf("got %q/%q, want %q/%q", out.ID, out.Name, tt.wantID, tt.wantName)
			}
		})
	}
}

// TestSelectProject covers caido_select_project (op SelectProject, field
// "selectProject" -> "currentProject" -> "project").
func TestSelectProject(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name:  "selects project",
			input: map[string]any{"id": "p2"},
			mockData: map[string]any{
				"selectProject": map[string]any{
					"currentProject": map[string]any{
						"project": map[string]any{"id": "p2", "name": "Beta"},
					},
				},
			},
			doMock:   true,
			wantID:   "p2",
			wantName: "Beta",
		},
		{
			name:    "rejects empty id",
			input:   map[string]any{"id": ""},
			wantErr: true,
		},
		{
			name:  "errors when currentProject is null",
			input: map[string]any{"id": "missing"},
			mockData: map[string]any{
				"selectProject": map[string]any{"currentProject": nil},
			},
			doMock:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterSelectProjectTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("SelectProject", tt.mockData)
			}

			result := env.CallTool(t, "caido_select_project", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.SelectProjectOutput](t, result)
			if out.ID != tt.wantID || out.Name != tt.wantName {
				t.Errorf("got %q/%q, want %q/%q", out.ID, out.Name, tt.wantID, tt.wantName)
			}
		})
	}
}

// TestRenameProject covers caido_rename_project (op RenameProject, field
// "renameProject" -> "project").
func TestRenameProject(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantName string
		wantErr  bool
	}{
		{
			name:  "renames project",
			input: map[string]any{"id": "p1", "name": "Renamed"},
			mockData: map[string]any{
				"renameProject": map[string]any{
					"project": map[string]any{"id": "p1", "name": "Renamed"},
				},
			},
			doMock:   true,
			wantName: "Renamed",
		},
		{
			name:    "rejects empty id",
			input:   map[string]any{"id": "", "name": "X"},
			wantErr: true,
		},
		{
			name:    "rejects empty name",
			input:   map[string]any{"id": "p1", "name": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterRenameProjectTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("RenameProject", tt.mockData)
			}

			result := env.CallTool(t, "caido_rename_project", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.RenameProjectOutput](t, result)
			if out.ID != "p1" || out.Name != tt.wantName {
				t.Errorf("got %q/%q, want p1/%q", out.ID, out.Name, tt.wantName)
			}
		})
	}
}

// TestDeleteProject covers caido_delete_project (op DeleteProject, field
// "deleteProject" -> "deletedId").
func TestDeleteProject(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantErr  bool
	}{
		{
			name:  "deletes project",
			input: map[string]any{"id": "p1"},
			mockData: map[string]any{
				"deleteProject": map[string]any{"deletedId": "p1"},
			},
			doMock: true,
		},
		{
			name:    "rejects empty id",
			input:   map[string]any{"id": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteProjectTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("DeleteProject", tt.mockData)
			}

			result := env.CallTool(t, "caido_delete_project", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteProjectOutput](t, result)
			if !out.Success {
				t.Error("expected Success=true")
			}
		})
	}
}

// TestListScopes covers caido_list_scopes (op ListScopes, field "scopes").
func TestListScopes(t *testing.T) {
	tests := []struct {
		name      string
		mockData  map[string]any
		doMock    bool
		wantCount int
		wantErr   bool
	}{
		{
			name: "lists scopes",
			mockData: map[string]any{
				"scopes": []map[string]any{
					{
						"id":        "s1",
						"name":      "Prod",
						"allowlist": []string{"example.com"},
						"denylist":  []string{"admin.example.com"},
						"indexed":   true,
					},
				},
			},
			doMock:    true,
			wantCount: 1,
		},
		{
			name:      "empty scope list",
			mockData:  map[string]any{"scopes": []map[string]any{}},
			doMock:    true,
			wantCount: 0,
		},
		{
			name:    "graphql error when op unmocked",
			doMock:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListScopesTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("ListScopes", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_scopes", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListScopesOutput](t, result)
			if len(out.Scopes) != tt.wantCount {
				t.Fatalf("got %d scopes, want %d", len(out.Scopes), tt.wantCount)
			}
			if tt.wantCount > 0 {
				s := out.Scopes[0]
				if s.ID == "" || s.Name == "" {
					t.Error("scope ID/Name empty")
				}
				if len(s.Allowlist) == 0 {
					t.Error("scope allowlist empty")
				}
			}
		})
	}
}

// TestCreateScope covers caido_create_scope (op CreateScope, field
// "createScope" -> "scope"). Note: the payload error field uses json:"-" with
// a custom unmarshaller keyed on "error", so omitting "error" means no error.
func TestCreateScope(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name: "creates scope",
			input: map[string]any{
				"name":      "Prod",
				"allowlist": []string{"example.com"},
			},
			mockData: map[string]any{
				"createScope": map[string]any{
					"scope": map[string]any{"id": "s1", "name": "Prod"},
				},
			},
			doMock:   true,
			wantID:   "s1",
			wantName: "Prod",
		},
		{
			name: "rejects empty name",
			input: map[string]any{
				"name":      "",
				"allowlist": []string{"example.com"},
			},
			wantErr: true,
		},
		{
			name: "rejects name over 200 chars",
			input: map[string]any{
				"name":      strings.Repeat("a", 201),
				"allowlist": []string{"example.com"},
			},
			wantErr: true,
		},
		{
			name: "rejects empty allowlist",
			input: map[string]any{
				"name":      "Prod",
				"allowlist": []string{},
			},
			wantErr: true,
		},
		{
			name: "errors when payload returns error type",
			input: map[string]any{
				"name":      "Prod",
				"allowlist": []string{"example.com"},
			},
			mockData: map[string]any{
				"createScope": map[string]any{
					"scope": nil,
					"error": map[string]any{"__typename": "InvalidGlobTermsUserError"},
				},
			},
			doMock:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateScopeTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("CreateScope", tt.mockData)
			}

			result := env.CallTool(t, "caido_create_scope", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateScopeOutput](t, result)
			if out.ID != tt.wantID || out.Name != tt.wantName {
				t.Errorf("got %q/%q, want %q/%q", out.ID, out.Name, tt.wantID, tt.wantName)
			}
		})
	}
}

// TestRenameScope covers caido_rename_scope (op RenameScope, field
// "renameScope" -> "scope").
func TestRenameScope(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantName string
		wantErr  bool
	}{
		{
			name:  "renames scope",
			input: map[string]any{"id": "s1", "name": "Staging"},
			mockData: map[string]any{
				"renameScope": map[string]any{
					"scope": map[string]any{"id": "s1", "name": "Staging"},
				},
			},
			doMock:   true,
			wantName: "Staging",
		},
		{
			name:    "rejects empty id",
			input:   map[string]any{"id": "", "name": "X"},
			wantErr: true,
		},
		{
			name:    "rejects empty name",
			input:   map[string]any{"id": "s1", "name": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterRenameScopeTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("RenameScope", tt.mockData)
			}

			result := env.CallTool(t, "caido_rename_scope", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.RenameScopeOutput](t, result)
			if out.ID != "s1" || out.Name != tt.wantName {
				t.Errorf("got %q/%q, want s1/%q", out.ID, out.Name, tt.wantName)
			}
		})
	}
}

// TestDeleteScope covers caido_delete_scope (op DeleteScope, field
// "deleteScope" -> "deletedId").
func TestDeleteScope(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		mockData map[string]any
		doMock   bool
		wantID   string
		wantErr  bool
	}{
		{
			name:  "deletes scope",
			input: map[string]any{"id": "s1"},
			mockData: map[string]any{
				"deleteScope": map[string]any{"deletedId": "s1"},
			},
			doMock: true,
			wantID: "s1",
		},
		{
			name:    "rejects empty id",
			input:   map[string]any{"id": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteScopeTool(s, c)
			})
			if tt.doMock {
				env.Mock.On("DeleteScope", tt.mockData)
			}

			result := env.CallTool(t, "caido_delete_scope", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteScopeOutput](t, result)
			if out.ID != tt.wantID || !out.Deleted {
				t.Errorf("got ID=%q Deleted=%v, want %q/true", out.ID, out.Deleted, tt.wantID)
			}
		})
	}
}
