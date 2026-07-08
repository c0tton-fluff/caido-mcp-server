package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameReplayCollectionInput struct {
	ID   string `json:"id" jsonschema:"required,Collection ID"`
	Name string `json:"name" jsonschema:"required,New collection name"`
}

type RenameReplayCollectionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameReplayCollectionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameReplayCollectionInput) (*mcp.CallToolResult, RenameReplayCollectionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameReplayCollectionInput,
	) (*mcp.CallToolResult, RenameReplayCollectionOutput, error) {
		resp, err := client.Replay.RenameCollection(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameReplayCollectionOutput{}, fmt.Errorf("rename collection: %w", err)
		}

		collection := resp.RenameReplaySessionCollection.Collection
		if collection == nil {
			return nil, RenameReplayCollectionOutput{}, fmt.Errorf("rename collection returned nil")
		}

		output := RenameReplayCollectionOutput{
			ID:   collection.Id,
			Name: collection.Name,
		}

		return nil, output, nil
	}
}

func RegisterRenameReplayCollectionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_replay_collection",
		Description: `Rename a replay collection.`,
		Annotations: writeAnn(false, true, false),
	}, renameReplayCollectionHandler(client))
}
