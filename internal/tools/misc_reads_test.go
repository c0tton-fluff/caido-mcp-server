package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetInstanceTool covers caido_get_instance.
// SDK op: GetRuntime -> data.runtime { version, platform, logs }.
func TestGetInstanceTool(t *testing.T) {
	tests := []struct {
		name         string
		mockRuntime  map[string]any
		mockErr      bool
		wantErr      bool
		wantVersion  string
		wantPlatform string
	}{
		{
			name: "success",
			mockRuntime: map[string]any{
				"runtime": map[string]any{
					"version":  "0.47.0",
					"platform": "MACOS",
					"logs":     "/tmp/caido.log",
				},
			},
			wantVersion:  "0.47.0",
			wantPlatform: "MACOS",
		},
		{
			name:    "graphql error",
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetInstanceTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("GetRuntime", tt.mockRuntime)
			}

			result := env.CallTool(t, "caido_get_instance", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetInstanceOutput](t, result)
			if out.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", out.Version, tt.wantVersion)
			}
			if out.Platform != tt.wantPlatform {
				t.Errorf("platform = %q, want %q", out.Platform, tt.wantPlatform)
			}
		})
	}
}

// TestListHostedFilesTool covers caido_list_hosted_files.
// SDK op: ListHostedFiles -> data.hostedFiles [{ id, name, path, size, status, ... }].
func TestListHostedFilesTool(t *testing.T) {
	tests := []struct {
		name      string
		mockData  map[string]any
		mockErr   bool
		wantErr   bool
		wantCount int
		wantFirst tools.HostedFileSummary
	}{
		{
			name: "success",
			mockData: map[string]any{
				"hostedFiles": []map[string]any{
					{
						"id":        "hf-1",
						"name":      "payload.bin",
						"path":      "/files/payload.bin",
						"size":      1024,
						"status":    "ACTIVE",
						"createdAt": "2024-05-05T00:00:00Z",
						"updatedAt": "2024-05-05T00:00:00Z",
					},
				},
			},
			wantCount: 1,
			wantFirst: tools.HostedFileSummary{
				ID: "hf-1", Name: "payload.bin",
				Path: "/files/payload.bin", Size: 1024, Status: "ACTIVE",
			},
		},
		{
			name:      "empty list",
			mockData:  map[string]any{"hostedFiles": []map[string]any{}},
			wantCount: 0,
		},
		{
			name:    "graphql error",
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListHostedFilesTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("ListHostedFiles", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_hosted_files", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListHostedFilesOutput](t, result)
			if len(out.Files) != tt.wantCount {
				t.Fatalf("files count = %d, want %d", len(out.Files), tt.wantCount)
			}
			if tt.wantCount > 0 && out.Files[0] != tt.wantFirst {
				t.Errorf("first file = %+v, want %+v", out.Files[0], tt.wantFirst)
			}
		})
	}
}

