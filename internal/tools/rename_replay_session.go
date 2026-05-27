package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameReplaySessionInput struct {
	ID   string `json:"id" jsonschema:"required,Replay session ID"`
	Name string `json:"name" jsonschema:"required,New session name"`
}

type RenameReplaySessionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameReplaySessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameReplaySessionInput) (*mcp.CallToolResult, RenameReplaySessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameReplaySessionInput,
	) (*mcp.CallToolResult, RenameReplaySessionOutput, error) {
		resp, err := client.Replay.RenameSession(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameReplaySessionOutput{}, fmt.Errorf("rename replay session: %w", err)
		}

		session := resp.RenameReplaySession.Session
		if session == nil {
			return nil, RenameReplaySessionOutput{}, fmt.Errorf("rename replay session returned nil")
		}

		return nil, RenameReplaySessionOutput{
			ID:   session.Id,
			Name: session.Name,
		}, nil
	}
}

func RegisterRenameReplaySessionTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_replay_session",
		Description: `Rename a replay session.`,
	}, renameReplaySessionHandler(client))
}
