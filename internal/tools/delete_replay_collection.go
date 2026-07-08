package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteReplayCollectionInput struct {
	ID string `json:"id" jsonschema:"required,Collection ID to delete"`
}

type DeleteReplayCollectionOutput struct {
	Success bool `json:"success"`
}

func deleteReplayCollectionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteReplayCollectionInput) (*mcp.CallToolResult, DeleteReplayCollectionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteReplayCollectionInput,
	) (*mcp.CallToolResult, DeleteReplayCollectionOutput, error) {
		_, err := client.Replay.DeleteCollection(ctx, input.ID)
		if err != nil {
			return nil, DeleteReplayCollectionOutput{}, fmt.Errorf("delete collection: %w", err)
		}

		return nil, DeleteReplayCollectionOutput{Success: true}, nil
	}
}

func RegisterDeleteReplayCollectionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_replay_collection",
		Description: `Delete a replay collection. Sessions in the collection will not be deleted.`,
		Annotations: writeAnn(true, true, false),
	}, deleteReplayCollectionHandler(client))
}
