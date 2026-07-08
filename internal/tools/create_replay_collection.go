package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateReplayCollectionInput struct {
	Name string `json:"name" jsonschema:"required,Collection name"`
}

type CreateReplayCollectionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createReplayCollectionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateReplayCollectionInput) (*mcp.CallToolResult, CreateReplayCollectionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateReplayCollectionInput,
	) (*mcp.CallToolResult, CreateReplayCollectionOutput, error) {
		createInput := &gen.CreateReplaySessionCollectionInput{
			Name: input.Name,
		}

		resp, err := client.Replay.CreateCollection(ctx, createInput)
		if err != nil {
			return nil, CreateReplayCollectionOutput{}, fmt.Errorf("create collection: %w", err)
		}

		collection := resp.CreateReplaySessionCollection.Collection
		if collection == nil {
			return nil, CreateReplayCollectionOutput{}, fmt.Errorf("create collection returned nil")
		}

		output := CreateReplayCollectionOutput{
			ID:   collection.Id,
			Name: collection.Name,
		}

		return nil, output, nil
	}
}

func RegisterCreateReplayCollectionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_replay_collection",
		Description: `Create a named replay collection to organize replay sessions.`,
		Annotations: writeAnn(false, false, false),
	}, createReplayCollectionHandler(client))
}
