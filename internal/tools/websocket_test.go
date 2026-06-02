package tools_test

import (
	"encoding/base64"
	"testing"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
)

// b64 encodes a string the way the GraphQL Blob scalar is serialized, so the
// tool's decodeWsBody can round-trip it back to plaintext.
func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// streamsData builds the "streams" connection payload for OpName ListStreams.
// hasNext drives pagination; endCursor is only meaningful when hasNext is true.
func streamsData(hasNext bool, endCursor string, ids ...string) map[string]any {
	edges := make([]map[string]any, len(ids))
	for i, id := range ids {
		edges[i] = map[string]any{
			"cursor": "c-" + id,
			"node": map[string]any{
				"id":        id,
				"host":      "echo.example.com",
				"port":      443,
				"path":      "/ws",
				"isTls":     true,
				"direction": "BOTH",
				"source":    "PROXY",
				"createdAt": int64(1714900000000),
			},
		}
	}
	var cursor any
	if endCursor != "" {
		cursor = endCursor
	}
	return map[string]any{
		"streams": map[string]any{
			"edges": edges,
			"pageInfo": map[string]any{
				"hasNextPage": hasNext,
				"endCursor":   cursor,
			},
		},
	}
}

// wsMessagesData builds the "streamWsMessages" connection payload for OpName
// ListStreamWsMessages. raw is provided already base64-encoded.
func wsMessagesData(hasNext bool, endCursor string, raws ...string) map[string]any {
	edges := make([]map[string]any, len(raws))
	for i, raw := range raws {
		edges[i] = map[string]any{
			"cursor": "mc-" + raw[:1],
			"node": map[string]any{
				"id": "msg-" + raw[:1],
				"head": map[string]any{
					"direction": "CLIENT",
					"format":    "TEXT",
					"length":    len(raw),
					"raw":       raw,
				},
			},
		}
	}
	var cursor any
	if endCursor != "" {
		cursor = endCursor
	}
	return map[string]any{
		"streamWsMessages": map[string]any{
			"edges": edges,
			"pageInfo": map[string]any{
				"hasNextPage": hasNext,
				"endCursor":   cursor,
			},
		},
	}
}

func TestListWsStreamsTool(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		mock        map[string]any
		registerOp  bool
		wantErr     bool
		wantStreams int
		wantHasMore bool
		wantCursor  string
		wantFirstID string
	}{
		{
			name:        "success single page",
			input:       map[string]any{"limit": 20},
			mock:        streamsData(false, "", "stream-1", "stream-2"),
			registerOp:  true,
			wantStreams: 2,
			wantHasMore: false,
			wantFirstID: "stream-1",
		},
		{
			name:        "pagination has more",
			input:       map[string]any{"limit": 1},
			mock:        streamsData(true, "cursor-xyz", "stream-1"),
			registerOp:  true,
			wantStreams: 1,
			wantHasMore: true,
			wantCursor:  "cursor-xyz",
			wantFirstID: "stream-1",
		},
		{
			// No mock registered -> server returns a GraphQL errors array,
			// which MakeRequest surfaces as an error.
			name:       "graphql error",
			input:      map[string]any{"limit": 5},
			registerOp: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListWsStreamsTool(s, c)
			})
			if tt.registerOp {
				env.Mock.On("ListStreams", tt.mock)
			}

			result := env.CallTool(t, "caido_list_ws_streams", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListWsStreamsOutput](t, result)
			if len(out.Streams) != tt.wantStreams {
				t.Fatalf("streams = %d, want %d", len(out.Streams), tt.wantStreams)
			}
			if out.HasMore != tt.wantHasMore {
				t.Fatalf("hasMore = %v, want %v", out.HasMore, tt.wantHasMore)
			}
			if out.NextCursor != tt.wantCursor {
				t.Fatalf("nextCursor = %q, want %q", out.NextCursor, tt.wantCursor)
			}
			if tt.wantStreams > 0 {
				first := out.Streams[0]
				if first.ID != tt.wantFirstID {
					t.Fatalf("first id = %q, want %q", first.ID, tt.wantFirstID)
				}
				if first.Host != "echo.example.com" || first.Port != 443 {
					t.Fatalf("unexpected host/port: %q:%d", first.Host, first.Port)
				}
				if !first.IsTLS || first.Source != "PROXY" {
					t.Fatalf("unexpected isTls/source: %v/%q", first.IsTLS, first.Source)
				}
			}
		})
	}
}

func TestListWsMessagesTool(t *testing.T) {
	const bigPayload = "0123456789ABCDEF" // 16 bytes, used for truncation case

	tests := []struct {
		name          string
		input         map[string]any
		mock          map[string]any
		registerOp    bool
		wantErr       bool
		wantMessages  int
		wantHasMore   bool
		wantCursor    string
		wantFirstBody string
		wantTruncated bool
	}{
		{
			name:          "success decodes base64 body",
			input:         map[string]any{"stream_id": "stream-1"},
			mock:          wsMessagesData(false, "", b64("hello-frame")),
			registerOp:    true,
			wantMessages:  1,
			wantHasMore:   false,
			wantFirstBody: "hello-frame",
			wantTruncated: false,
		},
		{
			name:         "pagination has more",
			input:        map[string]any{"stream_id": "stream-1", "limit": 1},
			mock:         wsMessagesData(true, "next-cursor", b64("frame-a")),
			registerOp:   true,
			wantMessages: 1,
			wantHasMore:  true,
			wantCursor:   "next-cursor",
		},
		{
			name: "body_limit truncates",
			input: map[string]any{
				"stream_id":  "stream-1",
				"body_limit": 4,
			},
			mock:          wsMessagesData(false, "", b64(bigPayload)),
			registerOp:    true,
			wantMessages:  1,
			wantFirstBody: bigPayload[:4],
			wantTruncated: true,
		},
		{
			// stream_id is required; the handler fails fast before any
			// GraphQL call, so no mock is needed.
			name:       "missing stream_id validation error",
			input:      map[string]any{},
			registerOp: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListWsMessagesTool(s, c)
			})
			if tt.registerOp {
				env.Mock.On("ListStreamWsMessages", tt.mock)
			}

			result := env.CallTool(t, "caido_list_ws_messages", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListWsMessagesOutput](t, result)
			if len(out.Messages) != tt.wantMessages {
				t.Fatalf("messages = %d, want %d", len(out.Messages), tt.wantMessages)
			}
			if out.HasMore != tt.wantHasMore {
				t.Fatalf("hasMore = %v, want %v", out.HasMore, tt.wantHasMore)
			}
			if out.NextCursor != tt.wantCursor {
				t.Fatalf("nextCursor = %q, want %q", out.NextCursor, tt.wantCursor)
			}
			if tt.wantMessages > 0 {
				first := out.Messages[0]
				if first.Direction != "CLIENT" || first.Format != "TEXT" {
					t.Fatalf("unexpected direction/format: %q/%q",
						first.Direction, first.Format)
				}
				if tt.wantFirstBody != "" && first.Body != tt.wantFirstBody {
					t.Fatalf("body = %q, want %q", first.Body, tt.wantFirstBody)
				}
				if first.Truncated != tt.wantTruncated {
					t.Fatalf("truncated = %v, want %v",
						first.Truncated, tt.wantTruncated)
				}
			}
		})
	}
}
