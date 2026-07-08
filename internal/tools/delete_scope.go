package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteScopeInput struct {
	ID string `json:"id" jsonschema:"required,ID of the scope to delete"`
}

type DeleteScopeOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func deleteScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteScopeInput) (*mcp.CallToolResult, DeleteScopeOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteScopeInput,
	) (*mcp.CallToolResult, DeleteScopeOutput, error) {
		if input.ID == "" {
			return nil, DeleteScopeOutput{}, fmt.Errorf("id is required")
		}

		resp, err := client.Scopes.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteScopeOutput{}, fmt.Errorf("delete scope: %w", err)
		}

		return nil, DeleteScopeOutput{
			ID:      resp.DeleteScope.DeletedId,
			Deleted: true,
		}, nil
	}
}

func RegisterDeleteScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_scope",
		Description: `Delete a scope by ID.`,
		Annotations: writeAnn(true, true, false),
	}, deleteScopeHandler(client))
}
