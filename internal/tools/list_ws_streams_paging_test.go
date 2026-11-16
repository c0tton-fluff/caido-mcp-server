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
// filter: a page holding mostly HTTP traffic must not end the listing before
// the requested number of WebSocket streams is reached.
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
	// Page 1: one HTTP stream, one WS stream.
	env.Mock.On("ListStreams", page([]map[string]any{
		wsStreamEdge("s-http", "HTTP", "c1"),
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
