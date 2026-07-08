package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameScopeInput struct {
	ID   string `json:"id" jsonschema:"required,ID of the scope to rename"`
	Name string `json:"name" jsonschema:"required,New name for the scope"`
}

type RenameScopeOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameScopeInput) (*mcp.CallToolResult, RenameScopeOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameScopeInput,
	) (*mcp.CallToolResult, RenameScopeOutput, error) {
		if input.ID == "" {
			return nil, RenameScopeOutput{}, fmt.Errorf("id is required")
		}
		if input.Name == "" {
			return nil, RenameScopeOutput{}, fmt.Errorf("name is required")
		}

		resp, err := client.Scopes.Rename(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameScopeOutput{}, fmt.Errorf("rename scope: %w", err)
		}

		scope := resp.RenameScope.Scope
		return nil, RenameScopeOutput{
			ID:   scope.Id,
			Name: scope.Name,
		}, nil
	}
}

func RegisterRenameScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_scope",
		Description: `Rename a scope by ID.`,
		Annotations: writeAnn(false, true, false),
	}, renameScopeHandler(client))
}
