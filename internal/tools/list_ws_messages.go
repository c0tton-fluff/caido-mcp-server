package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListWsMessagesInput is the input for the list_ws_messages tool.
type ListWsMessagesInput struct {
	StreamID  string `json:"streamId" jsonschema:"required,WebSocket stream ID (from caido_list_ws_streams)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max messages to return (default 50, max 200)"`
	After     string `json:"after,omitempty" jsonschema:"Pagination cursor from a previous nextCursor"`
	BodyLimit int    `json:"bodyLimit,omitempty" jsonschema:"Max bytes of each frame body to return (default 2000)"`
}

// WsMessageSummary is a single WebSocket frame.
type WsMessageSummary struct {
	ID         string `json:"id"`
	Direction  string `json:"direction"`
	Format     string `json:"format"`
	Length     int    `json:"length"`
	Body       string `json:"body"`
	Truncated  bool   `json:"truncated,omitempty"`
	Alteration string `json:"alteration"`
	CreatedAt  string `json:"createdAt"`
}

// ListWsMessagesOutput is the output of the list_ws_messages tool.
type ListWsMessagesOutput struct {
	Messages   []WsMessageSummary `json:"messages"`
	HasMore    bool               `json:"hasMore"`
	NextCursor string             `json:"nextCursor,omitempty"`
	Total      int                `json:"total"`
}

type listWsMessagesVars struct {
	StreamID string  `json:"streamId"`
	First    int     `json:"first"`
	After    *string `json:"after"`
}

type listWsMessagesResp struct {
	StreamWsMessages struct {
		Count struct {
			Value int `json:"value"`
		} `json:"count"`
		Edges []struct {
			Node struct {
				Id   string `json:"id"`
				Head struct {
					Direction  string `json:"direction"`
					Format     string `json:"format"`
					Length     int    `json:"length"`
					Raw        string `json:"raw"`
					Alteration string `json:"alteration"`
					CreatedAt  int64  `json:"createdAt"`
				} `json:"head"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"streamWsMessages"`
}

const listWsMessagesQuery = `
query ListWsMessages($streamId: ID!, $first: Int, $after: String) {
	streamWsMessages(streamId: $streamId, first: $first, after: $after, order: {by: ID, ordering: ASC}) {
		count { value }
		edges {
			node {
				id
				head { direction format length raw alteration createdAt }
			}
		}
		pageInfo { hasNextPage endCursor }
	}
}`

func listWsMessagesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWsMessagesInput) (*mcp.CallToolResult, ListWsMessagesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWsMessagesInput,
	) (*mcp.CallToolResult, ListWsMessagesOutput, error) {
		if input.StreamID == "" {
			return nil, ListWsMessagesOutput{}, fmt.Errorf("streamId is required")
		}

		limit := input.Limit
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		bodyLimit := input.BodyLimit
		if bodyLimit <= 0 {
			bodyLimit = 2000
		}

		vars := &listWsMessagesVars{StreamID: input.StreamID, First: limit}
		if input.After != "" {
			vars.After = &input.After
		}

		data := &listWsMessagesResp{}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ListWsMessages",
			Query:     listWsMessagesQuery,
			Variables: vars,
		}, &gql.Response{Data: data}); err != nil {
			return nil, ListWsMessagesOutput{}, fmt.Errorf("list ws messages: %w", err)
		}

		out := ListWsMessagesOutput{
			Messages: make([]WsMessageSummary, 0, len(data.StreamWsMessages.Edges)),
			Total:    data.StreamWsMessages.Count.Value,
		}
		for _, e := range data.StreamWsMessages.Edges {
			h := e.Node.Head
			body, truncated := decodeWsBody(h.Raw, bodyLimit)
			out.Messages = append(out.Messages, WsMessageSummary{
				ID:         e.Node.Id,
				Direction:  h.Direction,
				Format:     h.Format,
				Length:     h.Length,
				Body:       body,
				Truncated:  truncated,
				Alteration: h.Alteration,
				CreatedAt:  time.UnixMilli(h.CreatedAt).Format(time.RFC3339),
			})
		}
		if data.StreamWsMessages.PageInfo.HasNextPage {
			out.HasMore = true
			if data.StreamWsMessages.PageInfo.EndCursor != nil {
				out.NextCursor = *data.StreamWsMessages.PageInfo.EndCursor
			}
		}

		return nil, out, nil
	}
}

// decodeWsBody base64-decodes a WS frame Blob and truncates it to limit bytes.
// If decoding fails the raw value is returned as-is.
func decodeWsBody(raw string, limit int) (string, bool) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded = []byte(raw)
	}
	if len(decoded) > limit {
		return string(decoded[:limit]), true
	}
	return string(decoded), false
}

func RegisterListWsMessagesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_ws_messages",
		Description: `List WebSocket frames for a stream (from caido_list_ws_streams), ` +
			`like opening a connection in Caido's WebSocket history. Each frame returns ` +
			`direction (CLIENT/SERVER), format (TEXT/BINARY), length and decoded body. ` +
			`Bodies are truncated to bodyLimit bytes (default 2000).`,
	}, listWsMessagesHandler(client))
}
