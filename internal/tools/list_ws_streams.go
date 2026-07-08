package tools

import (
	"context"
	"fmt"

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
	CreatedAt int64  `json:"createdAt"`
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
		output.HasMore, output.NextCursor = pageCursor(
			conn.PageInfo.HasNextPage, conn.PageInfo.EndCursor,
		)

		return nil, output, nil
	}
}

// RegisterListWsStreamsTool registers the tool with the MCP server
func RegisterListWsStreamsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_ws_streams",
		Description: `List WebSocket streams (connections) from the WebSocket tab. ` +
			`Returns id/host/port/path/direction/source. ` +
			`Use the stream id with caido_list_ws_messages to read frames. ` +
			`Params: limit (default 20, max 100), after (cursor), scope_id (optional).`,
	}, listWsStreamsHandler(client))
}
