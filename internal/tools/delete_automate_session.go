package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteAutomateSessionInput struct {
	ID string `json:"id" jsonschema:"required,Automate session ID to delete"`
}

type DeleteAutomateSessionOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deleteAutomateSessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteAutomateSessionInput) (*mcp.CallToolResult, DeleteAutomateSessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteAutomateSessionInput,
	) (*mcp.CallToolResult, DeleteAutomateSessionOutput, error) {
		resp, err := client.Automate.DeleteSession(ctx, input.ID)
		if err != nil {
			return nil, DeleteAutomateSessionOutput{}, fmt.Errorf("delete automate session: %w", err)
		}

		out := DeleteAutomateSessionOutput{Success: true}
		if id := resp.DeleteAutomateSession.DeletedId; id != nil {
			out.DeletedID = *id
		}
		return nil, out, nil
	}
}

func RegisterDeleteAutomateSessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_automate_session",
		Description: `Delete an Automate (fuzzing) session and its results.`,
	}, deleteAutomateSessionHandler(client))
}
