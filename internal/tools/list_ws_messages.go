package tools

import (
	"context"
	"encoding/base64"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebSocket frame content lives on each message's head field (a
// StreamWsMessageEdit): direction (CLIENT/SERVER), format (TEXT/BINARY),
// length, and raw (a Blob the API serializes as base64, decoded here).

// ListWsMessagesInput is the input for the list_ws_messages tool
type ListWsMessagesInput struct {
	StreamID  string `json:"stream_id" jsonschema:"required,ID of the WebSocket stream (from caido_list_ws_streams)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of frames to return (default 20, max 100)"`
	After     string `json:"after,omitempty" jsonschema:"Cursor for pagination from previous response nextCursor"`
	BodyLimit int    `json:"body_limit,omitempty" jsonschema:"Max bytes of each frame body to return (default 4096, max 65536). Bodies are truncated to this size."`
}

// WsMessageSummary is a single WebSocket frame
type WsMessageSummary struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Format    string `json:"format"`
	Length    int    `json:"length"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ListWsMessagesOutput is the output of the list_ws_messages tool
type ListWsMessagesOutput struct {
	Messages   []WsMessageSummary `json:"messages"`
	HasMore    bool               `json:"hasMore"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

func listWsMessagesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWsMessagesInput) (*mcp.CallToolResult, ListWsMessagesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWsMessagesInput,
	) (*mcp.CallToolResult, ListWsMessagesOutput, error) {
		if input.StreamID == "" {
			return nil, ListWsMessagesOutput{}, fmt.Errorf(
				"stream_id is required",
			)
		}

		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		bodyLimit := input.BodyLimit
		if bodyLimit <= 0 {
			bodyLimit = 4096
		}
		if bodyLimit > 65536 {
			bodyLimit = 65536
		}

		opts := &caido.ListWsMessagesOptions{First: &limit}
		if input.After != "" {
			opts.After = &input.After
		}

		resp, err := client.Streams.ListWsMessages(ctx, input.StreamID, opts)
		if err != nil {
			return nil, ListWsMessagesOutput{}, fmt.Errorf(
				"failed to list ws messages: %w", err,
			)
		}

		conn := resp.StreamWsMessages
		output := ListWsMessagesOutput{
			Messages: make([]WsMessageSummary, 0, len(conn.Edges)),
		}
		for _, edge := range conn.Edges {
			head := edge.Node.Head
			body, truncated := decodeWsBody(head.Raw, bodyLimit)
			output.Messages = append(output.Messages, WsMessageSummary{
				ID:        edge.Node.Id,
				Direction: string(head.Direction),
				Format:    string(head.Format),
				Length:    head.Length,
				Body:      body,
				Truncated: truncated,
			})
		}
		if conn.PageInfo.HasNextPage {
			output.HasMore = true
			if conn.PageInfo.EndCursor != nil {
				output.NextCursor = *conn.PageInfo.EndCursor
			}
		}

		return nil, output, nil
	}
}

// decodeWsBody decodes a base64 Blob frame body and truncates it to limit
// bytes. If the payload is not valid base64 it is returned as-is (also
// truncated), matching the lenient handling in run_workflow.go.
func decodeWsBody(raw string, limit int) (string, bool) {
	body := raw
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		body = string(decoded)
	}
	if len(body) > limit {
		return body[:limit], true
	}
	return body, false
}

// RegisterListWsMessagesTool registers the tool with the MCP server
func RegisterListWsMessagesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_ws_messages",
		Description: `List WebSocket frames for a stream (from caido_list_ws_streams). ` +
			`Each frame has direction (CLIENT/SERVER), format (TEXT/BINARY), ` +
			`length, and decoded body (truncated to body_limit). ` +
			`Params: stream_id (required), limit (default 20, max 100), ` +
			`after (cursor), body_limit (default 4096, max 65536).`,
	}, listWsMessagesHandler(client))
}
