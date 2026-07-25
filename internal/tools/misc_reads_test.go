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

// TestGetSitemapTool covers caido_get_sitemap root path.
// SDK op: ListSitemapRootEntries -> data.sitemapRootEntries.edges[].node { id, label, kind, hasDescendants }.
func TestGetSitemapTool(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		mockData  map[string]any
		mockErr   bool
		wantErr   bool
		wantCount int
		wantLabel string
	}{
		{
			name:  "root success",
			input: map[string]any{},
			mockData: map[string]any{
				"sitemapRootEntries": map[string]any{
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
			wantCount: 1,
			wantLabel: "example.com",
		},
		{
			name:  "root empty",
			input: map[string]any{},
			mockData: map[string]any{
				"sitemapRootEntries": map[string]any{
					"edges": []map[string]any{},
				},
			},
			wantCount: 0,
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
			if !tt.mockErr {
				env.Mock.On("ListSitemapRootEntries", tt.mockData)
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
			if tt.wantCount > 0 && out.Entries[0].Label != tt.wantLabel {
				t.Errorf("label = %q, want %q", out.Entries[0].Label, tt.wantLabel)
			}
		})
	}
}

// TestGetSessionCookiesTool covers caido_get_session_cookies.
// This tool reads the in-memory cookie jar (no GraphQL fetch of cookies), so the
// provided GetSessionCookies op is NOT used. An explicit sessionId is passed to
// avoid the CreateReplaySession path (which mutates package-global default state).
func TestGetSessionCookiesTool(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		wantURL string
		wantSID string
	}{
		{
			name: "success empty jar",
			input: map[string]any{
				"sessionId": "sess-1",
				"url":       "https://example.com/app",
			},
			wantURL: "https://example.com/app",
			wantSID: "sess-1",
		},
		{
			name:    "missing required url",
			input:   map[string]any{"sessionId": "sess-1"},
			wantErr: true,
		},
		{
			name: "invalid url",
			input: map[string]any{
				"sessionId": "sess-1",
				"url":       "ht!tp://%zz",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetSessionCookiesTool(s, c)
			})

			result := env.CallTool(t, "caido_get_session_cookies", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetSessionCookiesOutput](t, result)
			if out.SessionID != tt.wantSID {
				t.Errorf("sessionId = %q, want %q", out.SessionID, tt.wantSID)
			}
			if out.URL != tt.wantURL {
				t.Errorf("url = %q, want %q", out.URL, tt.wantURL)
			}
			if out.Count != len(out.Cookies) {
				t.Errorf("count = %d, want len(cookies) = %d", out.Count, len(out.Cookies))
			}
		})
	}
}

// TestClearSessionCookiesTool covers caido_clear_session_cookies.
// Operates purely on the in-memory cookie store; an explicit sessionId avoids the
// CreateReplaySession path. Clearing an untracked jar returns cleared=false + a note.
func TestClearSessionCookiesTool(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterClearSessionCookiesTool(s, c)
	})

	result := env.CallTool(t, "caido_clear_session_cookies", map[string]any{
		"sessionId": "sess-untracked",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ClearSessionCookiesOutput](t, result)
	if out.SessionID != "sess-untracked" {
		t.Errorf("sessionId = %q, want %q", out.SessionID, "sess-untracked")
	}
	if out.Cleared {
		t.Errorf("cleared = true for untracked jar, want false")
	}
	if out.Note == "" {
		t.Errorf("expected a note explaining no jar was tracked")
	}
}
