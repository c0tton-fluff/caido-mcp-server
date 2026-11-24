package tools_test

import (
	"fmt"
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

// sitemapRoot builds one root DOMAIN entry for the root connection mock.
func sitemapRoot(id, label string, port int, isTLS bool) map[string]any {
	return map[string]any{"node": map[string]any{
		"id":             id,
		"label":          label,
		"kind":           "DOMAIN",
		"hasDescendants": true,
		"metadata":       map[string]any{"isTls": isTLS, "port": port},
	}}
}

func sitemapRoots(edges ...map[string]any) map[string]any {
	return map[string]any{"sitemapRootEntries": map[string]any{
		"count": map[string]any{"value": len(edges)},
		"edges": edges,
	}}
}

func requestCountResponse(count int) map[string]any {
	return map[string]any{"requests": map[string]any{
		"edges":    []any{},
		"pageInfo": map[string]any{"hasNextPage": false},
		"count":    map[string]any{"value": count},
	}}
}

// TestGetSitemapRootCountsAreOptInAndBounded covers the cost of the history
// count on root domains: none without includeRequestCounts, and with it one
// per returned entry, after limit. Counting every root before truncating
// made the default listing take 103 s on a project with 6,819 roots.
func TestGetSitemapRootCountsAreOptInAndBounded(t *testing.T) {
	newEnv := func(t *testing.T) *testutil.MCPTestEnv {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterGetSitemapTool(s, c)
		})
		env.Mock.ValidateAgainstSchema()
		env.Mock.On("SitemapRootEntries", sitemapRoots(
			sitemapRoot("1", "a.example", 443, true),
			sitemapRoot("2", "b.example", 443, true),
			sitemapRoot("3", "c.example", 443, true),
		))
		env.Mock.On("ListRequests", requestCountResponse(42))
		return env
	}

	t.Run("limit without counts", func(t *testing.T) {
		env := newEnv(t)
		result := env.CallTool(t, "caido_get_sitemap", map[string]any{"limit": 1})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		if n := env.Mock.Calls("ListRequests"); n != 0 {
			t.Errorf("history counts = %d, want 0", n)
		}
	})

	t.Run("counts follow limit", func(t *testing.T) {
		env := newEnv(t)
		result := env.CallTool(t, "caido_get_sitemap", map[string]any{
			"limit": 1, "includeRequestCounts": true,
		})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
		if len(out.Entries) != 1 || !out.Truncated || out.TotalEntries != 3 {
			t.Fatalf("entries/truncated/total = %d/%v/%d, want 1/true/3",
				len(out.Entries), out.Truncated, out.TotalEntries)
		}
		if got := out.Entries[0].CapturedRequests; got == nil || *got != 42 {
			t.Errorf("capturedRequests = %v, want 42", got)
		}
		if n := env.Mock.Calls("ListRequests"); n != 1 {
			t.Errorf("history counts = %d, want 1 (the returned entry only)", n)
		}
	})
}

// TestGetSitemapRootCountIsPerPortAndScheme asserts the history count selects
// one root entry, not every entry sharing its label: a host seen on two ports
// is two roots, and a host-only filter reports the same total on both.
func TestGetSitemapRootCountIsPerPortAndScheme(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetSitemapTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("SitemapRootEntries", sitemapRoots(
		sitemapRoot("1", "127.0.0.1", 8080, false),
	))
	env.Mock.On("ListRequests", requestCountResponse(7))

	result := env.CallTool(t, "caido_get_sitemap", map[string]any{
		"includeRequestCounts": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
	root := out.Entries[0]
	if root.Port == nil || *root.Port != 8080 || root.IsTLS {
		t.Errorf("port/isTls = %v/%v, want 8080/false", root.Port, root.IsTLS)
	}
	const want = `req.host.eq:"127.0.0.1" and req.port.eq:8080 and req.tls.eq:false`
	if got := fmt.Sprint(env.Mock.LastVariables("ListRequests")["filter"]); !strings.Contains(got, want) {
		t.Errorf("count filter = %s, want it to contain %s", got, want)
	}
}

// TestGetSitemapRootCountFailures asserts one uncountable root does not fail
// the listing, and that a listing where no count works says so.
func TestGetSitemapRootCountFailures(t *testing.T) {
	t.Run("one entry cannot be counted", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterGetSitemapTool(s, c)
		})
		env.Mock.On("SitemapRootEntries", sitemapRoots(
			sitemapRoot("1", `bad"label`, 443, true),
			sitemapRoot("2", "ok.example", 443, true),
		))
		env.Mock.On("ListRequests", requestCountResponse(5))
		result := env.CallTool(t, "caido_get_sitemap", map[string]any{
			"includeRequestCounts": true,
		})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		out := testutil.UnmarshalToolResult[tools.GetSitemapOutput](t, result)
		if out.Entries[0].CapturedRequests != nil {
			t.Errorf("uncountable entry has capturedRequests = %d", *out.Entries[0].CapturedRequests)
		}
		if got := out.Entries[1].CapturedRequests; got == nil || *got != 5 {
			t.Errorf("capturedRequests = %v, want 5", got)
		}
	})

	t.Run("no count works", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterGetSitemapTool(s, c)
		})
		env.Mock.On("SitemapRootEntries", sitemapRoots(
			sitemapRoot("1", "a.example", 443, true),
		))
		// No ListRequests mock: every count fails.
		result := env.CallTool(t, "caido_get_sitemap", map[string]any{
			"includeRequestCounts": true,
		})
		if !result.IsError {
			t.Fatal("expected an error when no count can be read")
		}
	})
}
