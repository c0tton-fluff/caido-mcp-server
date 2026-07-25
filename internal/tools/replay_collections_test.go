package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listReplaySessionsResponse builds the GraphQL data object for ListReplaySessions.
// Top-level field is "replaySessions" (a connection of edges->node).
func listReplaySessionsResponse(sessions ...map[string]any) map[string]any {
	edges := make([]map[string]any, len(sessions))
	for i, s := range sessions {
		edges[i] = map[string]any{"node": s}
	}
	return map[string]any{
		"replaySessions": map[string]any{
			"edges":    edges,
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"count":    map[string]any{"value": len(sessions)},
		},
	}
}

// listReplayCollectionsResponse builds the GraphQL data object for
// ListReplaySessionCollections. Top-level field is "replaySessionCollections".
func listReplayCollectionsResponse(collections ...map[string]any) map[string]any {
	edges := make([]map[string]any, len(collections))
	for i, c := range collections {
		edges[i] = map[string]any{"node": c}
	}
	return map[string]any{
		"replaySessionCollections": map[string]any{
			"edges":    edges,
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"count":    map[string]any{"value": len(collections)},
		},
	}
}

func TestListReplaySessions(t *testing.T) {
	tests := []struct {
		name      string
		mock      map[string]any
		wantNames []string
		wantErr   bool
	}{
		{
			name: "lists sessions with and without active entry",
			mock: listReplaySessionsResponse(
				map[string]any{
					"__typename": "ReplaySessionHttp",
					"id":         "sess-1",
					"name":       "auth",
					"activeEntry": map[string]any{
						"__typename": "ReplayEntryHttp",
						"id":         "entry-9",
					},
					"collection": map[string]any{"id": "col-1", "name": "main"},
				},
				map[string]any{
					"__typename":  "ReplaySessionHttp",
					"id":          "sess-2",
					"name":        "api",
					"activeEntry": nil,
					"collection":  map[string]any{"id": "col-1", "name": "main"},
				},
			),
			wantNames: []string{"auth", "api"},
		},
		{
			name:      "empty list",
			mock:      listReplaySessionsResponse(),
			wantNames: []string{},
		},
		{
			name:    "graphql error surfaces as tool error",
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListReplaySessionsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("ListReplaySessions", tt.mock)
			}

			result := env.CallTool(t, "caido_list_replay_sessions", map[string]any{})

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListReplaySessionsOutput](t, result)
			if len(out.Sessions) != len(tt.wantNames) {
				t.Fatalf("want %d sessions, got %d", len(tt.wantNames), len(out.Sessions))
			}
			for i, want := range tt.wantNames {
				if out.Sessions[i].Name != want {
					t.Errorf("session %d: want name %q, got %q", i, want, out.Sessions[i].Name)
				}
			}
			if len(out.Sessions) > 0 {
				if out.Sessions[0].ActiveEntryID == nil || *out.Sessions[0].ActiveEntryID != "entry-9" {
					t.Errorf("want activeEntryId entry-9 on first session, got %v", out.Sessions[0].ActiveEntryID)
				}
				if out.Sessions[1].ActiveEntryID != nil {
					t.Errorf("want nil activeEntryId on second session, got %v", *out.Sessions[1].ActiveEntryID)
				}
			}
		})
	}
}

func TestListReplayCollections(t *testing.T) {
	tests := []struct {
		name    string
		mock    map[string]any
		wantIDs []string
		wantErr bool
	}{
		{
			name: "lists collections",
			mock: listReplayCollectionsResponse(
				map[string]any{"id": "col-1", "name": "main", "sessions": []any{}},
				map[string]any{"id": "col-2", "name": "scratch", "sessions": []any{}},
			),
			wantIDs: []string{"col-1", "col-2"},
		},
		{
			name:    "empty list",
			mock:    listReplayCollectionsResponse(),
			wantIDs: []string{},
		},
		{
			name:    "graphql error surfaces as tool error",
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListReplayCollectionsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("ListReplaySessionCollections", tt.mock)
			}

			result := env.CallTool(t, "caido_list_replay_collections", map[string]any{})

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListReplayCollectionsOutput](t, result)
			if len(out.Collections) != len(tt.wantIDs) {
				t.Fatalf("want %d collections, got %d", len(tt.wantIDs), len(out.Collections))
			}
			for i, want := range tt.wantIDs {
				if out.Collections[i].ID != want {
					t.Errorf("collection %d: want id %q, got %q", i, want, out.Collections[i].ID)
				}
			}
		})
	}
}

