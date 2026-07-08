package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MoveReplaySessionInput struct {
	SessionID    string `json:"sessionId" jsonschema:"required,Session ID to move"`
	CollectionID string `json:"collectionId" jsonschema:"required,Target collection ID"`
}

type MoveReplaySessionOutput struct {
	ID           string `json:"id"`
	CollectionID string `json:"collectionId"`
}

func moveReplaySessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, MoveReplaySessionInput) (*mcp.CallToolResult, MoveReplaySessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input MoveReplaySessionInput,
	) (*mcp.CallToolResult, MoveReplaySessionOutput, error) {
		resp, err := client.Replay.MoveSession(ctx, input.SessionID, input.CollectionID)
		if err != nil {
			return nil, MoveReplaySessionOutput{}, fmt.Errorf("move session: %w", err)
		}

		if resp.MoveReplaySession.Session == nil {
			return nil, MoveReplaySessionOutput{}, fmt.Errorf("move session returned nil")
		}
		session := *resp.MoveReplaySession.Session

		output := MoveReplaySessionOutput{
			ID:           session.GetId(),
			CollectionID: input.CollectionID,
		}

		return nil, output, nil
	}
}

func RegisterMoveReplaySessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_move_replay_session",
		Description: `Move a replay session to a different collection.`,
		Annotations: writeAnn(false, true, false),
	}, moveReplaySessionHandler(client))
}