// TestListPluginsTool covers caido_list_plugins.
// SDK op: ListPluginPackages -> data.pluginPackages [{ id, name, description, version, ... }].
func TestListPluginsTool(t *testing.T) {
	tests := []struct {
		name      string
		mockData  map[string]any
		mockErr   bool
		wantErr   bool
		wantCount int
		wantID    string
		wantVer   string
	}{
		{
			name: "success",
			mockData: map[string]any{
				"pluginPackages": []map[string]any{
					{
						"id":          "pkg-1",
						"name":        "Authorize",
						"description": "Authz testing",
						"version":     "1.2.3",
						"manifestId":  "io.caido.authorize",
						"installedAt": "2024-05-05T00:00:00Z",
						"author":      nil,
						"plugins":     []map[string]any{},
					},
				},
			},
			wantCount: 1,
			wantID:    "pkg-1",
			wantVer:   "1.2.3",
		},
		{
			name:      "empty list",
			mockData:  map[string]any{"pluginPackages": []map[string]any{}},
			wantCount: 0,
		},
		{
			name:    "graphql error",
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListPluginsTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("ListPluginPackages", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_plugins", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListPluginsOutput](t, result)
			if len(out.Packages) != tt.wantCount {
				t.Fatalf("packages count = %d, want %d", len(out.Packages), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if out.Packages[0].ID != tt.wantID {
					t.Errorf("id = %q, want %q", out.Packages[0].ID, tt.wantID)
				}
				if out.Packages[0].Version != tt.wantVer {
					t.Errorf("version = %q, want %q", out.Packages[0].Version, tt.wantVer)
				}
			}
		})
	}
}

// TestGetReplayEntryTool covers caido_get_replay_entry.
// SDK op: GetReplayEntry -> data.replayEntry { ... }. Reuses testutil fixture.
func TestGetReplayEntryTool(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		mockData   map[string]any
		mockNil    bool
		wantErr    bool
		wantID     string
		wantStatus int
	}{
		{
			name:       "success",
			input:      map[string]any{"id": "entry-1"},
			mockData:   testutil.GetReplayEntryResponse("entry-1", "req-1", 200, "hello"),
			wantID:     "entry-1",
			wantStatus: 200,
		},
		{
			name:    "missing required id",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "entry not found",
			input:   map[string]any{"id": "entry-x"},
			mockNil: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetReplayEntryTool(s, c)
			})
			if tt.mockData != nil {
				env.Mock.On("GetReplayEntry", tt.mockData)
			}
			if tt.mockNil {
				env.Mock.On("GetReplayEntry", map[string]any{"replayEntry": nil})
			}

			result := env.CallTool(t, "caido_get_replay_entry", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetReplayEntryOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("id = %q, want %q", out.ID, tt.wantID)
			}
			if out.StatusCode != tt.wantStatus {
				t.Errorf("statusCode = %d, want %d", out.StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestGetSitemapTool covers caido_get_sitemap root listing, including the
// per-host captured request count that the tool adds on top of the tree.
func TestGetSitemapTool(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		mockData   map[string]any
		hostCounts []int
		mockErr    bool
		wantErr    bool
		wantCount  int
		wantLabel  string
		wantTotal  int
	}{
		{
			name:  "root with host count",
			input: map[string]any{},
			mockData: map[string]any{
				"sitemapRootEntries": map[string]any{
					"count": map[string]any{"value": 1},
					"edges": []map[string]any{
						{
							"node": map[string]any{
								"id":             "root-1",
								"label":          "example.com",
								"kind":           "DOMAIN",
								"hasDescendants": true,
							},
						},
					},
				},
			},
			hostCounts: []int{42},
			wantCount:  1,
			wantLabel:  "example.com",
			wantTotal:  1,
		},
		{
			name:  "root empty",
			input: map[string]any{},
			mockData: map[string]any{
				"sitemapRootEntries": map[string]any{
					"count": map[string]any{"value": 0},
					"edges": []map[string]any{},
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:    "graphql error",
			input:   map[string]any{},
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetSitemapTool(s, c)
			})
			env.Mock.ValidateAgainstSchema()
			if !tt.mockErr {
				env.Mock.On("SitemapRootEntries", tt.mockData)
				for _, count := range tt.hostCounts {
					env.Mock.On("ListRequests", map[string]any{
						"requests": map[string]any{
							"edges":    []any{},
							"pageInfo": map[string]any{"hasNextPage": false},
							"count":    map[string]any{"value": count},
						},
					})
				}
			}

			result := env.CallTool(t, "caido_get_sitemap", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
			if len(out.Entries) != tt.wantCount {
				t.Fatalf("entries count = %d, want %d", len(out.Entries), tt.wantCount)
			}
			if out.TotalEntries != tt.wantTotal {
				t.Errorf("totalEntries = %d, want %d", out.TotalEntries, tt.wantTotal)
			}
			if tt.wantCount > 0 {
				if out.Entries[0].Label != tt.wantLabel {
					t.Errorf("label = %q, want %q", out.Entries[0].Label, tt.wantLabel)
				}
				if tt.hostCounts != nil {
					got := out.Entries[0].CapturedRequests
					if got == nil || *got != tt.hostCounts[0] {
						t.Errorf(
							"capturedRequests = %v, want %d",
							got, tt.hostCounts[0],
						)
					}
				}
			}
		})
	}
}
