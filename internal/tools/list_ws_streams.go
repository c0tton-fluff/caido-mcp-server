package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListWsStreamsInput is the input for the list_ws_streams tool
type ListWsStreamsInput struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of streams to return (default 20, max 100)"`
	After   string `json:"after,omitempty" jsonschema:"Cursor for pagination from previous response nextCursor"`
	ScopeID string `json:"scope_id,omitempty" jsonschema:"Optional scope ID to filter streams"`
}

// WsStreamSummary is a minimal representation of a WebSocket stream
type WsStreamSummary struct {
	ID        string `json:"id"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	IsTLS     bool   `json:"isTls"`
	Direction string `json:"direction"`
	Source    string `json:"source"`
	// ReplaySessionID is the WS replay session that opened a REPLAY stream.
	ReplaySessionID string `json:"replaySessionId,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
}

// ListWsStreamsOutput is the output of the list_ws_streams tool
type ListWsStreamsOutput struct {
	Streams    []WsStreamSummary `json:"streams"`
	HasMore    bool              `json:"hasMore"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

func listWsStreamsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWsStreamsInput) (*mcp.CallToolResult, ListWsStreamsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWsStreamsInput,
	) (*mcp.CallToolResult, ListWsStreamsOutput, error) {
		limit := clampLimit(input.Limit, 20, 100)

		opts := &caido.ListStreamsOptions{
			First: &limit,
		}
		if input.After != "" {
			opts.After = &input.After
		}
		if input.ScopeID != "" {
			opts.ScopeID = &input.ScopeID
		}

		resp, err := client.Streams.List(ctx, opts)
		if err != nil {
			return nil, ListWsStreamsOutput{}, fmt.Errorf(
				"failed to list ws streams: %w", err,
			)
		}

		conn := resp.Streams
		output := ListWsStreamsOutput{
			Streams: make([]WsStreamSummary, 0, len(conn.Edges)),
		}
		for _, edge := range conn.Edges {
			n := edge.Node
			// Caido 0.57 removed the protocol query argument; filter to
			// WebSocket streams client-side.
			if n.Protocol != gen.StreamProtocolWs {
				continue
			}
			output.Streams = append(output.Streams, WsStreamSummary{
				ID:        n.Id,
				Host:      n.Host,
				Port:      n.Port,
				Path:      n.Path,
				IsTLS:     n.IsTls,
				Direction: string(n.Direction),
				Source:    string(n.Source),
				CreatedAt: n.CreatedAt,
			})
		}
		linkReplaySessions(ctx, client, output.Streams)
		output.HasMore, output.NextCursor = pageCursor(
			conn.PageInfo.HasNextPage, conn.PageInfo.EndCursor,
		)

		return nil, output, nil
	}
}

// linkReplaySessionsTimeout bounds the lookup: it walks every WS session's
// entries, which grows with the project.
const linkReplaySessionsTimeout = 5 * time.Second

// linkReplaySessions fills ReplaySessionID on REPLAY streams. It is best
// effort: a failed or slow lookup leaves the fields it did not reach empty
// rather than failing the listing.
func linkReplaySessions(ctx context.Context, client *caido.Client, streams []WsStreamSummary) {
	var ids []string
	for _, s := range streams {
		if s.Source == string(gen.SourceReplay) {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, linkReplaySessionsTimeout)
	defer cancel()
	owners, _ := replay.WSStreamSessions(ctx, client, ids)
	for i := range streams {
		streams[i].ReplaySessionID = owners[streams[i].ID]
	}
}

// RegisterListWsStreamsTool registers the tool with the MCP server
func RegisterListWsStreamsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_list_ws_streams",
		Title: "List WebSocket Streams",
		Description: `List WebSocket streams (connections) from the WebSocket tab. ` +
			`Returns id/host/port/path/direction/source, plus replaySessionId for streams opened from a WS replay session. ` +
			`Use the stream id with caido_list_ws_messages to read frames. ` +
			`Params: limit (default 20, max 100), after (cursor), scope_id (optional).`,
		Annotations: readOnlyAnn(),
	}, listWsStreamsHandler(client))
}
