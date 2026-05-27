package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteInterceptEntryInput struct {
	ID string `json:"id" jsonschema:"required,Intercept entry ID to delete"`
}

type DeleteInterceptEntryOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deleteInterceptEntryHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteInterceptEntryInput) (*mcp.CallToolResult, DeleteInterceptEntryOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteInterceptEntryInput,
	) (*mcp.CallToolResult, DeleteInterceptEntryOutput, error) {
		if input.ID == "" {
			return nil, DeleteInterceptEntryOutput{}, fmt.Errorf("id is required")
		}

		resp, err := client.Intercept.DeleteEntry(ctx, input.ID)
		if err != nil {
			return nil, DeleteInterceptEntryOutput{}, err
		}

		out := DeleteInterceptEntryOutput{Success: true}
		if id := resp.DeleteInterceptEntry.DeletedId; id != nil {
			out.DeletedID = *id
		}
		return nil, out, nil
	}
}

func RegisterDeleteInterceptEntryTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_intercept_entry",
		Description: `Delete a single intercept queue entry by ID.`,
	}, deleteInterceptEntryHandler(client))
}
