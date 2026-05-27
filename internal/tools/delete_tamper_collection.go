package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteTamperCollectionInput struct {
	ID string `json:"id" jsonschema:"required,Tamper rule collection ID to delete"`
}

type DeleteTamperCollectionOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deleteTamperCollectionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteTamperCollectionInput) (*mcp.CallToolResult, DeleteTamperCollectionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteTamperCollectionInput,
	) (*mcp.CallToolResult, DeleteTamperCollectionOutput, error) {
		resp, err := client.Tamper.DeleteCollection(ctx, input.ID)
		if err != nil {
			return nil, DeleteTamperCollectionOutput{}, err
		}

		out := DeleteTamperCollectionOutput{Success: true}
		if id := resp.DeleteTamperRuleCollection.DeletedId; id != nil {
			out.DeletedID = *id
		}
		return nil, out, nil
	}
}

func RegisterDeleteTamperCollectionTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_tamper_collection",
		Description: `Delete a Match & Replace (tamper) rule collection and its rules.`,
	}, deleteTamperCollectionHandler(client))
}