func TestCreateReplayCollection(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		mock     map[string]any
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name: "creates collection",
			args: map[string]any{"name": "fuzzing"},
			mock: map[string]any{
				"createReplaySessionCollection": map[string]any{
					"collection": map[string]any{"id": "col-new", "name": "fuzzing"},
				},
			},
			wantID:   "col-new",
			wantName: "fuzzing",
		},
		{
			name: "nil collection in payload errors",
			args: map[string]any{"name": "fuzzing"},
			mock: map[string]any{
				"createReplaySessionCollection": map[string]any{"collection": nil},
			},
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"name": "fuzzing"},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateReplayCollectionTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("CreateReplaySessionCollection", tt.mock)
			}

			result := env.CallTool(t, "caido_create_replay_collection", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateReplayCollectionOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("want id %q, got %q", tt.wantID, out.ID)
			}
			if out.Name != tt.wantName {
				t.Errorf("want name %q, got %q", tt.wantName, out.Name)
			}
		})
	}
}

func TestRenameReplayCollection(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		mock     map[string]any
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name: "renames collection",
			args: map[string]any{"id": "col-1", "name": "renamed"},
			mock: map[string]any{
				"renameReplaySessionCollection": map[string]any{
					"collection": map[string]any{"id": "col-1", "name": "renamed"},
				},
			},
			wantID:   "col-1",
			wantName: "renamed",
		},
		{
			name: "nil collection in payload errors",
			args: map[string]any{"id": "col-1", "name": "renamed"},
			mock: map[string]any{
				"renameReplaySessionCollection": map[string]any{"collection": nil},
			},
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"id": "col-1", "name": "renamed"},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterRenameReplayCollectionTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("RenameReplaySessionCollection", tt.mock)
			}

			result := env.CallTool(t, "caido_rename_replay_collection", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.RenameReplayCollectionOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("want id %q, got %q", tt.wantID, out.ID)
			}
			if out.Name != tt.wantName {
				t.Errorf("want name %q, got %q", tt.wantName, out.Name)
			}
		})
	}
}

func TestDeleteReplayCollection(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		mock        map[string]any
		wantSuccess bool
		wantErr     bool
	}{
		{
			name: "deletes collection",
			args: map[string]any{"id": "col-1"},
			mock: map[string]any{
				"deleteReplaySessionCollection": map[string]any{"deletedId": "col-1"},
			},
			wantSuccess: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"id": "col-1"},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteReplayCollectionTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("DeleteReplaySessionCollection", tt.mock)
			}

			result := env.CallTool(t, "caido_delete_replay_collection", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteReplayCollectionOutput](t, result)
			if out.Success != tt.wantSuccess {
				t.Errorf("want success %v, got %v", tt.wantSuccess, out.Success)
			}
		})
	}
}

func TestMoveReplaySession(t *testing.T) {
	tests := []struct {
		name             string
		args             map[string]any
		mock             map[string]any
		wantID           string
		wantCollectionID string
		wantErr          bool
	}{
		{
			name: "moves session to collection",
			args: map[string]any{"sessionId": "sess-1", "collectionId": "col-2"},
			mock: map[string]any{
				"moveReplaySession": map[string]any{
					"session": map[string]any{
						"__typename": "ReplaySessionHttp",
						"id":         "sess-1",
						"name":       "auth",
						"collection": map[string]any{"id": "col-2", "name": "scratch"},
					},
				},
			},
			wantID:           "sess-1",
			wantCollectionID: "col-2",
		},
		{
			name: "nil session in payload errors",
			args: map[string]any{"sessionId": "sess-1", "collectionId": "col-2"},
			mock: map[string]any{
				"moveReplaySession": map[string]any{"session": nil},
			},
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"sessionId": "sess-1", "collectionId": "col-2"},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterMoveReplaySessionTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("MoveReplaySession", tt.mock)
			}

			result := env.CallTool(t, "caido_move_replay_session", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.MoveReplaySessionOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("want id %q, got %q", tt.wantID, out.ID)
			}
			if out.CollectionID != tt.wantCollectionID {
				t.Errorf("want collectionId %q, got %q", tt.wantCollectionID, out.CollectionID)
			}
		})
	}
}

func TestDeleteReplaySessions(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]any
		mock           map[string]any
		wantDeletedIDs []string
		wantErr        bool
	}{
		{
			name: "deletes multiple sessions",
			args: map[string]any{"ids": []string{"sess-1", "sess-2"}},
			mock: map[string]any{
				"deleteReplaySessions": map[string]any{
					"deletedIds": []string{"sess-1", "sess-2"},
				},
			},
			wantDeletedIDs: []string{"sess-1", "sess-2"},
		},
		{
			name:    "empty ids rejected by handler validation",
			args:    map[string]any{"ids": []string{}},
			mock:    nil,
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"ids": []string{"sess-1"}},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteReplaySessionsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("DeleteReplaySessions", tt.mock)
			}

			result := env.CallTool(t, "caido_delete_replay_sessions", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteReplaySessionsOutput](t, result)
			if len(out.DeletedIDs) != len(tt.wantDeletedIDs) {
				t.Fatalf("want %d deleted ids, got %d", len(tt.wantDeletedIDs), len(out.DeletedIDs))
			}
			for i, want := range tt.wantDeletedIDs {
				if out.DeletedIDs[i] != want {
					t.Errorf("deleted id %d: want %q, got %q", i, want, out.DeletedIDs[i])
				}
			}
		})
	}
}
