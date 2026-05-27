package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameAutomateSessionInput struct {
	ID   string `json:"id" jsonschema:"required,Automate session ID"`
	Name string `json:"name" jsonschema:"required,New session name"`
}

type RenameAutomateSessionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameAutomateSessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameAutomateSessionInput) (*mcp.CallToolResult, RenameAutomateSessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameAutomateSessionInput,
	) (*mcp.CallToolResult, RenameAutomateSessionOutput, error) {
		resp, err := client.Automate.RenameSession(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameAutomateSessionOutput{}, fmt.Errorf("rename automate session: %w", err)
		}

		session := resp.RenameAutomateSession.Session
		if session == nil {
			return nil, RenameAutomateSessionOutput{}, fmt.Errorf("rename automate session returned nil")
		}

		return nil, RenameAutomateSessionOutput{
			ID:   session.Id,
			Name: session.Name,
		}, nil
	}
}

func RegisterRenameAutomateSessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_automate_session",
		Description: `Rename an Automate (fuzzing) session.`,
	}, renameAutomateSessionHandler(client))
}
