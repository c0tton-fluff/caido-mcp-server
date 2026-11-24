package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ClearWsMessageDraftInput is the input for caido_clear_ws_message_draft.
type ClearWsMessageDraftInput struct {
	ID string `json:"id" jsonschema:"required,WebSocket replay entry ID"`
}

// ClearWsMessageDraftOutput is the output of caido_clear_ws_message_draft.
type ClearWsMessageDraftOutput struct {
	EntryID string `json:"entryId"`
	Cleared bool   `json:"cleared"`
}

func clearWsMessageDraftHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ClearWsMessageDraftInput) (*mcp.CallToolResult, ClearWsMessageDraftOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ClearWsMessageDraftInput,
	) (*mcp.CallToolResult, ClearWsMessageDraftOutput, error) {
		if input.ID == "" {
			return nil, ClearWsMessageDraftOutput{}, fmt.Errorf("id is required")
		}
		if err := replay.ClearWsDraft(ctx, client, input.ID); err != nil {
			return nil, ClearWsMessageDraftOutput{}, err
		}
		return nil, ClearWsMessageDraftOutput{
			EntryID: input.ID, Cleared: true,
		}, nil
	}
}

// RegisterClearWsMessageDraftTool registers the tool with the server.
func RegisterClearWsMessageDraftTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_clear_ws_message_draft",
		Title: "Clear WebSocket Message Draft",
		Description: `Remove the pending message of a WebSocket replay entry, leaving the entry ` +
			`with its handshake and recorded frames only.`,
		Annotations: writeAnn(true, true, false),
	}, clearWsMessageDraftHandler(client))
}
