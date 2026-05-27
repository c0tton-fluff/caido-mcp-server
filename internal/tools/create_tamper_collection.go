package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateTamperCollectionInput struct {
	Name string `json:"name" jsonschema:"required,Collection name"`
}

type CreateTamperCollectionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createTamperCollectionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateTamperCollectionInput) (*mcp.CallToolResult, CreateTamperCollectionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateTamperCollectionInput,
	) (*mcp.CallToolResult, CreateTamperCollectionOutput, error) {
		resp, err := client.Tamper.CreateCollection(ctx, &gen.CreateTamperRuleCollectionInput{
			Name: input.Name,
		})
		if err != nil {
			return nil, CreateTamperCollectionOutput{}, err
		}

		coll := resp.CreateTamperRuleCollection.Collection
		if coll == nil {
			return nil, CreateTamperCollectionOutput{}, fmt.Errorf("create tamper collection returned nil")
		}

		return nil, CreateTamperCollectionOutput{
			ID:   coll.Id,
			Name: coll.Name,
		}, nil
	}
}

func RegisterCreateTamperCollectionTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_tamper_collection",
		Description: `Create a Match & Replace (tamper) rule collection. Use the ` +
			`returned id as collection_id for caido_create_tamper_rule.`,
	}, createTamperCollectionHandler(client))
}
