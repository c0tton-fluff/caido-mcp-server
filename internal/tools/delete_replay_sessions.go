package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteReplaySessionsInput struct {
	IDs []string `json:"ids" jsonschema:"required,Array of session IDs to delete"`
}

type DeleteReplaySessionsOutput struct {
	DeletedIDs []string `json:"deletedIds"`
}

func deleteReplaySessionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteReplaySessionsInput) (*mcp.CallToolResult, DeleteReplaySessionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteReplaySessionsInput,
	) (*mcp.CallToolResult, DeleteReplaySessionsOutput, error) {
		if len(input.IDs) == 0 {
			return nil, DeleteReplaySessionsOutput{}, fmt.Errorf("at least one session ID is required")
		}

		resp, err := client.Replay.DeleteSessions(ctx, input.IDs)
		if err != nil {
			return nil, DeleteReplaySessionsOutput{}, fmt.Errorf("delete sessions: %w", err)
		}

		return nil, DeleteReplaySessionsOutput{
			DeletedIDs: resp.DeleteReplaySessions.DeletedIds,
		}, nil
	}
}

func RegisterDeleteReplaySessionsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_replay_sessions",
		Description: `Delete one or more replay sessions by ID.`,
		Annotations: writeAnn(true, true, false),
	}, deleteReplaySessionsHandler(client))
}
