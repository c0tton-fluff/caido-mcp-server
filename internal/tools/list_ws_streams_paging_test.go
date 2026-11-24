package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wsStreamEdge builds one streams edge with the given protocol.
func wsStreamEdge(id, protocol, cursor string) map[string]any {
	return map[string]any{
		"cursor": cursor,
		"node": map[string]any{
			"id":        id,
			"host":      "ws.example.com",
			"port":      443,
			"path":      "/socket",
			"isTls":     true,
			"direction": "BOTH",
			"source":    "PROXY",
			"protocol":  protocol,
			"createdAt": int64(1714900000000),
		},
	}
}

// TestListWsStreamsPaginatesAcrossMixedPages covers the client-side protocol
// filter: a page that mixes SSE and WebSocket streams must not end the listing
// before the requested number of WebSocket streams is reached.
func TestListWsStreamsPaginatesAcrossMixedPages(t *testing.T) {
	page := func(edges []map[string]any, hasNext bool, endCursor any) map[string]any {
		return map[string]any{
			"streams": map[string]any{
				"edges": edges,
				"pageInfo": map[string]any{
					"hasNextPage": hasNext,
					"endCursor":   endCursor,
				},
			},
		}
	}

	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterListWsStreamsTool(s, c)
	})
	// Page 1: one SSE stream, one WS stream.
	env.Mock.On("ListStreams", page([]map[string]any{
		wsStreamEdge("s-sse", "SSE", "c1"),
		wsStreamEdge("s-ws1", "WS", "c2"),
	}, true, "c2"))
	// Page 2: two WS streams, no further pages.
	env.Mock.On("ListStreams", page([]map[string]any{
		wsStreamEdge("s-ws2", "WS", "c3"),
		wsStreamEdge("s-ws3", "WS", "c4"),
	}, false, nil))

	result := env.CallTool(t, "caido_list_ws_streams", map[string]any{"limit": 3})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListWsStreamsOutput](t, result)
	if len(out.Streams) != 3 {
		t.Fatalf("streams = %d, want 3", len(out.Streams))
	}
	if out.Streams[0].ID != "s-ws1" || out.Streams[2].ID != "s-ws3" {
		t.Errorf("ids = %q..%q", out.Streams[0].ID, out.Streams[2].ID)
	}
	if out.HasMore {
		t.Errorf("hasMore = true, want false at the end of the listing")
	}
}

// TestListWsStreamsStopsAtLimit covers truncation mid-page: the cursor must
// point at the last consumed edge so the next call continues there.
func TestListWsStreamsStopsAtLimit(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterListWsStreamsTool(s, c)
	})
	env.Mock.On("ListStreams", map[string]any{
		"streams": map[string]any{
			"edges": []map[string]any{
				wsStreamEdge("s-ws1", "WS", "c1"),
				wsStreamEdge("s-ws2", "WS", "c2"),
			},
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "c2"},
		},
	})

	result := env.CallTool(t, "caido_list_ws_streams", map[string]any{"limit": 1})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListWsStreamsOutput](t, result)
	if len(out.Streams) != 1 || out.Streams[0].ID != "s-ws1" {
		t.Fatalf("streams = %+v", out.Streams)
	}
	if !out.HasMore || out.NextCursor != "c1" {
		t.Fatalf("hasMore/nextCursor = %v/%q, want true/c1", out.HasMore, out.NextCursor)
	}
}

// TestListWsStreamsStopsReadingWhenFull covers the cost of a full response:
// once limit streams are collected the handler must return, not read further
// pages to learn whether another WebSocket stream exists.
func TestListWsStreamsStopsReadingWhenFull(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterListWsStreamsTool(s, c)
	})
	page := func(id, protocol, cursor string) map[string]any {
		return map[string]any{
			"streams": map[string]any{
				"edges": []map[string]any{wsStreamEdge(id, protocol, cursor)},
				"pageInfo": map[string]any{
					"hasNextPage": true, "endCursor": cursor,
				},
			},
		}
	}
	env.Mock.On("ListStreams", page("s-ws1", "WS", "p1"))
	// Every later page is SSE only. Reaching one means the handler kept
	// reading after it was full.
	env.Mock.On("ListStreams", page("s-sse", "SSE", "p2"))

	result := env.CallTool(t, "caido_list_ws_streams", map[string]any{"limit": 1})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListWsStreamsOutput](t, result)
	if len(out.Streams) != 1 || out.Streams[0].ID != "s-ws1" {
		t.Fatalf("streams = %+v", out.Streams)
	}
	if !out.HasMore || out.NextCursor != "p1" {
		t.Fatalf("hasMore/nextCursor = %v/%q, want true/p1", out.HasMore, out.NextCursor)
	}
	if after, sent := env.Mock.LastVariables("ListStreams")["after"]; sent && after != nil {
		t.Errorf("read a second page (after=%v) with the response already full", after)
	}
}

// TestListWsStreamsPageSizeIgnoresLimit covers the scan budget: the upstream
// page size must not shrink with limit, or limit 1 scans 20 streams in all.
func TestListWsStreamsPageSizeIgnoresLimit(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterListWsStreamsTool(s, c)
	})
	env.Mock.On("ListStreams", map[string]any{
		"streams": map[string]any{
			"edges":    []map[string]any{wsStreamEdge("s-ws1", "WS", "c1")},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
		},
	})
	result := env.CallTool(t, "caido_list_ws_streams", map[string]any{"limit": 1})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if first := env.Mock.LastVariables("ListStreams")["first"]; first != float64(100) {
		t.Errorf("upstream first = %v, want 100", first)
	}
}
