package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listFindingsResponse builds the GraphQL data object for ListFindings.
// Top-level field is "findings" (a connection of edges->node). Each node must
// carry a nested "request" object because the handler reads f.Request.Id.
func listFindingsResponse(hasNext bool, nodes ...map[string]any) map[string]any {
	edges := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		edges[i] = map[string]any{"cursor": "c", "node": n}
	}
	return map[string]any{
		"findings": map[string]any{
			"edges": edges,
			"pageInfo": map[string]any{
				"hasNextPage":     hasNext,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       "cursor-end",
			},
			"count": map[string]any{"value": len(nodes)},
		},
	}
}

// findingNode is a single ListFindings node. createdAt is unix millis (int64).
func findingNode(id, title, host, path, reporter string) map[string]any {
	return map[string]any{
		"id":          id,
		"title":       title,
		"description": nil,
		"host":        host,
		"path":        path,
		"reporter":    reporter,
		"hidden":      false,
		"dedupeKey":   nil,
		"createdAt":   int64(1700000000000),
		"request":     map[string]any{"id": "req-" + id},
	}
}

func TestListFindings(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		mock        map[string]any
		wantTitles  []string
		wantHasMore bool
		wantCursor  string
		wantErr     bool
	}{
		{
			name: "lists findings with pagination cursor",
			args: map[string]any{},
			mock: listFindingsResponse(
				true,
				findingNode("f-1", "SQLi", "example.com", "/login", "Claude"),
				findingNode("f-2", "XSS", "example.com", "/search", "Claude"),
			),
			wantTitles:  []string{"SQLi", "XSS"},
			wantHasMore: true,
			wantCursor:  "cursor-end",
		},
		{
			name:       "empty list",
			args:       map[string]any{},
			mock:       listFindingsResponse(false),
			wantTitles: []string{},
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListFindingsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("ListFindings", tt.mock)
			}

			result := env.CallTool(t, "caido_list_findings", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListFindingsOutput](t, result)
			if len(out.Findings) != len(tt.wantTitles) {
				t.Fatalf("want %d findings, got %d", len(tt.wantTitles), len(out.Findings))
			}
			for i, want := range tt.wantTitles {
				if out.Findings[i].Title != want {
					t.Errorf("finding %d: want title %q, got %q", i, want, out.Findings[i].Title)
				}
			}
			if out.HasMore != tt.wantHasMore {
				t.Errorf("want hasMore %v, got %v", tt.wantHasMore, out.HasMore)
			}
			if out.NextCursor != tt.wantCursor {
				t.Errorf("want nextCursor %q, got %q", tt.wantCursor, out.NextCursor)
			}
			if len(out.Findings) > 0 && out.Findings[0].RequestID != "req-f-1" {
				t.Errorf("want requestId req-f-1, got %q", out.Findings[0].RequestID)
			}
		})
	}
}

// createFindingPayload builds the GraphQL data object for CreateFinding.
// Top-level field is "createFinding". The "error" key is omitted on success;
// when present it must carry a valid CreateFindingError __typename so the SDK
// can unmarshal the interface.
func createFindingPayload(finding map[string]any, errType string) map[string]any {
	payload := map[string]any{"finding": finding}
	if errType != "" {
		payload["error"] = map[string]any{"__typename": errType}
	} else {
		payload["error"] = nil
	}
	return map[string]any{"createFinding": payload}
}

