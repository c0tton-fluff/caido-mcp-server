package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteFilterInput struct {
	ID string `json:"id" jsonschema:"required,ID of the filter preset to delete"`
}

type DeleteFilterOutput struct {
	Success bool `json:"success"`
}

func deleteFilterHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteFilterInput) (*mcp.CallToolResult, DeleteFilterOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteFilterInput,
	) (*mcp.CallToolResult, DeleteFilterOutput, error) {
		resp, err := client.Filters.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteFilterOutput{}, err
		}

		payload := resp.DeleteFilterPreset
		if payload.DeletedId == nil {
			return nil, DeleteFilterOutput{}, fmt.Errorf(
				"failed to delete filter",
			)
		}

		output := DeleteFilterOutput{
			Success: true,
		}

		return nil, output, nil
	}
}

func RegisterDeleteFilterTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_delete_filter",
		Description: `Delete a filter preset by ID. ` +
			`Returns success status.`,
	}, deleteFilterHandler(client))
}
