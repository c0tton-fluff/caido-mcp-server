package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameReplaySessionInput struct {
	SessionID string `json:"sessionId" jsonschema:"required,Session ID to rename"`
	Name      string `json:"name" jsonschema:"required,New session title (shown as the replay tab name)"`
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
		id, name, err := replay.RenameSession(ctx, client, input.SessionID, input.Name)
		if err != nil {
			return nil, RenameReplaySessionOutput{}, fmt.Errorf("rename session: %w", err)
		}
		return nil, RenameReplaySessionOutput{ID: id, Name: name}, nil
	}
}

func RegisterRenameReplaySessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_replay_session",
		Title:       "Rename Replay Session",
		Description: `Rename a replay session (its tab title).`,
		Annotations: writeAnn(false, true, false),
	}, renameReplaySessionHandler(client))
}