func TestCreateFinding(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		mock     map[string]any
		wantID   string
		wantHost string
		wantErr  bool
	}{
		{
			name: "creates finding",
			args: map[string]any{
				"requestId": "req-1",
				"title":     "Open redirect",
			},
			mock: createFindingPayload(map[string]any{
				"id":          "f-new",
				"title":       "Open redirect",
				"description": nil,
				"host":        "example.com",
				"path":        "/redirect",
				"reporter":    "Claude",
			}, ""),
			wantID:   "f-new",
			wantHost: "example.com",
		},
		{
			name: "title over max length rejected by handler validation",
			args: map[string]any{
				"requestId": "req-1",
				"title":     strings.Repeat("a", 501),
			},
			mock:    nil,
			wantErr: true,
		},
		{
			name: "description over max length rejected by handler validation",
			args: map[string]any{
				"requestId":   "req-1",
				"title":       "ok",
				"description": strings.Repeat("a", 10001),
			},
			mock:    nil,
			wantErr: true,
		},
		{
			name: "payload error surfaces as tool error",
			args: map[string]any{"requestId": "req-1", "title": "ok"},
			mock: createFindingPayload(map[string]any{
				"id": "", "title": "", "description": nil,
				"host": "", "path": "", "reporter": "",
			}, "OtherUserError"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateFindingTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("CreateFinding", tt.mock)
			}

			result := env.CallTool(t, "caido_create_finding", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.CreateFindingOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("want id %q, got %q", tt.wantID, out.ID)
			}
			if out.Host != tt.wantHost {
				t.Errorf("want host %q, got %q", tt.wantHost, out.Host)
			}
		})
	}
}

func TestDeleteFindings(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]any
		mock           map[string]any
		wantDeletedIDs []string
		wantErr        bool
	}{
		{
			name: "deletes by ids",
			args: map[string]any{"ids": []string{"f-1", "f-2"}},
			mock: map[string]any{
				"deleteFindings": map[string]any{
					"deletedIds": []string{"f-1", "f-2"},
				},
			},
			wantDeletedIDs: []string{"f-1", "f-2"},
		},
		{
			name: "deletes by reporter",
			args: map[string]any{"reporter": "Claude"},
			mock: map[string]any{
				"deleteFindings": map[string]any{
					"deletedIds": []string{"f-9"},
				},
			},
			wantDeletedIDs: []string{"f-9"},
		},
		{
			name:    "neither ids nor reporter rejected by handler validation",
			args:    map[string]any{},
			mock:    nil,
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"ids": []string{"f-1"}},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDeleteFindingsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("DeleteFindings", tt.mock)
			}

			result := env.CallTool(t, "caido_delete_findings", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.DeleteFindingsOutput](t, result)
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

func TestExportFindings(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]any
		mock         map[string]any
		wantExportID string
		wantErr      bool
	}{
		{
			name: "exports by ids",
			args: map[string]any{"ids": []string{"f-1"}},
			mock: map[string]any{
				"exportFindings": map[string]any{
					"export": map[string]any{"id": "export-123"},
					"error":  nil,
				},
			},
			wantExportID: "export-123",
		},
		{
			name: "exports by reporter",
			args: map[string]any{"reporter": "Claude"},
			mock: map[string]any{
				"exportFindings": map[string]any{
					"export": map[string]any{"id": "export-456"},
					"error":  nil,
				},
			},
			wantExportID: "export-456",
		},
		{
			name:    "neither ids nor reporter rejected by handler validation",
			args:    map[string]any{},
			mock:    nil,
			wantErr: true,
		},
		{
			name: "payload error surfaces as tool error",
			args: map[string]any{"ids": []string{"f-1"}},
			mock: map[string]any{
				"exportFindings": map[string]any{
					"export": nil,
					"error":  map[string]any{"__typename": "OtherUserError"},
				},
			},
			wantErr: true,
		},
		{
			name:    "graphql error surfaces as tool error",
			args:    map[string]any{"ids": []string{"f-1"}},
			mock:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterExportFindingsTool(s, c)
			})
			if tt.mock != nil {
				env.Mock.On("ExportFindings", tt.mock)
			}

			result := env.CallTool(t, "caido_export_findings", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ExportFindingsOutput](t, result)
			if out.ExportID != tt.wantExportID {
				t.Errorf("want exportId %q, got %q", tt.wantExportID, out.ExportID)
			}
		})
	}
}
