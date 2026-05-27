package tools

import (
	"context"
	"fmt"
	"time"

	gql "github.com/Khan/genqlient/graphql"
	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListWsStreamsInput is the input for the list_ws_streams tool.
type ListWsStreamsInput struct {
	ScopeID string `json:"scopeId,omitempty" jsonschema:"Restrict to a scope ID"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Max streams to return (default 20, max 100)"`
	After   string `json:"after,omitempty" jsonschema:"Pagination cursor from a previous nextCursor"`
}

// WsStreamSummary is a single WebSocket connection.
type WsStreamSummary struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Direction string `json:"direction"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

// ListWsStreamsOutput is the output of the list_ws_streams tool.
type ListWsStreamsOutput struct {
	Streams    []WsStreamSummary `json:"streams"`
	HasMore    bool              `json:"hasMore"`
	NextCursor string            `json:"nextCursor,omitempty"`
	Total      int               `json:"total"`
}

type listWsStreamsVars struct {
	First int     `json:"first"`
	After *string `json:"after"`
	Scope *string `json:"scopeId"`
}

type listWsStreamsResp struct {
	Streams struct {
		Count struct {
			Value int `json:"value"`
		} `json:"count"`
		Edges []struct {
			Node struct {
				Id        string `json:"id"`
				Host      string `json:"host"`
				Port      int    `json:"port"`
				Path      string `json:"path"`
				IsTls     bool   `json:"isTls"`
				Direction string `json:"direction"`
				Source    string `json:"source"`
				CreatedAt int64  `json:"createdAt"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"streams"`
}

const listWsStreamsQuery = `
query ListWsStreams($first: Int, $after: String, $scopeId: ID) {
	streams(protocol: WS, first: $first, after: $after, scopeId: $scopeId, order: {by: ID, ordering: ASC}) {
		count { value }
		edges {
			node { id host port path isTls direction source createdAt }
		}
		pageInfo { hasNextPage endCursor }
	}
}`

func listWsStreamsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWsStreamsInput) (*mcp.CallToolResult, ListWsStreamsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWsStreamsInput,
	) (*mcp.CallToolResult, ListWsStreamsOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		vars := &listWsStreamsVars{First: limit}
		if input.After != "" {
			vars.After = &input.After
		}
		if input.ScopeID != "" {
			vars.Scope = &input.ScopeID
		}

		data := &listWsStreamsResp{}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ListWsStreams",
			Query:     listWsStreamsQuery,
			Variables: vars,
		}, &gql.Response{Data: data}); err != nil {
			return nil, ListWsStreamsOutput{}, fmt.Errorf("list ws streams: %w", err)
		}

		out := ListWsStreamsOutput{
			Streams: make([]WsStreamSummary, 0, len(data.Streams.Edges)),
			Total:   data.Streams.Count.Value,
		}
		for _, e := range data.Streams.Edges {
			n := e.Node
			out.Streams = append(out.Streams, WsStreamSummary{
				ID:        n.Id,
				URL:       httputil.BuildURL(n.IsTls, n.Host, n.Port, n.Path, ""),
				Direction: n.Direction,
				Source:    n.Source,
				CreatedAt: time.UnixMilli(n.CreatedAt).Format(time.RFC3339),
			})
		}
		if data.Streams.PageInfo.HasNextPage {
			out.HasMore = true
			if data.Streams.PageInfo.EndCursor != nil {
				out.NextCursor = *data.Streams.PageInfo.EndCursor
			}
		}

		return nil, out, nil
	}
}

func RegisterListWsStreamsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_ws_streams",
		Description: `List WebSocket connections (streams) from history, like the ` +
			`WebSocket tab in Caido. Returns stream id/url/direction. Use the id with ` +
			`caido_list_ws_messages to read the frames of a connection.`,
	}, listWsStreamsHandler(client))
}
