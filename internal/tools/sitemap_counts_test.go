package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sitemapEntry builds one wire entry for the descendant connection mock.
func sitemapEntry(
	id, label, kind string, requestCount *int, request map[string]any,
) map[string]any {
	node := map[string]any{
		"id":             id,
		"label":          label,
		"kind":           kind,
		"parentId":       "1",
		"hasDescendants": kind == "DIRECTORY",
	}
	if requestCount != nil {
		node["requests"] = map[string]any{
			"count": map[string]any{"value": *requestCount},
		}
	}
	if request != nil {
		node["request"] = request
	}
	return map[string]any{"node": node}
}

// TestGetSitemapDepthAll asserts the whole subtree is requested, per-entry
// counts are read when asked for, and the depth/counts variables match.
func TestGetSitemapDepthAll(t *testing.T) {
	counts := map[string]int{"users": 12, "file": 7, "query": 0}

	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetSitemapTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("SitemapDescendantEntries", map[string]any{
		"sitemapDescendantEntries": map[string]any{
			"count": map[string]any{"value": 3},
			"edges": []map[string]any{
				sitemapEntry(
					"10", "users", "DIRECTORY", ptrInt(counts["users"]), nil,
				),
				sitemapEntry(
					"11", "/api/v1/users", "REQUEST", ptrInt(counts["file"]),
					map[string]any{
						"id": "req-1", "method": "GET",
						"response": map[string]any{"statusCode": 200},
					},
				),
				sitemapEntry(
					"12", "/api/v1/users?x=1", "REQUEST_QUERY",
					ptrInt(counts["query"]),
					map[string]any{
						"id": "req-2", "method": "GET", "response": nil,
					},
				),
			},
		},
	})

	result := env.CallTool(t, "caido_get_sitemap", map[string]any{
		"parentId":             "1",
		"depth":                "all",
		"includeRequestCounts": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	vars := env.Mock.LastVariables("SitemapDescendantEntries")
	if vars["depth"] != "ALL" {
		t.Errorf("depth variable = %v, want ALL", vars["depth"])
	}
	if vars["counts"] != true {
		t.Errorf("counts variable = %v, want true", vars["counts"])
	}

	out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
	if len(out.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(out.Entries))
	}
	if out.TotalEntries != 3 {
		t.Errorf("totalEntries = %d, want 3", out.TotalEntries)
	}
	if out.Entries[0].RequestCount == nil || *out.Entries[0].RequestCount != 12 {
		t.Errorf("directory requestCount = %v, want 12", out.Entries[0].RequestCount)
	}
	if out.Entries[2].RequestCount == nil || *out.Entries[2].RequestCount != 0 {
		t.Errorf("zero requestCount = %v, want an explicit 0", out.Entries[2].RequestCount)
	}
	if out.Entries[1].RequestID == nil || *out.Entries[1].RequestID != "req-1" {
		t.Errorf("requestId = %v", out.Entries[1].RequestID)
	}
	if out.Entries[1].StatusCode == nil || *out.Entries[1].StatusCode != 200 {
		t.Errorf("statusCode = %v", out.Entries[1].StatusCode)
	}
	if out.Entries[2].StatusCode != nil {
		t.Errorf("statusCode without a response = %v", *out.Entries[2].StatusCode)
	}
	for _, entry := range out.Entries {
		if entry.CapturedRequests != nil {
			t.Error("capturedRequests is only for root domain entries")
		}
	}
}

// TestGetSitemapDefaultDirect asserts the default call keeps depth DIRECT and
// omits the count field from the query.
func TestGetSitemapDefaultDirect(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetSitemapTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("SitemapDescendantEntries", map[string]any{
		"sitemapDescendantEntries": map[string]any{
			"count": map[string]any{"value": 1},
			"edges": []map[string]any{
				sitemapEntry("10", "users", "DIRECTORY", nil, nil),
			},
		},
	})

	result := env.CallTool(t, "caido_get_sitemap", map[string]any{
		"parentId": "1",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	vars := env.Mock.LastVariables("SitemapDescendantEntries")
	if vars["depth"] != "DIRECT" {
		t.Errorf("depth variable = %v, want DIRECT", vars["depth"])
	}
	if vars["counts"] != false {
		t.Errorf("counts variable = %v, want false by default", vars["counts"])
	}
	out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
	if out.Entries[0].RequestCount != nil {
		t.Errorf("requestCount sent without the option: %v", out.Entries[0].RequestCount)
	}
}

// TestGetSitemapLimit asserts entries are truncated locally and flagged,
// because Caido exposes no cursor for this connection.
func TestGetSitemapLimit(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetSitemapTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("SitemapDescendantEntries", map[string]any{
		"sitemapDescendantEntries": map[string]any{
			"count": map[string]any{"value": 3},
			"edges": []map[string]any{
				sitemapEntry("10", "a", "DIRECTORY", nil, nil),
				sitemapEntry("11", "b", "DIRECTORY", nil, nil),
				sitemapEntry("12", "c", "DIRECTORY", nil, nil),
			},
		},
	})

	result := env.CallTool(t, "caido_get_sitemap", map[string]any{
		"parentId": "1",
		"limit":    2,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
	if len(out.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(out.Entries))
	}
	if !out.Truncated {
		t.Error("expected truncated")
	}
	if out.TotalEntries != 3 {
		t.Errorf("totalEntries = %d, want the connection count 3", out.TotalEntries)
	}
}

// TestGetSitemapDepthValidation asserts depth is rejected without a parent and
// that the schema advertises the accepted values.
func TestGetSitemapDepthValidation(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetSitemapTool(s, c)
	})
	result := env.CallTool(t, "caido_get_sitemap", map[string]any{
		"depth": "all",
	})
	if !result.IsError {
		t.Fatal("expected an error for depth without parentId")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "depth requires parentId") {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if vars := env.Mock.LastVariables("SitemapRootEntries"); vars != nil {
		t.Errorf("invalid input still reached the API: %v", vars)
	}

	schema := advertisedSchema(t, "caido_get_sitemap")
	props, _ := schema["properties"].(map[string]any)
	depth, _ := props["depth"].(map[string]any)
	values, _ := depth["enum"].([]any)
	if len(values) != 2 || values[0] != "direct" || values[1] != "all" {
		t.Errorf("depth enum = %v, want [direct all]", values)
	}
}

func ptrInt(v int) *int { return &v }
